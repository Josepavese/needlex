package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/queryflow"
	"github.com/josepavese/needlex/internal/core/queryplan"
	"github.com/josepavese/needlex/internal/core/queryreview"
	"github.com/josepavese/needlex/internal/proof"
)

const (
	QueryDiscoveryOff      = "off"
	QueryDiscoverySameSite = "same_site_links"
	QueryDiscoveryWeb      = "web_search"

	seedlessQueryTimeout       = 28 * time.Second
	seedlessQueryReadTimeout   = 12 * time.Second
	seedlessRewriteMinTimeLeft = 14 * time.Second
)

type QueryRequest struct {
	Goal, SeedURL, SeedTraceID, Profile, UserAgent, DiscoveryMode, PruningProfile string
	RenderMode                                                                    string
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
	Goal                 string                   `json:"goal"`
	SeedURL              string                   `json:"seed_url"`
	Profile              string                   `json:"profile"`
	Budget               core.Budget              `json:"budget"`
	LaneMax              int                      `json:"lane_max"`
	Compiler             queryplan.QueryCompiler  `json:"compiler,omitempty"`
	DiscoveryMode        string                   `json:"discovery_mode,omitempty"`
	DiscoveryProvider    string                   `json:"discovery_provider,omitempty"`
	SelectedURL          string                   `json:"selected_url,omitempty"`
	CandidateURLs        []string                 `json:"candidate_urls,omitempty"`
	CandidateDiagnostics []queryreview.Diagnostic `json:"candidate_diagnostics,omitempty"`
	DomainHints          []string                 `json:"domain_hints,omitempty"`
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
	ctx, cancel := s.queryExecutionContext(ctx, req, discoveryMode)
	defer cancel()
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
	readResp, err := s.readQuerySelectedCandidate(ctx, req, profile, discoveryMode, &plan, discoveryResult.candidates)
	if err != nil {
		if discoveryMode == QueryDiscoveryOff && strings.TrimSpace(req.SeedURL) != "" && strings.Contains(strings.ToLower(err.Error()), "unexpected status code 404") {
			return QueryResponse{}, fmt.Errorf("seed_url returned 404; discovery_mode=off requires an exact canonical page. Use same_site_links with the same seed, or obtain a verified candidate URL with the agent's search tool and call web_read")
		}
		return QueryResponse{}, err
	}
	return finalizeQueryResponse(plan, baseCompiler, discoveryResult.candidates, readResp)
}

func (s *Service) readQuerySelectedCandidate(ctx context.Context, req QueryRequest, profile, discoveryMode string, plan *QueryPlan, candidates []DiscoverCandidate) (ReadResponse, error) {
	selected := strings.TrimSpace(plan.SelectedURL)
	if preRead, ok := queryreview.PreReadFallbackCandidate(req.SeedURL, discoveryMode, QueryDiscoveryWeb, queryReviewPlan(plan), candidates); ok {
		previous := plan.SelectedURL
		plan.SelectedURL = preRead.URL
		selected = preRead.URL
		queryreview.AnnotatePreReadFallback(&plan.Compiler, previous, preRead.URL, preRead.SelectedDiagnostic, preRead.Diagnostic)
	}
	readCtx, cancel := s.querySelectedReadContext(ctx, req, discoveryMode)
	defer cancel()
	readResp, err := s.Read(readCtx, s.prepareQuerySelectedReadRequest(req, profile, discoveryMode, selected))
	if err == nil {
		if fallbackResp, ok := s.maybePostReadSemanticFallback(ctx, req, profile, discoveryMode, plan, candidates, readResp); ok {
			return fallbackResp, nil
		}
		return readResp, nil
	}
	errorKind, recoverable := recoverableQueryReadErrorKind(err)
	if discoveryMode == QueryDiscoveryOff || !recoverable {
		return readResp, err
	}
	if hardenedResp, ok := s.retryQueryReadWithResilientFetch(readCtx, req, profile, selected); ok {
		return hardenedResp, nil
	}
	for _, candidateURL := range plan.CandidateURLs {
		if readCtx.Err() != nil {
			break
		}
		candidateURL = strings.TrimSpace(candidateURL)
		if candidateURL == "" || candidateURL == selected {
			continue
		}
		nextResp, nextErr := s.Read(readCtx, s.prepareQuerySelectedReadRequest(req, profile, discoveryMode, candidateURL))
		if nextErr != nil {
			if _, ok := recoverableQueryReadErrorKind(nextErr); ok {
				continue
			}
			return ReadResponse{}, err
		}
		previous := plan.SelectedURL
		plan.SelectedURL = candidateURL
		annotateQueryReadFallback(plan, previous, candidateURL, errorKind)
		return nextResp, nil
	}
	return ReadResponse{}, err
}

func queryReviewPlan(plan *QueryPlan) queryreview.Plan {
	if plan == nil {
		return queryreview.Plan{}
	}
	return queryreview.Plan{SelectedURL: plan.SelectedURL, CandidateURLs: plan.CandidateURLs, Diagnostics: plan.CandidateDiagnostics}
}

func (s *Service) prepareQuerySelectedReadRequest(req QueryRequest, profile, discoveryMode, selectedURL string) ReadRequest {
	readReq := s.readRequestForQuery(req, profile, selectedURL)
	if strings.TrimSpace(req.SeedURL) == "" && discoveryMode == QueryDiscoveryWeb {
		readReq.AgentReadableMode = "declared"
	}
	return readReq
}

func (s *Service) queryExecutionContext(ctx context.Context, req QueryRequest, discoveryMode string) (context.Context, context.CancelFunc) {
	if strings.TrimSpace(req.SeedURL) != "" || discoveryMode != QueryDiscoveryWeb {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, seedlessQueryTimeout)
}

func (s *Service) querySelectedReadContext(ctx context.Context, req QueryRequest, discoveryMode string) (context.Context, context.CancelFunc) {
	if strings.TrimSpace(req.SeedURL) != "" || discoveryMode != QueryDiscoveryWeb {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, seedlessQueryReadTimeout)
}

func (s *Service) retryQueryReadWithResilientFetch(ctx context.Context, req QueryRequest, profile, selectedURL string) (ReadResponse, bool) {
	if strings.TrimSpace(selectedURL) == "" {
		return ReadResponse{}, false
	}
	if strings.TrimSpace(req.FetchProfile) == "browser_like" && strings.TrimSpace(req.FetchRetryProfile) == "hardened" {
		return ReadResponse{}, false
	}
	readReq := s.readRequestForQuery(req, profile, selectedURL)
	if strings.TrimSpace(req.SeedURL) == "" {
		readReq.AgentReadableMode = "declared"
	}
	readReq.FetchProfile = "browser_like"
	readReq.FetchRetryProfile = "hardened"
	resp, err := s.Read(ctx, readReq)
	if err != nil {
		return ReadResponse{}, false
	}
	return resp, true
}

func recoverableQueryReadErrorKind(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "unsupported content type"):
		return "unsupported_content_type", true
	case strings.Contains(text, "unexpected status code 404"):
		return "status_404", true
	case strings.Contains(text, "unexpected status code 403"):
		return "status_403", true
	case strings.Contains(text, "unexpected status code 410"):
		return "status_410", true
	case strings.Contains(text, "tls:") || strings.Contains(text, "x509:") || strings.Contains(text, "certificate"):
		return "tls_certificate", true
	case strings.Contains(text, "context deadline exceeded") || strings.Contains(text, "client.timeout") || strings.Contains(text, "timeout"):
		return "fetch_timeout", true
	case strings.Contains(text, "connection refused") || strings.Contains(text, "no such host") || strings.Contains(text, "server misbehaving"):
		return "fetch_unavailable", true
	case strings.Contains(text, "no segments produced"):
		return "empty_segments", true
	default:
		return "", false
	}
}

func annotateQueryReadFallback(plan *QueryPlan, previousURL, selectedURL, errorKind string) {
	plan.Compiler.Decisions = append(plan.Compiler.Decisions, queryplan.QueryPlanDecision{
		Stage:      "select.candidate_runtime_fallback",
		Choice:     selectedURL,
		ReasonCode: queryplan.QueryPlanReasonSelection,
		Metadata: map[string]string{
			"previous_selected_url": strings.TrimSpace(previousURL),
			"runtime_error_class":   strings.TrimSpace(errorKind),
		},
	})
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
		RenderMode:                r.RenderMode,
		ForceLane:                 r.ForceLane,
		FingerprintEvidenceLoader: r.FingerprintEvidenceLoader,
	}
}
