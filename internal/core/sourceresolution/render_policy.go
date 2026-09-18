package sourceresolution

// Render escalation policy: when a page is rendered, how long it may wait, and
// how objective coverage grounds the decision. The execution mechanism lives in
// robots.go.

import (
	"context"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/rendering"
)

func renderPath(page rendering.Page) string {
	if page.Degraded || !strings.Contains(page.Browser, "cdp") {
		return "dump_dom"
	}
	return "cdp"
}

func autoRenderHasTime(ctx context.Context) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(deadline) >= AutoRenderDeadlineMinRemaining
}

const autoRenderDeadlineReserve = 2 * time.Second

// semanticRenderGapThreshold is calibrated against the local embedding runtime:
// navigation-like shells score around 0.35 against a concrete objective, while
// surfaces that genuinely cover it score 0.59 and above, including cross-language
// matches. The threshold sits in that measured gap.
const semanticRenderGapThreshold = 0.5

const semanticRenderEscalationReason = "semantic_coverage_gap"

func (r Resolver) semanticRenderGap(ctx context.Context, req Request, dom pipeline.SimplifiedDOM) (float64, bool) {
	if r.Semantic == nil || !semanticRenderGapObjectiveUsable(req.Objective) {
		return 0, false
	}
	text := discoverycore.CompactSemanticText(reducedSurfaceText(dom), 1600)
	if text == "" {
		return 0, false
	}
	scored, err := r.Semantic.Score(ctx, req.Objective, []intel.SemanticCandidate{{ID: "static_surface", Text: text}})
	if err != nil {
		return 0, false
	}
	best := 0.0
	for _, score := range scored {
		best = max(best, score.Similarity)
	}
	if best <= 0 || best >= semanticRenderGapThreshold {
		return 0, false
	}
	return best, true
}

func reducedSurfaceText(dom pipeline.SimplifiedDOM) string {
	parts := make([]string, 0, len(dom.Nodes))
	for _, node := range dom.Nodes {
		if text := strings.TrimSpace(node.Text); text != "" {
			parts = append(parts, text)
		}
	}
	body := strings.Join(parts, "\n")
	if title := strings.TrimSpace(dom.Title); title != "" {
		return title + "\n" + body
	}
	return body
}

// semanticRenderGapObjectiveUsable keeps placeholder objectives such as the
// crawl lane marker from grounding a coverage comparison.
func semanticRenderGapObjectiveUsable(objective string) bool {
	objective = strings.TrimSpace(objective)
	if len([]rune(objective)) < 16 {
		return false
	}
	return len(strings.Fields(objective)) >= 3
}

func autoBoundedRenderTimeout(ctx context.Context, mode string, configuredMS int64) time.Duration {
	timeout := time.Duration(configuredMS) * time.Millisecond
	if timeout <= 0 {
		timeout = AutoRenderFallbackTimeout
	}
	if mode != "auto" {
		return timeout
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline) - autoRenderDeadlineReserve
		if remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	if timeout < time.Second {
		return time.Second
	}
	return timeout
}

func normalizeRenderMode(mode string, renderHint bool) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "off":
		return "off"
	case "required":
		return "required"
	case "auto":
		return "auto"
	default:
		if renderHint {
			return "auto"
		}
		return "auto"
	}
}

func shouldRenderForRead(rawPage pipeline.RawPage, webIR core.WebIR, reasons []string) bool {
	if strings.TrimSpace(rawPage.SourceKind) != "" {
		return false
	}
	if strings.TrimSpace(webIR.Signals.SubstrateClass) == "client_rendered_app" {
		return true
	}
	for _, reason := range reasons {
		switch reason {
		case "low_node_count", "low_reduced_chars", "navigation_like_surface", "client_rendered_app_surface":
			return true
		}
	}
	return false
}
