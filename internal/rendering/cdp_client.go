package rendering

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type cdpClient struct {
	conn    *websocket.Conn
	id      int
	onEvent func(cdpMessage)
}

type cdpMessage struct {
	ID        int             `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *cdpClient) command(ctx context.Context, sessionID, method string, params any) (json.RawMessage, error) {
	c.id++
	id := c.id
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	msg := cdpMessage{ID: id, Method: method, Params: paramsJSON, SessionID: sessionID}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if err := c.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return nil, fmt.Errorf("write cdp command %s: %w", method, err)
	}
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return nil, fmt.Errorf("read cdp response %s: %w", method, err)
		}
		var response cdpMessage
		if err := json.Unmarshal(data, &response); err != nil {
			continue
		}
		if response.Method != "" && c.onEvent != nil {
			c.onEvent(response)
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("cdp %s failed: %s", method, response.Error.Message)
		}
		return response.Result, nil
	}
}

func (c *cdpClient) evaluateString(ctx context.Context, sessionID, expression string) (string, error) {
	result, err := c.command(ctx, sessionID, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	})
	if err != nil {
		return "", err
	}
	var evaluated struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(result, &evaluated); err != nil {
		return "", err
	}
	return evaluated.Result.Value, nil
}

func waitForApplicationSettled(ctx context.Context, cdp *cdpClient, sessionID string, collector *networkCollector, networkIdle, maxWait time.Duration) error {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	var readyAt time.Time
	started := time.Now()
	deadline := started.Add(maxWait)
	for {
		if maxWait > 0 && time.Now().After(deadline) {
			return context.DeadlineExceeded
		}
		state, err := cdp.evaluateString(ctx, sessionID, `document.readyState`)
		if err == nil && (state == "interactive" || state == "complete") && readyAt.IsZero() {
			readyAt = time.Now()
			collector.markActivity()
		}
		_ = collectNetworkBodies(ctx, cdp, sessionID, collector)
		if !readyAt.IsZero() && collector.settled(time.Since(readyAt), time.Since(started), networkIdle) {
			return nil
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func collectNetworkBodies(ctx context.Context, cdp *cdpClient, sessionID string, collector *networkCollector) error {
	for _, requestID := range collector.takePendingBodies() {
		result, err := cdp.command(ctx, sessionID, "Network.getResponseBody", map[string]any{"requestId": requestID})
		if err != nil {
			collector.markBodyUnavailable(requestID)
			continue
		}
		var body struct {
			Body          string `json:"body"`
			Base64Encoded bool   `json:"base64Encoded"`
		}
		if err := json.Unmarshal(result, &body); err != nil {
			collector.markBodyUnavailable(requestID)
			continue
		}
		data := body.Body
		if body.Base64Encoded {
			decoded, err := base64.StdEncoding.DecodeString(body.Body)
			if err != nil {
				collector.markBodyUnavailable(requestID)
				continue
			}
			data = string(decoded)
		}
		collector.appendResponseBody(requestID, data)
	}
	return nil
}

type networkCollector struct {
	baseHost                string
	resources               map[string]*NetworkResource
	order                   []string
	active                  map[string]struct{}
	activeEventSources      map[string]struct{}
	activeWebSockets        map[string]struct{}
	pendingBodies           map[string]struct{}
	totalBytes              int64
	maxBytes                int64
	resourceMaxBytes        int64
	maxResources            int
	maxMessages             int
	lastActivity            time.Time
	lastEventSourceActivity time.Time
	eventSourceMessages     int
	webSocketMessages       int
	truncated               bool
	idleReason              string
	bodyUnavailableCount    int
}

func newNetworkCollector(rawURL string, maxBytes, resourceMaxBytes int64, maxResources, maxMessages int) *networkCollector {
	parsed, _ := url.Parse(strings.TrimSpace(rawURL))
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
	return &networkCollector{
		baseHost:           host,
		resources:          map[string]*NetworkResource{},
		active:             map[string]struct{}{},
		activeEventSources: map[string]struct{}{},
		activeWebSockets:   map[string]struct{}{},
		pendingBodies:      map[string]struct{}{},
		maxBytes:           firstPositiveInt64(maxBytes, 64_000_000),
		resourceMaxBytes:   firstPositiveInt64(resourceMaxBytes, maxBytes, 64_000_000),
		maxResources:       firstPositiveInt(maxResources, 32),
		maxMessages:        firstPositiveInt(maxMessages, 4096),
		lastActivity:       time.Now(),
	}
}

func (c *networkCollector) handleEvent(msg cdpMessage) {
	switch msg.Method {
	case "Network.responseReceived":
		c.handleResponseReceived(msg.Params)
	case "Network.loadingFinished":
		c.handleLoadingDone(msg.Params, true)
	case "Network.loadingFailed":
		c.handleLoadingDone(msg.Params, false)
	case "Network.eventSourceMessageReceived":
		c.handleEventSourceMessage(msg.Params)
	case "Network.webSocketCreated":
		c.handleWebSocketCreated(msg.Params)
	case "Network.webSocketFrameReceived":
		c.handleWebSocketFrameReceived(msg.Params)
	case "Network.webSocketClosed":
		c.handleWebSocketClosed(msg.Params)
	}
}

func (c *networkCollector) handleResponseReceived(params json.RawMessage) {
	var event struct {
		RequestID string `json:"requestId"`
		Type      string `json:"type"`
		Response  struct {
			URL      string         `json:"url"`
			Status   int            `json:"status"`
			MimeType string         `json:"mimeType"`
			Headers  map[string]any `json:"headers"`
		} `json:"response"`
	}
	if json.Unmarshal(params, &event) != nil || strings.TrimSpace(event.RequestID) == "" {
		return
	}
	contentType := firstNonEmptyRenderString(event.Response.MimeType, headerValue(event.Response.Headers, "content-type"))
	if !c.relevantResource(event.Type, event.Response.URL, contentType) {
		return
	}
	res := c.ensureResource(event.RequestID, event.Response.URL, event.Type, contentType, "response")
	if res == nil {
		return
	}
	res.Status = event.Response.Status
	c.active[event.RequestID] = struct{}{}
	if strings.EqualFold(event.Type, "EventSource") || strings.Contains(strings.ToLower(contentType), "event-stream") {
		res.Source = "event_source"
		c.activeEventSources[event.RequestID] = struct{}{}
		c.markEventSourceActivity()
	}
	if c.shouldFetchResponseBody(res) {
		c.pendingBodies[event.RequestID] = struct{}{}
	}
	c.markActivity()
}

func (c *networkCollector) handleLoadingDone(params json.RawMessage, finished bool) {
	var event struct {
		RequestID string `json:"requestId"`
	}
	if json.Unmarshal(params, &event) != nil || strings.TrimSpace(event.RequestID) == "" {
		return
	}
	delete(c.active, event.RequestID)
	delete(c.activeWebSockets, event.RequestID)
	if res := c.resources[event.RequestID]; res != nil {
		if !c.shouldKeepEventSourceOpenUntilMessageIdle(res, finished) {
			delete(c.activeEventSources, event.RequestID)
		}
		res.Finished = finished
		if c.shouldFetchResponseBody(res) {
			c.pendingBodies[event.RequestID] = struct{}{}
		}
		c.markActivity()
	}
}
