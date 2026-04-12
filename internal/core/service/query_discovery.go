package service

import (
	"context"
	"net/url"
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
		if discoveryMode == QueryDiscoveryWeb {
			if recovered, ok := s.tryMemoryFamilyRecovery(ctx, req); ok && shouldPromoteRecoveredMemoryFamily(result.selected, recovered) {
				result.provider = "discovery_memory_same_site"
				result.selected = recovered.SelectedURL
				result.candidates = recovered.Candidates
				return finalizeQueryDiscoveryResult(result, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader), nil
			}
			if !queryflow.ShouldEscalateRewrite(result.selected, result.candidates) {
				return finalizeQueryDiscoveryResult(result, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader), nil
			}
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
		discovery, err := s.discoverQueryWeb(ctx, req, maxCandidates, &result)
		if err != nil {
			return queryDiscoveryResult{}, err
		}
		if discovery.finalized {
			return finalizeQueryDiscoveryResult(result, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader), nil
		}
	}
	return finalizeQueryDiscoveryResult(result, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader), nil
}

func preferRecoveredMemoryFamily(currentURL, recoveredURL string) bool {
	currentHost := hostFromURLString(currentURL)
	recoveredHost := hostFromURLString(recoveredURL)
	if currentHost == "" || recoveredHost == "" || currentHost != recoveredHost {
		return false
	}
	return urlPathDepth(recoveredURL) > urlPathDepth(currentURL)
}

func shouldPromoteRecoveredMemoryFamily(currentURL string, recovered DiscoverResponse) bool {
	if preferRecoveredMemoryFamily(currentURL, recovered.SelectedURL) {
		return true
	}
	if !sameNormalizedURL(currentURL, recovered.SelectedURL) {
		return false
	}
	return len(recovered.Candidates) > 1
}

func (s *Service) tryMemoryFamilyRecovery(ctx context.Context, req QueryRequest) (DiscoverResponse, bool) {
	if strings.TrimSpace(req.SeedURL) != "" || len(req.MemoryCandidates) == 0 {
		return DiscoverResponse{}, false
	}
	seed := selectMemoryFamilySeed(req.MemoryCandidates)
	if strings.TrimSpace(seed.URL) == "" {
		return DiscoverResponse{}, false
	}
	discovery, err := s.Discover(ctx, DiscoverRequest{
		Goal:          req.Goal,
		SeedURL:       seed.URL,
		UserAgent:     req.UserAgent,
		SameDomain:    true,
		MaxCandidates: min(5, s.cfg.Runtime.MaxPages),
		DomainHints:   mergeDomainHints(req.DomainHints, hostFromURLString(seed.URL)),
	})
	if err != nil || strings.TrimSpace(discovery.SelectedURL) == "" || len(discovery.Candidates) == 0 {
		return DiscoverResponse{}, false
	}
	discovery.SelectedURL = preferredRecoveredMemoryURL(seed.URL, discovery)
	if discovery.SelectedURL != "" {
		discovery.Candidates = ensureRecoveredCandidatePresent(discovery.Candidates, discovery.SelectedURL, seed)
		discovery.Candidates = promoteRecoveredCandidate(discovery.Candidates, discovery.SelectedURL)
	}
	return discovery, true
}

func selectMemoryFamilySeed(candidates []DiscoverCandidate) DiscoverCandidate {
	if len(candidates) == 0 {
		return DiscoverCandidate{}
	}
	type familyScore struct {
		URL       string
		Host      string
		Score     float64
		BestScore float64
	}
	byHost := map[string]familyScore{}
	for _, candidate := range candidates {
		host := hostFromURLString(candidate.URL)
		if host == "" {
			continue
		}
		current := byHost[host]
		current.Host = host
		current.Score += candidate.Score
		if current.URL == "" || candidate.Score > current.BestScore {
			current.URL = candidate.URL
			current.BestScore = candidate.Score
		}
		byHost[host] = current
	}
	best := DiscoverCandidate{}
	bestScore := -1.0
	for _, candidate := range candidates {
		host := hostFromURLString(candidate.URL)
		score := candidate.Score
		if family, ok := byHost[host]; ok {
			score += family.Score * 0.35
		}
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}
	return best
}

func urlPathDepth(raw string) int {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	path := strings.Trim(parsed.EscapedPath(), "/")
	if path == "" {
		return 0
	}
	return len(strings.Split(path, "/"))
}

func preferredRecoveredMemoryURL(seedURL string, discovery DiscoverResponse) string {
	best := strings.TrimSpace(seedURL)
	seedHost := hostFromURLString(seedURL)
	if seedHost == "" {
		return strings.TrimSpace(discovery.SelectedURL)
	}
	if urlPathDepth(seedURL) == 0 {
		best = strings.TrimSpace(discovery.SelectedURL)
		bestDepth := urlPathDepth(best)
		for _, candidate := range discovery.Candidates {
			host := hostFromURLString(candidate.URL)
			if host == "" || host != seedHost {
				continue
			}
			depth := urlPathDepth(candidate.URL)
			if depth > bestDepth {
				best = candidate.URL
				bestDepth = depth
			}
		}
		return best
	}
	for _, candidate := range discovery.Candidates {
		if sameNormalizedURL(seedURL, candidate.URL) {
			return candidate.URL
		}
	}
	return best
}

func sameNormalizedURL(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	pa, errA := url.Parse(strings.TrimSpace(a))
	pb, errB := url.Parse(strings.TrimSpace(b))
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(pa.Hostname(), pb.Hostname()) &&
		strings.Trim(pa.EscapedPath(), "/") == strings.Trim(pb.EscapedPath(), "/")
}

func promoteRecoveredCandidate(candidates []DiscoverCandidate, selectedURL string) []DiscoverCandidate {
	out := append([]DiscoverCandidate{}, candidates...)
	topScore := 0.0
	for _, candidate := range out {
		if candidate.Score > topScore {
			topScore = candidate.Score
		}
	}
	for i := range out {
		if strings.TrimSpace(out[i].URL) != strings.TrimSpace(selectedURL) {
			continue
		}
		out[i].Score = maxRecoveredFloat(out[i].Score+0.6, topScore+0.12)
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "memory_family_specific_page")
		if i == 0 {
			return out
		}
		selected := out[i]
		copy(out[1:i+1], out[0:i])
		out[0] = selected
		return out
	}
	return out
}

func ensureRecoveredCandidatePresent(candidates []DiscoverCandidate, selectedURL string, seed DiscoverCandidate) []DiscoverCandidate {
	for _, candidate := range candidates {
		if sameNormalizedURL(candidate.URL, selectedURL) {
			return candidates
		}
	}
	inserted := DiscoverCandidate{
		URL:      selectedURL,
		Label:    discoverycore.FirstNonEmpty(seed.Label, seed.URL),
		Score:    firstNonZero(seed.Score, 1.05),
		Reason:   discoverycore.AppendUniqueReason(append([]string{}, seed.Reason...), "memory_family_seed_preserved"),
		Metadata: cloneStringMap(seed.Metadata),
	}
	return append([]DiscoverCandidate{inserted}, candidates...)
}

func firstNonZero(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func maxRecoveredFloat(values ...float64) float64 {
	best := 0.0
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}

type queryWebDiscoveryResult struct {
	finalized bool
}

func (s *Service) discoverQueryWeb(ctx context.Context, req QueryRequest, maxCandidates int, result *queryDiscoveryResult) (queryWebDiscoveryResult, error) {
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
			return queryWebDiscoveryResult{finalized: true}, nil
		}
	}
	if ok := s.applyQueryDiscoveryRewrite(ctx, req, maxCandidates, result); ok {
		return queryWebDiscoveryResult{finalized: true}, nil
	}
	if err != nil {
		return queryWebDiscoveryResult{}, err
	}
	return queryWebDiscoveryResult{}, nil
}

func (s *Service) applyQueryDiscoveryRewrite(ctx context.Context, req QueryRequest, maxCandidates int, result *queryDiscoveryResult) bool {
	queries, rewrite, ok := s.maybeRewriteSearchQueries(ctx, req, QueryDiscoveryWeb)
	if !ok {
		return false
	}
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
	if rewriteErr != nil {
		return false
	}
	result.provider, result.selected, result.candidates = rewriteDiscovery.Provider, rewriteDiscovery.SelectedURL, rewriteDiscovery.Candidates
	result.rewriteApplied = true
	result.rewriteQueries = append([]string{}, queries...)
	result.rewrite = rewrite
	return true
}

func finalizeQueryDiscoveryResult(result queryDiscoveryResult, seedURL string, seedEvidence QueryFingerprintEvidence, loader func(string) (QueryFingerprintEvidence, bool)) queryDiscoveryResult {
	result.candidates, result.selected = queryflow.FinalizeDiscoveryResult(result.candidates, result.selected, seedURL, seedEvidence, loader)
	return result
}
