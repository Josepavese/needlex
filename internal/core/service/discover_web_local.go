package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/fetchpolicy"
	"github.com/josepavese/needlex/internal/core/targetkind"
	"github.com/josepavese/needlex/internal/core/webdiscover"
	"github.com/josepavese/needlex/internal/core/webirbuilder"
	"github.com/josepavese/needlex/internal/pipeline"
)

func (s *Service) discoverWebLocalFirst(ctx context.Context, req DiscoverWebRequest) (DiscoverWebResponse, bool) {
	discovery, err := s.Discover(ctx, DiscoverRequest{Goal: req.Goal, SeedURL: req.SeedURL, UserAgent: req.UserAgent, SameDomain: true, PreferSpecificSameSite: true, MaxCandidates: req.MaxCandidates, DomainHints: req.DomainHints})
	if err != nil || len(discovery.Candidates) == 0 {
		return DiscoverWebResponse{}, false
	}
	top := discovery.Candidates[0]
	if top.URL == discovery.SeedURL || !webdiscover.LocalSubstrateResolved(top) {
		return DiscoverWebResponse{}, false
	}
	if seed, ok := s.preservedLocalFirstSeed(ctx, req, discovery, top); ok {
		return DiscoverWebResponse{SeedURL: req.SeedURL, Provider: "local_same_site", SelectedURL: seed.URL, DiscoveryURL: discovery.DiscoveryURL, Candidates: []DiscoverCandidate{seed}}, true
	}
	top.Reason = discoverycore.AppendUniqueReason(top.Reason, "native_substrate")
	return DiscoverWebResponse{SeedURL: req.SeedURL, Provider: "local_same_site", SelectedURL: top.URL, DiscoveryURL: discovery.DiscoveryURL, Candidates: []DiscoverCandidate{top}}, true
}

func (s *Service) preservedLocalFirstSeed(ctx context.Context, req DiscoverWebRequest, discovery DiscoverResponse, top DiscoverCandidate) (DiscoverCandidate, bool) {
	if urlPathDepth(discovery.SeedURL) == 0 {
		return DiscoverCandidate{}, false
	}
	seed, ok := discoverCandidateByURL(discovery.Candidates, discovery.SeedURL)
	if !ok {
		return DiscoverCandidate{}, false
	}
	if targetkind.WeakCanonicalHome(targetkind.Infer(ctx, s.semantic, req.Goal)) {
		seed.Reason = discoverycore.AppendUniqueReason(seed.Reason, "semantic_seed_preserved")
		return seed, true
	}
	seedSemantic := localFirstSemanticScore(seed)
	topSemantic := localFirstSemanticScore(top)
	if seedSemantic > 0 || topSemantic > 0 {
		if topSemantic <= seedSemantic+0.08 {
			seed.Reason = discoverycore.AppendUniqueReason(seed.Reason, "semantic_seed_preserved")
			return seed, true
		}
		return DiscoverCandidate{}, false
	}
	if top.Score-seed.Score <= 0.25 && candidateHasAnyReason(seed, "seed_fallback") {
		seed.Reason = discoverycore.AppendUniqueReason(seed.Reason, "seed_context_preserved")
		return seed, true
	}
	return DiscoverCandidate{}, false
}

func discoverCandidateByURL(candidates []DiscoverCandidate, rawURL string) (DiscoverCandidate, bool) {
	for _, candidate := range candidates {
		if sameNormalizedURL(candidate.URL, rawURL) {
			return candidate, true
		}
	}
	return DiscoverCandidate{}, false
}

func localFirstSemanticScore(candidate DiscoverCandidate) float64 {
	for _, key := range []string{"semantic_goal_similarity", "candidate_goal_similarity", "semantic_evidence_similarity"} {
		value, err := strconv.ParseFloat(strings.TrimSpace(candidate.Metadata[key]), 64)
		if err == nil && value > 0 {
			return value
		}
	}
	return 0
}

func (s *Service) discoverWebBootstrap(ctx context.Context, baseURL string, req DiscoverWebRequest, query string) ([]DiscoverCandidate, string, error) {
	switch {
	case discoverycore.IsBraveProvider(baseURL):
		return s.discoverWebBootstrapBrave(ctx, req, query)
	}
	searchURL, err := discoverycore.WebSearchURL(baseURL, query)
	if err != nil {
		return nil, "", err
	}

	rawPage, err := s.acquirer.Acquire(ctx, s.fetchProviderBootstrapInput(searchURL, req.UserAgent))
	if err != nil {
		if discoverycore.IsDuckDuckGoProvider(baseURL) && (strings.Contains(err.Error(), "unexpected status code 403") || strings.Contains(err.Error(), "unexpected status code 202")) {
			return nil, "", fmt.Errorf("duckduckgo provider blocked by anti-bot challenge")
		}
		return nil, "", err
	}
	if discoverycore.IsDuckDuckGoProvider(baseURL) && discoverycore.LooksLikeDuckDuckGoAnomaly(rawPage.HTML) {
		return nil, rawPage.FinalURL, fmt.Errorf("duckduckgo provider blocked by anti-bot challenge")
	}

	results := discoverycore.ExtractSearchResults(rawPage.HTML, rawPage.FinalURL)
	return discoverycore.ScoreStructuralCandidates(req.SeedURL, "", results, req.DomainHints), rawPage.FinalURL, nil
}

func (s *Service) fetchProviderBootstrapInput(searchURL, userAgent string) pipeline.AcquireInput {
	// Provider result pages are a transport surface, not the target document. Keep
	// bootstrap stable and let configured retry profiles handle blocked responses.
	return fetchpolicy.Input(s.cfg, searchURL, pipeline.EffectiveUserAgent(userAgent, true), "standard", "", "")
}

func (s *Service) discoverWebBootstrapBrave(ctx context.Context, req DiscoverWebRequest, query string) ([]DiscoverCandidate, string, error) {
	if strings.TrimSpace(s.cfg.Discovery.BraveAPIKey) == "" {
		return nil, "", webdiscover.ProviderUnavailableError{Reason: "brave api key not configured"}
	}
	endpoint := "https://api.search.brave.com/res/v1/web/search?q=" + url.QueryEscape(strings.TrimSpace(query)) + "&count=" + strconv.Itoa(max(req.MaxCandidates, webProbeLimit))
	respBody, finalURL, err := s.doBootstrapJSON(ctx, http.MethodGet, endpoint, map[string]string{"Accept": "application/json", "X-Subscription-Token": strings.TrimSpace(s.cfg.Discovery.BraveAPIKey)}, nil)
	if err != nil {
		return nil, "", err
	}
	var payload struct {
		Web struct {
			Results []struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, "", fmt.Errorf("decode brave search response: %w", err)
	}
	links := make([]discoverycore.LinkCandidate, 0, len(payload.Web.Results))
	for _, item := range payload.Web.Results {
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		links = append(links, discoverycore.LinkCandidate{URL: strings.TrimSpace(item.URL), Label: strings.TrimSpace(item.Title)})
	}
	return discoverycore.ScoreStructuralCandidates(req.SeedURL, "", links, req.DomainHints), finalURL, nil
}

func (s *Service) doBootstrapJSON(ctx context.Context, method, endpoint string, headers map[string]string, body []byte) ([]byte, string, error) {
	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond}
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.Runtime.TimeoutMS)*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(reqCtx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	for key, value := range headers {
		if strings.TrimSpace(value) != "" {
			request.Header.Set(key, value)
		}
	}
	resp, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("bootstrap provider returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, s.cfg.Runtime.MaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > s.cfg.Runtime.MaxBytes {
		return nil, "", fmt.Errorf("bootstrap provider response exceeds max bytes budget")
	}
	return data, resp.Request.URL.String(), nil
}

func (s *Service) expandAndRerankWebCandidates(ctx context.Context, goal, userAgent string, domainHints []string, candidates []DiscoverCandidate, maxCandidates int) []DiscoverCandidate {
	if len(candidates) == 0 {
		return nil
	}
	probeCount := min(len(candidates), min(maxCandidates, webProbeLimit))
	if probeCount <= 0 {
		probeCount = min(len(candidates), webProbeLimit)
	}

	merged := discoverycore.NewSet(candidates)
	probeCandidates := semanticProbeCandidates(candidates, probeCount)
	browserProbeBudget := min(probeCount, webBrowserProbeLimit)
	for _, candidate := range probeCandidates[:probeCount] {
		probed, browserAttempted, err := s.probeWebCandidate(ctx, goal, userAgent, domainHints, candidate, browserProbeBudget > 0)
		if err != nil {
			continue
		}
		if browserAttempted {
			browserProbeBudget--
		}
		merged.Merge(probed)
	}
	return merged.Sorted()
}

func semanticProbeCandidates(candidates []DiscoverCandidate, probeCount int) []DiscoverCandidate {
	if probeCount <= 0 || len(candidates) <= probeCount {
		return candidates
	}
	out := make([]DiscoverCandidate, 0, len(candidates))
	used := map[string]struct{}{}
	seenFamilies := map[string]struct{}{}
	add := func(candidate DiscoverCandidate, slot int) {
		if _, ok := used[candidate.URL]; ok {
			return
		}
		candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "semantic_family_probe_diversity")
		candidate.Metadata = discoverycore.MergeMetadata(candidate.Metadata, map[string]string{
			"semantic_probe_slot": strconv.Itoa(slot),
		})
		used[candidate.URL] = struct{}{}
		out = append(out, candidate)
	}
	if len(candidates) > 0 {
		add(candidates[0], 1)
		if family, ok := webdiscover.CandidateFamily(candidates[0].URL); ok {
			seenFamilies[family] = struct{}{}
		}
	}
	for _, candidate := range candidates[1:] {
		family, ok := webdiscover.CandidateFamily(candidate.URL)
		if ok {
			if _, seen := seenFamilies[family]; seen {
				continue
			}
			seenFamilies[family] = struct{}{}
		}
		add(candidate, len(out)+1)
		if len(out) >= probeCount {
			break
		}
	}
	for _, candidate := range candidates {
		add(candidate, len(out)+1)
	}
	return out
}

func (s *Service) probeWebCandidate(ctx context.Context, goal, userAgent string, domainHints []string, candidate DiscoverCandidate, allowBrowserProbe bool) ([]DiscoverCandidate, bool, error) {
	rawPage, err := s.acquirer.Acquire(ctx, fetchpolicy.Input(s.cfg, candidate.URL, pipeline.EffectiveUserAgent(userAgent, true), "", "", ""))
	if err != nil {
		return nil, false, err
	}

	dom, err := s.reducer.Reduce(rawPage)
	if err != nil {
		return nil, false, err
	}
	webIR := webirbuilder.Build(dom)

	out := s.probeWebCandidatePage(ctx, goal, userAgent, domainHints, candidate, rawPage, dom, webIR, nil, true)
	if !s.shouldRenderWebCandidateProbe(ctx, rawPage, webIR) {
		return out, false, nil
	}
	if !allowBrowserProbe {
		if len(out) > 0 {
			out[0].Reason = discoverycore.AppendUniqueReason(out[0].Reason, "browser_probe_budget_exhausted")
			out[0].Metadata = discoverycore.MergeMetadata(out[0].Metadata, map[string]string{
				"browser_probe": "budget_exhausted",
			})
		}
		return out, false, nil
	}
	renderedPage, renderedDOM, renderedIR, renderMetadata, renderErr := s.renderWebCandidateProbe(ctx, rawPage, userAgent, core.WebIRUtilityReasons(webIR))
	if renderErr != nil {
		if len(out) > 0 {
			out[0].Reason = discoverycore.AppendUniqueReason(out[0].Reason, "browser_probe", "browser_render_failed")
			out[0].Metadata = discoverycore.MergeMetadata(out[0].Metadata, map[string]string{
				"browser_probe":    "attempted",
				"browser_rendered": "false",
				"browser_error":    discoverycore.CompactSemanticText(renderErr.Error(), 240),
			})
		}
		return out, true, nil
	}
	out = append(out, s.probeWebCandidatePage(ctx, goal, userAgent, domainHints, candidate, renderedPage, renderedDOM, renderedIR, renderMetadata, false)...)
	return discoverycore.NewSet(out).Sorted(), true, nil
}
