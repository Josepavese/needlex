package rendering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (c *networkCollector) markSettleError(err error) {
	if err == nil || c.idleReason != "" {
		return
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		c.idleReason = "settle_timeout"
		return
	}
	c.idleReason = "settle_error"
}

func (c *networkCollector) markActivity() {
	c.lastActivity = time.Now()
}

func (c *networkCollector) markEventSourceActivity() {
	now := time.Now()
	c.lastActivity = now
	c.lastEventSourceActivity = now
}

func (c *networkCollector) settled(readyElapsed, totalElapsed, idle time.Duration) bool {
	if idle <= 0 {
		idle = 1500 * time.Millisecond
	}
	if readyElapsed < 500*time.Millisecond {
		return false
	}
	idleElapsed := time.Since(c.lastActivity)
	if len(c.activeEventSources) > 0 {
		eventSourceIdle := maxDuration(idle*10, 15*time.Second)
		eventSourceLastActivity := c.lastEventSourceActivity
		if eventSourceLastActivity.IsZero() {
			eventSourceLastActivity = c.lastActivity
		}
		if totalElapsed < 12*time.Second {
			return false
		}
		if time.Since(eventSourceLastActivity) >= eventSourceIdle {
			c.idleReason = "event_source_idle"
			return true
		}
		return false
	}
	if idleElapsed >= idle {
		if len(c.activeWebSockets) > 0 {
			c.idleReason = "websocket_idle"
		} else if len(c.active) > 0 {
			c.idleReason = "network_idle_with_active_requests"
		} else {
			c.idleReason = "network_idle"
		}
		return true
	}
	return false
}

func (c *networkCollector) snapshot() ([]NetworkResource, NetworkStats) {
	out := make([]NetworkResource, 0, len(c.order))
	for _, requestID := range c.order {
		if res := c.resources[requestID]; res != nil && strings.TrimSpace(res.Body) != "" {
			out = append(out, *res)
		}
	}
	stats := NetworkStats{
		ResourceCount:       len(out),
		EventSourceMessages: c.eventSourceMessages,
		WebSocketMessages:   c.webSocketMessages,
		BodyBytes:           c.totalBytes,
		Truncated:           c.truncated,
		IdleReason:          firstNonEmptyRenderString(c.idleReason, "unknown"),
	}
	return out, stats
}

func isApplicationDataNetworkContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "" {
		return false
	}
	return strings.Contains(contentType, "json") ||
		strings.Contains(contentType, "xml") ||
		strings.Contains(contentType, "yaml") ||
		strings.Contains(contentType, "event-stream") ||
		strings.Contains(contentType, "graphql") ||
		strings.Contains(contentType, "ndjson") ||
		strings.HasPrefix(contentType, "text/plain") ||
		strings.HasPrefix(contentType, "text/markdown") ||
		strings.HasPrefix(contentType, "text/csv")
}

func headerValue(headers map[string]any, key string) string {
	for gotKey, value := range headers {
		if !strings.EqualFold(gotKey, key) {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		case []any:
			parts := []string{}
			for _, item := range typed {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, strings.TrimSpace(text))
				}
			}
			return strings.Join(parts, ", ")
		}
	}
	return ""
}

func freeLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate renderer cdp port: %w", err)
	}
	defer listener.Close() //nolint:errcheck
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("renderer cdp port is not tcp")
	}
	return addr.Port, nil
}

func waitForCDPWebSocketURL(ctx context.Context, port int) (string, error) {
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	client := http.Client{Timeout: 250 * time.Millisecond}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err == nil && resp != nil {
			func() {
				defer resp.Body.Close() //nolint:errcheck
				var payload struct {
					WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
				}
				if resp.StatusCode >= 200 && resp.StatusCode < 300 && json.NewDecoder(resp.Body).Decode(&payload) == nil && strings.TrimSpace(payload.WebSocketDebuggerURL) != "" {
					endpoint = strings.TrimSpace(payload.WebSocketDebuggerURL)
				}
			}()
			if strings.HasPrefix(endpoint, "ws://") {
				return endpoint, nil
			}
		}
		select {
		case <-tick.C:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func FindBrowserPath(configured string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		if _, err := os.Stat(strings.TrimSpace(configured)); err != nil {
			return "", fmt.Errorf("configured browser path unavailable: %w", err)
		}
		return strings.TrimSpace(configured), nil
	}
	for _, name := range []string{
		"chrome-headless-shell",
		"chromium",
		"chromium-browser",
		"google-chrome",
		"google-chrome-stable",
		"chrome",
		"msedge",
	} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("no Chrome/Chromium executable found for JS rendering")
}

func isHeadlessShell(browserPath string) bool {
	base := strings.ToLower(filepath.Base(browserPath))
	return strings.Contains(base, "chrome-headless-shell") || base == "headless_shell"
}

func firstPositiveDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func minPositiveInt64(values ...int64) int64 {
	out := int64(0)
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if out == 0 || value < out {
			out = value
		}
	}
	return out
}

func maxDuration(left, right time.Duration) time.Duration {
	if left > right {
		return left
	}
	return right
}

func renderSettleBudget(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 5 * time.Second
	}
	reserve := renderSnapshotBudget(timeout)
	if timeout <= reserve+time.Second {
		return timeout
	}
	return timeout - reserve
}
