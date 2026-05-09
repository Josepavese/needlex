package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/providerfusion"
	"github.com/josepavese/needlex/internal/core/semanticcalibrate"
	"github.com/josepavese/needlex/internal/core/semanticevidence"
	"github.com/josepavese/needlex/internal/core/semanticrank"
	"github.com/josepavese/needlex/internal/core/webdiscover"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
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
	webCandidateLimit = 8
	webProbeLimit     = 6
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
	queries := req.Queries
	if len(queries) == 0 {
		queries = []string{req.Goal}
	}
	collection := webBootstrapCollection{Candidates: discoverycore.NewSet(nil)}
	for _, provider := range providers {
		collection.MergeProviderResult(s.bootstrapProviderQueries(ctx, provider, req, queries, true))
	}
	if len(collection.Candidates.Sorted()) == 0 && len(queries) > 1 {
		for _, provider := range providers {
			collection.MergeProviderResult(s.bootstrapProviderQueries(ctx, provider, req, []string{req.Goal}, false))
		}
	}
	return collection
}

func (s *Service) bootstrapProviderQueries(ctx context.Context, provider discoveryProviderAdapter, req DiscoverWebRequest, queries []string, stopOnProviderFailure bool) webBootstrapCollection {
	out := webBootstrapCollection{Candidates: discoverycore.NewSet(nil)}
	providerName := provider.Name()
	for _, query := range queries {
		bootstrapped, bootURL, err := provider.Bootstrap(ctx, req, query)
		if err != nil {
			s.observeDiscoveryProvider(providerName, webdiscover.ProviderOutcome(err))
			out.LastErr = err
			if webdiscover.IsProviderUnavailable(err) || (stopOnProviderFailure && webdiscover.ProviderLevelFailure(err)) {
				break
			}
			continue
		}
		s.observeDiscoveryProvider(providerName, store.DiscoveryProviderOutcomeSuccess)
		out.DiscoveryURL = bootURL
		out.Candidates.Merge(providerfusion.AnnotateProvider(bootstrapped, providerName))
		out.ProviderNames = append(out.ProviderNames, providerName)
	}
	return out
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
	filtered = s.applyTargetKindRerank(ctx, req.Goal, filtered)
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

func (s *Service) semanticDisambiguateCandidateFamilies(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	if len(candidates) < 2 || strings.TrimSpace(goal) == "" {
		return candidates
	}
	families := make(map[string][]DiscoverCandidate)
	order := make([]string, 0)
	for _, candidate := range candidates {
		family, ok := webdiscover.CandidateFamily(candidate.URL)
		if !ok {
			family = strings.TrimSpace(candidate.URL)
		}
		if _, ok := families[family]; !ok {
			order = append(order, family)
		}
		families[family] = append(families[family], candidate)
	}
	if len(order) < 2 {
		return candidates
	}
	top := candidates[0]
	second := candidates[1]
	if top.Score-second.Score > 0.25 {
		return candidates
	}

	semanticCandidates := make([]intel.SemanticCandidate, 0, len(order))
	for _, family := range order {
		group := families[family]
		var texts []string
		limit := min(len(group), 3)
		for i := 0; i < limit; i++ {
			texts = append(texts, discoverycore.JoinNonEmpty(
				group[i].Metadata["host_root_title"],
				group[i].Metadata["host_root_context"],
				group[i].Metadata["page_title"],
				group[i].Metadata["web_ir_context"],
				group[i].Label,
			))
		}
		semanticCandidates = append(semanticCandidates, intel.SemanticCandidate{
			ID:   family,
			Text: discoverycore.JoinNonEmpty(texts...),
		})
	}
	scored, err := s.semantic.Score(ctx, goal, semanticCandidates)
	if err != nil || len(scored) == 0 {
		return candidates
	}
	byFamily := make(map[string]float64, len(scored))
	for _, item := range scored {
		byFamily[item.ID] = item.Similarity
	}
	out := append([]DiscoverCandidate{}, candidates...)
	for i := range out {
		family, ok := webdiscover.CandidateFamily(out[i].URL)
		if !ok {
			family = strings.TrimSpace(out[i].URL)
		}
		if similarity, ok := byFamily[family]; ok && similarity > 0 {
			out[i].Score += similarity * 0.90
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "semantic_family_alignment")
		}
	}
	discoverycore.SortCandidates(out)
	return out
}

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
	if weakCanonicalHomeProfile(s.inferTargetKindProfile(ctx, req.Goal)) {
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
	return s.fetchAcquireInputWithProfiles(searchURL, effectiveUserAgent(userAgent, true), "standard", "")
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
	rawPage, err := s.acquirer.Acquire(ctx, s.fetchAcquireInput(candidate.URL, effectiveUserAgent(userAgent, true)))
	if err != nil {
		return nil, err
	}

	dom, err := s.reducer.Reduce(rawPage)
	if err != nil {
		return nil, err
	}
	webIR := buildWebIR(dom)

	refined := webdiscover.RefineCandidate(goal, candidate, rawPage.FinalURL, dom.Title, webIR, domainHints)
	out := []DiscoverCandidate{refined}
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

	identityRefs := webdiscover.ExtractIdentityReferenceCandidates(rawPage.HTML, rawPage.FinalURL, discoverycore.FirstNonEmpty(dom.Title, candidate.Label))
	if len(identityRefs) > 0 {
		out = append(out, s.selectIdentityReferenceCandidates(ctx, goal, refined, identityRefs)...)
	}
	expanded := extractLinkCandidates(rawPage.HTML, rawPage.FinalURL, false)
	expandedScored := discoverycore.ScoreStructuralCandidates("", "", expanded, domainHints)
	if len(expandedScored) > 0 {
		out = append(out, s.selectExpandedRecoveryCandidates(ctx, goal, candidate, expandedScored)...)
	}
	out = append(out, webdiscover.ExtractEmbeddedURLCandidates(goal, refined, rawPage.FinalURL, rawPage.HTML, dom, domainHints)...)
	return out, nil
}

type hostRootIdentityProbe struct {
	URL      string
	Title    string
	Score    float64
	Reasons  []string
	Metadata map[string]string
}

func (s *Service) probeHostRootIdentity(ctx context.Context, _ string, userAgent, rawURL string) (hostRootIdentityProbe, error) {
	rootURL, ok := webdiscover.HostRootURL(rawURL)
	if !ok || strings.TrimSpace(rootURL) == strings.TrimSpace(rawURL) {
		return hostRootIdentityProbe{}, nil
	}

	rawPage, err := s.acquirer.Acquire(ctx, s.fetchAcquireInput(rootURL, effectiveUserAgent(userAgent, true)))
	if err != nil {
		return hostRootIdentityProbe{}, err
	}
	dom, err := s.reducer.Reduce(rawPage)
	if err != nil {
		return hostRootIdentityProbe{}, err
	}
	rootContext := webdiscover.IRContext(buildWebIR(dom), 900)
	if strings.TrimSpace(dom.Title) == "" {
		return hostRootIdentityProbe{
			URL: strings.TrimSpace(rawPage.FinalURL),
		}, nil
	}

	identityScore, reasons := discoverycore.ScoreStructuralURL(rawPage.FinalURL, false, nil)
	if identityScore <= 0 {
		return hostRootIdentityProbe{
			URL:   strings.TrimSpace(rawPage.FinalURL),
			Title: strings.TrimSpace(dom.Title),
			Metadata: map[string]string{
				"host_root_url":     strings.TrimSpace(rawPage.FinalURL),
				"host_root_title":   strings.TrimSpace(dom.Title),
				"host_root_context": rootContext,
			},
		}, nil
	}

	return hostRootIdentityProbe{
		URL:   strings.TrimSpace(rawPage.FinalURL),
		Title: strings.TrimSpace(dom.Title),
		Score: identityScore * 0.65,
		Reasons: discoverycore.AppendUniqueReason(
			reasons,
			"host_root_identity_probe",
		),
		Metadata: map[string]string{
			"host_root_url":     strings.TrimSpace(rawPage.FinalURL),
			"host_root_title":   strings.TrimSpace(dom.Title),
			"host_root_context": rootContext,
		},
	}, nil
}

func (s *Service) selectIdentityReferenceCandidates(ctx context.Context, goal string, source DiscoverCandidate, refs []webdiscover.IdentityReferenceCandidate) []DiscoverCandidate {
	if len(refs) == 0 {
		return nil
	}
	baseLinks, relationByURL := webdiscover.IdentityBaseLinks(source.URL, refs)
	scored := discoverycore.ScoreStructuralCandidates("", "", baseLinks, nil)
	if len(scored) == 0 {
		return nil
	}
	goalSimilarity := s.scoreCandidateSetToGoal(ctx, goal, webdiscover.IdentitySemanticCandidates(source, scored))
	return webdiscover.IdentityDiscoverCandidates(source, scored, relationByURL, goalSimilarity)
}

func (s *Service) selectExpandedRecoveryCandidates(ctx context.Context, goal string, source DiscoverCandidate, expanded []DiscoverCandidate) []DiscoverCandidate {
	if len(expanded) == 0 {
		return nil
	}
	ordered := append([]DiscoverCandidate{}, expanded...)
	goalSimilarity := s.scoreCandidateSetToGoal(ctx, goal, expandedRecoverySemanticCandidates(source, ordered))
	applyExpandedRecoveryScores(source.URL, ordered, goalSimilarity)
	discoverycore.SortCandidates(ordered)
	return selectExpandedRecoveryLeaders(ordered)
}

func expandedRecoverySemanticCandidates(source DiscoverCandidate, ordered []DiscoverCandidate) []intel.SemanticCandidate {
	semanticCandidates := make([]intel.SemanticCandidate, 0, len(ordered))
	for _, candidate := range ordered {
		semanticCandidates = append(semanticCandidates, intel.SemanticCandidate{
			ID: candidate.URL,
			Text: discoverycore.JoinNonEmpty(
				source.Metadata["host_root_title"],
				source.Metadata["host_root_context"],
				source.Metadata["page_title"],
				source.Metadata["web_ir_context"],
				source.Label,
				candidate.Metadata["source_context"],
				candidate.Label,
				candidate.Metadata["resource_class"],
			),
		})
	}
	return semanticCandidates
}

func applyExpandedRecoveryScores(sourceURL string, ordered []DiscoverCandidate, goalSimilarity map[string]float64) {
	sourceFamily, _ := webdiscover.CandidateFamily(sourceURL)
	sourceDepth := discoverycore.URLPathDepth(sourceURL)
	for i := range ordered {
		similarity := goalSimilarity[ordered[i].URL]
		family, familyOK := webdiscover.CandidateFamily(ordered[i].URL)
		sameFamily := familyOK && family != "" && family == sourceFamily
		candidateDepth := discoverycore.URLPathDepth(ordered[i].URL)
		if similarity > 0 {
			ordered[i].Score += similarity * 1.35
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "page_expand_semantic_grounding")
		}
		if sameFamily {
			switch {
			case candidateDepth > sourceDepth:
				ordered[i].Score += 0.42
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "same_family_child_recovery")
			case candidateDepth == sourceDepth:
				ordered[i].Score += 0.14
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "same_family_page_expand")
			default:
				ordered[i].Score -= 0.16
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "same_family_scope_regression")
			}
		}
		if familyOK && family != "" && family != sourceFamily {
			if similarity < 0.18 {
				ordered[i].Score -= 0.18
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "external_family_ungrounded")
			} else {
				ordered[i].Score += 0.60
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "external_family_recovery")
			}
		}
	}
}

func selectExpandedRecoveryLeaders(ordered []DiscoverCandidate) []DiscoverCandidate {
	out := make([]DiscoverCandidate, 0, 3)
	seenFamilies := map[string]struct{}{}
	for _, candidate := range ordered {
		family, _ := webdiscover.CandidateFamily(candidate.URL)
		if family != "" {
			if _, ok := seenFamilies[family]; ok {
				continue
			}
			seenFamilies[family] = struct{}{}
		}
		if candidateHasAnyReason(candidate, "same_family_scope_regression") {
			candidate.Score += 0.04
			candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "page_expand_scope_context")
		} else if candidateHasAnyReason(candidate, "same_family_child_recovery") {
			candidate.Score += 0.70
			candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "page_expand", "page_expand_child_context")
		} else {
			candidate.Score += 0.40
			candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "page_expand")
		}
		out = append(out, candidate)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func (s *Service) maybePromoteEndpointCandidate(ctx context.Context, goal, userAgent string, domainHints []string, candidates []DiscoverCandidate) []DiscoverCandidate {
	backend := strings.TrimSpace(s.cfg.Models.Backend)
	if len(candidates) == 0 || backend != intel.BackendOpenAICompatible {
		return candidates
	}
	pageInputs, allowed := s.collectEndpointPageInputs(ctx, goal, userAgent, candidates)
	if len(pageInputs) == 0 {
		return candidates
	}
	out, ok := s.runEndpointExtractor(ctx, goal, domainHints, pageInputs)
	selectedPage, found := allowed[out.SelectedURL]
	if !ok || !found || out.Confidence < 0.55 {
		return candidates
	}
	return webdiscover.PromoteEndpointCandidate(candidates, selectedPage, out)
}

func (s *Service) collectEndpointPageInputs(ctx context.Context, goal, userAgent string, candidates []DiscoverCandidate) ([]webdiscover.EndpointPageInput, map[string]webdiscover.EndpointPageInput) {
	pageInputs := make([]webdiscover.EndpointPageInput, 0, min(len(candidates), 4))
	allowed := map[string]webdiscover.EndpointPageInput{}
	for _, candidate := range s.orderEndpointCandidates(ctx, goal, candidates, min(len(candidates), 4)) {
		rawPage, err := s.acquirer.Acquire(ctx, s.fetchAcquireInput(candidate.URL, effectiveUserAgent(userAgent, true)))
		if err != nil {
			continue
		}
		dom, err := s.reducer.Reduce(rawPage)
		if err != nil {
			continue
		}
		embeddedURLs := webdiscover.EmbeddedURLsForPage(rawPage.FinalURL, rawPage.HTML, dom)
		if len(embeddedURLs) == 0 {
			continue
		}
		input := webdiscover.EndpointPageInput{
			PageURL:      strings.TrimSpace(rawPage.FinalURL),
			PageTitle:    strings.TrimSpace(dom.Title),
			EmbeddedURLs: embeddedURLs,
		}
		pageInputs = append(pageInputs, input)
		for _, embeddedURL := range embeddedURLs {
			allowed[embeddedURL] = input
		}
	}
	return pageInputs, allowed
}

func (s *Service) runEndpointExtractor(ctx context.Context, goal string, domainHints []string, pageInputs []webdiscover.EndpointPageInput) (webdiscover.EndpointExtractorResult, bool) {
	modelReq := intel.ModelRequest{
		Task:            intel.TaskEndpointExtract,
		ModelClass:      intel.ModelClassMicroSolver,
		MaxInputTokens:  1200,
		MaxOutputTokens: 180,
		TimeoutMS:       max(600, s.cfg.Models.MicroTimeoutMS),
		SchemaName:      "endpoint_extract.v1",
		Input: map[string]any{
			"goal":            strings.TrimSpace(goal),
			"domain_hints":    domainHints,
			"candidate_pages": pageInputs,
		},
	}
	resp, err := s.runtime.Run(ctx, modelReq)
	if err != nil {
		return webdiscover.EndpointExtractorResult{}, false
	}
	var out webdiscover.EndpointExtractorResult
	if err := json.Unmarshal([]byte(resp.OutputJSON), &out); err != nil {
		return webdiscover.EndpointExtractorResult{}, false
	}
	out.SelectedURL = strings.TrimSpace(out.SelectedURL)
	return out, true
}

func (s *Service) orderEndpointCandidates(ctx context.Context, goal string, candidates []DiscoverCandidate, limit int) []DiscoverCandidate {
	if len(candidates) == 0 || limit <= 0 {
		return nil
	}
	window := min(len(candidates), max(limit*2, limit))
	ordered := append([]DiscoverCandidate{}, candidates[:window]...)
	semanticCandidates := make([]intel.SemanticCandidate, 0, len(ordered))
	for _, candidate := range ordered {
		semanticCandidates = append(semanticCandidates, intel.SemanticCandidate{
			ID: candidate.URL,
			Text: discoverycore.JoinNonEmpty(
				candidate.Metadata["source_context"],
				candidate.Metadata["page_title"],
				candidate.Label,
				candidate.Metadata["resource_class"],
			),
		})
	}
	byURL := map[string]float64{}
	if len(semanticCandidates) > 0 {
		if scored, err := s.semantic.Score(ctx, goal, semanticCandidates); err == nil {
			for _, item := range scored {
				byURL[item.ID] = item.Similarity
			}
		}
	}
	for i := range ordered {
		resourceClass := discoverycore.ResourceClass(ordered[i].URL)
		switch resourceClass {
		case discoverycore.ResourceClassHTMLLike:
			ordered[i].Score += 0.08
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "endpoint_resource_class_html_like")
		case discoverycore.ResourceClassStructured:
			ordered[i].Score += 0.05
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "endpoint_resource_class_structured")
		case discoverycore.ResourceClassMediaAsset, discoverycore.ResourceClassArchiveFile:
			ordered[i].Score -= 0.08
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "endpoint_resource_class_penalty")
		}
		if similarity, ok := byURL[ordered[i].URL]; ok && similarity > 0 {
			ordered[i].Score += similarity * 1.2
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "endpoint_probe_semantic_alignment")
		}
		ordered[i].Metadata = discoverycore.MergeMetadata(ordered[i].Metadata, map[string]string{
			"resource_class": resourceClass,
		})
	}
	discoverycore.SortCandidates(ordered)
	return ordered[:min(len(ordered), limit)]
}
