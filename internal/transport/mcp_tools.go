package transport

import (
	"fmt"

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
		return mcpToolResult(map[string]any{"replay_report": report}), nil
	case "web_diff":
		report, err := r.loadDiff(stringArg(call.Arguments, "trace_a"), stringArg(call.Arguments, "trace_b"))
		if err != nil {
			return nil, err
		}
		return mcpToolResult(map[string]any{"diff_report": report}), nil
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
		return mcpToolResult(payload), nil
	case "web_prune":
		pruneAll := boolArg(call.Arguments, "all")
		olderThanHours, _ := intArg(call.Arguments, "older_than_hours")
		report, err := store.Prune(r.storeRoot, durationHours(olderThanHours), pruneAll, timeNowUTC())
		if err != nil {
			return nil, err
		}
		return mcpToolResult(map[string]any{"prune_report": report}), nil
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
		"documents":   resp.Documents,
		"summary":     resp.Summary,
		"stored_runs": artifacts.StoredRuns,
	}), nil
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
	return mcpToolResult(map[string]any{
		"plan":             resp.Plan,
		"document":         resp.Document,
		"web_ir":           resp.WebIR,
		"result_pack":      resp.ResultPack,
		"agent_context":    resp.AgentContext,
		"proof_refs":       resp.ProofRefs,
		"trace_id":         resp.TraceID,
		"trace_path":       artifacts.TracePath,
		"proof_path":       artifacts.ProofPath,
		"fingerprint_path": artifacts.FingerprintPath,
	}), nil
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
	return mcpToolResult(map[string]any{
		"document":         resp.Document,
		"web_ir":           resp.WebIR,
		"chunks":           resp.ResultPack.Chunks,
		"agent_context":    resp.AgentContext,
		"proof_refs":       resp.ResultPack.ProofRefs,
		"cost_report":      resp.ResultPack.CostReport,
		"profile":          resp.ResultPack.Profile,
		"trace_id":         resp.Trace.TraceID,
		"trace_path":       artifacts.TracePath,
		"proof_path":       artifacts.ProofPath,
		"fingerprint_path": artifacts.FingerprintPath,
	}), nil
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
	}
}

func mcpCrawlTool() mcpTool {
	return mcpTool{
		Name:        "web_crawl",
		Description: "Traverse linked pages starting from one seed URL.",
		InputSchema: toolSchema(map[string]any{
			"seed_url":    map[string]any{"type": "string"},
			"profile":     map[string]any{"type": "string"},
			"user_agent":  map[string]any{"type": "string"},
			"max_pages":   map[string]any{"type": "integer"},
			"max_depth":   map[string]any{"type": "integer"},
			"same_domain": map[string]any{"type": "boolean"},
		}, "seed_url"),
	}
}

func mcpQueryTool() mcpTool {
	return mcpTool{
		Name:        "web_query",
		Description: "Plan and execute a goal-oriented query with optional seed URL and discovery (same-site or web-search).",
		InputSchema: toolSchema(map[string]any{
			"goal":           map[string]any{"type": "string"},
			"seed_url":       map[string]any{"type": "string"},
			"profile":        map[string]any{"type": "string"},
			"user_agent":     map[string]any{"type": "string"},
			"discovery_mode": map[string]any{"type": "string"},
			"lane_max":       map[string]any{"type": "integer"},
		}, "goal"),
	}
}

func mcpReadTool() mcpTool {
	return mcpTool{
		Name:        "web_read",
		Description: "Read a web page into compact proof-carrying context.",
		InputSchema: toolSchema(map[string]any{
			"url":        map[string]any{"type": "string"},
			"profile":    map[string]any{"type": "string"},
			"objective":  map[string]any{"type": "string"},
			"user_agent": map[string]any{"type": "string"},
			"lane_max":   map[string]any{"type": "integer"},
		}, "url"),
	}
}

func mcpReplayTool() mcpTool {
	return mcpTool{
		Name:        "web_replay",
		Description: "Replay a stored trace and report deterministic stage completion.",
		InputSchema: toolSchema(map[string]any{
			"trace_id": map[string]any{"type": "string"},
		}, "trace_id"),
	}
}

func mcpDiffTool() mcpTool {
	return mcpTool{
		Name:        "web_diff",
		Description: "Compare two stored traces and report changed stages.",
		InputSchema: toolSchema(map[string]any{
			"trace_a": map[string]any{"type": "string"},
			"trace_b": map[string]any{"type": "string"},
		}, "trace_a", "trace_b"),
	}
}

func mcpProofTool() mcpTool {
	return mcpTool{
		Name:        "web_proof",
		Description: "Load proof records by trace id, proof id, or chunk id.",
		InputSchema: toolSchema(map[string]any{
			"trace_id": map[string]any{"type": "string"},
			"proof_id": map[string]any{"type": "string"},
			"chunk_id": map[string]any{"type": "string"},
		}),
	}
}

func mcpPruneTool() mcpTool {
	return mcpTool{
		Name:        "web_prune",
		Description: "Prune local traces, proofs, fingerprints, and genome files.",
		InputSchema: toolSchema(map[string]any{
			"all":              map[string]any{"type": "boolean"},
			"older_than_hours": map[string]any{"type": "integer"},
		}),
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

func intDefault(args map[string]any, key string, fallback int) int {
	value, ok := intArg(args, key)
	if !ok {
		return fallback
	}
	return value
}
