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
	Goal                 string                     `json:"goal"`
	SeedURL              string                     `json:"seed_url"`
	Profile              string                     `json:"profile"`
	Budget               core.Budget                `json:"budget"`
	LaneMax              int                        `json:"lane_max"`
	Compiler             queryplan.QueryCompiler    `json:"compiler,omitempty"`
	DiscoveryMode        string                     `json:"discovery_mode,omitempty"`
	DiscoveryProvider    string                     `json:"discovery_provider,omitempty"`
	SelectedURL          string                     `json:"selected_url,omitempty"`
	CandidateURLs        []string                   `json:"candidate_urls,omitempty"`
	CandidateDiagnostics []QueryCandidateDiagnostic `json:"candidate_diagnostics,omitempty"`
	DomainHints          []string                   `json:"domain_hints,omitempty"`
}

type QueryCandidateDiagnostic struct {
	URL                          string   `json:"url"`
	Score                        float64  `json:"score,omitempty"`
	ResourceClass                string   `json:"resource_class,omitempty"`
	SemanticRole                 string   `json:"semantic_role,omitempty"`
	SemanticRoleConfidence       float64  `json:"semantic_role_confidence,omitempty"`
	SemanticRoleIntent           float64  `json:"semantic_role_intent,omitempty"`
	SemanticOriginAlignment      float64  `json:"semantic_origin_alignment,omitempty"`
	SemanticDerivativeAlignment  float64  `json:"semantic_derivative_alignment,omitempty"`
	ClusterID                    string   `json:"cluster_id,omitempty"`
	ClusterSize                  int      `json:"cluster_size,omitempty"`
	LateInteractionScore         float64  `json:"late_interaction_score,omitempty"`
	LateInteractionConfidence    float64  `json:"late_interaction_confidence,omitempty"`
	SemanticEvidenceSimilarity   float64  `json:"semantic_evidence_similarity,omitempty"`
	SemanticEvidenceBoost        float64  `json:"semantic_evidence_boost,omitempty"`
	SemanticOriginSimilarity     float64  `json:"semantic_origin_similarity,omitempty"`
	SemanticDerivativeSimilarity float64  `json:"semantic_derivative_similarity,omitempty"`
	SemanticCommunitySimilarity  float64  `json:"semantic_community_similarity,omitempty"`
	SemanticAuthorityBoost       float64  `json:"semantic_authority_boost,omitempty"`
	SemanticAuthorityPenalty     float64  `json:"semantic_authority_penalty,omitempty"`
	SemanticCommunityPenalty     float64  `json:"semantic_community_penalty,omitempty"`
	SemanticQuorumProviderCount  int      `json:"semantic_quorum_provider_count,omitempty"`
	SemanticCalibrationScore     float64  `json:"semantic_calibration_score,omitempty"`
	SemanticProvenanceIdentity   float64  `json:"semantic_provenance_identity,omitempty"`
	SemanticProvenanceTopic      float64  `json:"semantic_provenance_topic,omitempty"`
	SemanticProvenanceBoost      float64  `json:"semantic_provenance_boost,omitempty"`
	SemanticProvenancePenalty    float64  `json:"semantic_provenance_penalty,omitempty"`
	SemanticFamilyEvidenceCount  int      `json:"semantic_family_evidence_count,omitempty"`
	SemanticFamilyEvidenceStrong int      `json:"semantic_family_evidence_strong,omitempty"`
	SemanticFamilyProvenance     int      `json:"semantic_family_evidence_provenance,omitempty"`
	SemanticFamilyEvidence       float64  `json:"semantic_family_evidence_support,omitempty"`
	SemanticNearTieMerit         float64  `json:"semantic_near_tie_merit,omitempty"`
	SemanticNearTieBoost         float64  `json:"semantic_near_tie_boost,omitempty"`
	Reasons                      []string `json:"reasons,omitempty"`
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
	readResp, err := s.readQuerySelectedCandidate(ctx, req, profile, discoveryMode, &plan)
	if err != nil {
		if discoveryMode == QueryDiscoveryOff && strings.TrimSpace(req.SeedURL) != "" && strings.Contains(strings.ToLower(err.Error()), "unexpected status code 404") {
			return QueryResponse{}, fmt.Errorf("seed_url returned 404; discovery_mode=off requires an exact canonical page. Use same_site_links or web_search first")
		}
		return QueryResponse{}, err
	}
	return finalizeQueryResponse(plan, baseCompiler, discoveryResult.candidates, readResp)
}

func (s *Service) readQuerySelectedCandidate(ctx context.Context, req QueryRequest, profile, discoveryMode string, plan *QueryPlan) (ReadResponse, error) {
	selected := strings.TrimSpace(plan.SelectedURL)
	readResp, err := s.Read(ctx, s.readRequestForQuery(req, profile, selected))
	errorKind, recoverable := recoverableQueryReadErrorKind(err)
	if err == nil || discoveryMode == QueryDiscoveryOff || !recoverable {
		return readResp, err
	}
	for _, candidateURL := range plan.CandidateURLs {
		candidateURL = strings.TrimSpace(candidateURL)
		if candidateURL == "" || candidateURL == selected {
			continue
		}
		nextResp, nextErr := s.Read(ctx, s.readRequestForQuery(req, profile, candidateURL))
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
		ForceLane:                 r.ForceLane,
		FingerprintEvidenceLoader: r.FingerprintEvidenceLoader,
	}
}
