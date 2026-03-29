package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
)

const (
	QueryDiscoveryOff      = "off"
	QueryDiscoverySameSite = "same_site_links"
	QueryDiscoveryWeb      = "web_search"
)

type QueryRequest struct {
	Goal, SeedURL, SeedTraceID, Profile, UserAgent, DiscoveryMode, PruningProfile string
	DomainHints                                                                   []string
	SeedStable, SeedNovelty                                                       float64
	SeedChanged, RenderHint                                                       bool
	ForceLane                                                                     int
	FingerprintEvidenceLoader                                                     func(string) (QueryFingerprintEvidence, bool) `json:"-"`
}

type QueryFingerprintEvidence struct {
	TraceID         string
	Stable, Novelty float64
	Changed         bool
}

type QueryPlan struct {
	Goal              string        `json:"goal"`
	SeedURL           string        `json:"seed_url"`
	Profile           string        `json:"profile"`
	Budget            core.Budget   `json:"budget"`
	LaneMax           int           `json:"lane_max"`
	Compiler          QueryCompiler `json:"compiler,omitempty"`
	DiscoveryMode     string        `json:"discovery_mode,omitempty"`
	DiscoveryProvider string        `json:"discovery_provider,omitempty"`
	SelectedURL       string        `json:"selected_url,omitempty"`
	CandidateURLs     []string      `json:"candidate_urls,omitempty"`
	DomainHints       []string      `json:"domain_hints,omitempty"`
}

type QueryResponse struct {
	Plan         QueryPlan           `json:"plan"`
	Document     core.Document       `json:"document"`
	WebIR        core.WebIR          `json:"web_ir"`
	ResultPack   core.ResultPack     `json:"result_pack"`
	ProofRefs    []string            `json:"proof_refs"`
	ProofRecords []proof.ProofRecord `json:"proof_records"`
	Trace        proof.RunTrace      `json:"trace"`
	TraceID      string              `json:"trace_id"`
	CostReport   core.CostReport     `json:"cost_report"`
}

func (s *Service) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	requestedMode := strings.TrimSpace(strings.ToLower(req.DiscoveryMode))
	profile, err := resolveProfile(req.Profile)
	if err != nil {
		return QueryResponse{}, err
	}
	if req.Goal == "" {
		return QueryResponse{}, fmt.Errorf("query request goal must not be empty")
	}
	discoveryMode, err := resolveDiscoveryMode(req.DiscoveryMode)
	if err != nil {
		return QueryResponse{}, err
	}
	req.SeedURL = strings.TrimSpace(req.SeedURL)
	req.DomainHints = normalizeDomainHints(req.DomainHints)
	if req.SeedURL == "" && discoveryMode == QueryDiscoverySameSite {
		discoveryMode = QueryDiscoveryWeb
	}
	if req.SeedURL == "" && discoveryMode != QueryDiscoveryWeb {
		return QueryResponse{}, fmt.Errorf("query request seed_url must not be empty when discovery_mode=%s", discoveryMode)
	}

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
		Compiler:      buildQueryCompiler(req.SeedURL, requestedMode, discoveryMode, req.Goal, profile, req.ForceLane, s.cfg.Budget, s.cfg.Runtime),
	}
	baseCompiler := plan.Compiler
	seedEvidence := QueryFingerprintEvidence{TraceID: req.SeedTraceID, Stable: req.SeedStable, Novelty: req.SeedNovelty, Changed: req.SeedChanged}
	plan.Compiler = annotateQueryCompilerWithFingerprintEvidence(plan.Compiler, req.SeedURL, seedEvidence.TraceID, seedEvidence.Stable, seedEvidence.Novelty, seedEvidence.Changed)

	selectedURL := req.SeedURL
	candidateURLs := []string{}
	discoveryCandidates := []DiscoverCandidate{}
	if req.SeedURL != "" {
		candidateURLs, discoveryCandidates = append(candidateURLs, req.SeedURL), append(discoveryCandidates, DiscoverCandidate{URL: req.SeedURL, Score: 0.35, Reason: []string{"seed_fallback"}})
	}
	if discoveryMode == QueryDiscoverySameSite {
		discovery, err := s.Discover(ctx, DiscoverRequest{
			Goal:          req.Goal,
			SeedURL:       req.SeedURL,
			UserAgent:     req.UserAgent,
			SameDomain:    true,
			MaxCandidates: min(5, s.cfg.Runtime.MaxPages),
			DomainHints:   req.DomainHints,
		})
		if err != nil {
			return QueryResponse{}, err
		}
		selectedURL = discovery.SelectedURL
		candidateURLs = discoveryURLs(discovery.Candidates)
		discoveryCandidates = rerankQueryCandidatesWithFingerprintEvidence(discovery.Candidates, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader)
		selectedURL = discoveryCandidateByURL(discoveryCandidates, "").URL
		plan.DiscoveryProvider = "local_same_site"
		plan.Compiler = annotateQueryCompilerWithPlanningWebIR(plan.Compiler, discoveryCandidateByURL(discoveryCandidates, selectedURL))
	} else if discoveryMode == QueryDiscoveryWeb {
		discovery, err := s.DiscoverWeb(ctx, DiscoverWebRequest{
			Goal:          req.Goal,
			SeedURL:       req.SeedURL,
			UserAgent:     req.UserAgent,
			MaxCandidates: min(5, s.cfg.Runtime.MaxPages),
			DomainHints:   req.DomainHints,
		})
		if err != nil {
			return QueryResponse{}, err
		}
		selectedURL = discovery.SelectedURL
		candidateURLs = discoveryURLs(discovery.Candidates)
		discoveryCandidates = rerankQueryCandidatesWithFingerprintEvidence(discovery.Candidates, req.SeedURL, seedEvidence, req.FingerprintEvidenceLoader)
		selectedURL = discoveryCandidateByURL(discoveryCandidates, "").URL
		plan.DiscoveryProvider = discovery.Provider
		plan.Compiler = annotateQueryCompilerWithPlanningWebIR(plan.Compiler, discoveryCandidateByURL(discoveryCandidates, selectedURL))
	}
	if selectedURL == "" {
		return QueryResponse{}, fmt.Errorf("query discovery returned empty selected_url")
	}
	plan.SelectedURL = selectedURL
	plan.CandidateURLs = candidateURLs
	plan.Compiler = finalizeQueryCompiler(plan.Compiler, req.SeedURL, discoveryMode, plan.DiscoveryProvider, selectedURL, discoveryCandidates)
	plan.Compiler = annotateQueryCompilerWithIntentBoundary(plan.Compiler)

	readResp, err := s.Read(ctx, ReadRequest{
		URL:            selectedURL,
		Objective:      req.Goal,
		Profile:        profile,
		UserAgent:      req.UserAgent,
		ForceLane:      req.ForceLane,
		PruningProfile: req.PruningProfile,
		RenderHint:     req.RenderHint,
	})
	if err != nil {
		return QueryResponse{}, err
	}
	readResp.ResultPack.Query = req.Goal

	resp := QueryResponse{
		Plan:         plan,
		Document:     readResp.Document,
		WebIR:        readResp.WebIR,
		ResultPack:   readResp.ResultPack,
		ProofRefs:    append([]string{}, readResp.ResultPack.ProofRefs...),
		ProofRecords: append([]proof.ProofRecord{}, readResp.ProofRecords...),
		Trace:        readResp.Trace,
		TraceID:      readResp.Trace.TraceID,
		CostReport:   readResp.ResultPack.CostReport,
	}
	resp.Plan.Compiler = annotateQueryCompilerWithWebIR(
		resp.Plan.Compiler,
		resp.WebIR.NodeCount,
		resp.WebIR.Signals.EmbeddedNodeCount,
		resp.WebIR.Signals.HeadingRatio,
		resp.WebIR.Signals.ShortTextRatio,
	)
	resp.Plan.Compiler = annotateQueryCompilerWithExecution(resp.Plan.Compiler, resp.Plan.SelectedURL, resp.Document.FinalURL, resp.ResultPack.CostReport.LanePath)
	resp.Plan.Compiler = annotateQueryCompilerWithBudgetOutcome(resp.Plan.Compiler, resp.Plan.Budget.MaxLatencyMS, resp.ResultPack.CostReport.LatencyMS, resp.Plan.LaneMax, maxLane(resp.ResultPack.CostReport.LanePath))
	escalations, budgetWarnings, runtimeErrors := traceEffectCounts(resp.Trace)
	resp.Plan.Compiler = annotateQueryCompilerWithRuntimeEffects(resp.Plan.Compiler, escalations, budgetWarnings, runtimeErrors)
	resp.Plan.Compiler = annotateQueryCompilerWithExecutionBoundary(resp.Plan.Compiler)
	resp.Plan.Compiler = annotateQueryCompilerWithPlanDiff(baseCompiler, resp.Plan.Compiler)
	return resp, resp.Validate()
}

func (r QueryResponse) Validate() error {
	if r.Plan.Goal == "" {
		return fmt.Errorf("query response plan.goal must not be empty")
	}
	if r.Plan.SelectedURL == "" {
		return fmt.Errorf("query response plan.selected_url must not be empty")
	}
	if err := r.Plan.Compiler.Validate(); err != nil {
		return fmt.Errorf("query response plan.compiler: %w", err)
	}
	if err := r.Document.Validate(); err != nil {
		return err
	}
	if err := r.WebIR.Validate(); err != nil {
		return err
	}
	if err := r.ResultPack.Validate(); err != nil {
		return err
	}
	for i, record := range r.ProofRecords {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("query response proof_records[%d]: %w", i, err)
		}
	}
	if err := r.Trace.Validate(); err != nil {
		return err
	}
	if r.TraceID == "" {
		return fmt.Errorf("query response trace_id must not be empty")
	}
	if err := r.CostReport.Validate(); err != nil {
		return err
	}
	return nil
}

func discoveryURLs(candidates []DiscoverCandidate) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.URL)
	}
	return out
}

func discoveryCandidateByURL(candidates []DiscoverCandidate, selectedURL string) DiscoverCandidate {
	if selectedURL == "" && len(candidates) > 0 {
		return candidates[0]
	}
	for _, candidate := range candidates {
		if candidate.URL == selectedURL {
			return candidate
		}
	}
	return DiscoverCandidate{}
}

func rerankQueryCandidatesWithFingerprintEvidence(candidates []DiscoverCandidate, seedURL string, seedEvidence QueryFingerprintEvidence, loader func(string) (QueryFingerprintEvidence, bool)) []DiscoverCandidate {
	if len(candidates) < 2 {
		return candidates
	}
	out := append([]DiscoverCandidate{}, candidates...)
	for i := range out {
		evidence, ok := QueryFingerprintEvidence{}, out[i].URL == seedURL && strings.TrimSpace(seedEvidence.TraceID) != ""
		if out[i].URL == seedURL {
			evidence = seedEvidence
		} else if loader != nil {
			evidence, ok = loader(out[i].URL)
		}
		if ok {
			out[i] = applyFingerprintEvidenceToCandidate(out[i], evidence, out[i].URL == seedURL)
		}
	}
	slices.SortStableFunc(out, func(left, right DiscoverCandidate) int {
		switch {
		case left.Score > right.Score:
			return -1
		case left.Score < right.Score:
			return 1
		case left.URL < right.URL:
			return -1
		case left.URL > right.URL:
			return 1
		default:
			return 0
		}
	})
	return out
}

func applyFingerprintEvidenceToCandidate(candidate DiscoverCandidate, evidence QueryFingerprintEvidence, isSeed bool) DiscoverCandidate {
	if strings.TrimSpace(evidence.TraceID) == "" {
		return candidate
	}
	candidate.Metadata = mergeCandidateMetadata(candidate.Metadata, map[string]string{"candidate_latest_trace_id": strings.TrimSpace(evidence.TraceID), "candidate_stable_ratio": fmt.Sprintf("%.3f", evidence.Stable), "candidate_novelty_ratio": fmt.Sprintf("%.3f", evidence.Novelty), "candidate_changed": fmt.Sprintf("%t", evidence.Changed)})
	switch {
	case evidence.Stable >= 0.80 && !evidence.Changed:
		candidate.Score -= 0.20
		if isSeed {
			candidate.Reason = appendUniqueReason(candidate.Reason, "stable_seed_penalty")
		} else {
			candidate.Reason = appendUniqueReason(candidate.Reason, "stable_candidate_penalty")
		}
	case evidence.Novelty > 0.20 || evidence.Changed:
		candidate.Score += 0.20
		if isSeed {
			candidate.Reason = appendUniqueReason(candidate.Reason, "novel_seed_bias")
		} else {
			candidate.Reason = appendUniqueReason(candidate.Reason, "novel_candidate_bias")
		}
	}
	return candidate
}

func traceEffectCounts(trace proof.RunTrace) (escalations, budgetWarnings, runtimeErrors int) {
	for _, event := range trace.Events {
		switch event.Type {
		case proof.EventEscalationTriggered:
			escalations++
		case proof.EventBudgetWarning:
			budgetWarnings++
		case proof.EventError:
			runtimeErrors++
		}
	}
	return escalations, budgetWarnings, runtimeErrors
}

func resolveDiscoveryMode(mode string) (string, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return QueryDiscoverySameSite, nil
	}
	switch mode {
	case QueryDiscoveryOff, QueryDiscoverySameSite, QueryDiscoveryWeb:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported query discovery mode %q", mode)
	}
}
