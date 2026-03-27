package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
)

func TestRunnerMCPInitializeAndToolsList(t *testing.T) {
	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	runner := NewRunner()
	runner.stdin = strings.NewReader(input)

	code := runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}

	responses := decodeMCPResponses(t, stdout.Bytes())
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	if !strings.Contains(string(responses[0]), `"protocolVersion":"2024-11-05"`) {
		t.Fatalf("expected initialize response, got %s", responses[0])
	}
	if !strings.Contains(string(responses[1]), `"web_crawl"`) {
		t.Fatalf("expected tools list to include web_crawl, got %s", responses[1])
	}
	for _, tool := range []string{"web_query", "web_read", "web_replay", "web_diff", "web_proof", "web_prune"} {
		if !strings.Contains(string(responses[1]), tool) {
			t.Fatalf("expected tools list to include %q, got %s", tool, responses[1])
		}
	}
}

func TestRunnerMCPReadReplayAndProof(t *testing.T) {
	root := t.TempDir()
	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "web_read",
				"arguments": map[string]any{
					"url":     "https://example.com",
					"profile": "tiny",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "web_replay",
				"arguments": map[string]any{
					"trace_id": "trace_mcp",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"id":      4,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "web_proof",
				"arguments": map[string]any{
					"chunk_id": "chk_1",
				},
			},
		},
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			resp := fakeResponse()
			resp.Trace.TraceID = "trace_mcp"
			resp.Trace.RunID = "trace_mcp"
			return resp, nil
		},
		stdin:     strings.NewReader(input),
		storeRoot: root,
	}

	code := runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}

	responses := decodeMCPResponses(t, stdout.Bytes())
	if len(responses) != 4 {
		t.Fatalf("expected 4 responses, got %d", len(responses))
	}
	if !strings.Contains(string(responses[1]), `"trace_id":"trace_mcp"`) {
		t.Fatalf("expected web_read response to include trace id, got %s", responses[1])
	}
	if !strings.Contains(string(responses[2]), `"replay_report"`) {
		t.Fatalf("expected replay report, got %s", responses[2])
	}
	if !strings.Contains(string(responses[3]), `"proof"`) {
		t.Fatalf("expected proof payload, got %s", responses[3])
	}
	assertMCPStructuredKeys(t, responses[1], "document", "chunks", "proof_refs", "cost_report")
	assertMCPStructuredKeys(t, responses[2], "replay_report")
	assertMCPStructuredKeys(t, responses[3], "proof_records")
}

func TestRunnerMCPQuery(t *testing.T) {
	root := t.TempDir()
	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "web_query",
				"arguments": map[string]any{
					"goal":     "proof replay deterministic",
					"seed_url": "https://example.com",
					"profile":  "tiny",
				},
			},
		},
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := Runner{
		loadConfig: config.Load,
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			return fakeQueryResponse(req), nil
		},
		stdin:     strings.NewReader(input),
		storeRoot: root,
	}

	code := runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}

	responses := decodeMCPResponses(t, stdout.Bytes())
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	if !strings.Contains(string(responses[1]), `"result_pack"`) {
		t.Fatalf("expected query payload, got %s", responses[1])
	}
	assertMCPStructuredKeys(t, responses[1], "plan", "result_pack", "proof_refs", "trace_id")
}

func TestRunnerMCPCrawl(t *testing.T) {
	root := t.TempDir()
	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "web_crawl",
				"arguments": map[string]any{
					"seed_url":    "https://example.com",
					"max_pages":   2,
					"max_depth":   1,
					"same_domain": true,
				},
			},
		},
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := Runner{
		loadConfig: config.Load,
		crawl: func(ctx context.Context, cfg config.Config, req coreservice.CrawlRequest) (coreservice.CrawlResponse, error) {
			return fakeCrawlResponse(), nil
		},
		stdin:     strings.NewReader(input),
		storeRoot: root,
	}

	code := runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}

	responses := decodeMCPResponses(t, stdout.Bytes())
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	if !strings.Contains(string(responses[1]), `"summary"`) {
		t.Fatalf("expected crawl summary, got %s", responses[1])
	}
	assertMCPStructuredKeys(t, responses[1], "documents", "summary", "stored_runs")
}

func framedMessages(t *testing.T, messages ...map[string]any) string {
	t.Helper()

	var buf bytes.Buffer
	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			t.Fatalf("marshal message: %v", err)
		}
		buf.WriteString("Content-Length: ")
		buf.WriteString(jsonLength(data))
		buf.WriteString("\r\n\r\n")
		buf.Write(data)
	}
	return buf.String()
}

func decodeMCPResponses(t *testing.T, data []byte) [][]byte {
	t.Helper()

	reader := bytes.NewReader(data)
	buffered := bufio.NewReader(reader)
	out := [][]byte{}
	for {
		frame, err := readMCPFrame(buffered)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("read frame: %v", err)
		}
		out = append(out, frame)
	}
	return out
}

func jsonLength(data []byte) string {
	return strconv.Itoa(len(data))
}

func assertMCPStructuredKeys(t *testing.T, frame []byte, keys ...string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(frame, &payload); err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %#v", payload["result"])
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("expected structuredContent object, got %#v", result["structuredContent"])
	}
	for _, key := range keys {
		if _, ok := structured[key]; !ok {
			t.Fatalf("expected structuredContent key %q, got %#v", key, structured)
		}
	}
}
