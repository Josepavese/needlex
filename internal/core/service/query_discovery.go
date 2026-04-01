package service

import (
	"context"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/queryflow"
)

func (s *Service) runQueryDiscovery(ctx context.Context, req QueryRequest, discoveryMode string, seedEvidence QueryFingerprintEvidence) (queryDiscoveryResult, error) {
	result := queryDiscoveryResult{selected: req.SeedURL}
	if req.SeedURL != "" {
		result.candidates = []DiscoverCandidate{{URL: req.SeedURL, Score: 0.35, Reason: []string{"seed_fallback"}}}
	}
	if strings.TrimSpace(req.SeedURL) == "" && len(req.MemoryCandidates) > 0 {
		result.provider = "discovery_memory"
		result.candidates = append([]DiscoverCandidate{}, req.MemoryCandidates...)
		result.selected = req.MemoryCandidates[0].URL
		if discoveryMode == QueryDiscoveryWeb && !queryflow.ShouldEscalateRewrite(result.selected, result.candidates) {
			return finalizeQueryDiscoveryResult(result, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader), nil
		}
	}
	if discoveryMode == QueryDiscoveryOff {
		return finalizeQueryDiscoveryResult(result, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader), nil
	}

	maxCandidates := min(5, s.cfg.Runtime.MaxPages)
	switch discoveryMode {
	case QueryDiscoverySameSite:
		discovery, err := s.Discover(ctx, DiscoverRequest{
			Goal:          req.Goal,
			SeedURL:       req.SeedURL,
			UserAgent:     req.UserAgent,
			SameDomain:    true,
			MaxCandidates: maxCandidates,
			DomainHints:   req.DomainHints,
		})
		if err != nil {
			return queryDiscoveryResult{}, err
		}
		result.provider, result.selected, result.candidates = "local_same_site", discovery.SelectedURL, discovery.Candidates
	case QueryDiscoveryWeb:
		discovery, err := s.DiscoverWeb(ctx, DiscoverWebRequest{
			Goal:          req.Goal,
			SeedURL:       req.SeedURL,
			UserAgent:     req.UserAgent,
			MaxCandidates: maxCandidates,
			DomainHints:   req.DomainHints,
		})
		if err == nil {
			result.provider, result.selected, result.candidates = discovery.Provider, discovery.SelectedURL, discovery.Candidates
			if !queryflow.ShouldEscalateRewrite(discovery.SelectedURL, discovery.Candidates) {
				return finalizeQueryDiscoveryResult(result, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader), nil
			}
		}
		queries, rewrite, ok := s.maybeRewriteSearchQueries(ctx, req, discoveryMode)
		if ok {
			rewriteReq := req.withQueries(queries)
			if len(rewrite.LocalityHints) > 0 {
				rewriteReq.DomainHints = discoverycore.NormalizeDomainHints(append(rewriteReq.DomainHints, rewrite.LocalityHints...))
			}
			rewriteDiscovery, rewriteErr := s.DiscoverWeb(ctx, DiscoverWebRequest{
				Goal:          rewriteReq.Goal,
				Queries:       append([]string{}, rewriteReq.SearchQueries...),
				SeedURL:       rewriteReq.SeedURL,
				UserAgent:     rewriteReq.UserAgent,
				MaxCandidates: maxCandidates,
				DomainHints:   rewriteReq.DomainHints,
			})
			if rewriteErr == nil {
				result.provider, result.selected, result.candidates = rewriteDiscovery.Provider, rewriteDiscovery.SelectedURL, rewriteDiscovery.Candidates
				result.rewriteApplied = true
				result.rewriteQueries = append([]string{}, queries...)
				result.rewrite = rewrite
				return finalizeQueryDiscoveryResult(result, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader), nil
			}
		}
		if err != nil {
			return queryDiscoveryResult{}, err
		}
	}
	return finalizeQueryDiscoveryResult(result, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader), nil
}

func finalizeQueryDiscoveryResult(result queryDiscoveryResult, seedURL string, seedEvidence QueryFingerprintEvidence, loader func(string) (QueryFingerprintEvidence, bool)) queryDiscoveryResult {
	result.candidates, result.selected = queryflow.FinalizeDiscoveryResult(result.candidates, result.selected, seedURL, seedEvidence, loader)
	return result
}
