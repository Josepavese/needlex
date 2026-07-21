package service

import (
	"strconv"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/webdiscover"
)

func semanticProvenanceTopicText(candidate DiscoverCandidate) string {
	return discoverycore.JoinNonEmpty(
		discoverycore.CompactSemanticText(candidate.Metadata["page_title"], 160),
		discoverycore.CompactSemanticText(candidate.Metadata["web_ir_context"], 320),
		discoverycore.CompactSemanticText(candidate.Metadata["source_context"], 220),
		discoverycore.CompactSemanticText(candidate.Label, 160),
	)
}

func maxSemanticScore(scores map[string]float64) float64 {
	best := 0.0
	for _, score := range scores {
		best = max(best, score)
	}
	return best
}

type semanticFamilyEvidence struct {
	Count       int
	Strong      int
	Provenance  int
	SemanticSum float64
	Support     float64
}

func applySemanticFamilyEvidenceBalance(candidates []DiscoverCandidate) []DiscoverCandidate {
	if len(candidates) < 2 {
		return candidates
	}
	window := min(len(candidates), webCandidateLimit)
	families := map[string]semanticFamilyEvidence{}
	for i := 0; i < window; i++ {
		family, ok := webdiscover.CandidateFamily(candidates[i].URL)
		if !ok || strings.TrimSpace(family) == "" {
			continue
		}
		evidence := semanticCandidateEvidence(candidates[i])
		if evidence == 0 && !candidateHasAnyReason(candidates[i], "semantic_goal_alignment", "semantic_evidence_probe", "page_expand_semantic_grounding") {
			continue
		}
		item := families[family]
		item.Count++
		item.SemanticSum += evidence
		if evidence >= 0.40 || candidateHasAnyReason(candidates[i], "semantic_evidence_probe", "semantic_family_alignment") {
			item.Strong++
		}
		if candidateHasAnyReason(candidates[i], "semantic_root_identity_probe", "host_root_identity_probe", "host_root_candidate", "identity_reference", "same_host_canonical_root", "same_family_canonical_root") {
			item.Provenance++
		}
		families[family] = item
	}
	bestSupport := 0.0
	for family, item := range families {
		if item.Count < 2 || (item.Count < 3 && item.Provenance == 0) {
			continue
		}
		avg := item.SemanticSum / float64(item.Count)
		item.Support = min(float64(item.Count), 4)*0.10 + min(avg*0.16, 0.14) + min(float64(item.Strong), 3)*0.04 + min(float64(item.Provenance), 2)*0.03
		families[family] = item
		bestSupport = max(bestSupport, item.Support)
	}
	if bestSupport == 0 {
		return candidates
	}
	out := append([]DiscoverCandidate{}, candidates...)
	for i := 0; i < window; i++ {
		family, ok := webdiscover.CandidateFamily(out[i].URL)
		if !ok {
			continue
		}
		item := families[family]
		if item.Support == 0 {
			continue
		}
		out[i].Score += item.Support
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "semantic_family_evidence_mass")
		out[i].Metadata = discoverycore.MergeMetadata(out[i].Metadata, map[string]string{
			"semantic_family_evidence_count":      strconv.Itoa(item.Count),
			"semantic_family_evidence_strong":     strconv.Itoa(item.Strong),
			"semantic_family_evidence_provenance": strconv.Itoa(item.Provenance),
			"semantic_family_evidence_support":    strconv.FormatFloat(item.Support, 'f', 3, 64),
		})
	}
	discoverycore.SortCandidates(out)
	return out
}

func semanticCandidateEvidence(candidate DiscoverCandidate) float64 {
	return maxMetadataFloat(candidate.Metadata,
		"semantic_goal_similarity",
		"candidate_goal_similarity",
		"semantic_evidence_similarity",
		"late_interaction_score",
		"semantic_provenance_topic",
	)
}

func applySemanticNearTieProvenanceReview(candidates []DiscoverCandidate) []DiscoverCandidate {
	if len(candidates) < 2 {
		return candidates
	}
	window := min(len(candidates), webCandidateLimit)
	topMerit := semanticNearTieMerit(candidates[0])
	out := append([]DiscoverCandidate{}, candidates...)
	for i := 1; i < window; i++ {
		gap := candidates[0].Score - out[i].Score
		if !semanticNearTieEligible(out[i]) {
			continue
		}
		if gap < 0 || gap > semanticNearTieGapLimit(out[i]) {
			continue
		}
		merit := semanticNearTieMerit(out[i])
		meritDelta := merit - topMerit
		if meritDelta < 0.04 {
			continue
		}
		boost := min(meritDelta*0.90+0.07, 0.48)
		boost = min(boost, gap+0.04)
		if boost <= 0 {
			continue
		}
		out[i].Score += boost
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "semantic_near_tie_provenance_review")
		out[i].Metadata = discoverycore.MergeMetadata(out[i].Metadata, map[string]string{
			"semantic_near_tie_merit":       strconv.FormatFloat(merit, 'f', 3, 64),
			"semantic_near_tie_top_merit":   strconv.FormatFloat(topMerit, 'f', 3, 64),
			"semantic_near_tie_margin":      strconv.FormatFloat(gap, 'f', 3, 64),
			"semantic_near_tie_boost":       strconv.FormatFloat(boost, 'f', 3, 64),
			"semantic_near_tie_merit_delta": strconv.FormatFloat(meritDelta, 'f', 3, 64),
		})
	}
	discoverycore.SortCandidates(out)
	return out
}

func semanticNearTieGapLimit(candidate DiscoverCandidate) float64 {
	if candidateHasAnyReason(candidate, "semantic_root_identity_probe", "host_root_identity_probe", "host_root_candidate", "identity_reference", "embedded_url_provenance") {
		return 0.46
	}
	if candidateHasAnyReason(candidate, "semantic_family_evidence_mass") {
		return 0.50
	}
	if candidateHasAnyReason(candidate, "candidate_cluster_representative") {
		return 0.46
	}
	if candidateHasAnyReason(candidate, "semantic_evidence_probe", "semantic_provider_consensus") {
		return 0.42
	}
	return 0
}

func semanticNearTieEligible(candidate DiscoverCandidate) bool {
	if semanticNearTieWeakDerivativeTrap(candidate) {
		return false
	}
	if candidateHasAnyReason(candidate, "target_kind_topology_mismatch") {
		return false
	}
	if candidateHasAnyReason(candidate, "semantic_root_identity_probe", "host_root_identity_probe", "host_root_candidate", "identity_reference", "embedded_url_provenance") {
		return true
	}
	if candidateHasAnyReason(candidate, "semantic_family_evidence_mass") && discoverycore.URLPathDepth(candidate.URL) > 0 {
		return true
	}
	if candidateHasAnyReason(candidate, "candidate_cluster_representative") {
		return true
	}
	if candidateHasAnyReason(candidate, "semantic_evidence_probe", "semantic_provider_consensus") {
		return true
	}
	return false
}

func semanticNearTieWeakDerivativeTrap(candidate DiscoverCandidate) bool {
	if candidateHasAnyReason(candidate, "semantic_provenance_identity", "semantic_family_evidence_mass", "identity_reference", "candidate_identity_alignment", "semantic_custodian_alignment") {
		return false
	}
	if maxMetadataFloat(candidate.Metadata, "semantic_family_intent_provenance") >= 2 {
		return false
	}
	origin := maxMetadataFloat(candidate.Metadata, "semantic_family_intent_origin", "semantic_origin_alignment", "semantic_provenance_identity")
	derivative := maxMetadataFloat(candidate.Metadata, "semantic_family_intent_derivative", "semantic_derivative_alignment", "semantic_community_similarity")
	if !candidateHasAnyReason(candidate, "semantic_derivative_surface_penalty") && derivative < 0.14 {
		return false
	}
	return derivative > origin
}

func semanticNearTieMerit(candidate DiscoverCandidate) float64 {
	return semanticNearTiePositiveMerit(candidate) - semanticNearTiePenalty(candidate)
}

func semanticNearTiePositiveMerit(candidate DiscoverCandidate) float64 {
	merit := 0.0
	if candidateHasAnyReason(candidate, "semantic_root_identity_probe", "host_root_identity_probe", "host_root_candidate", "identity_reference", "embedded_url_provenance") {
		merit += 0.28
	}
	if candidateHasAnyReason(candidate, "same_host_canonical_root", "same_family_canonical_root") && semanticNearTieEligible(candidate) {
		merit += 0.10
	}
	if candidateHasAnyReason(candidate, "semantic_family_evidence_mass") {
		merit += min(maxMetadataFloat(candidate.Metadata, "semantic_family_evidence_support")*0.50, 0.30)
	}
	if candidateHasAnyReason(candidate, "semantic_origin_probe", "semantic_custodian_alignment") {
		merit += 0.14
	}
	if candidateHasAnyReason(candidate, "same_family_shallow_preference") {
		merit += 0.08
	}
	if candidateHasAnyReason(candidate, "web_ir_probe") {
		merit += 0.11
	} else if candidateHasAnyReason(candidate, "page_title_probe") {
		merit -= 0.04
	}
	if candidateHasAnyReason(candidate, "page_expand_child_context", "embedded_url_contextual_evidence") {
		merit += 0.05
	}
	if candidateHasAnyReason(candidate, "candidate_cluster_representative") {
		merit += 0.14
	}
	if candidateHasAnyReason(candidate, "semantic_evidence_probe") {
		merit += 0.16
	}
	if candidateHasAnyReason(candidate, "semantic_provider_consensus") {
		merit += 0.09
	}
	merit += min(semanticCandidateEvidence(candidate)*0.12, 0.10)
	return merit
}

func semanticNearTiePenalty(candidate DiscoverCandidate) float64 {
	penalty := 0.0
	if candidateHasAnyReason(candidate, "semantic_derivative_surface_penalty") {
		penalty += 0.08
	}
	if candidateHasAnyReason(candidate, "candidate_cluster_redundant") {
		penalty += 0.10
	}
	if candidateHasAnyReason(candidate, "same_family_shallow_preference", "semantic_derivative_surface_penalty") &&
		!candidateHasAnyReason(candidate, "semantic_family_evidence_mass") {
		penalty += 0.24
	}
	if candidateHasAnyReason(candidate, "web_ir_embedded") {
		penalty += 0.08
	}
	if candidateHasAnyReason(candidate, "embedded_url_untyped_resource") && !candidateHasAnyReason(candidate, "endpoint_extract_llm") {
		penalty += 0.16
	}
	if candidateHasAnyReason(candidate, "embedded_url_non_html_source") &&
		candidateHasAnyReason(candidate, "semantic_derivative_surface_penalty") &&
		!candidateHasAnyReason(candidate, "endpoint_extract_llm") {
		penalty += 0.08
	}
	if candidateHasAnyReason(candidate, "weak_canonical_root_context_penalty", "weak_recovered_family_context_penalty", "cross_family_mirror_route_penalty") {
		penalty += 0.12
	}
	if candidateHasAnyReason(candidate, "target_kind_topology_mismatch") {
		penalty += 0.24
	}
	return penalty
}
