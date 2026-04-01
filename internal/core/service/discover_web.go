package service

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/pipeline"
)

type DiscoverWebRequest struct {
	Goal          string
	Queries       []string
	SeedURL       string
	UserAgent     string
	MaxCandidates int
	DomainHints   []string
}

type DiscoverWebResponse struct {
	SeedURL      string              `json:"seed_url"`
	Provider     string              `json:"provider"`
	SelectedURL  string              `json:"selected_url"`
	DiscoveryURL string              `json:"discovery_url"`
	Candidates   []DiscoverCandidate `json:"candidates"`
}

const webProbeLimit = 3

func (s *Service) DiscoverWeb(ctx context.Context, req DiscoverWebRequest) (DiscoverWebResponse, error) {
	if strings.TrimSpace(req.Goal) == "" {
		return DiscoverWebResponse{}, fmt.Errorf("discover web request goal must not be empty")
	}
	if req.MaxCandidates <= 0 {
		req.MaxCandidates = 5
	}
	if strings.TrimSpace(req.SeedURL) != "" {
		if native, ok := s.discoverWebLocalFirst(ctx, req); ok {
			return native, nil
		}
	}

	providers := discoverycore.WebSearchProviders(s.webDiscoverBaseURL)
	var (
		discoveryURL  string
		providerNames []string
		lastErr       error
	)
	candidates := discoverycore.NewSet(nil)
	queries := req.Queries
	if len(queries) == 0 {
		queries = []string{req.Goal}
	}
	for _, providerBaseURL := range providers {
		providerUsed := false
		for _, query := range queries {
			bootstrapped, bootURL, err := s.discoverWebBootstrap(ctx, providerBaseURL, req, query)
			if err != nil {
				lastErr = err
				continue
			}
			if discoveryURL == "" {
				discoveryURL = bootURL
			}
			candidates.Merge(bootstrapped)
			providerUsed = true
		}
		if providerUsed {
			providerNames = append(providerNames, discoverycore.ProviderName(providerBaseURL))
		}
	}
	if len(candidates.Sorted()) == 0 && len(queries) > 1 {
		for _, providerBaseURL := range providers {
			bootstrapped, bootURL, err := s.discoverWebBootstrap(ctx, providerBaseURL, req, req.Goal)
			if err != nil {
				lastErr = err
				continue
			}
			if discoveryURL == "" {
				discoveryURL = bootURL
			}
			candidates.Merge(bootstrapped)
			providerName := discoverycore.ProviderName(providerBaseURL)
			if !slices.Contains(providerNames, providerName) {
				providerNames = append(providerNames, providerName)
			}
		}
	}
	if len(candidates.Sorted()) == 0 {
		if lastErr != nil {
			return DiscoverWebResponse{}, lastErr
		}
		return DiscoverWebResponse{}, fmt.Errorf("discover web returned no candidates")
	}
	expanded := s.expandAndRerankWebCandidates(ctx, req.Goal, req.UserAgent, req.DomainHints, candidates.Sorted(), req.MaxCandidates)
	filtered := discoverycore.NewSet(s.semanticRerankDiscoverCandidates(ctx, req.Goal, expanded)).Limited(req.MaxCandidates)
	if len(filtered) == 0 {
		return DiscoverWebResponse{}, fmt.Errorf("discover web returned no candidates")
	}

	return DiscoverWebResponse{
		SeedURL:      req.SeedURL,
		Provider:     strings.Join(providerNames, ","),
		SelectedURL:  filtered[0].URL,
		DiscoveryURL: discoveryURL,
		Candidates:   filtered,
	}, nil
}

func (s *Service) discoverWebLocalFirst(ctx context.Context, req DiscoverWebRequest) (DiscoverWebResponse, bool) {
	discovery, err := s.Discover(ctx, DiscoverRequest{Goal: req.Goal, SeedURL: req.SeedURL, UserAgent: req.UserAgent, SameDomain: true, MaxCandidates: req.MaxCandidates, DomainHints: req.DomainHints})
	if err != nil || len(discovery.Candidates) == 0 {
		return DiscoverWebResponse{}, false
	}
	top := discovery.Candidates[0]
	if top.URL == discovery.SeedURL || !localSubstrateResolved(top) {
		return DiscoverWebResponse{}, false
	}
	top.Reason = discoverycore.AppendUniqueReason(top.Reason, "native_substrate")
	return DiscoverWebResponse{SeedURL: req.SeedURL, Provider: "local_same_site", SelectedURL: top.URL, DiscoveryURL: discovery.DiscoveryURL, Candidates: []DiscoverCandidate{top}}, true
}

func localSubstrateResolved(candidate DiscoverCandidate) bool {
	if candidate.Score >= 1.8 {
		return true
	}
	return slices.Contains(candidate.Reason, "semantic_goal_alignment") || slices.Contains(candidate.Reason, "path_hint")
}

func (s *Service) discoverWebBootstrap(ctx context.Context, baseURL string, req DiscoverWebRequest, query string) ([]DiscoverCandidate, string, error) {
	searchURL, err := discoverycore.WebSearchURL(baseURL, query)
	if err != nil {
		return nil, "", err
	}

	rawPage, err := s.acquirer.Acquire(ctx, pipeline.AcquireInput{
		URL:       searchURL,
		Timeout:   time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond,
		MaxBytes:  s.cfg.Runtime.MaxBytes,
		UserAgent: effectiveUserAgent(req.UserAgent, true),
	})
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
	return discoverycore.ScoreCandidates(req.Goal, req.SeedURL, "", results, req.DomainHints), rawPage.FinalURL, nil
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
	for _, candidate := range candidates[:probeCount] {
		probed, err := s.probeWebCandidate(ctx, goal, userAgent, domainHints, candidate)
		if err != nil {
			continue
		}
		merged.Merge(probed)
	}
	return merged.Sorted()
}

func (s *Service) probeWebCandidate(ctx context.Context, goal, userAgent string, domainHints []string, candidate DiscoverCandidate) ([]DiscoverCandidate, error) {
	rawPage, err := s.acquirer.Acquire(ctx, pipeline.AcquireInput{
		URL:       candidate.URL,
		Timeout:   time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond,
		MaxBytes:  s.cfg.Runtime.MaxBytes,
		UserAgent: effectiveUserAgent(userAgent, true),
	})
	if err != nil {
		return nil, err
	}

	dom, err := s.reducer.Reduce(rawPage)
	if err != nil {
		return nil, err
	}
	webIR := buildWebIR(dom)

	refined := refineWebCandidate(goal, candidate, rawPage.FinalURL, dom.Title, webIR, domainHints)
	out := []DiscoverCandidate{refined}

	expanded := extractLinkCandidates(rawPage.HTML, rawPage.FinalURL, true)
	expandedScored := discoverycore.ScoreCandidates(goal, "", "", expanded, domainHints)
	if len(expandedScored) > 0 {
		best := expandedScored[0]
		best.Score += 0.40
		best.Reason = discoverycore.AppendUniqueReason(best.Reason, "page_expand")
		out = append(out, best)
	}
	return out, nil
}

func refineWebCandidate(goal string, candidate DiscoverCandidate, finalURL, pageTitle string, webIR core.WebIR, domainHints []string) DiscoverCandidate {
	score, reasons := discoverycore.ScoreURL(goal, finalURL, discoverycore.JoinNonEmpty(pageTitle, candidate.Label), false, domainHints)
	if strings.TrimSpace(pageTitle) != "" {
		score += 0.35
		reasons = append(reasons, "page_title_probe")
	}
	if webIR.NodeCount > 0 {
		score += 0.10
		reasons = append(reasons, "web_ir_probe")
	}
	if webIR.Signals.EmbeddedNodeCount > 0 {
		score += 0.12
		reasons = append(reasons, "web_ir_embedded")
	}
	if strings.TrimSpace(finalURL) != "" && finalURL != candidate.URL {
		reasons = append(reasons, "redirect_resolved")
	}
	return DiscoverCandidate{
		URL:      finalURL,
		Label:    discoverycore.FirstNonEmpty(pageTitle, candidate.Label),
		Score:    max(score, candidate.Score),
		Reason:   discoverycore.AppendUniqueReason(append([]string{}, candidate.Reason...), reasons...),
		Metadata: discoverycore.MergeMetadata(candidate.Metadata, webIRDiscoveryMetadata(webIR)),
	}
}

func webIRDiscoveryMetadata(webIR core.WebIR) map[string]string {
	if webIR.NodeCount <= 0 {
		return nil
	}
	return map[string]string{
		"web_ir_node_count":          strconv.Itoa(webIR.NodeCount),
		"web_ir_embedded_node_count": strconv.Itoa(webIR.Signals.EmbeddedNodeCount),
		"web_ir_heading_ratio":       strconv.FormatFloat(webIR.Signals.HeadingRatio, 'f', 3, 64),
		"web_ir_short_text_ratio":    strconv.FormatFloat(webIR.Signals.ShortTextRatio, 'f', 3, 64),
	}
}
