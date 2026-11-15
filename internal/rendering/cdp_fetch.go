package rendering

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/platform"
)

// fetchInterceptor answers CDP Fetch pauses without waiting for the command
// that caused them. A synchronous command from the event callback would
// deadlock the CDP socket because the client dispatches events while waiting
// for the command response; writing the continuation directly keeps that
// command moving while its own response remains the next matching read.
type fetchInterceptor struct {
	cdp       *cdpClient
	ctx       context.Context
	sessionID string
	session   *platform.Session
	err       error
}

func (f *fetchInterceptor) handleEvent(msg cdpMessage) {
	if msg.Method != "Fetch.requestPaused" || f.err != nil {
		return
	}
	var event struct {
		RequestID string `json:"requestId"`
		Request   struct {
			URL     string         `json:"url"`
			Headers map[string]any `json:"headers"`
		} `json:"request"`
	}
	if json.Unmarshal(msg.Params, &event) != nil || strings.TrimSpace(event.RequestID) == "" {
		return
	}
	params := map[string]any{"requestId": event.RequestID}
	if f.session != nil {
		if rules := f.session.HeadersFor(event.Request.URL); len(rules) > 0 {
			params["headers"] = mergeFetchHeaders(fetchHeaders(event.Request.Headers), rules)
		}
	}
	if err := f.cdp.send(f.ctx, f.sessionID, "Fetch.continueRequest", params); err != nil {
		f.err = fmt.Errorf("continue intercepted browser request: %w", err)
	}
}

func (f *fetchInterceptor) takeError() error {
	if f == nil {
		return nil
	}
	return f.err
}

func fetchHeaders(raw map[string]any) map[string]string {
	headers := make(map[string]string, len(raw))
	for name, value := range raw {
		if text := headerValue(map[string]any{name: value}, name); text != "" {
			headers[name] = text
		}
	}
	return headers
}

func mergeFetchHeaders(original, rules map[string]string) []map[string]string {
	merged := make([]map[string]string, 0, len(original)+len(rules))
	for name, value := range original {
		managed := false
		for ruleName := range rules {
			if strings.EqualFold(name, ruleName) {
				managed = true
				break
			}
		}
		if !managed {
			merged = append(merged, map[string]string{"name": name, "value": value})
		}
	}
	for name, value := range rules {
		merged = append(merged, map[string]string{"name": name, "value": value})
	}
	return merged
}
