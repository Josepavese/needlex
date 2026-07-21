package sourceresolution

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/core/agentreadable"
	"github.com/josepavese/needlex/internal/core/fetchpolicy"
	"github.com/josepavese/needlex/internal/core/webirbuilder"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/rendering"
)

func filterCandidatesByRobots(candidates []agentreadable.Candidate, policy agentreadable.RobotsPolicy, loaded bool, userAgent string) ([]agentreadable.Candidate, int) {
	if !loaded {
		return candidates, 0
	}
	out := make([]agentreadable.Candidate, 0, len(candidates))
	disallowed := 0
	for _, candidate := range candidates {
		if policy.Allows(userAgent, candidate.URL) {
			out = append(out, candidate)
			continue
		}
		disallowed++
	}
	return out, disallowed
}

func (r Resolver) fetchAgentReadableAuxiliary(ctx context.Context, req Request, rawURL, accept string) (pipeline.RawPage, bool) {
	auxCtx, cancel, ok := agentReadableProbeContext(ctx)
	if !ok {
		return pipeline.RawPage{}, false
	}
	defer cancel()
	page, err := r.Acquirer.Acquire(auxCtx, fetchpolicy.Input(r.Config, rawURL, pipeline.EffectiveUserAgent(req.UserAgent, req.RenderHint), req.FetchProfile, req.FetchRetryProfile, accept))
	if err != nil {
		return pipeline.RawPage{}, false
	}
	return page, true
}

func agentReadableHasProbeTime(ctx context.Context) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(deadline) >= agentReadableProbeMinRemaining
}

func agentReadableProbeContext(ctx context.Context) (context.Context, context.CancelFunc, bool) {
	if !agentReadableHasProbeTime(ctx) {
		return nil, nil, false
	}
	timeout := AgentReadableProbeTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline) - agentReadableProbeMinRemaining
		if remaining <= 0 {
			return nil, nil, false
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	if timeout <= 0 {
		return ctx, func() {}, true
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	return probeCtx, cancel, true
}

func (r Resolver) MaybeRender(ctx context.Context, recorder *proof.Recorder, req Request, rawPage pipeline.RawPage, dom pipeline.SimplifiedDOM) (pipeline.RawPage, pipeline.SimplifiedDOM, error) {
	mode := normalizeRenderMode(req.RenderMode, req.RenderHint)
	if mode == "off" || strings.Contains(strings.ToLower(rawPage.SourceKind), "markdown") || rawPage.FetchMode == core.FetchModeRender {
		return rawPage, dom, nil
	}
	if mode == "auto" && !r.Config.Render.Enabled {
		return rawPage, dom, nil
	}
	webIR := webirbuilder.Build(webirbuilder.EnsureMinimum(dom))
	reasons := core.WebIRUtilityReasons(webIR)
	if mode == "auto" && !shouldRenderForRead(rawPage, webIR, reasons) {
		return rawPage, dom, nil
	}
	if mode == "auto" && !autoRenderHasTime(ctx) {
		return rawPage, dom, nil
	}
	rendered, err := r.renderPage(ctx, recorder, req, rawPage, mode, reasons)
	if err != nil {
		if mode == "required" {
			return pipeline.RawPage{}, pipeline.SimplifiedDOM{}, err
		}
		return rawPage, dom, nil
	}
	if r.Reduce == nil {
		return pipeline.RawPage{}, pipeline.SimplifiedDOM{}, fmt.Errorf("source resolver reduce callback is required")
	}
	renderedDOM, err := r.Reduce(recorder, rendered, req.PruningProfile)
	if err != nil {
		if mode == "required" {
			return pipeline.RawPage{}, pipeline.SimplifiedDOM{}, err
		}
		return rawPage, dom, nil
	}
	return rendered, renderedDOM, nil
}

func (r Resolver) renderPage(ctx context.Context, recorder *proof.Recorder, req Request, rawPage pipeline.RawPage, mode string, reasons []string) (pipeline.RawPage, error) {
	const stage = "render"
	if err := recorder.StageStarted(stage, rawPage, r.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	recorder.EscalationTriggered(stage, "NX_JS_RENDER_REQUIRED", "static source did not provide useful agent-readable content", 4, map[string]string{
		"mode":    mode,
		"reasons": strings.Join(reasons, ","),
	}, r.now().UTC())
	rendered, err := r.Renderer.Render(ctx, rendering.Request{
		URL:                     rawPage.FinalURL,
		UserAgent:               pipeline.EffectiveUserAgent(req.UserAgent, true),
		Timeout:                 autoBoundedRenderTimeout(ctx, mode, r.Config.Render.TimeoutMS),
		MaxBytes:                r.Config.Runtime.MaxBytes,
		NetworkIdle:             time.Duration(r.Config.Render.NetworkIdleMS) * time.Millisecond,
		NetworkMaxBytes:         r.Config.Render.NetworkMaxBytes,
		NetworkResourceMaxBytes: r.Config.Render.NetworkResourceMaxBytes,
		NetworkMaxResources:     r.Config.Render.NetworkMaxResources,
		NetworkMaxMessages:      r.Config.Render.NetworkMaxMessages,
	})
	if err != nil {
		recorder.Error(stage, "NX_JS_RENDER_FAILED", err.Error(), map[string]string{"mode": mode}, r.now().UTC())
		_ = recorder.StageCompleted(stage, rawPage, 0, map[string]string{
			"rendered": "false",
			"error":    err.Error(),
		}, r.now().UTC())
		return pipeline.RawPage{}, err
	}
	networkText := rendering.EvidenceText(rendered.NetworkResources)
	page := pipeline.RawPage{
		URL:              rawPage.URL,
		FinalURL:         rendered.FinalURL,
		StatusCode:       200,
		ContentType:      "text/html; charset=utf-8",
		HTML:             rendered.HTML,
		Partial:          rendered.Partial,
		FetchMode:        core.FetchModeRender,
		FetchProfile:     rawPage.FetchProfile,
		SourceKind:       "rendered_dom",
		SourceReason:     "js_render",
		SourceFrom:       rawPage.FinalURL,
		NetworkText:      networkText,
		NetworkBytes:     rendered.NetworkStats.BodyBytes,
		NetworkResources: rendered.NetworkStats.ResourceCount,
		NetworkTruncated: rendered.NetworkStats.Truncated,
		FetchedAt:        rendered.FetchedAt,
	}
	if page.FinalURL == "" {
		page.FinalURL = rawPage.FinalURL
	}
	networkIdleReason := strings.TrimSpace(rendered.NetworkStats.IdleReason)
	if networkIdleReason == "" {
		networkIdleReason = "not_collected"
	}
	if err := recorder.StageCompleted(stage, page, 1, map[string]string{
		"rendered":              "true",
		"browser":               rendered.Browser,
		"duration_ms":           fmt.Sprintf("%d", rendered.Duration.Milliseconds()),
		"partial":               fmt.Sprintf("%t", rendered.Partial),
		"network_resources":     fmt.Sprintf("%d", rendered.NetworkStats.ResourceCount),
		"network_bytes":         fmt.Sprintf("%d", rendered.NetworkStats.BodyBytes),
		"network_truncated":     fmt.Sprintf("%t", rendered.NetworkStats.Truncated),
		"event_source_messages": fmt.Sprintf("%d", rendered.NetworkStats.EventSourceMessages),
		"websocket_messages":    fmt.Sprintf("%d", rendered.NetworkStats.WebSocketMessages),
		"network_idle_reason":   networkIdleReason,
	}, r.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	return page, nil
}

func autoRenderHasTime(ctx context.Context) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(deadline) >= AutoRenderDeadlineMinRemaining
}

func autoBoundedRenderTimeout(ctx context.Context, mode string, configuredMS int64) time.Duration {
	timeout := time.Duration(configuredMS) * time.Millisecond
	if timeout <= 0 {
		timeout = AutoRenderDeadlineTimeout
	}
	if mode != "auto" {
		return timeout
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return timeout
	}
	if timeout > AutoRenderDeadlineTimeout {
		timeout = AutoRenderDeadlineTimeout
	}
	remaining := time.Until(deadline) - 2*time.Second
	if remaining > 0 && remaining < timeout {
		timeout = remaining
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
