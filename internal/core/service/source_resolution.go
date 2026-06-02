package service

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/core/agentreadable"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/rendering"
)

func (s *Service) resolveAgentReadable(ctx context.Context, recorder *proof.Recorder, req ReadRequest, rawPage pipeline.RawPage) (pipeline.RawPage, error) {
	const stage = "agent_readable"
	if !s.cfg.Agent.ReadableEnabled || !isHTMLLikeRawPage(rawPage) {
		return rawPage, nil
	}
	if err := recorder.StageStarted(stage, rawPage, s.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	candidates := agentreadable.Discover(rawPage, s.cfg.Agent.MaxCandidates)
	if !shouldProbeConventionalAgentSources(rawPage, req, s.reducer) {
		candidates = declaredAgentReadableCandidates(candidates)
	}
	metadata := map[string]string{
		"candidate_count": fmt.Sprintf("%d", len(candidates)),
		"selected":        "false",
	}
	for _, candidate := range candidates {
		page, ok := s.tryAgentReadableCandidate(ctx, req, rawPage, candidate, recorder)
		if !ok {
			continue
		}
		metadata["selected"] = "true"
		metadata["selected_url"] = page.FinalURL
		metadata["selected_kind"] = page.SourceKind
		metadata["selected_declared_by"] = candidate.DeclaredBy
		if err := recorder.StageCompleted(stage, page, 1, metadata, s.now().UTC()); err != nil {
			return pipeline.RawPage{}, err
		}
		return page, nil
	}
	if err := recorder.StageCompleted(stage, rawPage, len(candidates), metadata, s.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	return rawPage, nil
}

func shouldProbeConventionalAgentSources(rawPage pipeline.RawPage, req ReadRequest, reducer pipeline.Reducer) bool {
	dom, err := reducer.ReduceProfile(rawPage, req.PruningProfile)
	if err != nil {
		return false
	}
	webIR := buildWebIR(ensureMinimumDOM(dom))
	if webIR.Signals.SubstrateClass == "client_rendered_app" {
		return true
	}
	if !agentReadableSurfaceHint(rawPage) {
		return false
	}
	for _, reason := range core.WebIRUtilityReasons(webIR) {
		switch reason {
		case "low_node_count", "low_reduced_chars", "navigation_like_surface", "client_rendered_app_surface":
			return true
		}
	}
	return false
}

func agentReadableSurfaceHint(rawPage pipeline.RawPage) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawPage.FinalURL))
	if err != nil {
		return false
	}
	cleanPath := path.Clean("/" + strings.TrimSpace(parsed.Path))
	if cleanPath != "/" && strings.Count(strings.Trim(cleanPath, "/"), "/") >= 1 {
		return true
	}
	return false
}

func declaredAgentReadableCandidates(candidates []agentreadable.Candidate) []agentreadable.Candidate {
	out := make([]agentreadable.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		switch candidate.DeclaredBy {
		case "link_header", "html_link":
			out = append(out, candidate)
		}
	}
	return out
}

func (s *Service) tryAgentReadableCandidate(ctx context.Context, req ReadRequest, original pipeline.RawPage, candidate agentreadable.Candidate, recorder *proof.Recorder) (pipeline.RawPage, bool) {
	page, err := s.acquirer.Acquire(ctx, s.fetchAcquireInputWithProfilesAndAccept(candidate.URL, effectiveUserAgent(req.UserAgent, req.RenderHint), req.FetchProfile, req.FetchRetryProfile, agentreadable.RequestAccept(candidate)))
	if err != nil {
		recorder.Error("agent_readable", "NX_AGENT_READABLE_FETCH_FAILED", err.Error(), map[string]string{
			"candidate_url":  candidate.URL,
			"candidate_kind": candidate.Kind,
		}, s.now().UTC())
		return pipeline.RawPage{}, false
	}
	page.SourceKind = candidate.Kind
	page.SourceReason = candidate.DeclaredBy
	page.SourceFrom = original.FinalURL
	if candidate.Kind == agentreadable.KindLLMSIndex {
		if linked := agentreadable.BestLinkedMarkdownFor(original.FinalURL, page.FinalURL, page.HTML); linked != "" {
			linkedPage, ok := s.tryAgentReadableCandidate(ctx, req, original, agentreadable.Candidate{
				URL:        linked,
				Kind:       agentreadable.KindMarkdownVariant,
				DeclaredBy: "llms_txt",
				Priority:   candidate.Priority,
			}, recorder)
			if ok {
				return linkedPage, true
			}
		}
	}
	if agentreadable.IsAgentReadablePage(page) {
		return page, true
	}
	return pipeline.RawPage{}, false
}

func (s *Service) maybeRender(ctx context.Context, recorder *proof.Recorder, req ReadRequest, rawPage pipeline.RawPage, dom pipeline.SimplifiedDOM) (pipeline.RawPage, pipeline.SimplifiedDOM, error) {
	mode := normalizeRenderMode(req.RenderMode, req.RenderHint)
	if mode == "off" || strings.Contains(strings.ToLower(rawPage.SourceKind), "markdown") || rawPage.FetchMode == core.FetchModeRender {
		return rawPage, dom, nil
	}
	if mode == "auto" && !s.cfg.Render.Enabled {
		return rawPage, dom, nil
	}
	webIR := buildWebIR(ensureMinimumDOM(dom))
	reasons := core.WebIRUtilityReasons(webIR)
	if mode == "auto" && !shouldRenderForRead(rawPage, webIR, reasons) {
		return rawPage, dom, nil
	}
	rendered, err := s.renderPage(ctx, recorder, req, rawPage, mode, reasons)
	if err != nil {
		if mode == "required" {
			return pipeline.RawPage{}, pipeline.SimplifiedDOM{}, err
		}
		return rawPage, dom, nil
	}
	renderedDOM, err := s.reduce(recorder, rendered, req)
	if err != nil {
		if mode == "required" {
			return pipeline.RawPage{}, pipeline.SimplifiedDOM{}, err
		}
		return rawPage, dom, nil
	}
	return rendered, renderedDOM, nil
}

func (s *Service) renderPage(ctx context.Context, recorder *proof.Recorder, req ReadRequest, rawPage pipeline.RawPage, mode string, reasons []string) (pipeline.RawPage, error) {
	const stage = "render"
	if err := recorder.StageStarted(stage, rawPage, s.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	recorder.EscalationTriggered(stage, "NX_JS_RENDER_REQUIRED", "static source did not provide useful agent-readable content", 4, map[string]string{
		"mode":    mode,
		"reasons": strings.Join(reasons, ","),
	}, s.now().UTC())
	rendered, err := s.renderer.Render(ctx, rendering.Request{
		URL:       rawPage.FinalURL,
		UserAgent: effectiveUserAgent(req.UserAgent, true),
		Timeout:   time.Duration(s.cfg.Render.TimeoutMS) * time.Millisecond,
		MaxBytes:  s.cfg.Runtime.MaxBytes,
	})
	if err != nil {
		recorder.Error(stage, "NX_JS_RENDER_FAILED", err.Error(), map[string]string{"mode": mode}, s.now().UTC())
		_ = recorder.StageCompleted(stage, rawPage, 0, map[string]string{
			"rendered": "false",
			"error":    err.Error(),
		}, s.now().UTC())
		return pipeline.RawPage{}, err
	}
	page := pipeline.RawPage{
		URL:          rawPage.URL,
		FinalURL:     rendered.FinalURL,
		StatusCode:   200,
		ContentType:  "text/html; charset=utf-8",
		HTML:         rendered.HTML,
		Partial:      rendered.Partial,
		FetchMode:    core.FetchModeRender,
		FetchProfile: rawPage.FetchProfile,
		SourceKind:   "rendered_dom",
		SourceReason: "js_render",
		SourceFrom:   rawPage.FinalURL,
		FetchedAt:    rendered.FetchedAt,
	}
	if page.FinalURL == "" {
		page.FinalURL = rawPage.FinalURL
	}
	if err := recorder.StageCompleted(stage, page, 1, map[string]string{
		"rendered":    "true",
		"browser":     rendered.Browser,
		"duration_ms": fmt.Sprintf("%d", rendered.Duration.Milliseconds()),
		"partial":     fmt.Sprintf("%t", rendered.Partial),
	}, s.now().UTC()); err != nil {
		return pipeline.RawPage{}, err
	}
	return page, nil
}

func isHTMLLikeRawPage(page pipeline.RawPage) bool {
	contentType := strings.ToLower(strings.TrimSpace(page.ContentType))
	return contentType == "" || strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml+xml")
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
