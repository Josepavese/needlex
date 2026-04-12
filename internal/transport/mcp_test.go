package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr noise, got %q", stderr.String())
	}
	if !strings.Contains(string(responses[1]), `"web_crawl"`) {
		t.Fatalf("expected tools list to include web_crawl, got %s", responses[1])
	}
	for _, tool := range []string{"web_query", "web_read", "web_replay", "web_diff", "web_proof", "web_prune"} {
		if !strings.Contains(string(responses[1]), tool) {
			t.Fatalf("expected tools list to include %q, got %s", tool, responses[1])
		}
	}
	for _, tool := range []string{"memory_stats", "memory_search", "memory_prune", "memory_export", "memory_import", "memory_rebuild_index"} {
		if !strings.Contains(string(responses[1]), tool) {
			t.Fatalf("expected tools list to include %q, got %s", tool, responses[1])
		}
	}
}

func TestRunnerMCPInitializeAndToolsListRawJSON(t *testing.T) {
	input := rawMessages(
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
	if strings.Contains(stdout.String(), "Content-Length:") {
		t.Fatalf("expected raw json output, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr noise, got %q", stderr.String())
	}
	responses := decodeRawMCPResponses(t, stdout.Bytes())
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	if !strings.Contains(string(responses[0]), `"protocolVersion":"2024-11-05"`) {
		t.Fatalf("expected initialize response, got %s", responses[0])
	}
	if !strings.Contains(string(responses[1]), `"web_query"`) {
		t.Fatalf("expected tools list to include web_query, got %s", responses[1])
	}
}

func TestRunnerMCPCreateStableStateRootWhenUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("NEEDLEX_HOME", "")
	t.Setenv("NEEDLEX_MCP_LOG", filepath.Join(t.TempDir(), "needlex-mcp.log"))
	input := rawMessages(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"})
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	runner := NewRunner()
	runner.stdin = strings.NewReader(input)
	runner.storeRoot = ".needlex"

	code := runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}
	wantRoot := filepath.Join(home, ".local", "share", "needlex")
	if _, err := os.Stat(wantRoot); err != nil {
		t.Fatalf("expected stable store root %q to exist: %v", wantRoot, err)
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
	assertMCPStructuredKeys(t, responses[1], "document", "web_ir", "chunks", "agent_context", "proof_refs", "cost_report", "compact", "summary", "uncertainty")
	assertMCPStructuredKeys(t, responses[2], "replay_report")
	assertMCPStructuredKeys(t, responses[3], "proof_records")
}

func TestRunnerMCPProofByID(t *testing.T) {
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
					"url": "https://example.com",
				},
			},
		},
		map[string]any{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "web_proof",
				"arguments": map[string]any{
					"proof_id": "proof_1",
				},
			},
		},
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			return fakeResponse(), nil
		},
		stdin:     strings.NewReader(input),
		storeRoot: root,
	}

	code := runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}

	responses := decodeMCPResponses(t, stdout.Bytes())
	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}
	if !strings.Contains(string(responses[2]), `"proof_records"`) || !strings.Contains(string(responses[2]), `"trace_id":"trace_1"`) {
		t.Fatalf("expected proof lookup by proof_id, got %s", responses[2])
	}
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
	assertMCPStructuredKeys(t, responses[1], "plan", "document", "web_ir", "result_pack", "agent_context", "proof_refs", "trace_id", "compact", "summary", "selected_url")
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

func TestRunnerMCPMemoryTools(t *testing.T) {
	root := t.TempDir()
	semantic := newMemoryEmbeddingServer()
	defer semantic.Close()

	cfg := config.Defaults()
	cfg.Memory.Enabled = true
	cfg.Semantic.Enabled = true
	cfg.Semantic.Backend = "openai-embeddings"
	cfg.Semantic.BaseURL = semantic.URL
	cfg.Semantic.Model = "memory-test-embed"
	cfg.Memory.EmbeddingBackend = cfg.Semantic.Backend
	cfg.Memory.EmbeddingModel = cfg.Semantic.Model
	configPath := filepath.Join(root, "needlex-memory.json")
	rawCfg, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, rawCfg, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	seedMemoryDocument(t, root, cfg, semantic.Client(), "https://playwright.dev/docs/intro", "Installation | Playwright", "Install Playwright and run the installation command to download browser binaries.")
	exportDir := filepath.Join(root, "memory-export")
	importRoot := t.TempDir()

	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "memory_stats", "arguments": map[string]any{"config_path": configPath}}},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "memory_search", "arguments": map[string]any{"query": "playwright installation", "config_path": configPath}}},
		map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": "memory_export", "arguments": map[string]any{"out_dir": exportDir, "config_path": configPath}}},
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := Runner{loadConfig: config.Load, stdin: strings.NewReader(input), storeRoot: root}
	code := runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}
	responses := decodeMCPResponses(t, stdout.Bytes())
	if len(responses) != 4 {
		t.Fatalf("expected 4 responses, got %d", len(responses))
	}
	assertMCPStructuredKeys(t, responses[1], "stats", "compact")
	assertMCPStructuredKeys(t, responses[2], "candidates", "compact")
	assertMCPStructuredKeys(t, responses[3], "export", "compact")

	importCfgPath := filepath.Join(importRoot, "needlex-memory.json")
	if err := os.WriteFile(importCfgPath, rawCfg, 0o644); err != nil {
		t.Fatalf("write import config: %v", err)
	}
	input = framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "memory_import", "arguments": map[string]any{"in_dir": exportDir, "config_path": importCfgPath}}},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "memory_rebuild_index", "arguments": map[string]any{"config_path": importCfgPath}}},
	)
	stdout.Reset()
	stderr.Reset()
	runner = Runner{loadConfig: config.Load, stdin: strings.NewReader(input), storeRoot: importRoot}
	code = runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}
	responses = decodeMCPResponses(t, stdout.Bytes())
	if len(responses) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(responses))
	}
	assertMCPStructuredKeys(t, responses[1], "import", "compact")
	assertMCPStructuredKeys(t, responses[2], "stats", "compact")
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

func rawMessages(t *testing.T, messages ...map[string]any) string {
	t.Helper()
	var buf bytes.Buffer
	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			t.Fatalf("marshal message: %v", err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
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

func decodeRawMCPResponses(t *testing.T, data []byte) [][]byte {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	out := make([][]byte, 0, len(lines))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(line, &payload); err != nil {
			t.Fatalf("decode raw response: %v line=%q", err, line)
		}
		out = append(out, line)
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

func TestRunnerMCPQueryToolSchemaIncludesDiscoveryModeEnum(t *testing.T) {
	tools := mcpTools()
	for _, tool := range tools {
		if tool.Name != "web_query" {
			continue
		}
		props, _ := tool.InputSchema["properties"].(map[string]any)
		dm, _ := props["discovery_mode"].(map[string]any)
		enumVals, _ := dm["enum"].([]string)
		if len(enumVals) != 3 || enumVals[0] != "same_site_links" || enumVals[1] != "web_search" || enumVals[2] != "off" {
			t.Fatalf("unexpected discovery_mode enum: %#v", dm["enum"])
		}
		if !strings.Contains(dm["description"].(string), "same_site_links") {
			t.Fatalf("expected discovery_mode description to mention canonical values, got %#v", dm["description"])
		}
		if !strings.Contains(tool.Description, "exact canonical page") {
			t.Fatalf("expected web_query description to guide strict off mode, got %#v", tool.Description)
		}
		seedURL, _ := props["seed_url"].(map[string]any)
		if !strings.Contains(seedURL["description"].(string), "exact canonical page") {
			t.Fatalf("expected seed_url description to guide strict off mode, got %#v", seedURL["description"])
		}
		return
	}
	t.Fatal("web_query tool not found")
}

func TestRunnerMCPToolsExposeExamples(t *testing.T) {
	for _, tool := range mcpTools() {
		if tool.Name == "" {
			continue
		}
		examples, ok := tool.InputSchema["examples"].([]map[string]any)
		if !ok || len(examples) == 0 {
			t.Fatalf("tool %s missing schema examples", tool.Name)
		}
	}
}
