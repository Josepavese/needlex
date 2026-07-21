package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/webdiscover"
	"github.com/josepavese/needlex/internal/core/webirbuilder"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/rendering"
)

func (s *Service) probeWebCandidatePage(ctx context.Context, goal, userAgent string, domainHints []string, candidate DiscoverCandidate, rawPage pipeline.RawPage, dom pipeline.SimplifiedDOM, webIR core.WebIR, probeMetadata map[string]string, includeHostRoot bool) []DiscoverCandidate {
	refined := webdiscover.RefineCandidate(goal, candidate, rawPage.FinalURL, dom.Title, webIR, domainHints)
	if len(probeMetadata) > 0 {
		refined.Score += browserProbeScoreBoost(rawPage, webIR)
		refined.Reason = discoverycore.AppendUniqueReason(refined.Reason, "browser_probe", "browser_rendered")
		refined.Metadata = discoverycore.MergeMetadata(refined.Metadata, probeMetadata)
	}
	out := []DiscoverCandidate{refined}
	if rootProbe := s.probeSemanticRootIdentity(ctx, goal, rawPage.FinalURL, dom.Title, webIR); rootProbe.Score > 0 {
		refined.Score += rootProbe.Score
		refined.Reason = discoverycore.AppendUniqueReason(refined.Reason, rootProbe.Reasons...)
		refined.Metadata = discoverycore.MergeMetadata(refined.Metadata, rootProbe.Metadata)
		out[0] = refined
	}
	if includeHostRoot {
		if hostProbe, err := s.probeHostRootIdentity(ctx, goal, userAgent, rawPage.FinalURL); err == nil {
			if hostProbe.Score > 0 {
				refined.Score += hostProbe.Score
				refined.Reason = discoverycore.AppendUniqueReason(refined.Reason, hostProbe.Reasons...)
				refined.Metadata = discoverycore.MergeMetadata(refined.Metadata, hostProbe.Metadata)
				out[0] = refined
			}
			if strings.TrimSpace(hostProbe.URL) != "" && !discoverycore.SameCanonicalURL(hostProbe.URL, refined.URL) && hostProbe.Score > 0 {
				out = append(out, DiscoverCandidate{
					URL:      hostProbe.URL,
					Label:    discoverycore.FirstNonEmpty(hostProbe.Title, hostProbe.URL),
					Score:    hostProbe.Score + 0.20,
					Reason:   discoverycore.AppendUniqueReason(hostProbe.Reasons, "host_root_candidate"),
					Metadata: discoverycore.MergeMetadata(nil, hostProbe.Metadata),
				})
			}
		}
	}

	identityRefs := webdiscover.ExtractIdentityReferenceCandidates(rawPage.HTML, rawPage.FinalURL, discoverycore.FirstNonEmpty(dom.Title, candidate.Label))
	if len(identityRefs) > 0 {
		out = append(out, annotateBrowserProbeCandidates(s.selectIdentityReferenceCandidates(ctx, goal, refined, identityRefs), probeMetadata)...)
	}
	expanded := extractLinkCandidates(rawPage.HTML, rawPage.FinalURL, false)
	expandedScored := discoverycore.ScoreStructuralCandidates("", "", expanded, domainHints)
	if len(expandedScored) > 0 {
		out = append(out, annotateBrowserProbeCandidates(s.selectExpandedRecoveryCandidates(ctx, goal, refined, expandedScored), probeMetadata)...)
	}
	out = append(out, annotateBrowserProbeCandidates(webdiscover.ExtractEmbeddedURLCandidates(goal, refined, rawPage.FinalURL, rawPage.HTML, dom, domainHints), probeMetadata)...)
	return out
}

func (s *Service) shouldRenderWebCandidateProbe(ctx context.Context, rawPage pipeline.RawPage, webIR core.WebIR) bool {
	if !s.cfg.Render.Enabled || s.renderer == nil {
		return false
	}
	if !pipeline.IsHTMLLikeRawPage(rawPage) || rawPage.FetchMode == core.FetchModeRender {
		return false
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(rawPage.SourceKind)), "markdown") {
		return false
	}
	if !browserProbeHasTime(ctx) {
		return false
	}
	if strings.TrimSpace(webIR.Signals.SubstrateClass) == "client_rendered_app" {
		return true
	}
	for _, reason := range core.WebIRUtilityReasons(webIR) {
		if reason == "client_rendered_app_surface" {
			return true
		}
	}
	return false
}

func (s *Service) renderWebCandidateProbe(ctx context.Context, rawPage pipeline.RawPage, userAgent string, reasons []string) (pipeline.RawPage, pipeline.SimplifiedDOM, core.WebIR, map[string]string, error) {
	targetURL := discoverycore.FirstNonEmpty(rawPage.FinalURL, rawPage.URL)
	if strings.TrimSpace(targetURL) == "" {
		return pipeline.RawPage{}, pipeline.SimplifiedDOM{}, core.WebIR{}, nil, fmt.Errorf("browser probe target url is empty")
	}
	rendered, err := s.renderer.Render(ctx, rendering.Request{
		URL:                     targetURL,
		UserAgent:               pipeline.EffectiveUserAgent(userAgent, true),
		Timeout:                 seedlessBrowserProbeTimeout(ctx, s.cfg.Render.TimeoutMS),
		MaxBytes:                s.cfg.Runtime.MaxBytes,
		NetworkIdle:             seedlessBrowserProbeNetworkIdle(s.cfg.Render.NetworkIdleMS),
		NetworkMaxBytes:         seedlessBrowserProbeNetworkMaxBytes(s.cfg.Render.NetworkMaxBytes),
		NetworkResourceMaxBytes: seedlessBrowserProbeResourceMaxBytes(s.cfg.Render.NetworkResourceMaxBytes),
		NetworkMaxResources:     seedlessBrowserProbeMaxResources(s.cfg.Render.NetworkMaxResources),
		NetworkMaxMessages:      seedlessBrowserProbeMaxMessages(s.cfg.Render.NetworkMaxMessages),
	})
	if err != nil {
		return pipeline.RawPage{}, pipeline.SimplifiedDOM{}, core.WebIR{}, nil, err
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
		SourceReason:     "seedless_browser_probe",
		SourceFrom:       targetURL,
		NetworkText:      networkText,
		NetworkBytes:     rendered.NetworkStats.BodyBytes,
		NetworkResources: rendered.NetworkStats.ResourceCount,
		NetworkTruncated: rendered.NetworkStats.Truncated,
		FetchedAt:        rendered.FetchedAt,
	}
	if strings.TrimSpace(page.URL) == "" {
		page.URL = targetURL
	}
	if strings.TrimSpace(page.FinalURL) == "" {
		page.FinalURL = targetURL
	}
	if page.FetchedAt.IsZero() {
		page.FetchedAt = s.now().UTC()
	}
	dom, err := s.reducer.Reduce(page)
	if err != nil {
		return pipeline.RawPage{}, pipeline.SimplifiedDOM{}, core.WebIR{}, nil, err
	}
	dom = webirbuilder.EnsureMinimum(dom)
	webIR := webirbuilder.Build(dom)
	idleReason := strings.TrimSpace(rendered.NetworkStats.IdleReason)
	if idleReason == "" {
		idleReason = "not_collected"
	}
	metadata := map[string]string{
		"browser_probe":                 "true",
		"browser_rendered":              "true",
		"browser":                       strings.TrimSpace(rendered.Browser),
		"browser_duration_ms":           strconv.FormatInt(rendered.Duration.Milliseconds(), 10),
		"browser_partial":               strconv.FormatBool(rendered.Partial),
		"browser_network_resources":     strconv.Itoa(rendered.NetworkStats.ResourceCount),
		"browser_network_bytes":         strconv.FormatInt(rendered.NetworkStats.BodyBytes, 10),
		"browser_network_truncated":     strconv.FormatBool(rendered.NetworkStats.Truncated),
		"browser_event_source_messages": strconv.Itoa(rendered.NetworkStats.EventSourceMessages),
		"browser_websocket_messages":    strconv.Itoa(rendered.NetworkStats.WebSocketMessages),
		"browser_idle_reason":           idleReason,
		"browser_probe_reasons":         strings.Join(reasons, ","),
	}
	return page, dom, webIR, metadata, nil
}

func browserProbeHasTime(ctx context.Context) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(deadline) >= webBrowserProbeMinRemaining
}

func seedlessBrowserProbeTimeout(ctx context.Context, configuredMS int64) time.Duration {
	timeout := webBrowserProbeTimeout
	if configured := time.Duration(configuredMS) * time.Millisecond; configured > 0 && configured < timeout {
		timeout = configured
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline) - 2*time.Second
		if remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	if timeout < time.Second {
		return time.Second
	}
	return timeout
}

func seedlessBrowserProbeNetworkIdle(configuredMS int64) time.Duration {
	idle := webBrowserProbeNetworkIdle
	if configured := time.Duration(configuredMS) * time.Millisecond; configured > 0 && configured < idle {
		idle = configured
	}
	return idle
}

func seedlessBrowserProbeNetworkMaxBytes(configured int64) int64 {
	if configured > 0 && configured < webBrowserProbeNetworkBytes {
		return configured
	}
	return webBrowserProbeNetworkBytes
}

func seedlessBrowserProbeResourceMaxBytes(configured int64) int64 {
	if configured > 0 && configured < webBrowserProbeResourceBytes {
		return configured
	}
	return webBrowserProbeResourceBytes
}

func seedlessBrowserProbeMaxResources(configured int) int {
	if configured > 0 && configured < webBrowserProbeResources {
		return configured
	}
	return webBrowserProbeResources
}

func seedlessBrowserProbeMaxMessages(configured int) int {
	if configured > 0 && configured < webBrowserProbeMessages {
		return configured
	}
	return webBrowserProbeMessages
}

func browserProbeScoreBoost(rawPage pipeline.RawPage, webIR core.WebIR) float64 {
	boost := 0.08
	if webIR.NodeCount > 1 {
		boost += 0.08
	}
	if rawPage.NetworkResources > 0 || strings.TrimSpace(rawPage.NetworkText) != "" {
		boost += 0.10
	}
	if rawPage.NetworkTruncated {
		boost -= 0.04
	}
	return boost
}

func annotateBrowserProbeCandidates(candidates []DiscoverCandidate, metadata map[string]string) []DiscoverCandidate {
	if len(metadata) == 0 || len(candidates) == 0 {
		return candidates
	}
	out := append([]DiscoverCandidate{}, candidates...)
	for i := range out {
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "browser_probe", "browser_rendered")
		out[i].Metadata = discoverycore.MergeMetadata(out[i].Metadata, metadata)
	}
	return out
}

type hostRootIdentityProbe struct {
	URL      string
	Title    string
	Score    float64
	Reasons  []string
	Metadata map[string]string
}
