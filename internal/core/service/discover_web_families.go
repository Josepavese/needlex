package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/core/webdiscover"
	"github.com/josepavese/needlex/internal/intel"
)

func (s *Service) semanticDisambiguateCandidateFamilies(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	if len(candidates) < 2 || strings.TrimSpace(goal) == "" {
		return candidates
	}
	families := make(map[string][]DiscoverCandidate)
	order := make([]string, 0)
	for _, candidate := range candidates {
		family, ok := webdiscover.CandidateFamily(candidate.URL)
		if !ok {
			family = strings.TrimSpace(candidate.URL)
		}
		if _, ok := families[family]; !ok {
			order = append(order, family)
		}
		families[family] = append(families[family], candidate)
	}
	if len(order) < 2 {
		return candidates
	}
	top := candidates[0]
	second := candidates[1]
	if top.Score-second.Score > 0.25 {
		return candidates
	}

	semanticCandidates := make([]intel.SemanticCandidate, 0, len(order))
	for _, family := range order {
		group := families[family]
		var texts []string
		limit := min(len(group), 3)
		for i := 0; i < limit; i++ {
			texts = append(texts, discoverycore.JoinNonEmpty(
				discoverycore.CompactSemanticText(group[i].Metadata["host_root_title"], 160),
				discoverycore.CompactSemanticText(group[i].Metadata["host_root_context"], 260),
				discoverycore.CompactSemanticText(group[i].Metadata["page_title"], 160),
				discoverycore.CompactSemanticText(group[i].Metadata["web_ir_context"], 320),
				discoverycore.CompactSemanticText(group[i].Label, 160),
			))
		}
		semanticCandidates = append(semanticCandidates, intel.SemanticCandidate{
			ID:   family,
			Text: discoverycore.JoinNonEmpty(texts...),
		})
	}
	scored := s.scoreSemanticFamilyIntent(ctx, goal, semanticCandidates)
	if len(scored) == 0 {
		return candidates
	}
	byFamily := make(map[string]float64, len(scored))
	for _, item := range scored {
		byFamily[item.ID] = item.Similarity
	}
	out := append([]DiscoverCandidate{}, candidates...)
	for i := range out {
		family, ok := webdiscover.CandidateFamily(out[i].URL)
		if !ok {
			family = strings.TrimSpace(out[i].URL)
		}
		if similarity, ok := byFamily[family]; ok && similarity > 0 {
			out[i].Score += similarity * 0.90
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "semantic_family_alignment")
		}
	}
	discoverycore.SortCandidates(out)
	return out
}

func (s *Service) applySemanticProvenanceBalance(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	if len(candidates) < 2 || strings.TrimSpace(goal) == "" {
		return candidates
	}
	window := min(len(candidates), webCandidateLimit)
	identityScores, topicScores := semanticProvenanceExistingScores(candidates[:window])
	if len(identityScores) < 2 {
		identityScores = mergeSemanticScoreMaps(identityScores, s.scoreCandidateSetToGoal(ctx, goal, semanticProvenanceIdentityCandidates(candidates[:window])))
	}
	if len(topicScores) < 2 {
		topicScores = mergeSemanticScoreMaps(topicScores, s.scoreCandidateSetToGoal(ctx, goal, semanticProvenanceTopicCandidates(candidates[:window])))
	}
	if len(identityScores) == 0 && len(topicScores) == 0 {
		return candidates
	}
	maxIdentity := maxSemanticScore(identityScores)
	if maxIdentity < 0.04 {
		return candidates
	}
	out := append([]DiscoverCandidate{}, candidates...)
	for i := 0; i < window; i++ {
		identity := identityScores[out[i].URL]
		topic := topicScores[out[i].URL]
		boost, penalty := semanticProvenanceAdjustment(identity, topic, maxIdentity)
		if boost == 0 && penalty == 0 {
			continue
		}
		out[i].Score += boost - penalty
		if boost > 0 {
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "semantic_provenance_identity")
		}
		if penalty > 0 {
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "semantic_topic_without_identity_penalty")
		}
		out[i].Metadata = discoverycore.MergeMetadata(out[i].Metadata, map[string]string{
			"semantic_provenance_identity": strconv.FormatFloat(identity, 'f', 3, 64),
			"semantic_provenance_topic":    strconv.FormatFloat(topic, 'f', 3, 64),
			"semantic_provenance_boost":    strconv.FormatFloat(boost, 'f', 3, 64),
			"semantic_provenance_penalty":  strconv.FormatFloat(penalty, 'f', 3, 64),
		})
	}
	discoverycore.SortCandidates(out)
	return out
}

type semanticFamilyIntentAggregate struct {
	Family        string
	IdentityParts []string
	TopicParts    []string
	Count         int
	Provenance    int
	Origin        float64
	Derivative    float64
	Topic         float64
}

type semanticFamilyIntentScore struct {
	Family     string
	Score      float64
	Identity   float64
	Topic      float64
	Merit      float64
	Boost      float64
	Origin     float64
	Derivative float64
	Count      int
	Provenance int
}

func (s *Service) applySemanticFamilyIntentRecovery(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	if len(candidates) < 2 || strings.TrimSpace(goal) == "" || s.semantic == nil {
		return candidates
	}
	window := min(len(candidates), webCandidateLimit)
	semanticCandidates, aggregates := semanticFamilyIntentCandidates(candidates[:window])
	if len(aggregates) < 2 || len(semanticCandidates) < 2 {
		return candidates
	}
	scored := s.scoreSemanticFamilyIntent(ctx, goal, semanticCandidates)
	merits := semanticFamilyIntentMerits(scored, aggregates)
	if len(merits) < 2 {
		return candidates
	}
	topFamily, ok := webdiscover.CandidateFamily(candidates[0].URL)
	if !ok {
		return candidates
	}
	top := merits[topFamily]
	if top.Family == "" {
		return candidates
	}
	mean := meanSemanticFamilyIntentMerit(merits)
	out := append([]DiscoverCandidate{}, candidates...)
	for i := 0; i < window; i++ {
		family, ok := webdiscover.CandidateFamily(out[i].URL)
		if !ok {
			continue
		}
		score := merits[family]
		if score.Family == "" {
			continue
		}
		boost := semanticFamilyIntentBoost(score, top, mean, out[i], candidates[0])
		score.Boost = max(boost, 0)
		out[i].Metadata = discoverycore.MergeMetadata(out[i].Metadata, semanticFamilyIntentMetadata(score))
		if boost <= 0 {
			continue
		}
		out[i].Score += boost
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "semantic_family_intent_recovery")
	}
	discoverycore.SortCandidates(out)
	return out
}

func (s *Service) scoreSemanticFamilyIntent(ctx context.Context, goal string, candidates []intel.SemanticCandidate) []intel.SemanticScore {
	if s.semantic == nil || !seedlessQueryHasTimeLeft(ctx, 2*time.Second) {
		return nil
	}
	scored, err := s.semantic.Score(ctx, goal, candidates)
	if err != nil || len(scored) == 0 {
		if !seedlessQueryHasTimeLeft(ctx, 5*time.Second) {
			return nil
		}
		semantic := intel.NewSemanticAligner(s.cfg, s.httpClient)
		scored, err = semantic.Score(ctx, goal, candidates)
		if err != nil || len(scored) == 0 {
			return nil
		}
	}
	return scored
}

func semanticFamilyIntentCandidates(candidates []DiscoverCandidate) ([]intel.SemanticCandidate, map[string]semanticFamilyIntentAggregate) {
	aggregates := map[string]semanticFamilyIntentAggregate{}
	order := []string{}
	for _, candidate := range candidates {
		family, ok := webdiscover.CandidateFamily(candidate.URL)
		if !ok || strings.TrimSpace(family) == "" {
			continue
		}
		item := aggregates[family]
		if item.Family == "" {
			item.Family = family
			order = append(order, family)
		}
		item.Count++
		item.Origin = max(item.Origin, maxMetadataFloat(candidate.Metadata, "semantic_origin_alignment", "semantic_provenance_identity"))
		item.Derivative = max(item.Derivative, maxMetadataFloat(candidate.Metadata, "semantic_derivative_alignment", "semantic_community_similarity"))
		item.Topic = max(item.Topic, maxMetadataFloat(candidate.Metadata, "semantic_provenance_topic", "semantic_goal_similarity", "candidate_goal_similarity", "semantic_evidence_similarity", "late_interaction_score"))
		if candidateHasAnyReason(candidate, "semantic_root_identity_probe", "host_root_identity_probe", "host_root_candidate", "identity_reference", "semantic_custodian_alignment", "semantic_quorum_provider_fusion", "semantic_family_evidence_mass", "semantic_provenance_identity", "candidate_identity_alignment", "same_host_canonical_root", "same_family_canonical_root") {
			item.Provenance++
		}
		item.IdentityParts = append(item.IdentityParts, semanticProvenanceIdentityText(candidate))
		item.TopicParts = append(item.TopicParts, semanticProvenanceTopicText(candidate))
		aggregates[family] = item
	}
	out := make([]intel.SemanticCandidate, 0, len(order))
	for _, family := range order {
		identityText := discoverycore.JoinNonEmpty(aggregates[family].IdentityParts...)
		if strings.TrimSpace(identityText) != "" {
			out = append(out, intel.SemanticCandidate{
				ID:   semanticFamilyIntentIdentityID(family),
				Text: discoverycore.CompactSemanticText(identityText, 900),
			})
		}
		topicText := discoverycore.JoinNonEmpty(aggregates[family].TopicParts...)
		if strings.TrimSpace(topicText) != "" {
			out = append(out, intel.SemanticCandidate{
				ID:   semanticFamilyIntentTopicID(family),
				Text: discoverycore.CompactSemanticText(topicText, 900),
			})
		}
	}
	return out, aggregates
}
