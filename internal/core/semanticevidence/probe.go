package semanticevidence

import (
	"context"
	"strconv"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
)

const (
	ReasonSemanticEvidenceProbe = "semantic_evidence_probe"
	ReasonSemanticOriginProbe   = "semantic_origin_probe"
	ReasonSemanticDerivative    = "semantic_derivative_surface_penalty"
	ReasonSemanticCommunity     = "semantic_community_surface_penalty"
	minEvidenceChars            = 16
	maxEvidenceChars            = 1800
)

type Config struct {
	Window        int
	MinSimilarity float64
	Scale         float64
	MaxBoost      float64
}

type Reranker struct {
	Semantic intel.SemanticAligner
	Config   Config
}

func DefaultConfig() Config {
	return Config{
		Window:        8,
		MinSimilarity: 0.04,
		Scale:         0.72,
		MaxBoost:      0.55,
	}
}

func (r Reranker) Rerank(ctx context.Context, goal string, candidates []discoverycore.Candidate) []discoverycore.Candidate {
	if strings.TrimSpace(goal) == "" || len(candidates) < 2 || r.Semantic == nil {
		return candidates
	}
	cfg := r.Config
	if cfg.Window <= 0 {
		cfg = DefaultConfig()
	}
	window := cfg.Window
	if len(candidates) < window {
		window = len(candidates)
	}
	if window < 2 {
		return candidates
	}
	semanticCandidates, evidenceChars := evidenceSemanticCandidates(candidates[:window], cfg)
	if len(semanticCandidates) == 0 {
		return candidates
	}
	scored, err := r.Semantic.Score(ctx, goal, semanticCandidates)
	if err != nil || len(scored) == 0 {
		return candidates
	}
	scores := make(map[string]float64, len(scored))
	for _, item := range scored {
		scores[item.ID] = item.Similarity
	}
	originScores := scoreByID(ctx, r.Semantic, semanticOriginProfile(), semanticCandidates)
	derivativeScores := scoreByID(ctx, r.Semantic, semanticDerivativeProfile(), semanticCandidates)
	communityScores := scoreByID(ctx, r.Semantic, semanticCommunityProfile(), semanticCandidates)
	out := append([]discoverycore.Candidate{}, candidates...)
	applyEvidenceScores(out[:window], cfg, scores, originScores, derivativeScores, communityScores, evidenceChars)
	discoverycore.SortCandidates(out)
	return out
}

func applyEvidenceScores(candidates []discoverycore.Candidate, cfg Config, goal, origin, derivative, community map[string]float64, chars map[string]int) {
	for i := range candidates {
		similarity, ok := goal[candidates[i].URL]
		if !ok {
			continue
		}
		boost := evidenceBoost(similarity, cfg)
		originScore, derivativeScore, communityScore := origin[candidates[i].URL], derivative[candidates[i].URL], community[candidates[i].URL]
		authorityBoost, authorityPenalty := authorityAdjustment(originScore, derivativeScore, hasReason(candidates[i], "web_ir_embedded"))
		communityPenalty := communityAdjustment(originScore, communityScore)
		candidates[i].Metadata = discoverycore.MergeMetadata(candidates[i].Metadata, evidenceMetadata(similarity, boost, originScore, derivativeScore, communityScore, authorityBoost, authorityPenalty, communityPenalty, chars[candidates[i].URL]))
		candidates[i].Score += boost + authorityBoost - authorityPenalty - communityPenalty
		candidates[i].Reason = appendEvidenceReasons(candidates[i].Reason, boost, authorityBoost, authorityPenalty, communityPenalty)
	}
}

func evidenceBoost(similarity float64, cfg Config) float64 {
	if similarity < cfg.MinSimilarity {
		return 0
	}
	return min(similarity*cfg.Scale, cfg.MaxBoost)
}

func evidenceMetadata(similarity, boost, origin, derivative, community, authorityBoost, authorityPenalty, communityPenalty float64, chars int) map[string]string {
	return map[string]string{
		"semantic_evidence_similarity":   formatFloat(similarity),
		"semantic_evidence_boost":        formatFloat(boost),
		"semantic_evidence_chars":        strconv.Itoa(chars),
		"semantic_origin_similarity":     formatFloat(origin),
		"semantic_derivative_similarity": formatFloat(derivative),
		"semantic_community_similarity":  formatFloat(community),
		"semantic_authority_boost":       formatFloat(authorityBoost),
		"semantic_authority_penalty":     formatFloat(authorityPenalty),
		"semantic_community_penalty":     formatFloat(communityPenalty),
	}
}

func appendEvidenceReasons(reasons []string, boost, authorityBoost, authorityPenalty, communityPenalty float64) []string {
	if boost > 0 {
		reasons = discoverycore.AppendUniqueReason(reasons, ReasonSemanticEvidenceProbe)
	}
	if authorityBoost > 0 {
		reasons = discoverycore.AppendUniqueReason(reasons, ReasonSemanticOriginProbe)
	}
	if authorityPenalty > 0 {
		reasons = discoverycore.AppendUniqueReason(reasons, ReasonSemanticDerivative)
	}
	if communityPenalty > 0 {
		reasons = discoverycore.AppendUniqueReason(reasons, ReasonSemanticCommunity)
	}
	return reasons
}

func scoreByID(ctx context.Context, semantic intel.SemanticAligner, objective string, candidates []intel.SemanticCandidate) map[string]float64 {
	scored, err := semantic.Score(ctx, objective, candidates)
	if err != nil || len(scored) == 0 {
		return nil
	}
	out := make(map[string]float64, len(scored))
	for _, item := range scored {
		if item.Similarity > 0 {
			out[item.ID] = item.Similarity
		}
	}
	return out
}

func authorityAdjustment(origin, derivative float64, embedded bool) (float64, float64) {
	embeddedPenalty := 0.0
	if embedded && origin > 0.14 {
		embeddedPenalty = 0.34
	}
	switch {
	case origin > 0.16 && origin >= derivative+0.05:
		return min(origin*1.70+(origin-derivative)*2.00, 0.95), embeddedPenalty
	case derivative > 0.16 && derivative >= origin+0.025:
		return 0, max(embeddedPenalty, min(derivative*1.05+(derivative-origin)*1.55, 0.72))
	case embedded && derivative > 0.18 && derivative >= origin-0.015:
		return 0, max(embeddedPenalty, 0.22)
	default:
		return 0, embeddedPenalty
	}
}

func communityAdjustment(origin, community float64) float64 {
	if community > 0.24 && community >= origin+0.16 {
		return min(community*0.70+(community-origin)*0.90, 0.35)
	}
	return 0
}

func semanticOriginProfile() string {
	return "Primary custodian origin surface: the responsible entity, project, institution, publisher, standard body, or product speaks for itself through its maintained official presence. Fonte ufficiale, source de reference, fuente primaria, 公式情報."
}

func semanticDerivativeProfile() string {
	return "Derivative secondary representation: tutorial, encyclopedia, mirror, aggregator, documentation browser, collection that republishes references from many projects, generated knowledge base, commentary, comparison, review, directory, external summary, community wiki, user group, forum, or third-party explanation about another source. Fonte secondaria, repubblicazione, raccolta esterna, aggregated documentation portal, 非一次情報."
}

func semanticCommunityProfile() string {
	return "Community-maintained user surface: wiki, forum, user group, community notes, unofficial support, discussion space, repository-derived knowledge base, externally maintained knowledge around another project. Comunità utenti, forum, wiki non ufficiale."
}

func hasReason(candidate discoverycore.Candidate, want string) bool {
	for _, reason := range candidate.Reason {
		if reason == want {
			return true
		}
	}
	return false
}

func evidenceSemanticCandidates(candidates []discoverycore.Candidate, cfg Config) ([]intel.SemanticCandidate, map[string]int) {
	out := make([]intel.SemanticCandidate, 0, len(candidates))
	chars := map[string]int{}
	for _, candidate := range candidates {
		text := EvidenceText(candidate)
		if len([]rune(text)) < minEvidenceChars {
			continue
		}
		out = append(out, intel.SemanticCandidate{ID: candidate.URL, Text: text})
		chars[candidate.URL] = len([]rune(text))
	}
	return out, chars
}

func EvidenceText(candidate discoverycore.Candidate) string {
	meta := candidate.Metadata
	return compactEvidenceText(discoverycore.JoinNonEmpty(
		meta["page_title"],
		meta["host_root_title"],
		meta["host_root_context"],
		meta["web_ir_context"],
		meta["source_context"],
	), maxEvidenceChars)
}

func compactEvidenceText(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return strings.TrimSpace(string(runes[:maxRunes]))
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}
