package service

import (
	"fmt"
	"strconv"
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

func queryCandidateDiagnostics(candidates []DiscoverCandidate) []QueryCandidateDiagnostic {
	const limit = 8
	out := make([]QueryCandidateDiagnostic, 0, min(len(candidates), limit))
	for i, candidate := range candidates {
		if i >= limit {
			break
		}
		out = append(out, QueryCandidateDiagnostic{
			URL:                          strings.TrimSpace(candidate.URL),
			Score:                        candidate.Score,
			ResourceClass:                strings.TrimSpace(candidate.Metadata["resource_class"]),
			SemanticRole:                 strings.TrimSpace(candidate.Metadata["semantic_role"]),
			SemanticRoleConfidence:       parseDiagnosticFloat(candidate.Metadata["semantic_role_confidence"]),
			SemanticRoleIntent:           parseDiagnosticFloat(candidate.Metadata["semantic_role_intent"]),
			SemanticOriginAlignment:      parseDiagnosticFloat(candidate.Metadata["semantic_origin_alignment"]),
			SemanticDerivativeAlignment:  parseDiagnosticFloat(candidate.Metadata["semantic_derivative_alignment"]),
			ClusterID:                    strings.TrimSpace(candidate.Metadata["cluster_id"]),
			ClusterSize:                  parseDiagnosticInt(candidate.Metadata["cluster_size"]),
			LateInteractionScore:         parseDiagnosticFloat(candidate.Metadata["late_interaction_score"]),
			LateInteractionConfidence:    parseDiagnosticFloat(candidate.Metadata["late_interaction_confidence"]),
			SemanticEvidenceSimilarity:   parseDiagnosticFloat(candidate.Metadata["semantic_evidence_similarity"]),
			SemanticEvidenceBoost:        parseDiagnosticFloat(candidate.Metadata["semantic_evidence_boost"]),
			SemanticOriginSimilarity:     parseDiagnosticFloat(candidate.Metadata["semantic_origin_similarity"]),
			SemanticDerivativeSimilarity: parseDiagnosticFloat(candidate.Metadata["semantic_derivative_similarity"]),
			SemanticCommunitySimilarity:  parseDiagnosticFloat(candidate.Metadata["semantic_community_similarity"]),
			SemanticAuthorityBoost:       parseDiagnosticFloat(candidate.Metadata["semantic_authority_boost"]),
			SemanticAuthorityPenalty:     parseDiagnosticFloat(candidate.Metadata["semantic_authority_penalty"]),
			SemanticCommunityPenalty:     parseDiagnosticFloat(candidate.Metadata["semantic_community_penalty"]),
			SemanticQuorumProviderCount:  parseDiagnosticInt(candidate.Metadata["semantic_quorum_provider_count"]),
			SemanticCalibrationScore:     parseDiagnosticFloat(candidate.Metadata["semantic_calibration_score"]),
			SemanticProvenanceIdentity:   parseDiagnosticFloat(candidate.Metadata["semantic_provenance_identity"]),
			SemanticProvenanceTopic:      parseDiagnosticFloat(candidate.Metadata["semantic_provenance_topic"]),
			SemanticProvenanceBoost:      parseDiagnosticFloat(candidate.Metadata["semantic_provenance_boost"]),
			SemanticProvenancePenalty:    parseDiagnosticFloat(candidate.Metadata["semantic_provenance_penalty"]),
			SemanticFamilyEvidenceCount:  parseDiagnosticInt(candidate.Metadata["semantic_family_evidence_count"]),
			SemanticFamilyEvidenceStrong: parseDiagnosticInt(candidate.Metadata["semantic_family_evidence_strong"]),
			SemanticFamilyProvenance:     parseDiagnosticInt(candidate.Metadata["semantic_family_evidence_provenance"]),
			SemanticFamilyEvidence:       parseDiagnosticFloat(candidate.Metadata["semantic_family_evidence_support"]),
			SemanticNearTieMerit:         parseDiagnosticFloat(candidate.Metadata["semantic_near_tie_merit"]),
			SemanticNearTieBoost:         parseDiagnosticFloat(candidate.Metadata["semantic_near_tie_boost"]),
			Reasons:                      append([]string{}, candidate.Reason...),
		})
	}
	return out
}

func parseDiagnosticFloat(raw string) float64 {
	value, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return value
}

func parseDiagnosticInt(raw string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(raw))
	return value
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
