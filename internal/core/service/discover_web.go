package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/store"
	"golang.org/x/net/html"
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

const webProbeLimit = 5

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

	providers := s.orderedDiscoveryProviders(discoverycore.WebSearchProviders(s.webDiscoverBaseURL))
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
		providerName := discoverycore.ProviderName(providerBaseURL)
		providerUsed := false
		for _, query := range queries {
			bootstrapped, bootURL, err := s.discoverWebBootstrap(ctx, providerBaseURL, req, query)
			if err != nil {
				s.observeDiscoveryProvider(providerName, classifyDiscoveryProviderOutcome(err))
				if isProviderUnavailable(err) {
					break
				}
				lastErr = err
				if discoveryProviderLevelFailure(err) {
					break
				}
				continue
			}
			s.observeDiscoveryProvider(providerName, store.DiscoveryProviderOutcomeSuccess)
			if discoveryURL == "" {
				discoveryURL = bootURL
			}
			candidates.Merge(bootstrapped)
			providerUsed = true
		}
		if providerUsed {
			providerNames = append(providerNames, providerName)
		}
	}
	if len(candidates.Sorted()) == 0 && len(queries) > 1 {
		for _, providerBaseURL := range providers {
			providerName := discoverycore.ProviderName(providerBaseURL)
			bootstrapped, bootURL, err := s.discoverWebBootstrap(ctx, providerBaseURL, req, req.Goal)
			if err != nil {
				s.observeDiscoveryProvider(providerName, classifyDiscoveryProviderOutcome(err))
				if isProviderUnavailable(err) {
					continue
				}
				lastErr = err
				continue
			}
			s.observeDiscoveryProvider(providerName, store.DiscoveryProviderOutcomeSuccess)
			if discoveryURL == "" {
				discoveryURL = bootURL
			}
			candidates.Merge(bootstrapped)
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
	bootstrapped := s.semanticRerankDiscoverCandidates(ctx, req.Goal, candidates.Sorted())
	expanded := s.expandAndRerankWebCandidates(ctx, req.Goal, req.UserAgent, req.DomainHints, bootstrapped, req.MaxCandidates)
	filtered := discoverycore.NewSet(s.semanticRerankDiscoverCandidates(ctx, req.Goal, expanded)).Limited(req.MaxCandidates)
	filtered = canonicalizeCandidateFamilies(filtered)
	filtered = s.semanticDisambiguateCandidateFamilies(ctx, req.Goal, filtered)
	filtered = s.applyCandidateIntelligence(ctx, req.Goal, filtered)
	filtered = s.maybePromoteEndpointCandidate(ctx, req.Goal, req.UserAgent, req.DomainHints, filtered)
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

func (s *Service) orderedDiscoveryProviders(providers []string) []string {
	if len(providers) < 2 {
		return providers
	}
	type providerSlot struct {
		baseURL string
		index   int
		state   store.DiscoveryProviderState
		ok      bool
	}
	at := s.now().UTC()
	slots := make([]providerSlot, 0, len(providers))
	for index, provider := range providers {
		state, err := s.discoveryProviders.Load(discoverycore.ProviderName(provider))
		slots = append(slots, providerSlot{
			baseURL: provider,
			index:   index,
			state:   state,
			ok:      err == nil,
		})
	}
	sort.SliceStable(slots, func(i, j int) bool {
		left, right := slots[i], slots[j]
		leftCooling := left.ok && left.state.CoolingDown(at)
		rightCooling := right.ok && right.state.CoolingDown(at)
		if leftCooling != rightCooling {
			return !leftCooling
		}
		leftScore := 0.0
		rightScore := 0.0
		if left.ok {
			leftScore = left.state.HealthScore(at)
		}
		if right.ok {
			rightScore = right.state.HealthScore(at)
		}
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return left.index < right.index
	})
	out := make([]string, 0, len(slots))
	for _, slot := range slots {
		out = append(out, slot.baseURL)
	}
	return out
}

func (s *Service) observeDiscoveryProvider(name, outcome string) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(outcome) == "" {
		return
	}
	_, _, _ = s.discoveryProviders.Observe(store.DiscoveryProviderObservation{
		Name:                name,
		Outcome:             outcome,
		FailureCooldown:     time.Duration(s.cfg.Discovery.ProviderFailureCooldownMS) * time.Millisecond,
		BlockedCooldown:     time.Duration(s.cfg.Discovery.ProviderBlockedCooldownMS) * time.Millisecond,
		TimeoutCooldown:     time.Duration(s.cfg.Discovery.ProviderTimeoutCooldownMS) * time.Millisecond,
		UnavailableCooldown: time.Duration(s.cfg.Discovery.ProviderUnavailableCooldownMS) * time.Millisecond,
	})
}

func classifyDiscoveryProviderOutcome(err error) string {
	if err == nil {
		return store.DiscoveryProviderOutcomeSuccess
	}
	if isProviderUnavailable(err) {
		return store.DiscoveryProviderOutcomeUnavailable
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(text, "anti-bot challenge"),
		strings.Contains(text, "rate limit"),
		strings.Contains(text, "unexpected status code 403"),
		strings.Contains(text, "unexpected status code 429"),
		strings.Contains(text, "bootstrap provider returned 403"),
		strings.Contains(text, "bootstrap provider returned 429"):
		return store.DiscoveryProviderOutcomeBlocked
	case strings.Contains(text, "deadline exceeded"),
		strings.Contains(text, "timeout"),
		strings.Contains(text, "context deadline exceeded"),
		strings.Contains(text, "client.timeout exceeded"):
		return store.DiscoveryProviderOutcomeTimeout
	default:
		return store.DiscoveryProviderOutcomeFailure
	}
}

func discoveryProviderLevelFailure(err error) bool {
	switch classifyDiscoveryProviderOutcome(err) {
	case store.DiscoveryProviderOutcomeBlocked, store.DiscoveryProviderOutcomeTimeout, store.DiscoveryProviderOutcomeUnavailable:
		return true
	default:
		return false
	}
}

func canonicalizeCandidateFamilies(candidates []DiscoverCandidate) []DiscoverCandidate {
	if len(candidates) < 2 {
		return candidates
	}
	out := append([]DiscoverCandidate{}, candidates...)
	for i := range out {
		for j := range out {
			if i == j {
				continue
			}
			left := out[i]
			right := out[j]
			if !sameCandidateFamily(left.URL, right.URL) {
				continue
			}
			leftDepth := discoverycore.URLPathDepth(left.URL)
			rightDepth := discoverycore.URLPathDepth(right.URL)
			if rightDepth >= leftDepth {
				continue
			}
			if left.Score-right.Score > 0.35 {
				continue
			}
			boost := 0.12
			reason := "same_family_shallow_preference"
			if sameDiscoverHost(left.URL, right.URL) && rightDepth == 0 {
				boost = 0.28
				reason = "same_host_canonical_root"
			} else if rightDepth == 0 {
				boost = 0.18
				reason = "same_family_canonical_root"
			}
			out[j].Score += boost
			out[j].Reason = discoverycore.AppendUniqueReason(out[j].Reason, reason)
		}
	}
	discoverycore.SortCandidates(out)
	return out
}

func (s *Service) semanticDisambiguateCandidateFamilies(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	if len(candidates) < 2 || strings.TrimSpace(goal) == "" {
		return candidates
	}
	families := make(map[string][]DiscoverCandidate)
	order := make([]string, 0)
	for _, candidate := range candidates {
		family, ok := candidateFamily(candidate.URL)
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
				group[i].Metadata["page_title"],
				group[i].Label,
				discoverycore.HostTokenText(group[i].URL),
				family,
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
		family, ok := candidateFamily(out[i].URL)
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

func sameCandidateFamily(leftURL, rightURL string) bool {
	leftDomain, leftErr := discoverycore.RegistrableDomain(leftURL)
	rightDomain, rightErr := discoverycore.RegistrableDomain(rightURL)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return leftDomain == rightDomain
}

func candidateFamily(rawURL string) (string, bool) {
	if family, err := discoverycore.RegistrableDomain(rawURL); err == nil && strings.TrimSpace(family) != "" {
		return family, true
	}
	if host, ok := discoverycore.Hostname(rawURL); ok {
		return host, true
	}
	return "", false
}

func sameDiscoverHost(leftURL, rightURL string) bool {
	leftHost, leftOK := discoverycore.Hostname(leftURL)
	rightHost, rightOK := discoverycore.Hostname(rightURL)
	return leftOK && rightOK && leftHost == rightHost
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
	return slices.Contains(candidate.Reason, "semantic_goal_alignment") ||
		slices.Contains(candidate.Reason, "structure_hint")
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

	rawPage, err := s.acquirer.Acquire(ctx, s.fetchAcquireInput(searchURL, effectiveUserAgent(req.UserAgent, true)))
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

type providerUnavailableError struct{ reason string }

func (e providerUnavailableError) Error() string {
	if strings.TrimSpace(e.reason) == "" {
		return "provider unavailable"
	}
	return e.reason
}

func isProviderUnavailable(err error) bool {
	_, ok := err.(providerUnavailableError)
	return ok
}

func (s *Service) discoverWebBootstrapBrave(ctx context.Context, req DiscoverWebRequest, query string) ([]DiscoverCandidate, string, error) {
	if strings.TrimSpace(s.cfg.Discovery.BraveAPIKey) == "" {
		return nil, "", providerUnavailableError{reason: "brave api key not configured"}
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
	return discoverycore.ScoreCandidates(req.Goal, req.SeedURL, "", links, req.DomainHints), finalURL, nil
}

func (s *Service) doBootstrapJSON(ctx context.Context, method, endpoint string, headers map[string]string, body []byte) ([]byte, string, error) {
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
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
	defer resp.Body.Close()
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

	refined := refineWebCandidate(goal, candidate, rawPage.FinalURL, dom.Title, webIR, domainHints)
	out := []DiscoverCandidate{refined}
	if hostProbe, err := s.probeHostRootIdentity(ctx, goal, userAgent, rawPage.FinalURL); err == nil {
		if hostProbe.Score > 0 {
			refined.Score += hostProbe.Score
			refined.Reason = discoverycore.AppendUniqueReason(refined.Reason, hostProbe.Reasons...)
			refined.Metadata = discoverycore.MergeMetadata(refined.Metadata, hostProbe.Metadata)
			out[0] = refined
		}
		if strings.TrimSpace(hostProbe.URL) != "" && strings.TrimSpace(hostProbe.URL) != strings.TrimSpace(refined.URL) && hostProbe.Score > 0 {
			out = append(out, DiscoverCandidate{
				URL:      hostProbe.URL,
				Label:    discoverycore.FirstNonEmpty(hostProbe.Title, hostProbe.URL),
				Score:    hostProbe.Score + 0.20,
				Reason:   discoverycore.AppendUniqueReason(hostProbe.Reasons, "host_root_candidate"),
				Metadata: discoverycore.MergeMetadata(nil, hostProbe.Metadata),
			})
		}
	}

	identityRefs := extractIdentityReferenceCandidates(rawPage.HTML, rawPage.FinalURL, discoverycore.FirstNonEmpty(dom.Title, candidate.Label))
	if len(identityRefs) > 0 {
		out = append(out, s.selectIdentityReferenceCandidates(ctx, goal, candidate, identityRefs)...)
	}
	expanded := extractLinkCandidates(rawPage.HTML, rawPage.FinalURL, false)
	expandedScored := discoverycore.ScoreCandidates(goal, "", "", expanded, domainHints)
	if len(expandedScored) > 0 {
		out = append(out, s.selectExpandedRecoveryCandidates(ctx, goal, candidate, expandedScored)...)
	}
	out = append(out, extractLiteralURLCandidates(goal, candidate, rawPage.FinalURL, rawPage.HTML, dom, domainHints)...)
	return out, nil
}

type hostRootIdentityProbe struct {
	URL      string
	Title    string
	Score    float64
	Reasons  []string
	Metadata map[string]string
}

func (s *Service) probeHostRootIdentity(ctx context.Context, goal, userAgent, rawURL string) (hostRootIdentityProbe, error) {
	rootURL, ok := hostRootURL(rawURL)
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
	if strings.TrimSpace(dom.Title) == "" {
		return hostRootIdentityProbe{
			URL: strings.TrimSpace(rawPage.FinalURL),
		}, nil
	}

	identityScore, reasons := discoverycore.ScoreURL(goal, rawPage.FinalURL, dom.Title, false, nil)
	if identityScore <= 0 {
		return hostRootIdentityProbe{
			URL:   strings.TrimSpace(rawPage.FinalURL),
			Title: strings.TrimSpace(dom.Title),
			Metadata: map[string]string{
				"host_root_url":   strings.TrimSpace(rawPage.FinalURL),
				"host_root_title": strings.TrimSpace(dom.Title),
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
			"host_root_url":   strings.TrimSpace(rawPage.FinalURL),
			"host_root_title": strings.TrimSpace(dom.Title),
		},
	}, nil
}

func refineWebCandidate(goal string, candidate DiscoverCandidate, finalURL, pageTitle string, webIR core.WebIR, domainHints []string) DiscoverCandidate {
	score, reasons := discoverycore.ScoreURL(goal, finalURL, discoverycore.JoinNonEmpty(pageTitle, candidate.Label), false, domainHints)
	resourceClass := discoverycore.ResourceClass(finalURL)
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
	metadata := discoverycore.MergeMetadata(candidate.Metadata, webIRDiscoveryMetadata(webIR))
	if metadata == nil {
		metadata = map[string]string{}
	}
	if strings.TrimSpace(pageTitle) != "" {
		metadata["page_title"] = strings.TrimSpace(pageTitle)
	}
	if host, ok := discoverycore.Hostname(finalURL); ok {
		metadata["final_host"] = host
	}
	metadata["resource_class"] = resourceClass
	return DiscoverCandidate{
		URL:      finalURL,
		Label:    discoverycore.FirstNonEmpty(pageTitle, candidate.Label),
		Score:    max(score, candidate.Score),
		Reason:   discoverycore.AppendUniqueReason(append([]string{}, candidate.Reason...), reasons...),
		Metadata: metadata,
	}
}

var literalURLPattern = regexp.MustCompile(`https?://[^\s"'<>` + "`" + `)]+`)

func extractLiteralURLCandidates(goal string, candidate DiscoverCandidate, finalURL, rawHTML string, dom pipeline.SimplifiedDOM, domainHints []string) []DiscoverCandidate {
	finalFamily, ok := candidateFamily(finalURL)
	if !ok {
		return nil
	}
	sourceClass := discoverycore.ResourceClass(finalURL)

	texts := make([]string, 0, len(dom.Nodes)+2)
	if sourceClass != discoverycore.ResourceClassHTMLLike {
		if trimmed := strings.TrimSpace(rawHTML); trimmed != "" {
			texts = append(texts, trimmed)
		}
	}
	if trimmed := strings.TrimSpace(dom.Title); trimmed != "" {
		texts = append(texts, trimmed)
	}
	for _, node := range dom.Nodes {
		if trimmed := strings.TrimSpace(node.Text); trimmed != "" {
			texts = append(texts, trimmed)
		}
	}

	literalLinks := make([]discoverycore.LinkCandidate, 0, 4)
	seen := map[string]struct{}{}
	for _, text := range texts {
		for _, raw := range literalURLPattern.FindAllString(text, -1) {
			literalURL := trimLiteralURL(raw)
			if literalURL == "" {
				continue
			}
			if _, ok := seen[literalURL]; ok {
				continue
			}
			literalFamily, ok := candidateFamily(literalURL)
			if !ok || literalFamily != finalFamily {
				continue
			}
			seen[literalURL] = struct{}{}
			literalLinks = append(literalLinks, discoverycore.LinkCandidate{
				URL:   literalURL,
				Label: discoverycore.JoinNonEmpty(dom.Title, candidate.Label),
			})
			if len(literalLinks) >= 4 {
				break
			}
		}
		if len(literalLinks) >= 4 {
			break
		}
	}
	if len(literalLinks) == 0 {
		return nil
	}

	scored := discoverycore.ScoreCandidates(goal, "", discoverycore.JoinNonEmpty(dom.Title, candidate.Label), literalLinks, domainHints)
	out := make([]DiscoverCandidate, 0, min(len(scored), 2))
	for _, item := range scored {
		resourceClass := discoverycore.ResourceClass(item.URL)
		if sourceClass == discoverycore.ResourceClassHTMLLike && resourceClass == discoverycore.ResourceClassMediaAsset {
			continue
		}
		boost := 1.10
		if discoverycore.URLPathDepth(item.URL) >= 3 {
			boost += 0.12
		}
		out = append(out, DiscoverCandidate{
			URL:   item.URL,
			Label: discoverycore.FirstNonEmpty(item.Label, dom.Title, candidate.Label),
			Score: item.Score + boost,
			Reason: discoverycore.AppendUniqueReason(
				append([]string{}, item.Reason...),
				"literal_url_probe",
				"literal_url_same_family",
			),
			Metadata: discoverycore.MergeMetadata(candidate.Metadata, map[string]string{
				"literal_url_source": candidate.URL,
				"page_title":         strings.TrimSpace(dom.Title),
				"resource_class":     resourceClass,
			}),
		})
		if len(out) >= 2 {
			break
		}
	}
	return out
}

func trimLiteralURL(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimRight(value, ".,;:)]}\"'")
	parsed, err := url.Parse(value)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return ""
	}
	return parsed.String()
}

type identityReferenceCandidate struct {
	URL      string
	Label    string
	Relation string
}

func extractIdentityReferenceCandidates(rawHTML, baseURL, label string) []identityReferenceCandidate {
	root, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	out := make([]identityReferenceCandidate, 0, 6)
	seen := map[string]struct{}{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch strings.ToLower(strings.TrimSpace(node.Data)) {
			case "link":
				rel := strings.ToLower(strings.TrimSpace(htmlAttr(node, "rel")))
				if rel == "canonical" || strings.Contains(rel, "alternate") {
					if href := resolveReferenceURL(base, htmlAttr(node, "href")); href != "" {
						if _, ok := seen[href]; !ok {
							seen[href] = struct{}{}
							relation := "alternate"
							if rel == "canonical" {
								relation = "canonical"
							}
							out = append(out, identityReferenceCandidate{URL: href, Label: strings.TrimSpace(label), Relation: relation})
						}
					}
				}
			case "meta":
				property := strings.ToLower(strings.TrimSpace(htmlAttr(node, "property")))
				if property == "og:url" {
					if href := resolveReferenceURL(base, htmlAttr(node, "content")); href != "" {
						if _, ok := seen[href]; !ok {
							seen[href] = struct{}{}
							out = append(out, identityReferenceCandidate{URL: href, Label: strings.TrimSpace(label), Relation: "og_url"})
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return out
}

func (s *Service) selectIdentityReferenceCandidates(ctx context.Context, goal string, source DiscoverCandidate, refs []identityReferenceCandidate) []DiscoverCandidate {
	if len(refs) == 0 {
		return nil
	}
	baseLinks := make([]discoverycore.LinkCandidate, 0, len(refs))
	relationByURL := make(map[string]string, len(refs))
	sourceURL := strings.TrimSpace(source.URL)
	for _, ref := range refs {
		if strings.TrimSpace(ref.URL) == sourceURL {
			continue
		}
		baseLinks = append(baseLinks, discoverycore.LinkCandidate{URL: ref.URL, Label: ref.Label})
		relationByURL[ref.URL] = ref.Relation
	}
	scored := discoverycore.ScoreCandidates(goal, "", source.Label, baseLinks, nil)
	if len(scored) == 0 {
		return nil
	}
	semanticCandidates := make([]intel.SemanticCandidate, 0, len(scored))
	for _, candidate := range scored {
		semanticCandidates = append(semanticCandidates, intel.SemanticCandidate{
			ID: candidate.URL,
			Text: discoverycore.JoinNonEmpty(
				source.Metadata["host_root_title"],
				source.Metadata["page_title"],
				source.Label,
				candidate.Label,
				discoverycore.URLTokenText(candidate.URL),
			),
		})
	}
	goalSimilarity := s.scoreCandidateSetToGoal(ctx, goal, semanticCandidates)
	sourceFamily, _ := candidateFamily(source.URL)
	out := make([]DiscoverCandidate, 0, 2)
	for _, candidate := range scored {
		similarity := goalSimilarity[candidate.URL]
		switch relationByURL[candidate.URL] {
		case "alternate":
			if similarity < 0.22 {
				continue
			}
		case "og_url":
			if similarity < 0.18 {
				continue
			}
		}
		boost := 1.10
		if similarity > 0 {
			boost += similarity * 1.4
		}
		if family, ok := candidateFamily(candidate.URL); ok && family != "" && family != sourceFamily {
			boost += 0.45
		}
		switch relationByURL[candidate.URL] {
		case "canonical":
			boost += 0.75
		case "og_url":
			boost += 0.60
		case "alternate":
			boost += 0.35
		}
		out = append(out, DiscoverCandidate{
			URL:   candidate.URL,
			Label: discoverycore.FirstNonEmpty(candidate.Label, source.Label, candidate.URL),
			Score: candidate.Score + boost,
			Reason: discoverycore.AppendUniqueReason(candidate.Reason,
				"identity_reference",
				"external_family_recovery",
				"identity_reference_"+discoverycore.FirstNonEmpty(relationByURL[candidate.URL], "unknown"),
			),
			Metadata: discoverycore.MergeMetadata(source.Metadata, map[string]string{
				"identity_reference_source": source.URL,
				"identity_reference_kind":   relationByURL[candidate.URL],
				"resource_class":            discoverycore.ResourceClass(candidate.URL),
			}),
		})
		if len(out) >= 2 {
			break
		}
	}
	return out
}

func resolveReferenceURL(base *url.URL, raw string) string {
	ref, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	return resolved.String()
}

func htmlAttr(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(strings.TrimSpace(attr.Key), key) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func (s *Service) selectExpandedRecoveryCandidates(ctx context.Context, goal string, source DiscoverCandidate, expanded []DiscoverCandidate) []DiscoverCandidate {
	if len(expanded) == 0 {
		return nil
	}
	ordered := append([]DiscoverCandidate{}, expanded...)
	semanticCandidates := make([]intel.SemanticCandidate, 0, len(ordered))
	for _, candidate := range ordered {
		semanticCandidates = append(semanticCandidates, intel.SemanticCandidate{
			ID: candidate.URL,
			Text: discoverycore.JoinNonEmpty(
				source.Metadata["host_root_title"],
				source.Metadata["page_title"],
				source.Label,
				candidate.Label,
				discoverycore.URLTokenText(candidate.URL),
			),
		})
	}
	goalSimilarity := s.scoreCandidateSetToGoal(ctx, goal, semanticCandidates)
	sourceFamily, _ := candidateFamily(source.URL)
	for i := range ordered {
		similarity := goalSimilarity[ordered[i].URL]
		family, familyOK := candidateFamily(ordered[i].URL)
		sameFamily := familyOK && family != "" && family == sourceFamily
		if similarity > 0 {
			ordered[i].Score += similarity * 1.35
			ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "page_expand_semantic_grounding")
		}
		if sameFamily {
			if discoverycore.URLPathDepth(ordered[i].URL) > discoverycore.URLPathDepth(source.URL) {
				ordered[i].Score += 0.42
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "same_family_child_recovery")
			} else {
				ordered[i].Score += 0.14
				ordered[i].Reason = discoverycore.AppendUniqueReason(ordered[i].Reason, "same_family_page_expand")
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
	discoverycore.SortCandidates(ordered)

	out := make([]DiscoverCandidate, 0, 3)
	seenFamilies := map[string]struct{}{}
	for _, candidate := range ordered {
		family, _ := candidateFamily(candidate.URL)
		if family != "" {
			if _, ok := seenFamilies[family]; ok {
				continue
			}
			seenFamilies[family] = struct{}{}
		}
		candidate.Score += 0.40
		candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, "page_expand")
		out = append(out, candidate)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

type endpointExtractorResult struct {
	SelectedURL     string  `json:"selected_url"`
	EvidencePageURL string  `json:"evidence_page_url"`
	Kind            string  `json:"kind"`
	Confidence      float64 `json:"confidence"`
}

func (s *Service) maybePromoteEndpointCandidate(ctx context.Context, goal, userAgent string, domainHints []string, candidates []DiscoverCandidate) []DiscoverCandidate {
	backend := strings.TrimSpace(s.cfg.Models.Backend)
	if len(candidates) == 0 || (backend != intel.BackendOpenAICompatible && backend != intel.BackendOllama) {
		return candidates
	}

	type pageInput struct {
		PageURL     string   `json:"page_url"`
		PageTitle   string   `json:"page_title"`
		LiteralURLs []string `json:"literal_urls"`
	}
	pageInputs := make([]pageInput, 0, min(len(candidates), 4))
	allowed := map[string]pageInput{}

	for _, candidate := range s.orderEndpointCandidates(ctx, goal, candidates, min(len(candidates), 4)) {
		rawPage, err := s.acquirer.Acquire(ctx, s.fetchAcquireInput(candidate.URL, effectiveUserAgent(userAgent, true)))
		if err != nil {
			continue
		}
		dom, err := s.reducer.Reduce(rawPage)
		if err != nil {
			continue
		}
		literals := literalURLsForPage(rawPage.FinalURL, rawPage.HTML, dom)
		if len(literals) == 0 {
			continue
		}
		input := pageInput{
			PageURL:     strings.TrimSpace(rawPage.FinalURL),
			PageTitle:   strings.TrimSpace(dom.Title),
			LiteralURLs: literals,
		}
		pageInputs = append(pageInputs, input)
		for _, literal := range literals {
			allowed[literal] = input
		}
	}
	if len(pageInputs) == 0 {
		return candidates
	}

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
		return candidates
	}
	var out endpointExtractorResult
	if err := json.Unmarshal([]byte(resp.OutputJSON), &out); err != nil {
		return candidates
	}
	out.SelectedURL = strings.TrimSpace(out.SelectedURL)
	selectedPage, ok := allowed[out.SelectedURL]
	if !ok || out.Confidence < 0.55 {
		return candidates
	}

	boosted := append([]DiscoverCandidate{}, candidates...)
	boosted = append(boosted, DiscoverCandidate{
		URL:   out.SelectedURL,
		Label: discoverycore.FirstNonEmpty(strings.TrimSpace(selectedPage.PageTitle), out.SelectedURL),
		Score: 4.50 + out.Confidence,
		Reason: discoverycore.AppendUniqueReason(nil,
			"endpoint_extract_llm",
			"literal_url_probe",
		),
		Metadata: map[string]string{
			"endpoint_extract_kind":          strings.TrimSpace(out.Kind),
			"endpoint_extract_evidence_page": strings.TrimSpace(selectedPage.PageURL),
		},
	})
	return discoverycore.NewSet(boosted).Sorted()
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
				candidate.Label,
				discoverycore.URLTokenText(candidate.URL),
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

func literalURLsForPage(finalURL, rawHTML string, dom pipeline.SimplifiedDOM) []string {
	family, ok := candidateFamily(finalURL)
	if !ok {
		return nil
	}
	sourceClass := discoverycore.ResourceClass(finalURL)
	texts := make([]string, 0, len(dom.Nodes)+2)
	if sourceClass != discoverycore.ResourceClassHTMLLike {
		if trimmed := strings.TrimSpace(rawHTML); trimmed != "" {
			texts = append(texts, trimmed)
		}
	}
	if trimmed := strings.TrimSpace(dom.Title); trimmed != "" {
		texts = append(texts, trimmed)
	}
	for _, node := range dom.Nodes {
		if trimmed := strings.TrimSpace(node.Text); trimmed != "" {
			texts = append(texts, trimmed)
		}
	}
	out := make([]string, 0, 8)
	seen := map[string]struct{}{}
	for _, text := range texts {
		for _, raw := range literalURLPattern.FindAllString(text, -1) {
			literalURL := trimLiteralURL(raw)
			if literalURL == "" {
				continue
			}
			if _, ok := seen[literalURL]; ok {
				continue
			}
			literalFamily, ok := candidateFamily(literalURL)
			if !ok || literalFamily != family {
				continue
			}
			seen[literalURL] = struct{}{}
			out = append(out, literalURL)
			if len(out) >= 8 {
				return out
			}
		}
	}
	return out
}

func hostRootURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return "", false
	}
	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: "/"}).String(), true
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
