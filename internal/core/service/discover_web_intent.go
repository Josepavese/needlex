package service

import (
	"strconv"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/webdiscover"
	"github.com/josepavese/needlex/internal/intel"
)

func semanticFamilyIntentMerits(scored []intel.SemanticScore, aggregates map[string]semanticFamilyIntentAggregate) map[string]semanticFamilyIntentScore {
	identityScores := map[string]float64{}
	topicScores := map[string]float64{}
	for _, item := range scored {
		id := strings.TrimSpace(item.ID)
		switch {
		case strings.HasPrefix(id, semanticFamilyIntentIdentityPrefix):
			family := strings.TrimSpace(strings.TrimPrefix(id, semanticFamilyIntentIdentityPrefix))
			identityScores[family] = max(identityScores[family], max(item.Similarity, 0))
		case strings.HasPrefix(id, semanticFamilyIntentTopicPrefix):
			family := strings.TrimSpace(strings.TrimPrefix(id, semanticFamilyIntentTopicPrefix))
			topicScores[family] = max(topicScores[family], max(item.Similarity, 0))
		}
	}
	out := map[string]semanticFamilyIntentScore{}
	for family, aggregate := range aggregates {
		if family == "" || aggregate.Family == "" {
			continue
		}
		identity := max(identityScores[family], aggregate.Origin)
		topic := max(topicScores[family], aggregate.Topic)
		if identity <= 0 && topic <= 0 {
			continue
		}
		originAdvantage := max(aggregate.Origin-aggregate.Derivative, 0)
		derivativeAdvantage := max(aggregate.Derivative-aggregate.Origin, 0)
		merit := identity*1.10 + topic*0.18 + originAdvantage*0.50 - derivativeAdvantage*0.25 + min(float64(aggregate.Provenance), 2)*0.04
		out[family] = semanticFamilyIntentScore{
			Family:     family,
			Score:      identity,
			Identity:   identity,
			Topic:      topic,
			Merit:      merit,
			Origin:     aggregate.Origin,
			Derivative: aggregate.Derivative,
			Count:      aggregate.Count,
			Provenance: aggregate.Provenance,
		}
	}
	return out
}

const (
	semanticFamilyIntentIdentityPrefix = "identity:"
	semanticFamilyIntentTopicPrefix    = "topic:"
)

func semanticFamilyIntentIdentityID(family string) string {
	return semanticFamilyIntentIdentityPrefix + strings.TrimSpace(family)
}

func semanticFamilyIntentTopicID(family string) string {
	return semanticFamilyIntentTopicPrefix + strings.TrimSpace(family)
}

func meanSemanticFamilyIntentMerit(scores map[string]semanticFamilyIntentScore) float64 {
	if len(scores) == 0 {
		return 0
	}
	sum := 0.0
	for _, score := range scores {
		sum += score.Merit
	}
	return sum / float64(len(scores))
}

func semanticFamilyIntentBoost(score, top semanticFamilyIntentScore, mean float64, candidate, leader DiscoverCandidate) float64 {
	if score.Family == top.Family {
		return 0
	}
	advantage := score.Merit - top.Merit
	if advantage < 0.04 {
		return 0
	}
	strongEmbeddingRecovery := semanticFamilyIntentStrongEmbeddingRecovery(score, top, advantage)
	if !semanticFamilyIntentRecoverable(score) && !strongEmbeddingRecovery {
		return 0
	}
	gap := leader.Score - candidate.Score
	if gap < 0 {
		return 0
	}
	boost := advantage*0.52 + max(score.Merit-mean, 0)*0.18
	if strongEmbeddingRecovery {
		boost = advantage*1.45 + max(score.Identity-top.Identity, 0)*0.75 + max(score.Topic-top.Topic, 0)*0.12
	}
	boost = min(boost, 0.62)
	boost = min(boost, gap+0.04)
	if boost <= 0.03 {
		return 0
	}
	return boost
}

func semanticFamilyIntentRecoverable(score semanticFamilyIntentScore) bool {
	if score.Origin >= score.Derivative && score.Origin > 0 {
		return true
	}
	if score.Provenance >= 2 {
		return true
	}
	if score.Count >= 2 && score.Provenance >= 1 {
		return true
	}
	if score.Count >= 3 {
		return true
	}
	return false
}

func semanticFamilyIntentStrongEmbeddingRecovery(score, top semanticFamilyIntentScore, advantage float64) bool {
	derivativeRelief := top.Derivative == 0 || score.Derivative <= top.Derivative-0.04
	return score.Identity >= 0.24 &&
		score.Topic >= 0.50 &&
		score.Identity-top.Identity >= 0.07 &&
		advantage >= 0.08 &&
		derivativeRelief
}

func semanticFamilyIntentMetadata(score semanticFamilyIntentScore) map[string]string {
	return map[string]string{
		"semantic_family_intent_score":      strconv.FormatFloat(score.Score, 'f', 3, 64),
		"semantic_family_intent_identity":   strconv.FormatFloat(score.Identity, 'f', 3, 64),
		"semantic_family_intent_topic":      strconv.FormatFloat(score.Topic, 'f', 3, 64),
		"semantic_family_intent_merit":      strconv.FormatFloat(score.Merit, 'f', 3, 64),
		"semantic_family_intent_boost":      strconv.FormatFloat(score.Boost, 'f', 3, 64),
		"semantic_family_intent_origin":     strconv.FormatFloat(score.Origin, 'f', 3, 64),
		"semantic_family_intent_derivative": strconv.FormatFloat(score.Derivative, 'f', 3, 64),
		"semantic_family_intent_count":      strconv.Itoa(score.Count),
		"semantic_family_intent_provenance": strconv.Itoa(score.Provenance),
	}
}

func semanticProvenanceExistingScores(candidates []DiscoverCandidate) (map[string]float64, map[string]float64) {
	identityScores := map[string]float64{}
	topicScores := map[string]float64{}
	for _, candidate := range candidates {
		if value := maxMetadataFloat(candidate.Metadata, "candidate_host_similarity", "semantic_origin_alignment"); value > 0 {
			identityScores[candidate.URL] = value
		}
		if value := maxMetadataFloat(candidate.Metadata, "candidate_page_similarity", "candidate_goal_similarity", "semantic_evidence_similarity"); value > 0 {
			topicScores[candidate.URL] = value
		}
	}
	return identityScores, topicScores
}

func mergeSemanticScoreMaps(existing, incoming map[string]float64) map[string]float64 {
	if len(existing) == 0 {
		existing = map[string]float64{}
	}
	for key, value := range incoming {
		existing[key] = max(existing[key], value)
	}
	return existing
}

func maxMetadataFloat(metadata map[string]string, keys ...string) float64 {
	best := 0.0
	for _, key := range keys {
		value, err := strconv.ParseFloat(strings.TrimSpace(metadata[key]), 64)
		if err == nil {
			best = max(best, value)
		}
	}
	return best
}

func semanticProvenanceAdjustment(identity, topic, maxIdentity float64) (float64, float64) {
	boost := 0.0
	if identity >= 0.04 {
		boost = min(identity*0.85, 0.40)
		if topic >= 0.08 {
			boost += min(topic*0.10, 0.07)
		}
		if identity >= maxIdentity-0.03 {
			boost += 0.10
		}
	}
	penalty := 0.0
	if topic >= 0.08 && identity < maxIdentity-0.025 {
		identityGap := maxIdentity - identity
		topicGap := max(0, topic-identity)
		penalty = min(identityGap*0.95+topicGap*0.25, 0.52)
	}
	return boost, penalty
}

func semanticProvenanceIdentityCandidates(candidates []DiscoverCandidate) []intel.SemanticCandidate {
	out := make([]intel.SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		text := semanticProvenanceIdentityText(candidate)
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, intel.SemanticCandidate{ID: candidate.URL, Text: text})
	}
	return out
}

func semanticProvenanceIdentityText(candidate DiscoverCandidate) string {
	identityParts := []string{
		discoverycore.CompactSemanticText(candidate.Metadata["host_root_title"], 160),
		discoverycore.CompactSemanticText(candidate.Metadata["host_root_context"], 260),
	}
	if rootURL := strings.TrimSpace(candidate.Metadata["host_root_url"]); rootURL != "" {
		identityParts = append(identityParts, "root identity "+rootURL)
	}
	if family, ok := webdiscover.CandidateFamily(candidate.URL); ok && strings.TrimSpace(family) != "" {
		identityParts = append(identityParts, "registrable family identity "+semanticHostIdentityPhrase(family))
	}
	if host, ok := discoverycore.Hostname(candidate.URL); ok && strings.TrimSpace(host) != "" {
		identityParts = append(identityParts, "host identity "+semanticHostIdentityPhrase(host))
	}
	return discoverycore.JoinNonEmpty(identityParts...)
}

func semanticHostIdentityPhrase(value string) string {
	clean := strings.Trim(strings.TrimSpace(strings.ToLower(value)), ".")
	if clean == "" {
		return ""
	}
	parts := strings.FieldsFunc(clean, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})
	return discoverycore.JoinNonEmpty(clean, strings.Join(parts, " "))
}

func semanticProvenanceTopicCandidates(candidates []DiscoverCandidate) []intel.SemanticCandidate {
	out := make([]intel.SemanticCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		text := semanticProvenanceTopicText(candidate)
		if strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, intel.SemanticCandidate{ID: candidate.URL, Text: text})
	}
	return out
}
