package queryreview

import (
	"strconv"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

type Diagnostic struct {
	URL                            string   `json:"url"`
	Score                          float64  `json:"score,omitempty"`
	ResourceClass                  string   `json:"resource_class,omitempty"`
	SemanticRole                   string   `json:"semantic_role,omitempty"`
	SemanticRoleConfidence         float64  `json:"semantic_role_confidence,omitempty"`
	SemanticRoleIntent             float64  `json:"semantic_role_intent,omitempty"`
	SemanticOriginAlignment        float64  `json:"semantic_origin_alignment,omitempty"`
	SemanticDerivativeAlignment    float64  `json:"semantic_derivative_alignment,omitempty"`
	ClusterID                      string   `json:"cluster_id,omitempty"`
	ClusterSize                    int      `json:"cluster_size,omitempty"`
	LateInteractionScore           float64  `json:"late_interaction_score,omitempty"`
	LateInteractionConfidence      float64  `json:"late_interaction_confidence,omitempty"`
	SemanticEvidenceSimilarity     float64  `json:"semantic_evidence_similarity,omitempty"`
	SemanticEvidenceBoost          float64  `json:"semantic_evidence_boost,omitempty"`
	SemanticOriginSimilarity       float64  `json:"semantic_origin_similarity,omitempty"`
	SemanticDerivativeSimilarity   float64  `json:"semantic_derivative_similarity,omitempty"`
	SemanticCommunitySimilarity    float64  `json:"semantic_community_similarity,omitempty"`
	SemanticAuthorityBoost         float64  `json:"semantic_authority_boost,omitempty"`
	SemanticAuthorityPenalty       float64  `json:"semantic_authority_penalty,omitempty"`
	SemanticCommunityPenalty       float64  `json:"semantic_community_penalty,omitempty"`
	SemanticQuorumProviderCount    int      `json:"semantic_quorum_provider_count,omitempty"`
	SemanticCalibrationScore       float64  `json:"semantic_calibration_score,omitempty"`
	SemanticProvenanceIdentity     float64  `json:"semantic_provenance_identity,omitempty"`
	SemanticProvenanceTopic        float64  `json:"semantic_provenance_topic,omitempty"`
	SemanticProvenanceBoost        float64  `json:"semantic_provenance_boost,omitempty"`
	SemanticProvenancePenalty      float64  `json:"semantic_provenance_penalty,omitempty"`
	SemanticFamilyIntentScore      float64  `json:"semantic_family_intent_score,omitempty"`
	SemanticFamilyIntentIdentity   float64  `json:"semantic_family_intent_identity,omitempty"`
	SemanticFamilyIntentTopic      float64  `json:"semantic_family_intent_topic,omitempty"`
	SemanticFamilyIntentMerit      float64  `json:"semantic_family_intent_merit,omitempty"`
	SemanticFamilyIntentBoost      float64  `json:"semantic_family_intent_boost,omitempty"`
	SemanticFamilyIntentOrigin     float64  `json:"semantic_family_intent_origin,omitempty"`
	SemanticFamilyIntentDerivative float64  `json:"semantic_family_intent_derivative,omitempty"`
	SemanticFamilyIntentCount      int      `json:"semantic_family_intent_count,omitempty"`
	SemanticFamilyIntentProvenance int      `json:"semantic_family_intent_provenance,omitempty"`
	SemanticFamilyEvidenceCount    int      `json:"semantic_family_evidence_count,omitempty"`
	SemanticFamilyEvidenceStrong   int      `json:"semantic_family_evidence_strong,omitempty"`
	SemanticFamilyProvenance       int      `json:"semantic_family_evidence_provenance,omitempty"`
	SemanticFamilyEvidence         float64  `json:"semantic_family_evidence_support,omitempty"`
	SemanticNearTieMerit           float64  `json:"semantic_near_tie_merit,omitempty"`
	SemanticNearTieBoost           float64  `json:"semantic_near_tie_boost,omitempty"`
	Reasons                        []string `json:"reasons,omitempty"`
}

func Diagnostics(candidates []discoverycore.Candidate, limit int) []Diagnostic {
	if limit <= 0 || limit > len(candidates) {
		limit = len(candidates)
	}
	out := make([]Diagnostic, 0, limit)
	for _, candidate := range candidates[:limit] {
		out = append(out, FromCandidate(candidate))
	}
	return out
}

func FromCandidate(candidate discoverycore.Candidate) Diagnostic {
	value := func(key string) float64 {
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(candidate.Metadata[key]), 64)
		return parsed
	}
	integer := func(key string) int {
		parsed, _ := strconv.Atoi(strings.TrimSpace(candidate.Metadata[key]))
		return parsed
	}
	return Diagnostic{
		URL:                            strings.TrimSpace(candidate.URL),
		Score:                          candidate.Score,
		ResourceClass:                  strings.TrimSpace(candidate.Metadata["resource_class"]),
		SemanticRole:                   strings.TrimSpace(candidate.Metadata["semantic_role"]),
		SemanticRoleConfidence:         value("semantic_role_confidence"),
		SemanticRoleIntent:             value("semantic_role_intent"),
		SemanticOriginAlignment:        value("semantic_origin_alignment"),
		SemanticDerivativeAlignment:    value("semantic_derivative_alignment"),
		ClusterID:                      strings.TrimSpace(candidate.Metadata["cluster_id"]),
		ClusterSize:                    integer("cluster_size"),
		LateInteractionScore:           value("late_interaction_score"),
		LateInteractionConfidence:      value("late_interaction_confidence"),
		SemanticEvidenceSimilarity:     value("semantic_evidence_similarity"),
		SemanticEvidenceBoost:          value("semantic_evidence_boost"),
		SemanticOriginSimilarity:       value("semantic_origin_similarity"),
		SemanticDerivativeSimilarity:   value("semantic_derivative_similarity"),
		SemanticCommunitySimilarity:    value("semantic_community_similarity"),
		SemanticAuthorityBoost:         value("semantic_authority_boost"),
		SemanticAuthorityPenalty:       value("semantic_authority_penalty"),
		SemanticCommunityPenalty:       value("semantic_community_penalty"),
		SemanticQuorumProviderCount:    integer("semantic_quorum_provider_count"),
		SemanticCalibrationScore:       value("semantic_calibration_score"),
		SemanticProvenanceIdentity:     value("semantic_provenance_identity"),
		SemanticProvenanceTopic:        value("semantic_provenance_topic"),
		SemanticProvenanceBoost:        value("semantic_provenance_boost"),
		SemanticProvenancePenalty:      value("semantic_provenance_penalty"),
		SemanticFamilyIntentScore:      value("semantic_family_intent_score"),
		SemanticFamilyIntentIdentity:   value("semantic_family_intent_identity"),
		SemanticFamilyIntentTopic:      value("semantic_family_intent_topic"),
		SemanticFamilyIntentMerit:      value("semantic_family_intent_merit"),
		SemanticFamilyIntentBoost:      value("semantic_family_intent_boost"),
		SemanticFamilyIntentOrigin:     value("semantic_family_intent_origin"),
		SemanticFamilyIntentDerivative: value("semantic_family_intent_derivative"),
		SemanticFamilyIntentCount:      integer("semantic_family_intent_count"),
		SemanticFamilyIntentProvenance: integer("semantic_family_intent_provenance"),
		SemanticFamilyEvidenceCount:    integer("semantic_family_evidence_count"),
		SemanticFamilyEvidenceStrong:   integer("semantic_family_evidence_strong"),
		SemanticFamilyProvenance:       integer("semantic_family_evidence_provenance"),
		SemanticFamilyEvidence:         value("semantic_family_evidence_support"),
		SemanticNearTieMerit:           value("semantic_near_tie_merit"),
		SemanticNearTieBoost:           value("semantic_near_tie_boost"),
		Reasons:                        append([]string{}, candidate.Reason...),
	}
}
