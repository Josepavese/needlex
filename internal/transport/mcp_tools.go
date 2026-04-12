package transport

import (
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/store"
)

func (r Runner) callMCPTool(call mcpToolCall) (map[string]any, error) {
	switch call.Name {
	case "web_crawl":
		return r.callMCPCrawlTool(call.Arguments)
	case "web_query":
		return r.callMCPQueryTool(call.Arguments)
	case "web_read":
		return r.callMCPReadTool(call.Arguments)
	case "web_replay":
		report, err := r.loadReplay(stringArg(call.Arguments, "trace_id"))
		if err != nil {
			return nil, err
		}
		return mcpToolResult(map[string]any{"replay_report": report}, map[string]any{"replay_report": report}), nil
	case "web_diff":
		report, err := r.loadDiff(stringArg(call.Arguments, "trace_a"), stringArg(call.Arguments, "trace_b"))
		if err != nil {
			return nil, err
		}
		return mcpToolResult(map[string]any{"diff_report": report}, map[string]any{"diff_report": report}), nil
	case "web_proof":
		lookup := stringArg(call.Arguments, "chunk_id")
		if lookup == "" {
			lookup = stringArg(call.Arguments, "proof_id")
		}
		if lookup == "" {
			lookup = stringArg(call.Arguments, "trace_id")
		}
		result, err := r.loadProof(lookup)
		if err != nil {
			return nil, err
		}
		payload := map[string]any{
			"trace_id":      result.TraceID,
			"proof_records": result.Records,
		}
		if len(result.Records) == 1 {
			payload["proof"] = result.Records[0]
		}
		return mcpToolResult(payload, payload), nil
	case "web_prune":
		pruneAll := boolArg(call.Arguments, "all")
		olderThanHours, _ := intArg(call.Arguments, "older_than_hours")
		report, err := store.Prune(r.storeRoot, durationHours(olderThanHours), pruneAll, timeNowUTC())
		if err != nil {
			return nil, err
		}
		return mcpToolResult(map[string]any{"prune_report": report}, map[string]any{"prune_report": report}), nil
	case "memory_stats":
		return r.callMCPMemoryStatsTool(call.Arguments)
	case "memory_search":
		return r.callMCPMemorySearchTool(call.Arguments)
	case "memory_prune":
		return r.callMCPMemoryPruneTool(call.Arguments)
	case "memory_export":
		return r.callMCPMemoryExportTool(call.Arguments)
	case "memory_import":
		return r.callMCPMemoryImportTool(call.Arguments)
	case "memory_rebuild_index":
		return r.callMCPMemoryRebuildIndexTool(call.Arguments)
	default:
		return nil, fmt.Errorf("unsupported tool %q", call.Name)
	}
}

func (r Runner) callMCPCrawlTool(args map[string]any) (map[string]any, error) {
	cfg, err := config.Load(stringArg(args, "config_path"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	resp, artifacts, err := r.executeCrawl(cfg, coreservice.CrawlRequest{
		SeedURL:    stringArg(args, "seed_url"),
		Profile:    stringArg(args, "profile"),
		UserAgent:  stringArg(args, "user_agent"),
		MaxPages:   intDefault(args, "max_pages", 0),
		MaxDepth:   intDefault(args, "max_depth", 0),
		SameDomain: boolArg(args, "same_domain"),
	})
	if err != nil {
		return nil, err
	}
	return mcpToolResult(map[string]any{
		"kind":        "bounded_crawl",
		"summary":     resp.Summary,
		"documents":   compactCrawlResponse(resp, artifacts).Documents,
		"stored_runs": artifacts.StoredRuns,
		"compact":     compactCrawlResponse(resp, artifacts),
	}, compactCrawlResponse(resp, artifacts)), nil
}

func (r Runner) callMCPQueryTool(args map[string]any) (map[string]any, error) {
	cfg, err := config.Load(stringArg(args, "config_path"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if laneMax, ok := intArg(args, "lane_max"); ok {
		cfg.Runtime.LaneMax = laneMax
	}
	resp, artifacts, err := r.executeQuery(cfg, coreservice.QueryRequest{
		Goal:          stringArg(args, "goal"),
		SeedURL:       stringArg(args, "seed_url"),
		Profile:       stringArg(args, "profile"),
		UserAgent:     stringArg(args, "user_agent"),
		DiscoveryMode: stringArg(args, "discovery_mode"),
	})
	if err != nil {
		return nil, err
	}
	compact := compactQueryResponse(resp)
	payload := map[string]any{
		"kind":             compact.Kind,
		"goal":             compact.Goal,
		"seed_url":         compact.SeedURL,
		"selected_url":     compact.SelectedURL,
		"summary":          compact.Summary,
		"uncertainty":      compact.Uncertainty,
		"selection_why":    compact.SelectionWhy,
		"provider":         compact.Provider,
		"profile":          compact.Profile,
		"trace_id":         compact.TraceID,
		"chunks":           compact.Chunks,
		"candidates":       compact.Candidates,
		"signals":          compact.Signals,
		"web_ir_summary":   compact.WebIRSummary,
		"cost_report":      compact.CostReport,
		"compact":          compact,
		"plan":             resp.Plan,
		"document":         resp.Document,
		"web_ir":           resp.WebIR,
		"result_pack":      resp.ResultPack,
		"agent_context":    resp.AgentContext,
		"proof_refs":       resp.ProofRefs,
		"trace_path":       artifacts.TracePath,
		"proof_path":       artifacts.ProofPath,
		"fingerprint_path": artifacts.FingerprintPath,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPReadTool(args map[string]any) (map[string]any, error) {
	cfg, err := config.Load(stringArg(args, "config_path"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if laneMax, ok := intArg(args, "lane_max"); ok {
		cfg.Runtime.LaneMax = laneMax
	}
	resp, artifacts, err := r.executeRead(cfg, coreservice.ReadRequest{
		URL:       stringArg(args, "url"),
		Objective: stringArg(args, "objective"),
		Profile:   stringArg(args, "profile"),
		UserAgent: stringArg(args, "user_agent"),
	})
	if err != nil {
		return nil, err
	}
	compact := compactReadResponse(resp)
	payload := map[string]any{
		"kind":             compact.Kind,
		"url":              compact.URL,
		"title":            compact.Title,
		"summary":          compact.Summary,
		"uncertainty":      compact.Uncertainty,
		"profile":          compact.Profile,
		"trace_id":         compact.TraceID,
		"outline":          compact.Outline,
		"chunks":           compact.Chunks,
		"links":            compact.Links,
		"signals":          compact.Signals,
		"web_ir_summary":   compact.WebIRSummary,
		"cost_report":      compact.CostReport,
		"compact":          compact,
		"document":         resp.Document,
		"web_ir":           resp.WebIR,
		"agent_context":    resp.AgentContext,
		"proof_refs":       resp.ResultPack.ProofRefs,
		"trace_path":       artifacts.TracePath,
		"proof_path":       artifacts.ProofPath,
		"fingerprint_path": artifacts.FingerprintPath,
	}
	return mcpToolResult(payload, compact), nil
}

func mcpTools() []mcpTool {
	return []mcpTool{
		mcpCrawlTool(),
		mcpQueryTool(),
		mcpReadTool(),
		mcpReplayTool(),
		mcpDiffTool(),
		mcpProofTool(),
		mcpPruneTool(),
		mcpMemoryStatsTool(),
		mcpMemorySearchTool(),
		mcpMemoryPruneTool(),
		mcpMemoryExportTool(),
		mcpMemoryImportTool(),
		mcpMemoryRebuildIndexTool(),
	}
}

func mcpCrawlTool() mcpTool {
	return mcpTool{
		Name:        "web_crawl",
		Description: "Traverse linked pages starting from one seed URL.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"seed_url":    map[string]any{"type": "string"},
			"profile":     map[string]any{"type": "string"},
			"user_agent":  map[string]any{"type": "string"},
			"max_pages":   map[string]any{"type": "integer"},
			"max_depth":   map[string]any{"type": "integer"},
			"same_domain": map[string]any{"type": "boolean"},
		}, "seed_url"),
			map[string]any{"seed_url": "https://example.com/docs", "same_domain": true, "max_pages": 5, "max_depth": 1},
		),
	}
}

func mcpQueryTool() mcpTool {
	return mcpTool{
		Name:        "web_query",
		Description: "Plan and execute a goal-oriented query with optional seed URL. Use discovery_mode=same_site_links to expand from the seed site, web_search for external bootstrap, or off only when seed_url is already the exact canonical page you want to read.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"goal":       map[string]any{"type": "string", "description": "Retrieval objective or question to answer."},
			"seed_url":   map[string]any{"type": "string", "description": "Optional starting URL. If present, same_site_links expands from this site. When discovery_mode=off, this must be the exact canonical page and must already exist."},
			"profile":    map[string]any{"type": "string"},
			"user_agent": map[string]any{"type": "string"},
			"discovery_mode": map[string]any{
				"type":        "string",
				"enum":        []string{"same_site_links", "web_search", "off"},
				"description": "Discovery strategy. same_site_links = follow links from the seed site. web_search = bootstrap with search. off = do not expand beyond the seed URL and should be used only after the exact page has already been verified.",
			},
			"lane_max": map[string]any{"type": "integer"},
		}, "goal"),
			map[string]any{"goal": "Find authentication flow details", "seed_url": "https://agentclientprotocol.com/protocol/overview", "discovery_mode": "same_site_links"},
			map[string]any{"goal": "OpenAI API pricing", "discovery_mode": "web_search"},
			map[string]any{"goal": "Read the verified initialize method page", "seed_url": "https://agentclientprotocol.com/protocol/initialization", "discovery_mode": "off"},
		),
	}
}

func mcpReadTool() mcpTool {
	return mcpTool{
		Name:        "web_read",
		Description: "Read one URL and return compact proof-carrying context first, plus diagnostic fields for deeper inspection.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"url":        map[string]any{"type": "string"},
			"profile":    map[string]any{"type": "string"},
			"objective":  map[string]any{"type": "string"},
			"user_agent": map[string]any{"type": "string"},
			"lane_max":   map[string]any{"type": "integer"},
		}, "url"),
			map[string]any{"url": "https://example.com", "objective": "Extract pricing and policy details"},
		),
	}
}

func mcpReplayTool() mcpTool {
	return mcpTool{
		Name:        "web_replay",
		Description: "Replay a stored trace and report deterministic stage completion.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"trace_id": map[string]any{"type": "string"},
		}, "trace_id"), map[string]any{"trace_id": "trace_123"}),
	}
}

func mcpDiffTool() mcpTool {
	return mcpTool{
		Name:        "web_diff",
		Description: "Compare two stored traces and report changed stages.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"trace_a": map[string]any{"type": "string"},
			"trace_b": map[string]any{"type": "string"},
		}, "trace_a", "trace_b"), map[string]any{"trace_a": "trace_a", "trace_b": "trace_b"}),
	}
}

func mcpProofTool() mcpTool {
	return mcpTool{
		Name:        "web_proof",
		Description: "Load proof records by trace id, proof id, or chunk id.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"trace_id": map[string]any{"type": "string"},
			"proof_id": map[string]any{"type": "string"},
			"chunk_id": map[string]any{"type": "string"},
		}), map[string]any{"chunk_id": "chk_123"}, map[string]any{"trace_id": "trace_123"}),
	}
}

func mcpPruneTool() mcpTool {
	return mcpTool{
		Name:        "web_prune",
		Description: "Prune local traces, proofs, fingerprints, and genome files.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"all":              map[string]any{"type": "boolean"},
			"older_than_hours": map[string]any{"type": "integer"},
		}), map[string]any{"older_than_hours": 24}),
	}
}

func (r Runner) callMCPMemoryStatsTool(args map[string]any) (map[string]any, error) {
	cfg, err := config.Load(stringArg(args, "config_path"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	stats, err := r.loadMemoryStats(cfg)
	if err != nil {
		return nil, err
	}
	compact := compactStats(stats)
	payload := map[string]any{
		"kind":    "memory_stats",
		"stats":   compact,
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPMemorySearchTool(args map[string]any) (map[string]any, error) {
	cfg, err := config.Load(stringArg(args, "config_path"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	limit := intDefault(args, "limit", 5)
	query := stringArg(args, "query")
	if query == "" {
		query = stringArg(args, "goal")
	}
	candidates, err := r.searchMemory(cfg, query, limit, csvOrListArg(args, "domain_hints"))
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind":       "memory_search",
		"query":      query,
		"candidates": compactMemoryCandidates(candidates),
	}
	payload := map[string]any{
		"kind":       "memory_search",
		"query":      query,
		"candidates": compactMemoryCandidates(candidates),
		"compact":    compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPMemoryPruneTool(args map[string]any) (map[string]any, error) {
	cfg, err := config.Load(stringArg(args, "config_path"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	before, after, policy, err := r.pruneMemory(cfg)
	if err != nil {
		return nil, err
	}
	removed := map[string]int{
		"documents":  before.DocumentCount - after.DocumentCount,
		"edges":      before.EdgeCount - after.EdgeCount,
		"embeddings": before.EmbeddingCount - after.EmbeddingCount,
	}
	compact := map[string]any{
		"kind":    "memory_prune",
		"before":  compactStats(before),
		"after":   compactStats(after),
		"removed": removed,
	}
	payload := map[string]any{
		"kind":    "memory_prune",
		"before":  compactStats(before),
		"after":   compactStats(after),
		"policy":  policy,
		"removed": removed,
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPMemoryExportTool(args map[string]any) (map[string]any, error) {
	cfg, err := config.Load(stringArg(args, "config_path"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	outDir := stringArg(args, "out_dir")
	if outDir == "" {
		return nil, fmt.Errorf("memory_export requires out_dir")
	}
	exported, err := r.exportMemory(cfg, outDir)
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind":   "memory_export",
		"export": exported,
	}
	payload := map[string]any{
		"kind":    "memory_export",
		"export":  exported,
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPMemoryImportTool(args map[string]any) (map[string]any, error) {
	cfg, err := config.Load(stringArg(args, "config_path"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	inDir := stringArg(args, "in_dir")
	if inDir == "" {
		return nil, fmt.Errorf("memory_import requires in_dir")
	}
	imported, err := r.importMemory(cfg, inDir)
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind":   "memory_import",
		"import": imported,
	}
	payload := map[string]any{
		"kind":    "memory_import",
		"import":  imported,
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPMemoryRebuildIndexTool(args map[string]any) (map[string]any, error) {
	cfg, err := config.Load(stringArg(args, "config_path"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	stats, err := r.rebuildMemoryIndex(cfg)
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind":  "memory_rebuild_index",
		"stats": compactStats(stats),
	}
	payload := map[string]any{
		"kind":    "memory_rebuild_index",
		"stats":   compactStats(stats),
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
}

func mcpMemoryStatsTool() mcpTool {
	return mcpTool{
		Name:        "memory_stats",
		Description: "Inspect Discovery Memory counts, freshness, and database path.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"config_path": map[string]any{"type": "string"},
		}), map[string]any{}),
	}
}

func mcpMemorySearchTool() mcpTool {
	return mcpTool{
		Name:        "memory_search",
		Description: "Search Discovery Memory semantically before public bootstrap. Use this to inspect what local memory already knows.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"query":        map[string]any{"type": "string", "description": "Semantic retrieval query."},
			"goal":         map[string]any{"type": "string", "description": "Alias for query."},
			"limit":        map[string]any{"type": "integer"},
			"domain_hints": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"config_path":  map[string]any{"type": "string"},
		}), map[string]any{"query": "playwright installation", "limit": 5}),
	}
}

func mcpMemoryPruneTool() mcpTool {
	return mcpTool{
		Name:        "memory_prune",
		Description: "Prune Discovery Memory to configured bounds.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"config_path": map[string]any{"type": "string"},
		}), map[string]any{}),
	}
}

func mcpMemoryExportTool() mcpTool {
	return mcpTool{
		Name:        "memory_export",
		Description: "Export Discovery Memory canonical rows to JSONL files for inspection, backup, or migration.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"out_dir":     map[string]any{"type": "string"},
			"config_path": map[string]any{"type": "string"},
		}, "out_dir"), map[string]any{"out_dir": ".needlex/discovery/export"}),
	}
}

func mcpMemoryImportTool() mcpTool {
	return mcpTool{
		Name:        "memory_import",
		Description: "Import Discovery Memory canonical rows from JSONL files.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"in_dir":      map[string]any{"type": "string"},
			"config_path": map[string]any{"type": "string"},
		}, "in_dir"), map[string]any{"in_dir": ".needlex/discovery/export"}),
	}
}

func mcpMemoryRebuildIndexTool() mcpTool {
	return mcpTool{
		Name:        "memory_rebuild_index",
		Description: "Rebuild or refresh Discovery Memory acceleration state from canonical local rows.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"config_path": map[string]any{"type": "string"},
		}), map[string]any{}),
	}
}

func csvOrListArg(args map[string]any, key string) []string {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case string:
		return splitCSV(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
				out = append(out, strings.TrimSpace(value))
			}
		}
		return out
	case []string:
		return typed
	default:
		return nil
	}
}

func toolSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func schemaExamples(schema map[string]any, examples ...map[string]any) map[string]any {
	if len(examples) == 0 {
		return schema
	}
	schema["examples"] = examples
	return schema
}

func intDefault(args map[string]any, key string, fallback int) int {
	value, ok := intArg(args, key)
	if !ok {
		return fallback
	}
	return value
}
