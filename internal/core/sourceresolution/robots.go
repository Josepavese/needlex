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
	if mode == "auto" && !pipeline.IsHTMLLikeRawPage(rawPage) {
		return rawPage, dom, nil
	}
	webIR := webirbuilder.Build(webirbuilder.EnsureMinimum(dom))
	reasons := core.WebIRUtilityReasons(webIR)
	semanticGap := 0.0
	if mode == "auto" && !shouldRenderForRead(rawPage, webIR, reasons) {
		similarity, gap := r.semanticRenderGap(ctx, req, dom)
		if !gap {
			return rawPage, dom, nil
		}
		semanticGap = similarity
		reasons = append(reasons, semanticRenderEscalationReason)
	}
	if mode == "auto" && !autoRenderHasTime(ctx) {
		return rawPage, dom, nil
	}
	rendered, err := r.renderPage(ctx, recorder, req, rawPage, mode, reasons, semanticGap)
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

func (r Resolver) renderPage(ctx context.Context, recorder *proof.Recorder, req Request, rawPage pipeline.RawPage, mode string, reasons []string, semanticGap float64) (pipeline.RawPage, error) {
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
	stats := rendered.NetworkStats
	networkTruncated := stats.Truncated || stats.StreamsOpen > 0
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
		NetworkBytes:     stats.BodyBytes,
		NetworkResources: stats.ResourceCount,
		NetworkTruncated: networkTruncated,
		FetchedAt:        rendered.FetchedAt,
	}
	if page.FinalURL == "" {
		page.FinalURL = rawPage.FinalURL
	}
	networkIdleReason := strings.TrimSpace(stats.IdleReason)
	if networkIdleReason == "" {
		networkIdleReason = "not_collected"
	}
	metadata := map[string]string{
		"rendered":              "true",
		"render_path":           renderPath(rendered),
		"browser":               rendered.Browser,
		"duration_ms":           fmt.Sprintf("%d", rendered.Duration.Milliseconds()),
		"partial":               fmt.Sprintf("%t", rendered.Partial),
		"network_resources":     fmt.Sprintf("%d", stats.ResourceCount),
		"network_observed":      fmt.Sprintf("%d", stats.ObservedResources),
		"network_body_missing":  fmt.Sprintf("%d", stats.BodyUnavailable),
		"network_streams_open":  fmt.Sprintf("%d", stats.StreamsOpen),
		"network_bytes":         fmt.Sprintf("%d", stats.BodyBytes),
		"network_truncated":     fmt.Sprintf("%t", networkTruncated),
		"event_source_messages": fmt.Sprintf("%d", stats.EventSourceMessages),
		"websocket_messages":    fmt.Sprintf("%d", stats.WebSocketMessages),
		"network_idle_reason":   networkIdleReason,
	}
	if len(rendered.NetworkStats.BodyUnavailableURLs) > 0 {
		metadata["network_body_missing_sample"] = strings.Join(rendered.NetworkStats.BodyUnavailableURLs, " | ")
	}
	if rendered.Degraded {
		metadata["render_degraded"] = "true"
		metadata["render_degrade_reason"] = rendered.DegradeReason
	}
	if semanticGap > 0 {
		metadata["semantic_gap_similarity"] = fmt.Sprintf("%.4f", semanticGap)
	}
	if err := recorder.StageCompleted(stage, page, 1, metadata, r.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	return page, nil
}
