package transport

import (
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/config"
)

const (
	retrievalEffortMinimal    = "minimal"
	retrievalEffortLight      = "light"
	retrievalEffortBalanced   = "balanced"
	retrievalEffortStandard   = "standard"
	retrievalEffortExhaustive = "exhaustive"
)

var retrievalEffortLanes = map[string]int{
	retrievalEffortMinimal:    0,
	retrievalEffortLight:      1,
	retrievalEffortBalanced:   2,
	retrievalEffortStandard:   3,
	retrievalEffortExhaustive: 4,
}

func retrievalEffortSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"enum":        []string{retrievalEffortMinimal, retrievalEffortLight, retrievalEffortBalanced, retrievalEffortStandard, retrievalEffortExhaustive},
		"default":     retrievalEffortStandard,
		"description": "Optional retrieval effort for semantic extraction and internal escalation. This is not a result count, page count, crawl depth, or candidate limit. Omit it for standard behavior.",
	}
}

func renderModeSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"enum":        []string{"auto", "off", "required"},
		"default":     "auto",
		"description": "Optional JavaScript rendering mode for the final page read. auto uses declared agent-readable sources first and renders when needed; off forbids browser rendering; required fails if the rendered DOM cannot be obtained.",
	}
}

func applyMCPRetrievalEffort(args map[string]any, cfg *config.Config) error {
	if _, ok := args["lane_max"]; ok {
		return fmt.Errorf("unsupported field lane_max; use retrieval_effort with one of: %s", retrievalEffortValues())
	}
	if _, ok := args["quality_budget"]; ok {
		return fmt.Errorf("unsupported field quality_budget; use retrieval_effort with one of: %s", retrievalEffortValues())
	}
	raw, ok := args["retrieval_effort"]
	if !ok || raw == nil {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return fmt.Errorf("retrieval_effort must be a string with one of: %s", retrievalEffortValues())
	}
	return applyRetrievalEffort(value, cfg)
}

func applyRetrievalEffort(value string, cfg *config.Config) error {
	effort := strings.ToLower(strings.TrimSpace(value))
	if effort == "" {
		return nil
	}
	lane, ok := retrievalEffortLanes[effort]
	if !ok {
		return fmt.Errorf("retrieval_effort must be one of: %s", retrievalEffortValues())
	}
	cfg.Runtime.LaneMax = lane
	return nil
}

func retrievalEffortValues() string {
	return strings.Join([]string{retrievalEffortMinimal, retrievalEffortLight, retrievalEffortBalanced, retrievalEffortStandard, retrievalEffortExhaustive}, ", ")
}
