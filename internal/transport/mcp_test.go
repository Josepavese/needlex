package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/analytics"
	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/intel"
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
	for _, tool := range []string{"memory", "analytics"} {
		if !strings.Contains(string(responses[1]), tool) {
			t.Fatalf("expected tools list to include %q, got %s", tool, responses[1])
		}
	}
	for _, tool := range []string{"memory_stats", "memory_search", "memory_prune", "memory_export", "memory_import", "memory_rebuild_index", "analytics_stats", "analytics_recent_runs", "analytics_value_report", "analytics_hosts", "analytics_providers", "analytics_failures", "analytics_daily", "analytics_export"} {
		if strings.Contains(string(responses[1]), `"`+tool+`"`) {
			t.Fatalf("expected retired tool %q not to be advertised, got %s", tool, responses[1])
		}
	}
}

func TestRunnerMCPPruneEmbeddingCacheDryRun(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, "data", "embeddings", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("create embedding cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "entry.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("seed embedding cache: %v", err)
	}
	runner := Runner{storeRoot: root}
	result, err := runner.callMCPPruneTool(map[string]any{"embedding_cache": true, "dry_run": true})
	if err != nil {
		t.Fatalf("mcp prune embedding cache: %v", err)
	}
	structured := result["structuredContent"].(map[string]any)
	report := structured["prune_report"].(intel.EmbeddingCachePruneReport)
	if report.MatchedFiles != 1 || !report.DryRun {
		t.Fatalf("unexpected mcp prune report: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "entry.json")); err != nil {
		t.Fatalf("dry-run must not remove cache file: %v", err)
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

func TestMCPToolErrorMessageSuggestsNextCall(t *testing.T) {
	message := mcpToolErrorMessage(fmt.Errorf("seed_url returned 404; discovery_mode=off requires an exact canonical page"))
	if !strings.Contains(message, "next_recommended_call") || !strings.Contains(message, "same_site_links") || !strings.Contains(message, "web_read") || strings.Contains(message, `discovery_mode="web_search"`) {
		t.Fatalf("expected actionable MCP error, got %q", message)
	}
}

func TestRunnerMCPQueryWithoutSeedRequiresExplicitExperimentalOptIn(t *testing.T) {
	runner := Runner{storeRoot: t.TempDir()}
	_, err := runner.callMCPQueryTool(map[string]any{"goal": "proof replay deterministic"})
	if err == nil || !strings.Contains(err.Error(), "seedless discovery is experimental") || !strings.Contains(err.Error(), `discovery_mode="web_search"`) {
		t.Fatalf("expected explicit experimental opt-in error, got %v", err)
	}
	message := mcpToolErrorMessage(err)
	if !strings.Contains(message, "host agent's search tool") || !strings.Contains(message, "web_read") {
		t.Fatalf("expected stable workflow guidance, got %q", message)
	}
}

func TestMCPToolErrorMessageGuidesRenamedRetrievalEffort(t *testing.T) {
	message := mcpToolErrorMessage(fmt.Errorf("unsupported field lane_max"))
	if !strings.Contains(message, "retrieval_effort") || !strings.Contains(message, "exhaustive") {
		t.Fatalf("expected guided retrieval_effort error, got %q", message)
	}
}

func TestMCPRetrievalEffortSchemaGuidesAgents(t *testing.T) {
	for _, tool := range []mcpTool{mcpQueryTool(), mcpReadTool()} {
		props, ok := tool.InputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s: expected properties map", tool.Name)
		}
		if _, ok := props["lane_max"]; ok {
			t.Fatalf("%s: lane_max should not be exposed in agent-facing schema", tool.Name)
		}
		retrievalEffort, ok := props["retrieval_effort"].(map[string]any)
		if !ok {
			t.Fatalf("%s: expected retrieval_effort schema", tool.Name)
		}
		if retrievalEffort["type"] != "string" {
			t.Fatalf("%s: expected string retrieval_effort, got %#v", tool.Name, retrievalEffort)
		}
		if retrievalEffort["default"] != retrievalEffortStandard {
			t.Fatalf("%s: expected default retrieval_effort %q, got %#v", tool.Name, retrievalEffortStandard, retrievalEffort["default"])
		}
		desc, _ := retrievalEffort["description"].(string)
		if !strings.Contains(desc, "not a result count") || !strings.Contains(desc, "page count") {
			t.Fatalf("%s: expected agent-guiding retrieval_effort description, got %q", tool.Name, desc)
		}
	}
}

func TestMCPMemorySchemaUsesCurrentSearchContract(t *testing.T) {
	props, ok := mcpMemoryTool().InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected memory schema properties")
	}
	if _, ok := props["goal"]; ok {
		t.Fatal("memory search must expose query, not goal alias")
	}
	domainHints, ok := props["domain_hints"].(map[string]any)
	if !ok || domainHints["type"] != "array" {
		t.Fatalf("expected domain_hints array schema, got %#v", domainHints)
	}
}

func TestRunnerMCPCreateStableStateRootWhenUnset(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("NEEDLEX_HOME", "")
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

func TestRunnerMCPToolErrorsGoToRuntimeLog(t *testing.T) {
	root := t.TempDir()
	input := rawMessages(
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
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	runner := Runner{
		loadConfig: config.Load,
		read: func(context.Context, config.Config, coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			return coreservice.ReadResponse{}, errors.New("unexpected status code 403 TOKEN=super-secret")
		},
		stdin:     strings.NewReader(input),
		storeRoot: root,
	}

	code := runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr noise, got %q", stderr.String())
	}
	responses := decodeRawMCPResponses(t, stdout.Bytes())
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	if !strings.Contains(string(responses[1]), "diagnostic_id") || strings.Contains(string(responses[1]), "super-secret") {
		t.Fatalf("expected redacted MCP diagnostic id, got %s", responses[1])
	}
	events, err := runner.runtimeLogger().Tail(1)
	if err != nil {
		t.Fatalf("tail runtime log: %v", err)
	}
	if len(events) != 1 || events[0].Surface != "mcp" || events[0].Operation != "web_read" {
		t.Fatalf("unexpected runtime event: %+v", events)
	}
	if strings.Contains(events[0].Error, "super-secret") || !strings.Contains(events[0].Error, "[REDACTED]") {
		t.Fatalf("expected redacted MCP error log, got %q", events[0].Error)
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
	assertMCPStructuredKeys(t, responses[1], "chunks", "cost_report", "summary", "uncertainty", "trace_id", "trace_path", "proof_path", "fingerprint_path")
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
	if !strings.Contains(string(responses[1]), `"selected_url"`) || !strings.Contains(string(responses[1]), `"chunks"`) {
		t.Fatalf("expected query compact payload, got %s", responses[1])
	}
	assertMCPStructuredKeys(t, responses[1], "goal", "selected_url", "chunks", "cost_report", "summary", "uncertainty", "trace_id", "trace_path", "proof_path", "fingerprint_path")
	assertMCPStructuredAbsentKeys(t, responses[1], "plan", "document", "web_ir", "result_pack", "agent_context", "compact", "proof_refs")
}

func TestRunnerMCPQueryAppliesRetrievalEffort(t *testing.T) {
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
					"goal":             "proof replay deterministic",
					"seed_url":         "https://example.com",
					"profile":          "tiny",
					"retrieval_effort": "exhaustive",
					"discovery_mode":   "same_site_links",
				},
			},
		},
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	observedLaneMax := -1
	runner := Runner{
		loadConfig: config.Load,
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			observedLaneMax = cfg.Runtime.LaneMax
			return fakeQueryResponse(req), nil
		},
		stdin:     strings.NewReader(input),
		storeRoot: root,
	}

	code := runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}
	if observedLaneMax != retrievalEffortLanes[retrievalEffortExhaustive] {
		t.Fatalf("expected retrieval_effort exhaustive to set lane %d, got %d", retrievalEffortLanes[retrievalEffortExhaustive], observedLaneMax)
	}
	responses := decodeMCPResponses(t, stdout.Bytes())
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	if strings.Contains(string(responses[1]), `"warnings"`) {
		t.Fatalf("expected no warning for valid retrieval_effort, got %s", responses[1])
	}
}

func TestRunnerMCPRejectsUnsupportedLaneMax(t *testing.T) {
	runner := Runner{storeRoot: t.TempDir()}
	_, err := runner.callMCPTool(mcpToolCall{Name: "web_query", Arguments: map[string]any{
		"goal":     "proof replay deterministic",
		"seed_url": "https://example.com",
		"lane_max": 5,
	}})
	if err == nil || !strings.Contains(err.Error(), "unsupported field lane_max") || !strings.Contains(err.Error(), "retrieval_effort") {
		t.Fatalf("expected lane_max rejection, got %v", err)
	}
}

func TestRunnerMCPRejectsAmbiguousQualityBudget(t *testing.T) {
	runner := Runner{storeRoot: t.TempDir()}
	_, err := runner.callMCPTool(mcpToolCall{Name: "web_read", Arguments: map[string]any{
		"url":            "https://example.com",
		"quality_budget": "maximum",
	}})
	if err == nil || !strings.Contains(err.Error(), "unsupported field quality_budget") || !strings.Contains(err.Error(), "retrieval_effort") {
		t.Fatalf("expected ambiguous quality_budget rejection, got %v", err)
	}
}

func TestRunnerMCPRejectsStringDomainHints(t *testing.T) {
	runner := Runner{storeRoot: t.TempDir()}
	_, err := runner.callMCPTool(mcpToolCall{Name: "memory", Arguments: map[string]any{
		"action":       "search",
		"query":        "playwright installation",
		"domain_hints": "playwright.dev",
	}})
	if err == nil || !strings.Contains(err.Error(), "domain_hints must be an array of strings") {
		t.Fatalf("expected domain_hints type rejection, got %v", err)
	}
}

func TestIntArgRejectsNonIntegralFloat(t *testing.T) {
	if value, ok := intArg(map[string]any{"limit": float64(1.9)}, "limit"); ok || value != 0 {
		t.Fatalf("expected non-integral float to be rejected, got value=%d ok=%t", value, ok)
	}
	if value, ok := intArg(map[string]any{"limit": float64(2)}, "limit"); !ok || value != 2 {
		t.Fatalf("expected integral float to be accepted, got value=%d ok=%t", value, ok)
	}
}

func TestMCPToolSchemasDisallowAdditionalProperties(t *testing.T) {
	for _, tool := range mcpTools() {
		if got := tool.InputSchema["additionalProperties"]; got != false {
			t.Fatalf("%s: expected additionalProperties=false, got %#v", tool.Name, got)
		}
	}
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
					"seed_url":         "https://example.com",
					"max_pages":        2,
					"max_depth":        1,
					"same_domain":      true,
					"retrieval_effort": "exhaustive",
				},
			},
		},
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	observedLaneMax := -1
	runner := Runner{
		loadConfig: config.Load,
		crawl: func(ctx context.Context, cfg config.Config, req coreservice.CrawlRequest) (coreservice.CrawlResponse, error) {
			observedLaneMax = cfg.Runtime.LaneMax
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
	if observedLaneMax != retrievalEffortLanes[retrievalEffortExhaustive] {
		t.Fatalf("expected retrieval_effort exhaustive to set lane %d, got %d", retrievalEffortLanes[retrievalEffortExhaustive], observedLaneMax)
	}
	if !strings.Contains(string(responses[1]), `"summary"`) {
		t.Fatalf("expected crawl summary, got %s", responses[1])
	}
	assertMCPStructuredKeys(t, responses[1], "documents", "summary", "stored_runs")
	assertMCPStructuredAbsentKeys(t, responses[1], "compact")
}

func TestRunnerMCPMemoryTools(t *testing.T) {
	root := t.TempDir()

	cfg := config.Defaults()
	cfg.Memory.Enabled = true
	cfg.Semantic.VectorSpace = intel.DenseSemanticVectorSpace
	cfg.Semantic.EmbeddingURL = newTransportEmbeddingServer(t).URL
	configPath := filepath.Join(root, "needlex-memory.json")
	rawCfg, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, rawCfg, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	seedMemoryDocument(t, root, cfg, "https://playwright.dev/docs/intro", "Installation | Playwright", "Install Playwright and run the installation command to download browser binaries.")
	exportDir := filepath.Join(root, "memory-export")
	importRoot := t.TempDir()

	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "memory", "arguments": map[string]any{"action": "stats", "config_path": configPath}}},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "memory", "arguments": map[string]any{"action": "search", "query": "playwright installation", "config_path": configPath}}},
		map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": "memory", "arguments": map[string]any{"action": "export", "out_dir": exportDir, "config_path": configPath}}},
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
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "memory", "arguments": map[string]any{"action": "import", "in_dir": exportDir, "config_path": importCfgPath}}},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "memory", "arguments": map[string]any{"action": "rebuild_index", "config_path": importCfgPath}}},
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

func TestRunnerMCPAnalyticsTools(t *testing.T) {
	root := t.TempDir()
	store := analytics.NewSQLiteStore(root)
	if err := store.AppendRun(context.Background(), analytics.RunRecord{
		RunID:                "run_1",
		StartedAt:            time.Now().UTC().Add(-time.Second),
		CompletedAt:          time.Now().UTC(),
		Operation:            "query",
		Surface:              "cli",
		Profile:              "standard",
		GoalHash:             "goal_hash",
		GoalLengthChars:      12,
		DiscoveryMode:        "web_search",
		SelectedURL:          "https://example.com",
		Provider:             "discovery_memory_same_site",
		Success:              true,
		TraceID:              "trace_1",
		LatencyMS:            250,
		PacketBytes:          128,
		FinalContextChars:    100,
		ChunkCount:           1,
		SourceCount:          1,
		LinkCount:            2,
		ProofRefCount:        1,
		ProofUsable:          true,
		PublicBootstrapUsed:  false,
		LocalMemoryUsed:      true,
		TopicNodeUsed:        true,
		SameSiteRecoveryUsed: true,
		RawFetchChars:        1000,
		RawFetchBytes:        1000,
	}, nil); err != nil {
		t.Fatalf("seed analytics db: %v", err)
	}

	input := framedMessages(
		t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "analytics", "arguments": map[string]any{"action": "stats"}}},
		map[string]any{"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": map[string]any{"name": "analytics", "arguments": map[string]any{"action": "recent_runs", "limit": 5}}},
		map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": "analytics", "arguments": map[string]any{"action": "value_report"}}},
		map[string]any{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": map[string]any{"name": "analytics", "arguments": map[string]any{"action": "hosts", "limit": 5}}},
		map[string]any{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": map[string]any{"name": "analytics", "arguments": map[string]any{"action": "providers", "limit": 5}}},
		map[string]any{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": map[string]any{"name": "analytics", "arguments": map[string]any{"action": "daily", "limit": 5}}},
		map[string]any{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": map[string]any{"name": "analytics", "arguments": map[string]any{"action": "export", "out_dir": filepath.Join(root, "analytics-export")}}},
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := Runner{loadConfig: config.Load, stdin: strings.NewReader(input), storeRoot: root}
	code := runner.runMCP(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", code, stderr.String())
	}
	responses := decodeMCPResponses(t, stdout.Bytes())
	if len(responses) != 8 {
		t.Fatalf("expected 8 responses, got %d", len(responses))
	}
	assertMCPStructuredKeys(t, responses[1], "stats", "compact")
	assertMCPStructuredKeys(t, responses[2], "runs", "compact")
	assertMCPStructuredKeys(t, responses[3], "report", "compact")
	assertMCPStructuredKeys(t, responses[4], "hosts", "compact")
	assertMCPStructuredKeys(t, responses[5], "providers", "compact")
	assertMCPStructuredKeys(t, responses[6], "days", "compact")
	assertMCPStructuredKeys(t, responses[7], "export", "compact")
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

	structured := mcpStructuredContent(t, frame)
	for _, key := range keys {
		if _, ok := structured[key]; !ok {
			t.Fatalf("expected structuredContent key %q, got %#v", key, structured)
		}
	}
}

func assertMCPStructuredAbsentKeys(t *testing.T, frame []byte, keys ...string) {
	t.Helper()

	structured := mcpStructuredContent(t, frame)
	for _, key := range keys {
		if _, ok := structured[key]; ok {
			t.Fatalf("expected structuredContent to omit key %q, got keys %#v", key, structured)
		}
	}
}

func mcpStructuredContent(t *testing.T, frame []byte) map[string]any {
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
	return structured
}

func largeMCPReadResponse() coreservice.ReadResponse {
	resp := fakeResponse()
	for i := 0; i < 120; i++ {
		resp.WebIR.Nodes = append(resp.WebIR.Nodes, core.WebIRNode{
			Path: fmt.Sprintf("/article[1]/p[%d]", i),
			Tag:  "p",
			Kind: "paragraph",
			Text: strings.Repeat("compiled web ir node text ", 12),
		})
	}
	resp.WebIR.NodeCount = len(resp.WebIR.Nodes)
	for i := 0; i < 80; i++ {
		resp.ResultPack.Chunks = append(resp.ResultPack.Chunks, core.Chunk{
			ID:          fmt.Sprintf("chk_%d", i),
			DocID:       "doc_1",
			Text:        strings.Repeat("result pack chunk body text for diagnostics ", 8),
			HeadingPath: []string{"Diagnostics"},
			Score:       0.8,
			Fingerprint: "fp_diag",
			Confidence:  0.9,
		})
	}
	for i := 0; i < 4; i++ {
		resp.AgentContext.Chunks = append(resp.AgentContext.Chunks, coreservice.AgentChunk{
			ID:          fmt.Sprintf("agent_chk_%d", i),
			Text:        strings.Repeat("selected compact chunk evidence for the objective ", 6),
			HeadingPath: []string{"Section"},
			Confidence:  0.9,
		})
	}
	return resp
}

func TestRunnerMCPReadPayloadIsCompactByDefaultWithDiagnosticsOptIn(t *testing.T) {
	runner := Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			return largeMCPReadResponse(), nil
		},
		storeRoot: t.TempDir(),
	}

	def, err := runner.callMCPReadTool(map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("default web_read: %v", err)
	}
	diagnosticsResult, err := runner.callMCPReadTool(map[string]any{"url": "https://example.com", "include_diagnostics": true})
	if err != nil {
		t.Fatalf("diagnostics web_read: %v", err)
	}

	defStructured := def["structuredContent"].(map[string]any)
	optStructured := diagnosticsResult["structuredContent"].(map[string]any)
	for _, key := range []string{"document", "web_ir", "result_pack", "agent_context", "plan", "compact"} {
		if _, ok := defStructured[key]; ok {
			t.Fatalf("default structuredContent must omit %q, got keys %#v", key, defStructured)
		}
	}
	for _, key := range []string{"trace_path", "proof_path", "fingerprint_path"} {
		if _, ok := defStructured[key]; !ok {
			t.Fatalf("default structuredContent must expose %q, got keys %#v", key, defStructured)
		}
	}
	if _, ok := defStructured["diagnostics"]; ok {
		t.Fatalf("default structuredContent must not include diagnostics: %#v", defStructured)
	}
	diagnostics, ok := optStructured["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("include_diagnostics=true must add a nested diagnostics object, got keys %#v", optStructured)
	}
	for _, key := range []string{"document", "web_ir", "result_pack", "agent_context"} {
		if _, ok := diagnostics[key]; !ok {
			t.Fatalf("diagnostics must include %q, got keys %#v", key, diagnostics)
		}
	}
	if _, ok := diagnostics["plan"]; ok {
		t.Fatalf("web_read diagnostics must not include plan: %#v", diagnostics)
	}

	// The compact value is the single canonical shape: content.text and the
	// default structuredContent carry the same compact keys.
	text := def["content"].([]map[string]any)[0]["text"].(string)
	var compactText map[string]any
	if err := json.Unmarshal([]byte(text), &compactText); err != nil {
		t.Fatalf("decode compact content.text: %v", err)
	}
	for key := range compactText {
		if _, ok := defStructured[key]; !ok {
			t.Fatalf("structuredContent lost compact key %q", key)
		}
	}
	for key := range defStructured {
		switch key {
		case "trace_path", "proof_path", "fingerprint_path", "diagnostics":
			continue
		}
		if _, ok := compactText[key]; !ok {
			t.Fatalf("structuredContent introduced non-compact key %q", key)
		}
	}

	defJSON := mustJSON(defStructured)
	optJSON := mustJSON(optStructured)
	if len(optJSON) < len(defJSON)*3 {
		t.Fatalf("expected diagnostics payload to be materially larger: default=%d diagnostics=%d", len(defJSON), len(optJSON))
	}
}

func TestRunnerMCPQueryPayloadIsCompactByDefaultWithDiagnosticsOptIn(t *testing.T) {
	runner := Runner{
		loadConfig: config.Load,
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			return largeMCPQueryResponse(req), nil
		},
		storeRoot: t.TempDir(),
	}

	def, err := runner.callMCPQueryTool(map[string]any{"goal": "objective evidence", "seed_url": "https://example.com"})
	if err != nil {
		t.Fatalf("default web_query: %v", err)
	}
	diagnosticsResult, err := runner.callMCPQueryTool(map[string]any{"goal": "objective evidence", "seed_url": "https://example.com", "include_diagnostics": true})
	if err != nil {
		t.Fatalf("diagnostics web_query: %v", err)
	}

	defStructured := def["structuredContent"].(map[string]any)
	optStructured := diagnosticsResult["structuredContent"].(map[string]any)
	for _, key := range []string{"plan", "document", "web_ir", "result_pack", "agent_context", "compact"} {
		if _, ok := defStructured[key]; ok {
			t.Fatalf("default structuredContent must omit %q, got keys %#v", key, defStructured)
		}
	}
	diagnostics, ok := optStructured["diagnostics"].(map[string]any)
	if !ok {
		t.Fatalf("include_diagnostics=true must add a nested diagnostics object, got keys %#v", optStructured)
	}
	for _, key := range []string{"plan", "document", "web_ir", "result_pack", "agent_context"} {
		if _, ok := diagnostics[key]; !ok {
			t.Fatalf("web_query diagnostics must include %q, got keys %#v", key, diagnostics)
		}
	}
	defJSON := mustJSON(defStructured)
	optJSON := mustJSON(optStructured)
	if len(optJSON) < len(defJSON)*3 {
		t.Fatalf("expected diagnostics payload to be materially larger: default=%d diagnostics=%d", len(defJSON), len(optJSON))
	}
}

func largeMCPQueryResponse(req coreservice.QueryRequest) coreservice.QueryResponse {
	read := largeMCPReadResponse()
	out := fakeQueryResponse(req)
	out.Document = read.Document
	out.WebIR = read.WebIR
	out.ResultPack = read.ResultPack
	out.AgentContext.Chunks = read.AgentContext.Chunks
	out.ProofRefs = read.ResultPack.ProofRefs
	return out
}

func TestMCPReadAndQueryExposeIncludeDiagnostics(t *testing.T) {
	for _, tool := range []mcpTool{mcpReadTool(), mcpQueryTool()} {
		props, ok := tool.InputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s: expected properties map", tool.Name)
		}
		if _, ok := props["include_diagnostics"]; !ok {
			t.Fatalf("%s: expected include_diagnostics argument, got %#v", tool.Name, props)
		}
		diag, ok := props["include_diagnostics"].(map[string]any)
		if !ok || diag["type"] != "boolean" || diag["default"] != includeDiagnosticsDefault {
			t.Fatalf("%s: unexpected include_diagnostics schema %#v", tool.Name, props["include_diagnostics"])
		}
		desc, _ := diag["description"].(string)
		if !strings.Contains(desc, "diagnostics") || !strings.Contains(desc, "trace_path") {
			t.Fatalf("%s: expected diagnostics guidance, got %q", tool.Name, desc)
		}
		if !strings.Contains(tool.Description, "include_diagnostics") || !strings.Contains(tool.Description, "compact packet") {
			t.Fatalf("%s: expected tool description to document compact default and opt-in diagnostics, got %q", tool.Name, tool.Description)
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
		if !strings.Contains(dm["description"].(string), "EXPERIMENTAL") || !strings.Contains(dm["description"].(string), "never choose it automatically") {
			t.Fatalf("expected web_search to be explicit experimental opt-in, got %#v", dm["description"])
		}
		if !strings.Contains(tool.Description, "exact canonical page") {
			t.Fatalf("expected web_query description to guide strict off mode, got %#v", tool.Description)
		}
		seedURL, _ := props["seed_url"].(map[string]any)
		if !strings.Contains(seedURL["description"].(string), "exact canonical page") {
			t.Fatalf("expected seed_url description to guide strict off mode, got %#v", seedURL["description"])
		}
		if _, ok := tool.InputSchema["anyOf"].([]any); !ok {
			t.Fatalf("expected schema to require seed_url or explicit web_search opt-in, got %#v", tool.InputSchema)
		}
		examples, _ := tool.InputSchema["examples"].([]map[string]any)
		for _, example := range examples {
			if _, seeded := example["seed_url"]; !seeded {
				t.Fatalf("stable web_query examples must remain seeded, got %#v", example)
			}
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
