package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/queryflow"
	"github.com/josepavese/needlex/internal/core/queryplan"
	"github.com/josepavese/needlex/internal/proof"
)

const (
	QueryDiscoveryOff      = "off"
	QueryDiscoverySameSite = "same_site_links"
	QueryDiscoveryWeb      = "web_search"
)

type QueryRequest struct {
	Goal, SeedURL, SeedTraceID, Profile, UserAgent, DiscoveryMode, PruningProfile string
	FetchProfile, FetchRetryProfile                                               string
	DomainHints                                                                   []string
	SearchQueries                                                                 []string            `json:"-"`
	MemoryCandidates                                                              []DiscoverCandidate `json:"-"`
	SeedStable, SeedNovelty                                                       float64
	SeedChanged, RenderHint                                                       bool
	ForceLane                                                                     int
	FingerprintEvidenceLoader                                                     func(string) (QueryFingerprintEvidence, bool) `json:"-"`
}

type QueryFingerprintEvidence = queryflow.FingerprintEvidence

type queryDiscoveryResult struct {
	provider       string
	selected       string
	candidates     []DiscoverCandidate
	rewriteApplied bool
	rewriteQueries []string
	rewrite        queryRewriteResult
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
	AgentContext AgentContext        `json:"agent_context,omitempty"`
	ProofRefs    []string            `json:"proof_refs"`
	ProofRecords []proof.ProofRecord `json:"proof_records"`
	Trace        proof.RunTrace      `json:"trace"`
	TraceID      string              `json:"trace_id"`
	CostReport   core.CostReport     `json:"cost_report"`
}

func (s *Service) Query(ctx context.Context, req QueryRequest) (QueryResponse, error) {
	profile, requestedMode, discoveryMode, req, err := s.prepareQuery(req)
	if err != nil {
		return QueryResponse{}, err
	}
	plan, baseCompiler, seedEvidence := s.buildQueryPlan(req, profile, requestedMode, discoveryMode)
	discoveryResult, err := s.runQueryDiscovery(ctx, req, discoveryMode, seedEvidence)
	if err != nil {
		return QueryResponse{}, err
	}
	if discoveryResult.rewriteApplied {
		plan.Compiler = queryplan.AnnotateQueryCompilerWithRewrite(plan.Compiler, discoveryResult.rewriteQueries, discoveryResult.rewrite.CanonicalEntity, discoveryResult.rewrite.LocalityHints, discoveryResult.rewrite.CategoryHints, discoveryResult.rewrite.Confidence)
		if len(discoveryResult.rewrite.LocalityHints) > 0 {
			req.DomainHints = discoverycore.NormalizeDomainHints(append(req.DomainHints, discoveryResult.rewrite.LocalityHints...))
			plan.DomainHints = append([]string{}, req.DomainHints...)
		}
	}
	if err := s.applyDiscoveryToPlan(&plan, req, discoveryMode, discoveryResult); err != nil {
		return QueryResponse{}, err
	}
	readResp, err := s.Read(ctx, s.readRequestForQuery(req, profile, plan.SelectedURL))
	if err != nil {
		if discoveryMode == QueryDiscoveryOff && strings.TrimSpace(req.SeedURL) != "" && strings.Contains(strings.ToLower(err.Error()), "unexpected status code 404") {
			return QueryResponse{}, fmt.Errorf("seed_url returned 404; discovery_mode=off requires an exact canonical page. Use same_site_links or web_search first")
		}
		return QueryResponse{}, err
	}
	return finalizeQueryResponse(plan, baseCompiler, discoveryResult.candidates, readResp)
}

func (r QueryRequest) withQueries(queries []string) QueryRequest {
	return QueryRequest{
		Goal:                      r.Goal,
		SeedURL:                   r.SeedURL,
		SeedTraceID:               r.SeedTraceID,
		Profile:                   r.Profile,
		FetchProfile:              r.FetchProfile,
		FetchRetryProfile:         r.FetchRetryProfile,
		UserAgent:                 r.UserAgent,
		DiscoveryMode:             r.DiscoveryMode,
		PruningProfile:            r.PruningProfile,
		DomainHints:               append([]string{}, r.DomainHints...),
		SearchQueries:             append([]string{}, queries...),
		MemoryCandidates:          append([]DiscoverCandidate{}, r.MemoryCandidates...),
		SeedStable:                r.SeedStable,
		SeedNovelty:               r.SeedNovelty,
		SeedChanged:               r.SeedChanged,
		RenderHint:                r.RenderHint,
		ForceLane:                 r.ForceLane,
		FingerprintEvidenceLoader: r.FingerprintEvidenceLoader,
	}
}
