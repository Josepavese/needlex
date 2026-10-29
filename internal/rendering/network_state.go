package rendering

import (
	"context"
	"errors"
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

func (c *networkCollector) markStreamActivity() {
	now := time.Now()
	c.lastActivity = now
	c.lastStreamActivity = now
}

func (c *networkCollector) activeStreamCount() int {
	return len(c.activeEventSources) + len(c.activeStreams)
}

func (c *networkCollector) isOpenStream(requestID string) bool {
	if _, ok := c.activeStreams[requestID]; ok {
		return true
	}
	_, ok := c.activeEventSources[requestID]
	return ok
}

func (c *networkCollector) streamQuietSince() time.Time {
	last := c.lastStreamActivity
	if c.lastEventSourceActivity.After(last) {
		last = c.lastEventSourceActivity
	}
	if last.IsZero() {
		return c.lastActivity
	}
	return last
}

func (c *networkCollector) settled(readyElapsed, totalElapsed, idle time.Duration) bool {
	if idle <= 0 {
		idle = 1500 * time.Millisecond
	}
	if readyElapsed < 500*time.Millisecond {
		return false
	}
	if c.activeStreamCount() > 0 {
		if totalElapsed < streamMinObservation {
			return false
		}
		streamIdle := max(idle*4, 6*time.Second)
		if time.Since(c.streamQuietSince()) < streamIdle {
			return false
		}
		c.idleReason = "stream_idle"
		if len(c.activeEventSources) > 0 {
			c.idleReason = "event_source_idle"
		}
		return true
	}
	if len(c.active) > 0 {
		if totalElapsed < streamMinObservation {
			return false
		}
		if time.Since(c.lastActivity) < max(idle*4, 6*time.Second) {
			return false
		}
		c.idleReason = "network_idle_with_active_requests"
		return true
	}
	if time.Since(c.lastActivity) < idle {
		return false
	}
	if len(c.activeWebSockets) > 0 {
		c.idleReason = "websocket_idle"
	} else {
		c.idleReason = "network_idle"
	}
	return true
}

func (c *networkCollector) snapshot() ([]NetworkResource, NetworkStats) {
	out := make([]NetworkResource, 0, len(c.order))
	openStreams := c.activeStreamCount()
	for _, requestID := range c.order {
		res := c.resources[requestID]
		if res == nil {
			continue
		}
		if c.isOpenStream(requestID) {
			res.Truncated = true
		}
		if strings.TrimSpace(res.Body) != "" {
			out = append(out, *res)
		}
	}
	stats := NetworkStats{
		HTTPRequests:        c.httpRequests,
		ResourceCount:       len(out),
		ObservedResources:   len(c.order),
		BodyUnavailable:     c.bodyUnavailableCount,
		BodyUnavailableURLs: append([]string(nil), c.bodyUnavailableSample...),
		StreamsOpen:         openStreams,
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

const streamMinObservation = 8 * time.Second

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
