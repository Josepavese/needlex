package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/providerfusion"
	"github.com/josepavese/needlex/internal/core/semanticcalibrate"
	"github.com/josepavese/needlex/internal/core/semanticevidence"
	"github.com/josepavese/needlex/internal/core/semanticrank"
	"github.com/josepavese/needlex/internal/core/targetkind"
	"github.com/josepavese/needlex/internal/core/webdiscover"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/store"
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

const (
	webCandidateLimit             = 8
	webProbeLimit                 = 6
	webBrowserProbeLimit          = 1
	webBrowserProbeTimeout        = 6 * time.Second
	webBrowserProbeMinRemaining   = 8 * time.Second
	webBrowserProbeNetworkIdle    = 800 * time.Millisecond
	webBrowserProbeNetworkBytes   = int64(8_000_000)
	webBrowserProbeResourceBytes  = int64(2_000_000)
	webBrowserProbeResources      = 8
	webBrowserProbeMessages       = 256
	hostRootSemanticMinSimilarity = 0.24
	hostRootOriginMinSimilarity   = 0.16
	hostRootDerivativeMargin      = 0.015
)

func (s *Service) DiscoverWeb(ctx context.Context, req DiscoverWebRequest) (DiscoverWebResponse, error) {
	var err error
	req, err = normalizeDiscoverWebRequest(req)
	if err != nil {
		return DiscoverWebResponse{}, err
	}
	if strings.TrimSpace(req.SeedURL) != "" {
		if native, ok := s.discoverWebLocalFirst(ctx, req); ok {
			return native, nil
		}
	}

	bootstrap := s.collectWebBootstrapCandidates(ctx, req)
	if len(bootstrap.Candidates.Sorted()) == 0 {
		return DiscoverWebResponse{}, bootstrap.NoCandidateError()
	}
	filtered := s.finalizeWebCandidates(ctx, req, bootstrap.Candidates.Sorted())
	if len(filtered) == 0 {
		return DiscoverWebResponse{}, fmt.Errorf("discover web returned no candidates")
	}

	return DiscoverWebResponse{
		SeedURL:      req.SeedURL,
		Provider:     strings.Join(bootstrap.ProviderNames, ","),
		SelectedURL:  filtered[0].URL,
		DiscoveryURL: bootstrap.DiscoveryURL,
		Candidates:   filtered,
	}, nil
}

type webBootstrapCollection struct {
	Candidates    discoverycore.Set
	DiscoveryURL  string
	ProviderNames []string
	LastErr       error
}

func normalizeDiscoverWebRequest(req DiscoverWebRequest) (DiscoverWebRequest, error) {
	if strings.TrimSpace(req.Goal) == "" {
		return DiscoverWebRequest{}, fmt.Errorf("discover web request goal must not be empty")
	}
	if req.MaxCandidates <= 0 {
		req.MaxCandidates = webCandidateLimit
	}
	return req, nil
}

func (s *Service) collectWebBootstrapCandidates(ctx context.Context, req DiscoverWebRequest) webBootstrapCollection {
	providers := s.discoveryProviderAdapters(discoverycore.WebSearchProviders(s.webDiscoverBaseURL))
	return s.collectWebBootstrapCandidatesFromProviders(ctx, req, providers)
}

func (s *Service) collectWebBootstrapCandidatesFromProviders(ctx context.Context, req DiscoverWebRequest, providers []discoveryProviderAdapter) webBootstrapCollection {
	queries := semanticBootstrapQueryPortfolio(req)
	collection := webBootstrapCollection{Candidates: discoverycore.NewSet(nil)}
	if len(providers) == 0 || len(queries) == 0 {
		return collection
	}

	disabled := make(map[string]struct{}, len(providers))
	for queryIndex, query := range queries {
		for _, provider := range providers {
			providerName := provider.Name()
			if _, ok := disabled[providerName]; ok {
				continue
			}
			result := s.bootstrapProviderQuery(ctx, provider, req, query)
			collection.MergeProviderResult(result)
			if result.LastErr != nil && webdiscover.ProviderLevelFailure(result.LastErr) {
				disabled[providerName] = struct{}{}
			}
		}
		if semanticBootstrapCollectionSufficient(collection, queryIndex+1, len(queries), len(providers)) {
			break
		}
	}
	return collection
}

func (s *Service) bootstrapProviderQuery(ctx context.Context, provider discoveryProviderAdapter, req DiscoverWebRequest, query string) webBootstrapCollection {
	out := webBootstrapCollection{Candidates: discoverycore.NewSet(nil)}
	providerName := provider.Name()
	bootstrapped, bootURL, err := provider.Bootstrap(ctx, req, query)
	if err != nil {
		s.observeDiscoveryProvider(providerName, webdiscover.ProviderOutcome(err))
		out.LastErr = err
		return out
	}
	s.observeDiscoveryProvider(providerName, store.DiscoveryProviderOutcomeSuccess)
	out.DiscoveryURL = bootURL
	out.Candidates.Merge(providerfusion.AnnotateProvider(bootstrapped, providerName))
	out.ProviderNames = append(out.ProviderNames, providerName)
	return out
}

func semanticBootstrapQueryPortfolio(req DiscoverWebRequest) []string {
	queries := make([]string, 0, len(req.Queries)+1)
	add := func(query string) {
		query = strings.TrimSpace(query)
		if query == "" || slices.Contains(queries, query) {
			return
		}
		queries = append(queries, query)
	}
	if len(req.Queries) == 0 {
		add(req.Goal)
		return queries
	}
	for _, query := range req.Queries {
		add(query)
	}
	add(req.Goal)
	return queries
}

func semanticBootstrapCollectionSufficient(collection webBootstrapCollection, roundsCompleted, totalQueries, providerCount int) bool {
	candidates := collection.Candidates.Sorted()
	if len(candidates) < max(webProbeLimit, webCandidateLimit) {
		return false
	}
	if providerCount > 1 && len(collection.ProviderNames) < 2 {
		return false
	}
	minRounds := 1
	if totalQueries > 1 {
		minRounds = 2
	}
	if roundsCompleted < minRounds {
		return false
	}
	families := semanticBootstrapFamilyCount(candidates)
	return families >= min(5, len(candidates))
}

func semanticBootstrapFamilyCount(candidates []DiscoverCandidate) int {
	families := map[string]struct{}{}
	for _, candidate := range candidates {
		if family, ok := webdiscover.CandidateFamily(candidate.URL); ok && strings.TrimSpace(family) != "" {
			families[family] = struct{}{}
		}
	}
	return len(families)
}

func (c *webBootstrapCollection) MergeProviderResult(in webBootstrapCollection) {
	if in.LastErr != nil {
		c.LastErr = in.LastErr
	}
	if c.DiscoveryURL == "" {
		c.DiscoveryURL = in.DiscoveryURL
	}
	c.Candidates.Merge(in.Candidates.Sorted())
	for _, providerName := range in.ProviderNames {
		if !slices.Contains(c.ProviderNames, providerName) {
			c.ProviderNames = append(c.ProviderNames, providerName)
		}
	}
}

func (c webBootstrapCollection) NoCandidateError() error {
	if c.LastErr != nil {
		return c.LastErr
	}
	return fmt.Errorf("discover web returned no candidates")
}

func (s *Service) finalizeWebCandidates(ctx context.Context, req DiscoverWebRequest, candidates []DiscoverCandidate) []DiscoverCandidate {
	bootstrapped := s.semanticRerankDiscoverCandidates(ctx, req.Goal, candidates)
	expanded := s.expandAndRerankWebCandidates(ctx, req.Goal, req.UserAgent, req.DomainHints, bootstrapped, req.MaxCandidates)
	expanded = s.applySemanticEvidenceProbe(ctx, req.Goal, expanded)
	filtered := discoverycore.NewSet(s.semanticRerankDiscoverCandidates(ctx, req.Goal, expanded)).Sorted()
	filtered = webdiscover.CanonicalizeCandidateFamilies(filtered)
	filtered = webdiscover.DampenWeakProvenanceTraps(filtered)
	filtered = webdiscover.DampenCrossFamilyMirrorRoutes(filtered)
	filtered = webdiscover.PromoteRecoveredCanonicalOrigins(filtered)
	filtered = s.semanticDisambiguateCandidateFamilies(ctx, req.Goal, filtered)
	filtered = s.applyCandidateIntelligence(ctx, req.Goal, filtered)
	filtered = s.applySemanticSelectionStack(ctx, req.Goal, filtered)
	filtered = targetkind.Apply(ctx, s.semantic, req.Goal, filtered)
	filtered = s.applySemanticProvenanceBalance(ctx, req.Goal, filtered)
	filtered = s.applySemanticFamilyIntentRecovery(ctx, req.Goal, filtered)
	filtered = applySemanticFamilyEvidenceBalance(filtered)
	filtered = applySemanticNearTieProvenanceReview(filtered)
	filtered = webdiscover.DampenCrossFamilyMirrorRoutes(filtered)
	filtered = webdiscover.DampenWeakProvenanceTraps(filtered)
	filtered = discoverycore.NewSet(filtered).Limited(req.MaxCandidates)
	return s.maybePromoteEndpointCandidate(ctx, req.Goal, req.UserAgent, req.DomainHints, filtered)
}

func (s *Service) applySemanticEvidenceProbe(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	semantic := intel.NewSemanticAligner(s.cfg, s.httpClient)
	return semanticevidence.Reranker{Semantic: semantic, Config: semanticevidence.DefaultConfig()}.Rerank(ctx, goal, candidates)
}

func (s *Service) applySemanticSelectionStack(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	if len(candidates) < 2 || strings.TrimSpace(goal) == "" {
		return candidates
	}
	out := providerfusion.Apply(candidates)
	out = semanticrank.Reranker{Semantic: s.semantic, Config: semanticrank.DefaultConfig()}.Rerank(ctx, goal, out)
	return semanticcalibrate.Apply(out, semanticcalibrate.DefaultModel())
}
