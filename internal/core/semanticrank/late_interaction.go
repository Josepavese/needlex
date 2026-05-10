package semanticrank

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
)

const ReasonLateInteraction = "semantic_late_interaction"

type Mode string

const (
	ModeOff    Mode = "off"
	ModeActive Mode = "active"
)

type Config struct {
	Mode           Mode
	Window         int
	MaxBoost       float64
	Scale          float64
	MinScore       float64
	MinMargin      float64
	FamilyMaxBoost float64
	ReasonName     string
}

type Reranker struct {
	Semantic intel.SemanticAligner
	Config   Config
}

type Score struct {
	URL         string
	Score       float64
	Confidence  float64
	SpanCount   int
	Boost       float64
	Family      string
	FamilyMass  float64
	FamilyBoost float64
}

func DefaultConfig() Config {
	return Config{
		Mode:           ModeActive,
		Window:         6,
		MaxBoost:       0.28,
		Scale:          1.10,
		MinScore:       0.05,
		MinMargin:      0.015,
		FamilyMaxBoost: 0.18,
		ReasonName:     ReasonLateInteraction,
	}
}

func (r Reranker) Rerank(ctx context.Context, goal string, candidates []discoverycore.Candidate) []discoverycore.Candidate {
	if strings.TrimSpace(goal) == "" || len(candidates) < 2 || r.Semantic == nil {
		return candidates
	}
	cfg := normalizeConfig(r.Config)
	if cfg.Mode == ModeOff {
		return candidates
	}
	window := min(len(candidates), cfg.Window)
	spans, spanIndex := candidateSpans(candidates[:window])
	if len(spans) == 0 {
		return candidates
	}
	raw, err := r.Semantic.Score(ctx, goal, spans)
	if err != nil || len(raw) == 0 {
		return candidates
	}
	scores := reduceSpanScores(raw, spanIndex, candidates[:window], cfg)
	if len(scores) == 0 {
		return candidates
	}
	out := append([]discoverycore.Candidate{}, candidates...)
	for i := 0; i < window; i++ {
		score, ok := scores[out[i].URL]
		if !ok {
			continue
		}
		out[i].Metadata = discoverycore.MergeMetadata(out[i].Metadata, map[string]string{
			"late_interaction_score":        formatFloat(score.Score),
			"late_interaction_confidence":   formatFloat(score.Confidence),
			"late_interaction_span_count":   strconv.Itoa(score.SpanCount),
			"late_interaction_boost":        formatFloat(score.Boost),
			"late_interaction_family":       score.Family,
			"late_interaction_family_mass":  formatFloat(score.FamilyMass),
			"late_interaction_family_boost": formatFloat(score.FamilyBoost),
		})
		if score.Boost == 0 {
			continue
		}
		out[i].Score += score.Boost
		out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, cfg.ReasonName)
		if score.FamilyBoost > 0 {
			out[i].Reason = discoverycore.AppendUniqueReason(out[i].Reason, "semantic_family_late_interaction")
		}
	}
	discoverycore.SortCandidates(out)
	return out
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Mode == "" {
		cfg.Mode = def.Mode
	}
	if cfg.Window <= 0 {
		cfg.Window = def.Window
	}
	if cfg.MaxBoost <= 0 {
		cfg.MaxBoost = def.MaxBoost
	}
	if cfg.Scale <= 0 {
		cfg.Scale = def.Scale
	}
	if cfg.MinScore <= 0 {
		cfg.MinScore = def.MinScore
	}
	if cfg.MinMargin <= 0 {
		cfg.MinMargin = def.MinMargin
	}
	if cfg.FamilyMaxBoost <= 0 {
		cfg.FamilyMaxBoost = def.FamilyMaxBoost
	}
	if strings.TrimSpace(cfg.ReasonName) == "" {
		cfg.ReasonName = def.ReasonName
	}
	switch cfg.Mode {
	case ModeOff, ModeActive:
	default:
		cfg.Mode = def.Mode
	}
	return cfg
}

type candidateSpanRef struct {
	URL  string
	Kind string
}

func candidateSpans(candidates []discoverycore.Candidate) ([]intel.SemanticCandidate, map[string]candidateSpanRef) {
	out := []intel.SemanticCandidate{}
	index := map[string]candidateSpanRef{}
	for _, candidate := range candidates {
		for _, span := range semanticSpans(candidate) {
			span.Text = strings.Join(strings.Fields(span.Text), " ")
			if span.Text == "" {
				continue
			}
			id := fmt.Sprintf("%s#%s#%d", candidate.URL, span.Kind, len(out))
			out = append(out, intel.SemanticCandidate{ID: id, Text: span.Text})
			index[id] = candidateSpanRef{URL: candidate.URL, Kind: span.Kind}
		}
	}
	return out, index
}

type semanticSpan struct {
	Kind string
	Text string
}

type preliminarySpanScore struct {
	url        string
	maxScore   float64
	second     float64
	coverage   float64
	roleBoost  float64
	spanCount  int
	confidence float64
}

func semanticSpans(candidate discoverycore.Candidate) []semanticSpan {
	meta := candidate.Metadata
	spans := []semanticSpan{
		{Kind: "topic", Text: discoverycore.JoinNonEmpty(meta["page_title"], meta["web_ir_context"], meta["source_context"], strings.TrimSpace(candidate.Label))},
		{Kind: "identity", Text: discoverycore.JoinNonEmpty(meta["host_root_title"], meta["host_root_context"], meta["page_title"], strings.TrimSpace(candidate.Label))},
	}
	if role := strings.TrimSpace(meta["semantic_role"]); role != "" {
		spans = append(spans, semanticSpan{Kind: "role", Text: semanticRoleText(role)})
	}
	if class := strings.TrimSpace(meta["resource_class"]); class != "" && class != discoverycore.ResourceClassHTMLLike {
		spans = append(spans, semanticSpan{Kind: "resource", Text: resourceClassText(class)})
	}
	return spans
}

func reduceSpanScores(raw []intel.SemanticScore, refs map[string]candidateSpanRef, candidates []discoverycore.Candidate, cfg Config) map[string]Score {
	preliminary := buildPreliminarySpanScores(raw, refs, candidates, cfg)
	if len(preliminary) == 0 {
		return nil
	}
	mean := meanMaxSpanScore(preliminary)
	out := map[string]Score{}
	for _, item := range preliminary {
		score, ok := finalSpanScore(item, mean, cfg)
		if ok {
			out[item.url] = score
		}
	}
	applyFamilyMass(out, candidates, cfg)
	return out
}

func buildPreliminarySpanScores(raw []intel.SemanticScore, refs map[string]candidateSpanRef, candidates []discoverycore.Candidate, cfg Config) []preliminarySpanScore {
	byURL := spanScoresByURL(raw, refs)
	preliminary := []preliminarySpanScore{}
	for _, candidate := range candidates {
		score, ok := preliminarySpanCandidateScore(candidate, byURL[candidate.URL], cfg)
		if ok {
			preliminary = append(preliminary, score)
		}
	}
	return preliminary
}

func spanScoresByURL(raw []intel.SemanticScore, refs map[string]candidateSpanRef) map[string][]float64 {
	byURL := map[string][]float64{}
	for _, item := range raw {
		ref, ok := refs[item.ID]
		if !ok {
			continue
		}
		if item.Similarity <= 0 {
			continue
		}
		byURL[ref.URL] = append(byURL[ref.URL], item.Similarity)
	}
	return byURL
}

func preliminarySpanCandidateScore(candidate discoverycore.Candidate, values []float64, cfg Config) (preliminarySpanScore, bool) {
	if len(values) == 0 {
		return preliminarySpanScore{}, false
	}
	sort.Slice(values, func(i, j int) bool { return values[i] > values[j] })
	maxScore := values[0]
	if maxScore < cfg.MinScore {
		return preliminarySpanScore{}, false
	}
	second := 0.0
	if len(values) > 1 {
		second = values[1]
	}
	coverage := spanCoverage(values, cfg.MinScore)
	roleBoost := semanticRoleEvidenceBoost(candidate.Metadata)
	return preliminarySpanScore{
		url:        candidate.URL,
		maxScore:   maxScore,
		second:     second,
		coverage:   coverage,
		roleBoost:  roleBoost,
		spanCount:  len(values),
		confidence: maxScore - second + coverage*0.05 + max(0, roleBoost)*0.18,
	}, true
}

func meanMaxSpanScore(preliminary []preliminarySpanScore) float64 {
	mean := 0.0
	for _, score := range preliminary {
		mean += score.maxScore
	}
	return mean / float64(len(preliminary))
}

func finalSpanScore(item preliminarySpanScore, mean float64, cfg Config) (Score, bool) {
	relative := item.maxScore - mean
	boost := relative*cfg.Scale + item.coverage*0.025 + item.roleBoost
	if boost > 0 {
		boost = min(cfg.MaxBoost, boost)
	}
	if boost < 0 {
		boost = max(-cfg.MaxBoost, boost)
	}
	if boost == 0 && item.confidence < cfg.MinMargin {
		return Score{}, false
	}
	return Score{URL: item.url, Score: item.maxScore, Confidence: item.confidence, SpanCount: item.spanCount, Boost: boost}, true
}

type familyMass struct {
	count      int
	best       float64
	sum        float64
	provenance int
}

func applyFamilyMass(scores map[string]Score, candidates []discoverycore.Candidate, cfg Config) {
	families := map[string]familyMass{}
	for _, candidate := range candidates {
		score, ok := scores[candidate.URL]
		if !ok {
			continue
		}
		family := candidateFamily(candidate.URL)
		if family == "" {
			continue
		}
		item := families[family]
		item.count++
		item.best = max(item.best, score.Score)
		item.sum += score.Score
		if hasSemanticProvenance(candidate) {
			item.provenance++
		}
		families[family] = item
	}
	for _, candidate := range candidates {
		score, ok := scores[candidate.URL]
		if !ok {
			continue
		}
		family := candidateFamily(candidate.URL)
		item := families[family]
		if family == "" || item.count < 2 || item.best < cfg.MinScore {
			continue
		}
		mean := item.sum / float64(item.count)
		boost := min(cfg.FamilyMaxBoost, mean*0.08+float64(min(item.count, 4))*0.025+float64(min(item.provenance, 3))*0.025)
		if boost <= 0 {
			continue
		}
		score.Family = family
		score.FamilyMass = mean
		score.FamilyBoost = boost
		score.Boost = min(max(score.Boost+boost, -cfg.MaxBoost), cfg.MaxBoost+cfg.FamilyMaxBoost)
		scores[candidate.URL] = score
	}
}

func candidateFamily(rawURL string) string {
	if family, err := discoverycore.RegistrableDomain(rawURL); err == nil {
		return strings.TrimSpace(family)
	}
	if host, ok := discoverycore.Hostname(rawURL); ok {
		return strings.TrimSpace(host)
	}
	return ""
}

func hasSemanticProvenance(candidate discoverycore.Candidate) bool {
	for _, reason := range candidate.Reason {
		switch strings.TrimSpace(reason) {
		case "semantic_root_identity_probe",
			"host_root_candidate",
			"host_root_identity_probe",
			"identity_reference",
			"semantic_family_alignment",
			"semantic_custodian_alignment",
			"semantic_quorum_provider_fusion",
			"semantic_provider_consensus",
			"semantic_family_evidence_mass",
			"entity_family_graph_recall",
			"semantic_family_memory":
			return true
		}
	}
	return false
}

func spanCoverage(values []float64, minScore float64) float64 {
	if len(values) == 0 {
		return 0
	}
	count := 0
	for _, value := range values {
		if value >= minScore {
			count++
		}
	}
	return float64(count) / float64(len(values))
}

func semanticRoleEvidenceBoost(meta map[string]string) float64 {
	origin := parseFloat(meta["semantic_origin_alignment"])
	derivative := parseFloat(meta["semantic_derivative_alignment"])
	roleIntent := parseFloat(meta["semantic_role_intent"])
	boost := 0.0
	if origin > 0 {
		boost += min(origin*0.10, 0.08)
	}
	if derivative > origin+0.02 && roleIntent > 0 {
		boost -= min((derivative-origin)*0.12, 0.10)
	}
	return boost
}

func parseFloat(raw string) float64 {
	value, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}
