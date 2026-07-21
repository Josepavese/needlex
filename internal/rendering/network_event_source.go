package rendering

import (
	"encoding/json"
	"net/url"
	"strings"
)

func (c *networkCollector) shouldKeepEventSourceOpenUntilMessageIdle(res *NetworkResource, finished bool) bool {
	if res == nil || !finished || res.MessageCount == 0 {
		return false
	}
	return strings.EqualFold(res.Source, "event_source") ||
		strings.EqualFold(res.Type, "EventSource") ||
		strings.Contains(strings.ToLower(res.ContentType), "event-stream")
}

func (c *networkCollector) handleEventSourceMessage(params json.RawMessage) {
	var event struct {
		RequestID string `json:"requestId"`
		Data      string `json:"data"`
		EventName string `json:"eventName"`
		EventID   string `json:"eventId"`
	}
	if json.Unmarshal(params, &event) != nil || strings.TrimSpace(event.RequestID) == "" {
		return
	}
	res := c.resources[event.RequestID]
	if res == nil {
		return
	}
	res.Type = "EventSource"
	res.ContentType = firstNonEmptyRenderString(res.ContentType, "text/event-stream")
	res.Source = "event_source"
	c.activeEventSources[event.RequestID] = struct{}{}
	c.eventSourceMessages++
	res.MessageCount++
	data := strings.TrimSpace(event.Data)
	if c.networkMessageBudgetAvailable(res) {
		c.appendToResource(res, "data: "+data+"\n\n")
	}
	if data == "end" {
		res.Finished = true
		delete(c.activeEventSources, event.RequestID)
		delete(c.active, event.RequestID)
	}
	c.markEventSourceActivity()
}

func (c *networkCollector) handleWebSocketCreated(params json.RawMessage) {
	var event struct {
		RequestID string `json:"requestId"`
		URL       string `json:"url"`
	}
	if json.Unmarshal(params, &event) != nil || strings.TrimSpace(event.RequestID) == "" {
		return
	}
	if !c.relevantHost(event.URL) {
		return
	}
	res := c.ensureResource(event.RequestID, event.URL, "WebSocket", "text/plain", "websocket")
	if res == nil {
		return
	}
	c.activeWebSockets[event.RequestID] = struct{}{}
	c.active[event.RequestID] = struct{}{}
	c.markActivity()
}

func (c *networkCollector) handleWebSocketFrameReceived(params json.RawMessage) {
	var event struct {
		RequestID string `json:"requestId"`
		Response  struct {
			Opcode      float64 `json:"opcode"`
			PayloadData string  `json:"payloadData"`
		} `json:"response"`
	}
	if json.Unmarshal(params, &event) != nil || strings.TrimSpace(event.RequestID) == "" {
		return
	}
	if event.Response.PayloadData == "" || event.Response.Opcode != 1 {
		return
	}
	res := c.resources[event.RequestID]
	if res == nil {
		return
	}
	res.Type = "WebSocket"
	res.ContentType = firstNonEmptyRenderString(res.ContentType, "text/plain")
	res.Source = "websocket"
	c.webSocketMessages++
	res.MessageCount++
	if c.networkMessageBudgetAvailable(res) {
		c.appendToResource(res, event.Response.PayloadData+"\n")
	}
	c.markActivity()
}

func (c *networkCollector) handleWebSocketClosed(params json.RawMessage) {
	var event struct {
		RequestID string `json:"requestId"`
	}
	if json.Unmarshal(params, &event) != nil || strings.TrimSpace(event.RequestID) == "" {
		return
	}
	delete(c.active, event.RequestID)
	delete(c.activeWebSockets, event.RequestID)
	if res := c.resources[event.RequestID]; res != nil {
		res.Finished = true
		c.markActivity()
	}
}

func (c *networkCollector) relevantResource(resourceType, rawURL, contentType string) bool {
	if !c.relevantHost(rawURL) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(resourceType)) {
	case "xhr", "fetch", "eventsource", "websocket":
		return true
	case "document", "other", "manifest":
		return isApplicationDataNetworkContentType(contentType)
	default:
		return false
	}
}

func (c *networkCollector) relevantHost(rawURL string) bool {
	if strings.TrimSpace(c.baseHost) == "" || strings.TrimSpace(rawURL) == "" {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Hostname(), "www."))
	return host == "" || host == c.baseHost || strings.HasSuffix(host, "."+c.baseHost)
}

func (c *networkCollector) ensureResource(requestID, rawURL, resourceType, contentType, source string) *NetworkResource {
	if res := c.resources[requestID]; res != nil {
		if strings.TrimSpace(rawURL) != "" && strings.TrimSpace(res.URL) == "" {
			res.URL = strings.TrimSpace(rawURL)
		}
		if strings.TrimSpace(resourceType) != "" {
			res.Type = strings.TrimSpace(resourceType)
		}
		if strings.TrimSpace(contentType) != "" {
			res.ContentType = strings.TrimSpace(contentType)
		}
		if strings.TrimSpace(source) != "" {
			res.Source = strings.TrimSpace(source)
		}
		return res
	}
	if c.maxResources > 0 && len(c.order) >= c.maxResources {
		c.truncated = true
		return nil
	}
	res := &NetworkResource{
		URL:         strings.TrimSpace(rawURL),
		Type:        strings.TrimSpace(resourceType),
		ContentType: strings.TrimSpace(contentType),
		Source:      strings.TrimSpace(source),
	}
	c.resources[requestID] = res
	c.order = append(c.order, requestID)
	return res
}

func (c *networkCollector) shouldFetchResponseBody(res *NetworkResource) bool {
	if res == nil || strings.EqualFold(res.Source, "event_source") || strings.EqualFold(res.Source, "websocket") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(res.Type)) {
	case "xhr", "fetch":
		return true
	default:
		return isApplicationDataNetworkContentType(res.ContentType)
	}
}

func (c *networkCollector) appendToResource(res *NetworkResource, data string) {
	if res == nil || data == "" {
		return
	}
	originalBytes := int64(len([]byte(data)))
	availableTotal := c.maxBytes - c.totalBytes
	availableResource := c.resourceMaxBytes - res.BodyBytes
	available := minPositiveInt64(availableTotal, availableResource)
	if available <= 0 {
		res.Truncated = true
		c.truncated = true
		return
	}
	if originalBytes > available {
		data = string([]byte(data)[:int(available)])
		res.Truncated = true
		c.truncated = true
	}
	res.Body += data
	written := int64(len([]byte(data)))
	res.BodyBytes += written
	c.totalBytes += written
	if originalBytes > written {
		res.Truncated = true
		c.truncated = true
	}
}

func (c *networkCollector) networkMessageBudgetAvailable(res *NetworkResource) bool {
	if c.maxMessages <= 0 || res == nil || res.MessageCount <= c.maxMessages {
		return true
	}
	res.Truncated = true
	c.truncated = true
	return false
}

func (c *networkCollector) takePendingBodies() []string {
	out := make([]string, 0, len(c.pendingBodies))
	for requestID := range c.pendingBodies {
		out = append(out, requestID)
		delete(c.pendingBodies, requestID)
	}
	return out
}

func (c *networkCollector) appendResponseBody(requestID, data string) {
	res := c.resources[requestID]
	if res == nil || strings.TrimSpace(data) == "" {
		return
	}
	c.appendToResource(res, data)
	c.markActivity()
}

func (c *networkCollector) markBodyUnavailable(requestID string) {
	if c.resources[requestID] != nil {
		c.bodyUnavailableCount++
	}
}
