package transport

import (
	"fmt"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

const (
	mcpLaneMaxMin               = 0
	mcpLaneMaxSchemaDescription = "Optional reasoning/escalation lane budget. Valid range is 0..4. Omit lane_max unless explicit budget control is needed; MCP clamps out-of-range values to the nearest valid value with a warning."
	mcpLaneMaxGuidance          = "omit lane_max unless you need explicit budget control; valid lane_max range is 0..4; MCP web_read and web_query clamp out-of-range lane_max to the nearest valid value"
)

func laneMaxSchema() map[string]any {
	return map[string]any{
		"type":        "integer",
		"minimum":     mcpLaneMaxMin,
		"maximum":     core.MaxLane,
		"default":     config.Defaults().Runtime.LaneMax,
		"description": mcpLaneMaxSchemaDescription,
	}
}

func applyMCPLaneMax(args map[string]any, cfg *config.Config) []string {
	laneMax, ok := intArg(args, "lane_max")
	if !ok {
		return nil
	}
	clamped := clampMCPLaneMax(laneMax)
	cfg.Runtime.LaneMax = clamped
	if clamped == laneMax {
		return nil
	}
	return []string{fmt.Sprintf("lane_max %d was clamped to %d; omit lane_max unless explicit budget control is needed; valid range is %d..%d", laneMax, clamped, mcpLaneMaxMin, core.MaxLane)}
}

func clampMCPLaneMax(laneMax int) int {
	if laneMax < mcpLaneMaxMin {
		return mcpLaneMaxMin
	}
	if laneMax > core.MaxLane {
		return core.MaxLane
	}
	return laneMax
}
