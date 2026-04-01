package service

import (
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/queryplan"
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
	if req.SeedURL == "" && discoveryMode == QueryDiscoverySameSite {
		discoveryMode = QueryDiscoveryWeb
	}
	if req.SeedURL == "" && discoveryMode != QueryDiscoveryWeb {
		return "", "", "", QueryRequest{}, fmt.Errorf("query request seed_url must not be empty when discovery_mode=%s", discoveryMode)
	}
	return profile, requestedMode, discoveryMode, req, nil
}

func (s *Service) buildQueryPlan(req QueryRequest, profile, requestedMode, discoveryMode string) (QueryPlan, QueryCompiler, QueryFingerprintEvidence) {
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
	plan.DiscoveryProvider = discoveryResult.provider
	plan.Compiler = queryplan.AnnotateQueryCompilerWithPlanningWebIR(plan.Compiler, queryPlanCandidate(candidates.ByURL(selectedURL)))
	if selectedURL == "" {
		return fmt.Errorf("query discovery returned empty selected_url")
	}
	plan.SelectedURL = selectedURL
	plan.CandidateURLs = candidates.URLs()
	plan.Compiler = queryplan.FinalizeQueryCompiler(plan.Compiler, req.SeedURL, discoveryMode, plan.DiscoveryProvider, selectedURL, queryPlanCandidates(discoveryCandidates))
	plan.Compiler = queryplan.AnnotateQueryCompilerWithIntentBoundary(plan.Compiler)
	return nil
}

func (s *Service) readRequestForQuery(req QueryRequest, profile, selectedURL string) ReadRequest {
	return ReadRequest{
		URL:            selectedURL,
		Objective:      req.Goal,
		Profile:        profile,
		UserAgent:      req.UserAgent,
		ForceLane:      req.ForceLane,
		PruningProfile: req.PruningProfile,
		RenderHint:     req.RenderHint,
	}
}
