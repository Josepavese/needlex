package service

import (
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/queryplan"
	"github.com/josepavese/needlex/internal/core/queryreview"
)

func (s *Service) prepareQuery(req QueryRequest) (string, string, string, QueryRequest, error) {
	requestedMode := strings.TrimSpace(strings.ToLower(req.DiscoveryMode))
	profile, err := resolveProfile(req.Profile)
	if err != nil {
		return "", "", "", QueryRequest{}, err
	}
	if req.Goal == "" {
		return "", "", "", QueryRequest{}, fmt.Errorf("query request goal must not be empty")
	}
	discoveryMode, err := resolveDiscoveryMode(req.DiscoveryMode)
	if err != nil {
		return "", "", "", QueryRequest{}, err
	}
	req.SeedURL = strings.TrimSpace(req.SeedURL)
	req.DomainHints = discoverycore.NormalizeDomainHints(req.DomainHints)
	if req.SeedURL == "" {
		if requestedMode == "" {
			return "", "", "", QueryRequest{}, fmt.Errorf("query request seed_url is required for stable use; seedless discovery is experimental and requires explicit discovery_mode=%s", QueryDiscoveryWeb)
		}
		if discoveryMode != QueryDiscoveryWeb {
			return "", "", "", QueryRequest{}, fmt.Errorf("query request seed_url must not be empty when discovery_mode=%s; seedless discovery is experimental and requires explicit discovery_mode=%s", discoveryMode, QueryDiscoveryWeb)
		}
	}
	return profile, requestedMode, discoveryMode, req, nil
}

func (s *Service) buildQueryPlan(req QueryRequest, profile, requestedMode, discoveryMode string) (QueryPlan, queryplan.QueryCompiler, QueryFingerprintEvidence) {
	plan := QueryPlan{
		Goal:    req.Goal,
		SeedURL: req.SeedURL,
		Profile: profile,
		Budget: core.Budget{
			MaxTokens:    s.cfg.Budget.MaxTokens,
			MaxLatencyMS: s.cfg.Budget.MaxLatencyMS,
			MaxPages:     s.cfg.Runtime.MaxPages,
			MaxDepth:     s.cfg.Runtime.MaxDepth,
			MaxBytes:     s.cfg.Runtime.MaxBytes,
		},
		LaneMax:       s.cfg.Runtime.LaneMax,
		DiscoveryMode: discoveryMode,
		DomainHints:   append([]string{}, req.DomainHints...),
		Compiler:      queryplan.BuildQueryCompiler(req.SeedURL, requestedMode, discoveryMode, req.Goal, profile, req.ForceLane, s.cfg.Budget, s.cfg.Runtime),
	}
	baseCompiler := plan.Compiler
	seedEvidence := QueryFingerprintEvidence{TraceID: req.SeedTraceID, Stable: req.SeedStable, Novelty: req.SeedNovelty, Changed: req.SeedChanged}
	plan.Compiler = queryplan.AnnotateQueryCompilerWithFingerprintEvidence(plan.Compiler, req.SeedURL, seedEvidence.TraceID, seedEvidence.Stable, seedEvidence.Novelty, seedEvidence.Changed)
	return plan, baseCompiler, seedEvidence
}

func (s *Service) applyDiscoveryToPlan(plan *QueryPlan, req QueryRequest, discoveryMode string, discoveryResult queryDiscoveryResult) error {
	selectedURL, discoveryCandidates := discoveryResult.selected, discoveryResult.candidates
	candidates := discoverycore.NewSet(discoveryCandidates)
	sortedCandidates := candidates.Sorted()
	plan.DiscoveryProvider = discoveryResult.provider
	plan.Compiler = queryplan.AnnotateQueryCompilerWithPlanningWebIR(plan.Compiler, queryPlanCandidate(candidates.ByURL(selectedURL)))
	if selectedURL == "" {
		return fmt.Errorf("query discovery returned empty selected_url")
	}
	plan.SelectedURL = selectedURL
	plan.CandidateURLs = queryCandidateURLs(sortedCandidates)
	plan.CandidateDiagnostics = queryCandidateDiagnostics(sortedCandidates)
	plan.Compiler = queryplan.FinalizeQueryCompiler(plan.Compiler, req.SeedURL, discoveryMode, plan.DiscoveryProvider, selectedURL, queryPlanCandidates(discoveryCandidates))
	plan.Compiler = queryplan.AnnotateQueryCompilerWithIntentBoundary(plan.Compiler)
	return nil
}

func queryCandidateURLs(candidates []DiscoverCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if url := strings.TrimSpace(candidate.URL); url != "" {
			out = append(out, url)
		}
	}
	return out
}

func queryCandidateDiagnostics(candidates []DiscoverCandidate) []queryreview.Diagnostic {
	return queryreview.Diagnostics(candidates, 8)
}

func (s *Service) readRequestForQuery(req QueryRequest, profile, selectedURL string) ReadRequest {
	return ReadRequest{
		URL:               selectedURL,
		Objective:         req.Goal,
		Profile:           profile,
		FetchProfile:      req.FetchProfile,
		FetchRetryProfile: req.FetchRetryProfile,
		UserAgent:         req.UserAgent,
		ForceLane:         req.ForceLane,
		PruningProfile:    req.PruningProfile,
		RenderHint:        req.RenderHint,
		RenderMode:        req.RenderMode,
	}
}
