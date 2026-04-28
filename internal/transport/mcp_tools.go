package transport

import (
	"context"
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/analytics"
	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/store"
)

type mcpToolSpec struct {
	Definition mcpTool
	Handler    func(Runner, map[string]any) (map[string]any, error)
}

func (r Runner) callMCPTool(call mcpToolCall) (map[string]any, error) {
	handler, ok := r.mcpToolHandlers()[call.Name]
	if !ok {
		legacyHandler, legacyOK := r.legacyMCPToolHandlers()[call.Name]
		if !legacyOK {
			return nil, fmt.Errorf("unsupported tool %q", call.Name)
		}
		return legacyHandler(call.Arguments)
	}
	return handler(call.Arguments)
}

func (r Runner) mcpToolHandlers() map[string]func(map[string]any) (map[string]any, error) {
	out := make(map[string]func(map[string]any) (map[string]any, error))
	for _, spec := range mcpToolSpecs() {
		handler := spec.Handler
		out[spec.Definition.Name] = func(args map[string]any) (map[string]any, error) {
			return handler(r, args)
		}
	}
	return out
}

func (r Runner) legacyMCPToolHandlers() map[string]func(map[string]any) (map[string]any, error) {
	return map[string]func(map[string]any) (map[string]any, error){
		"memory_stats":           r.callMCPMemoryStatsTool,
		"memory_search":          r.callMCPMemorySearchTool,
		"memory_prune":           r.callMCPMemoryPruneTool,
		"memory_export":          r.callMCPMemoryExportTool,
		"memory_import":          r.callMCPMemoryImportTool,
		"memory_rebuild_index":   r.callMCPMemoryRebuildIndexTool,
		"analytics_stats":        r.callMCPAnalyticsStatsTool,
		"analytics_recent_runs":  r.callMCPAnalyticsRecentRunsTool,
		"analytics_value_report": r.callMCPAnalyticsValueReportTool,
		"analytics_hosts":        r.callMCPAnalyticsHostsTool,
		"analytics_providers":    r.callMCPAnalyticsProvidersTool,
		"analytics_failures":     r.callMCPAnalyticsFailuresTool,
		"analytics_daily":        r.callMCPAnalyticsDailyTool,
		"analytics_export":       r.callMCPAnalyticsExportTool,
	}
}

func (r Runner) callMCPReplayTool(args map[string]any) (map[string]any, error) {
	report, err := r.loadReplay(stringArg(args, "trace_id"))
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"replay_report": report}
	return mcpToolResult(payload, payload), nil
}

func (r Runner) callMCPDiffTool(args map[string]any) (map[string]any, error) {
	report, err := r.loadDiff(stringArg(args, "trace_a"), stringArg(args, "trace_b"))
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"diff_report": report}
	return mcpToolResult(payload, payload), nil
}

func (r Runner) callMCPProofTool(args map[string]any) (map[string]any, error) {
	lookup := firstNonEmptyString(
		stringArg(args, "chunk_id"),
		stringArg(args, "proof_id"),
		stringArg(args, "trace_id"),
	)
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
}

func (r Runner) callMCPPruneTool(args map[string]any) (map[string]any, error) {
	pruneAll := boolArg(args, "all")
	olderThanHours, _ := intArg(args, "older_than_hours")
	report, err := store.Prune(r.storeRoot, durationHours(olderThanHours), pruneAll, timeNowUTC())
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"prune_report": report}
	return mcpToolResult(payload, payload), nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (r Runner) callMCPCrawlTool(args map[string]any) (map[string]any, error) {
	cfg, err := config.Load(stringArg(args, "config_path"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	resp, artifacts, err := r.executeCrawlWithSurface(cfg, coreservice.CrawlRequest{
		SeedURL:    stringArg(args, "seed_url"),
		Profile:    stringArg(args, "profile"),
		UserAgent:  stringArg(args, "user_agent"),
		MaxPages:   intDefault(args, "max_pages", 0),
		MaxDepth:   intDefault(args, "max_depth", 0),
		SameDomain: boolArg(args, "same_domain"),
	}, "mcp")
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
	if err := applyMCPRetrievalEffort(args, &cfg); err != nil {
		return nil, err
	}
	resp, artifacts, err := r.executeQueryWithSurface(cfg, coreservice.QueryRequest{
		Goal:          stringArg(args, "goal"),
		SeedURL:       stringArg(args, "seed_url"),
		Profile:       stringArg(args, "profile"),
		UserAgent:     stringArg(args, "user_agent"),
		DiscoveryMode: stringArg(args, "discovery_mode"),
	}, "mcp")
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
	if err := applyMCPRetrievalEffort(args, &cfg); err != nil {
		return nil, err
	}
	resp, artifacts, err := r.executeReadWithSurface(cfg, coreservice.ReadRequest{
		URL:       stringArg(args, "url"),
		Objective: stringArg(args, "objective"),
		Profile:   stringArg(args, "profile"),
		UserAgent: stringArg(args, "user_agent"),
	}, "mcp")
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
	specs := mcpToolSpecs()
	out := make([]mcpTool, 0, len(specs))
	for _, spec := range specs {
		out = append(out, spec.Definition)
	}
	return out
}

func mcpToolSpecs() []mcpToolSpec {
	return []mcpToolSpec{
		{Definition: mcpCrawlTool(), Handler: Runner.callMCPCrawlTool},
		{Definition: mcpQueryTool(), Handler: Runner.callMCPQueryTool},
		{Definition: mcpReadTool(), Handler: Runner.callMCPReadTool},
		{Definition: mcpReplayTool(), Handler: Runner.callMCPReplayTool},
		{Definition: mcpDiffTool(), Handler: Runner.callMCPDiffTool},
		{Definition: mcpProofTool(), Handler: Runner.callMCPProofTool},
		{Definition: mcpPruneTool(), Handler: Runner.callMCPPruneTool},
		{Definition: mcpMemoryTool(), Handler: Runner.callMCPMemoryTool},
		{Definition: mcpAnalyticsTool(), Handler: Runner.callMCPAnalyticsTool},
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
		Description: "Plan and execute a goal-oriented query with optional seed URL. Discovery Memory is consulted first for seedless queries; web_search is public bootstrap fallback. Use same_site_links to expand from a seed site, and off only when seed_url is the exact canonical page.",
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
			"retrieval_effort": retrievalEffortSchema(),
		}, "goal"),
			map[string]any{"goal": "Find authentication flow details", "seed_url": "https://agentclientprotocol.com/protocol/overview", "discovery_mode": "same_site_links"},
			map[string]any{"goal": "OpenAI API pricing", "discovery_mode": "web_search", "retrieval_effort": "standard"},
			map[string]any{"goal": "Read the verified initialize method page", "seed_url": "https://agentclientprotocol.com/protocol/initialization", "discovery_mode": "off"},
		),
	}
}

func mcpReadTool() mcpTool {
	return mcpTool{
		Name:        "web_read",
		Description: "Read one URL and return compact proof-carrying context first. Successful reads automatically feed local Discovery Memory and Analytics PAL for future seedless reuse.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"url":              map[string]any{"type": "string"},
			"profile":          map[string]any{"type": "string"},
			"objective":        map[string]any{"type": "string"},
			"user_agent":       map[string]any{"type": "string"},
			"retrieval_effort": retrievalEffortSchema(),
		}, "url"),
			map[string]any{"url": "https://example.com", "objective": "Extract pricing and policy details", "retrieval_effort": "standard"},
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

func (r Runner) callMCPMemoryTool(args map[string]any) (map[string]any, error) {
	switch strings.TrimSpace(stringArg(args, "action")) {
	case "stats":
		return r.callMCPMemoryStatsTool(args)
	case "search":
		return r.callMCPMemorySearchTool(args)
	case "prune":
		return r.callMCPMemoryPruneTool(args)
	case "export":
		return r.callMCPMemoryExportTool(args)
	case "import":
		return r.callMCPMemoryImportTool(args)
	case "rebuild_index":
		return r.callMCPMemoryRebuildIndexTool(args)
	default:
		return nil, fmt.Errorf("memory requires action: stats, search, prune, export, import, or rebuild_index")
	}
}

func mcpMemoryTool() mcpTool {
	return mcpTool{
		Name:        "memory",
		Description: "Advanced non-core Discovery Memory control. Use only when explicitly inspecting or maintaining Needle-X local semantic memory; normal web retrieval already uses memory automatically. Actions: stats shows counts/freshness; search checks local semantic recall; prune applies retention policy; export/import move canonical JSONL rows; rebuild_index refreshes acceleration state.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"stats", "search", "prune", "export", "import", "rebuild_index"},
				"description": "Required operation. Prefer stats or search for debugging; prune/export/import/rebuild_index are maintenance actions.",
			},
			"query":        map[string]any{"type": "string", "description": "Semantic query for action=search."},
			"goal":         map[string]any{"type": "string", "description": "Alias for query when action=search."},
			"limit":        map[string]any{"type": "integer", "description": "Maximum rows for action=search."},
			"domain_hints": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional host/domain hints for action=search."},
			"out_dir":      map[string]any{"type": "string", "description": "Destination directory for action=export."},
			"in_dir":       map[string]any{"type": "string", "description": "Source directory for action=import."},
			"config_path":  map[string]any{"type": "string", "description": "Optional Needle-X JSON config path."},
		}, "action"),
			map[string]any{"action": "stats"},
			map[string]any{"action": "search", "query": "playwright installation", "limit": 5},
			map[string]any{"action": "export", "out_dir": "needlex-discovery-export"},
		),
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

func (r Runner) callMCPAnalyticsTool(args map[string]any) (map[string]any, error) {
	switch strings.TrimSpace(stringArg(args, "action")) {
	case "stats":
		return r.callMCPAnalyticsStatsTool(args)
	case "recent_runs":
		return r.callMCPAnalyticsRecentRunsTool(args)
	case "value_report":
		return r.callMCPAnalyticsValueReportTool(args)
	case "hosts":
		return r.callMCPAnalyticsHostsTool(args)
	case "providers":
		return r.callMCPAnalyticsProvidersTool(args)
	case "failures":
		return r.callMCPAnalyticsFailuresTool(args)
	case "daily":
		return r.callMCPAnalyticsDailyTool(args)
	case "export":
		return r.callMCPAnalyticsExportTool(args)
	default:
		return nil, fmt.Errorf("analytics requires action: stats, recent_runs, value_report, hosts, providers, failures, daily, or export")
	}
}

func mcpAnalyticsTool() mcpTool {
	return mcpTool{
		Name:        "analytics",
		Description: "Advanced non-core Analytics PAL control. Use only for diagnostics, value reporting, or maintenance; normal web_read/web_query/web_crawl record analytics automatically. Actions: value_report is the user-facing WOW report; stats/recent_runs are quick diagnostics; hosts/providers/failures/daily are maintainer rollups; export writes full analytics artifacts.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"stats", "recent_runs", "value_report", "hosts", "providers", "failures", "daily", "export"},
				"description": "Required operation. Prefer value_report for product value, stats/recent_runs for quick debug, failures/providers for reliability analysis.",
			},
			"limit":   map[string]any{"type": "integer", "description": "Maximum rows for recent_runs, hosts, providers, failures, or daily."},
			"out_dir": map[string]any{"type": "string", "description": "Destination directory for action=export."},
		}, "action"),
			map[string]any{"action": "value_report"},
			map[string]any{"action": "recent_runs", "limit": 10},
			map[string]any{"action": "failures", "limit": 20},
			map[string]any{"action": "export", "out_dir": "needlex-analytics-export"},
		),
	}
}

func (r Runner) callMCPAnalyticsStatsTool(_ map[string]any) (map[string]any, error) {
	stats, err := analytics.NewSQLiteStore(r.storeRoot).Stats(context.Background())
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind":  "analytics_stats",
		"stats": stats,
	}
	payload := map[string]any{
		"kind":    "analytics_stats",
		"stats":   stats,
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPAnalyticsRecentRunsTool(args map[string]any) (map[string]any, error) {
	runs, err := analytics.NewSQLiteStore(r.storeRoot).RecentRuns(context.Background(), intDefault(args, "limit", 10))
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind": "analytics_recent_runs",
		"runs": runs,
	}
	payload := map[string]any{
		"kind":    "analytics_recent_runs",
		"runs":    runs,
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPAnalyticsValueReportTool(_ map[string]any) (map[string]any, error) {
	report, err := analytics.NewSQLiteStore(r.storeRoot).ValueReport(context.Background())
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind":   "analytics_value_report",
		"report": report,
	}
	payload := map[string]any{
		"kind":    "analytics_value_report",
		"report":  report,
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPAnalyticsHostsTool(args map[string]any) (map[string]any, error) {
	hosts, err := analytics.NewSQLiteStore(r.storeRoot).Hosts(context.Background(), intDefault(args, "limit", 20))
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind":  "analytics_hosts",
		"hosts": hosts,
	}
	payload := map[string]any{
		"kind":    "analytics_hosts",
		"hosts":   hosts,
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPAnalyticsProvidersTool(args map[string]any) (map[string]any, error) {
	providers, err := analytics.NewSQLiteStore(r.storeRoot).Providers(context.Background(), intDefault(args, "limit", 20))
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind":      "analytics_providers",
		"providers": providers,
	}
	payload := map[string]any{
		"kind":      "analytics_providers",
		"providers": providers,
		"compact":   compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPAnalyticsFailuresTool(args map[string]any) (map[string]any, error) {
	failures, err := analytics.NewSQLiteStore(r.storeRoot).Failures(context.Background(), intDefault(args, "limit", 20))
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind":     "analytics_failures",
		"failures": failures,
	}
	payload := map[string]any{
		"kind":     "analytics_failures",
		"failures": failures,
		"compact":  compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPAnalyticsDailyTool(args map[string]any) (map[string]any, error) {
	days, err := analytics.NewSQLiteStore(r.storeRoot).Daily(context.Background(), intDefault(args, "limit", 30))
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind": "analytics_daily",
		"days": days,
	}
	payload := map[string]any{
		"kind":    "analytics_daily",
		"days":    days,
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
}

func (r Runner) callMCPAnalyticsExportTool(args map[string]any) (map[string]any, error) {
	outDir := stringArg(args, "out_dir")
	if outDir == "" {
		return nil, fmt.Errorf("analytics_export requires out_dir")
	}
	exported, err := analytics.NewSQLiteStore(r.storeRoot).ExportJSON(context.Background(), outDir)
	if err != nil {
		return nil, err
	}
	compact := map[string]any{
		"kind":   "analytics_export",
		"export": exported,
	}
	payload := map[string]any{
		"kind":    "analytics_export",
		"export":  exported,
		"compact": compact,
	}
	return mcpToolResult(payload, compact), nil
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
