package rendering

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestExecRendererCapturesRenderedDOMAndFinalLocationWhenBrowserAvailable(t *testing.T) {
	browserPath, err := FindBrowserPath("")
	if err != nil {
		t.Skipf("render browser unavailable: %v", err)
	}
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><head><title>Render</title></head><body><main>static</main><script>history.replaceState(null, "", "%s/rendered-final"); document.querySelector("main").textContent = "Rendered JavaScript payload";</script></body></html>`, serverURL)
	}))
	serverURL = server.URL
	defer server.Close()

	page, err := ExecDumpDOMRenderer{BrowserPath: browserPath, Timeout: 6 * time.Second}.Render(context.Background(), Request{
		URL:      server.URL,
		Timeout:  6 * time.Second,
		MaxBytes: 1_000_000,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if page.FinalURL != server.URL+"/rendered-final" {
		t.Fatalf("expected final JS location, got %q", page.FinalURL)
	}
	if !strings.Contains(page.HTML, "Rendered JavaScript payload") {
		t.Fatalf("expected rendered DOM payload, got %q", page.HTML)
	}
}

func TestExecRendererCapturesApplicationNetworkDataWhenBrowserAvailable(t *testing.T) {
	browserPath, err := FindBrowserPath("")
	if err != nil {
		t.Skipf("render browser unavailable: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><title>Network App</title></head><body><main>shell</main><script>
				fetch("/api/data").then(r => r.json()).then(v => document.body.dataset.fetchDone = v.name);
				const events = new EventSource("/stream");
				events.onmessage = event => { if (event.data === "end") events.close(); };
				const ws = new WebSocket("ws://" + location.host + "/ws");
				ws.onmessage = () => ws.close();
			</script></body></html>`)
		case "/api/data":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = fmt.Fprint(w, `{"name":"Fetch Palazzo Aurora","city":"Firenze"}`)
		case "/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = fmt.Fprint(w, "data: {\"name\":\"SSE Villa Serena\",\"city\":\"Siena\"}\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			time.Sleep(50 * time.Millisecond)
			_, _ = fmt.Fprint(w, "data: end\n\n")
		case "/ws":
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "done") //nolint:errcheck
			_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"name":"WebSocket Casa Tramonto","city":"Lucca"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	page, err := ExecDumpDOMRenderer{BrowserPath: browserPath, Timeout: 8 * time.Second}.Render(context.Background(), Request{
		URL:                 server.URL,
		Timeout:             8 * time.Second,
		MaxBytes:            1_000_000,
		NetworkIdle:         500 * time.Millisecond,
		NetworkMaxBytes:     2_000_000,
		NetworkMaxResources: 8,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	networkText := networkResourceBodies(page.NetworkResources)
	for _, expected := range []string{"Fetch Palazzo Aurora", "SSE Villa Serena", "WebSocket Casa Tramonto"} {
		if !strings.Contains(networkText, expected) {
			t.Fatalf("expected network payload %q in resources %#v", expected, page.NetworkResources)
		}
	}
	if page.NetworkStats.ResourceCount < 3 {
		t.Fatalf("expected at least fetch, sse and websocket resources, got %#v", page.NetworkStats)
	}
	if page.NetworkStats.EventSourceMessages == 0 {
		t.Fatalf("expected event source messages, got %#v", page.NetworkStats)
	}
	if page.NetworkStats.WebSocketMessages == 0 {
		t.Fatalf("expected websocket messages, got %#v", page.NetworkStats)
	}
}

func networkResourceBodies(resources []NetworkResource) string {
	parts := make([]string, 0, len(resources))
	for _, resource := range resources {
		parts = append(parts, resource.Body)
	}
	return strings.Join(parts, "\n")
}

func TestNetworkCollectorAppliesMessageBudget(t *testing.T) {
	collector := newNetworkCollector("https://example.com/app", 10_000, 10_000, 4, 1)
	collector.handleResponseReceived(json.RawMessage(`{"requestId":"sse-1","type":"EventSource","response":{"url":"https://example.com/stream","status":200,"mimeType":"text/event-stream","headers":{}}}`))
	collector.handleEventSourceMessage(json.RawMessage(`{"requestId":"sse-1","data":"{\"name\":\"first\"}"}`))
	collector.handleEventSourceMessage(json.RawMessage(`{"requestId":"sse-1","data":"{\"name\":\"second\"}"}`))

	resources, stats := collector.snapshot()
	if len(resources) != 1 {
		t.Fatalf("expected one resource, got %#v", resources)
	}
	if !strings.Contains(resources[0].Body, "first") || strings.Contains(resources[0].Body, "second") {
		t.Fatalf("expected only first message body, got %q", resources[0].Body)
	}
	if resources[0].MessageCount != 2 || stats.EventSourceMessages != 2 || !stats.Truncated {
		t.Fatalf("expected observed messages with truncation, got resource=%#v stats=%#v", resources[0], stats)
	}
}

func TestNetworkCollectorSettlesLongIdleEventSourceWithoutClose(t *testing.T) {
	collector := newNetworkCollector("https://example.com/app", 10_000, 10_000, 4, 16)
	collector.handleResponseReceived(json.RawMessage(`{"requestId":"sse-1","type":"EventSource","response":{"url":"https://example.com/stream","status":200,"mimeType":"text/event-stream","headers":{}}}`))
	collector.handleEventSourceMessage(json.RawMessage(`{"requestId":"sse-1","data":"{\"name\":\"still-open\"}"}`))
	collector.lastActivity = time.Now().Add(-16 * time.Second)
	collector.lastEventSourceActivity = time.Now().Add(-16 * time.Second)

	if collector.settled(600*time.Millisecond, 11*time.Second, 500*time.Millisecond) {
		t.Fatal("expected active event source to wait for the minimum observation window")
	}
	if !collector.settled(600*time.Millisecond, 12*time.Second, 500*time.Millisecond) {
		t.Fatal("expected active idle event source to settle after the minimum observation window")
	}
	_, stats := collector.snapshot()
	if stats.IdleReason != "event_source_idle" {
		t.Fatalf("expected event_source_idle, got %#v", stats)
	}
}

func TestNetworkCollectorDoesNotCloseActiveEventSourceOnLoadingFinishedAfterMessages(t *testing.T) {
	collector := newNetworkCollector("https://example.com/app", 10_000, 10_000, 4, 16)
	collector.handleResponseReceived(json.RawMessage(`{"requestId":"sse-1","type":"EventSource","response":{"url":"https://example.com/stream","status":200,"mimeType":"text/event-stream","headers":{}}}`))
	collector.handleEventSourceMessage(json.RawMessage(`{"requestId":"sse-1","data":"{\"name\":\"partial\"}"}`))
	collector.handleLoadingDone(json.RawMessage(`{"requestId":"sse-1"}`), true)
	if _, ok := collector.activeEventSources["sse-1"]; !ok {
		t.Fatal("expected event source with messages to stay active until message idle or explicit end")
	}

	collector.handleEventSourceMessage(json.RawMessage(`{"requestId":"sse-1","data":"end"}`))
	if _, ok := collector.activeEventSources["sse-1"]; ok {
		t.Fatal("expected explicit end message to close active event source")
	}
}

func TestNetworkCollectorFiltersStaticAssetsAndUnknownMessageSources(t *testing.T) {
	collector := newNetworkCollector("https://example.com/app", 10_000, 10_000, 8, 16)
	collector.handleResponseReceived(json.RawMessage(`{"requestId":"doc","type":"Document","response":{"url":"https://example.com/app","status":200,"mimeType":"text/html","headers":{}}}`))
	collector.handleResponseReceived(json.RawMessage(`{"requestId":"css","type":"Stylesheet","response":{"url":"https://example.com/app.css","status":200,"mimeType":"text/css","headers":{}}}`))
	collector.handleResponseReceived(json.RawMessage(`{"requestId":"script","type":"Script","response":{"url":"https://example.com/app.js","status":200,"mimeType":"text/javascript","headers":{}}}`))
	collector.handleEventSourceMessage(json.RawMessage(`{"requestId":"foreign-sse","data":"{\"name\":\"should-not-appear\"}"}`))
	collector.handleWebSocketFrameReceived(json.RawMessage(`{"requestId":"foreign-ws","response":{"opcode":1,"payloadData":"{\"name\":\"should-not-appear\"}"}}`))

	resources, stats := collector.snapshot()
	if len(resources) != 0 || stats.EventSourceMessages != 0 || stats.WebSocketMessages != 0 {
		t.Fatalf("expected static assets and unknown messages to be ignored, resources=%#v stats=%#v", resources, stats)
	}

	collector.handleResponseReceived(json.RawMessage(`{"requestId":"api","type":"Other","response":{"url":"https://example.com/data.json","status":200,"mimeType":"application/json","headers":{}}}`))
	collector.appendResponseBody("api", `{"name":"Application JSON payload"}`)
	resources, _ = collector.snapshot()
	if len(resources) != 1 || !strings.Contains(resources[0].Body, "Application JSON payload") {
		t.Fatalf("expected same-site application JSON resource, got %#v", resources)
	}
}
