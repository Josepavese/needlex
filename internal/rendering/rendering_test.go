package rendering

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestExecRendererCapturesRenderedDOMAndFinalLocationWhenBrowserAvailable(t *testing.T) {
	browserPath := runnableBrowserOrSkip(t)
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><head><title>Render</title></head><body><main>static</main><script>history.replaceState(null, "", "%s/rendered-final"); document.querySelector("main").textContent = "Rendered JavaScript payload";</script></body></html>`, serverURL)
	}))
	serverURL = server.URL
	defer server.Close()

	page, err := ExecDumpDOMRenderer{BrowserPath: browserPath, Timeout: 15 * time.Second}.Render(context.Background(), Request{
		URL:      server.URL,
		Timeout:  15 * time.Second,
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
	browserPath := runnableBrowserOrSkip(t)
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

func TestExecRendererCapturesStreamingFetchBodyWhenBrowserAvailable(t *testing.T) {
	browserPath := runnableBrowserOrSkip(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><title>Streaming Fetch App</title></head><body><main>stream shell</main><script>
				fetch("/api/stream").then(r => r.text()).then(t => { document.body.dataset.streamDone = String(t.length); });
			</script></body></html>`)
		case "/api/stream":
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = fmt.Fprint(w, "data: {\"name\":\"Streaming Fetch Palazzo\"}\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			time.Sleep(50 * time.Millisecond)
			_, _ = fmt.Fprint(w, "data: end\n\n")
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
	if !strings.Contains(networkText, "Streaming Fetch Palazzo") {
		t.Fatalf("expected streaming fetch payload in resources %#v", page.NetworkResources)
	}
	if page.NetworkStats.ResourceCount == 0 {
		t.Fatalf("expected retained streaming resource, got %#v", page.NetworkStats)
	}
}

func TestExecRendererCapturesSlowEventSourceWhenBrowserAvailable(t *testing.T) {
	browserPath := runnableBrowserOrSkip(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprint(w, `<html><head><title>Slow SSE App</title></head><body><main>slow shell</main><script>
				const events = new EventSource("/slow-stream");
				events.onmessage = event => { if (event.data === "end") events.close(); };
			</script></body></html>`)
		case "/slow-stream":
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher, _ := w.(http.Flusher)
			for i := 1; i <= 3; i++ {
				_, _ = fmt.Fprintf(w, "data: {\"name\":\"Slow SSE batch %d\"}\n\n", i)
				if flusher != nil {
					flusher.Flush()
				}
				time.Sleep(3200 * time.Millisecond)
			}
			_, _ = fmt.Fprint(w, "data: end\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	page, err := ExecDumpDOMRenderer{BrowserPath: browserPath, Timeout: 20 * time.Second}.Render(context.Background(), Request{
		URL:                 server.URL,
		Timeout:             20 * time.Second,
		MaxBytes:            1_000_000,
		NetworkIdle:         800 * time.Millisecond,
		NetworkMaxBytes:     4_000_000,
		NetworkMaxResources: 8,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if page.NetworkStats.EventSourceMessages < 3 {
		t.Fatalf("expected slow event source batches beyond the former render cap, got %#v", page.NetworkStats)
	}
	networkText := networkResourceBodies(page.NetworkResources)
	for _, expected := range []string{"Slow SSE batch 1", "Slow SSE batch 3"} {
		if !strings.Contains(networkText, expected) {
			t.Fatalf("expected slow event source payload %q in resources %#v", expected, page.NetworkResources)
		}
	}
}

func TestExecRendererReportsDegradedDumpDOMFallback(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "fake-browser.sh")
	script := "#!/bin/sh\nfor arg in \"$@\"; do\n  case \"$arg\" in\n    --remote-debugging-port=*) exit 1;;\n  esac\ndone\nprintf '<html><body><main>fallback dom payload</main></body></html>'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil { //nolint:gosec
		t.Fatalf("write fake browser: %v", err)
	}

	page, err := ExecDumpDOMRenderer{BrowserPath: scriptPath}.Render(context.Background(), Request{
		URL:     "https://example.com/app",
		Timeout: 1200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("expected dump-dom fallback to succeed, got %v", err)
	}
	if !page.Degraded || !strings.Contains(page.DegradeReason, "cdp_unavailable") {
		t.Fatalf("expected degraded render report, got %#v", page)
	}
	if !strings.Contains(page.HTML, "fallback dom payload") {
		t.Fatalf("expected fallback DOM payload, got %q", page.HTML)
	}
	if page.NetworkStats.ObservedResources != 0 {
		t.Fatalf("expected no network evidence on degraded render, got %#v", page.NetworkStats)
	}
}

func runnableBrowserOrSkip(t *testing.T) string {
	t.Helper()
	browserPath, err := FindBrowserPath("")
	if err != nil {
		t.Skipf("render browser unavailable: %v", err)
	}
	probeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := []string{}
	if !isHeadlessShell(browserPath) {
		args = append(args, "--headless=new")
	}
	args = append(args,
		"--no-sandbox",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--dump-dom",
		"data:text/html,<main>Needle-X%20browser%20capability%20probe</main>",
	)
	output, err := exec.CommandContext(probeCtx, browserPath, args...).CombinedOutput()
	if err != nil || !strings.Contains(string(output), "Needle-X browser capability probe") {
		message := strings.TrimSpace(string(output))
		if len(message) > 300 {
			message = message[:300]
		}
		t.Skipf("render browser is installed but not runnable in this environment: err=%v output=%q", err, message)
	}
	return browserPath
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

	if collector.settled(600*time.Millisecond, 7*time.Second, 500*time.Millisecond) {
		t.Fatal("expected active event source to respect the minimum observation window")
	}
	if !collector.settled(600*time.Millisecond, 9*time.Second, 500*time.Millisecond) {
		t.Fatal("expected quiet active event source to settle after the minimum observation window")
	}
	resources, stats := collector.snapshot()
	if stats.IdleReason != "event_source_idle" {
		t.Fatalf("expected event_source_idle, got %#v", stats)
	}
	if stats.StreamsOpen != 1 || !resources[0].Truncated {
		t.Fatalf("expected open stream to be reported as truncated, resources=%#v stats=%#v", resources, stats)
	}
}

func TestNetworkCollectorCapturesStreamingFetchResponse(t *testing.T) {
	collector := newNetworkCollector("https://example.com/app", 10_000, 10_000, 4, 16)
	collector.handleResponseReceived(json.RawMessage(`{"requestId":"stream-1","type":"Fetch","response":{"url":"https://example.com/api/stream","status":200,"mimeType":"text/event-stream; charset=utf-8","headers":{}}}`))

	if _, ok := collector.activeStreams["stream-1"]; !ok {
		t.Fatal("expected streaming fetch response to be tracked as an active stream")
	}
	if collector.resources["stream-1"].Source == "event_source" {
		t.Fatal("expected streaming fetch response to keep response provenance so its body stays capturable")
	}
	if !collector.shouldFetchResponseBody(collector.resources["stream-1"]) {
		t.Fatal("expected streaming fetch body to be capturable")
	}
	collector.handleDataReceived(json.RawMessage(`{"requestId":"stream-1","dataLength":4096}`))
	if time.Since(collector.lastStreamActivity) > time.Second {
		t.Fatal("expected received stream data to count as stream activity")
	}
	collector.handleLoadingDone(json.RawMessage(`{"requestId":"stream-1"}`), true)
	collector.appendResponseBody("stream-1", "data: {\"name\":\"Streaming Palazzo\"}\n\n")

	resources, stats := collector.snapshot()
	if len(resources) != 1 || !strings.Contains(resources[0].Body, "Streaming Palazzo") {
		t.Fatalf("expected streaming fetch payload to be retained, resources=%#v stats=%#v", resources, stats)
	}
	if stats.StreamsOpen != 0 || stats.ObservedResources != 1 || stats.ResourceCount != 1 {
		t.Fatalf("expected closed stream to be counted as observed and retained, got %#v", stats)
	}
}

func TestNetworkCollectorReportsUnreadBodiesAndObservedResources(t *testing.T) {
	collector := newNetworkCollector("https://example.com/app", 10_000, 10_000, 8, 16)
	collector.handleResponseReceived(json.RawMessage(`{"requestId":"api-1","type":"XHR","response":{"url":"https://example.com/api/one","status":200,"mimeType":"application/json","headers":{}}}`))
	collector.handleResponseReceived(json.RawMessage(`{"requestId":"api-2","type":"XHR","response":{"url":"https://example.com/api/two","status":200,"mimeType":"application/json","headers":{}}}`))
	collector.markBodyUnavailable("api-1")

	_, stats := collector.snapshot()
	if stats.ObservedResources != 2 || stats.ResourceCount != 0 || stats.BodyUnavailable != 1 {
		t.Fatalf("expected observed resources with unread body to be reported, got %#v", stats)
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
