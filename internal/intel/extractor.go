package intel

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

type ExtractResult struct {
	Text           string
	Invocation     core.ModelInvocation
	AdditionalRisk []string
}

func Extract(cfg config.Config, semantic SemanticAligner, decision Decision, chunk core.Chunk, objective, profile string) ExtractResult {
	if decision.Lane < 2 {
		return ExtractResult{Text: chunk.Text}
	}

	text := strings.TrimSpace(chunk.Text)
	best := bestFocusSpan(cfg, semantic, text, objective, profile)
	if best == "" {
		best = text
	}
	if best != text {
		return ExtractResult{
			Text: best,
			Invocation: core.ModelInvocation{
				Model:     cfg.Models.Extractor,
				Purpose:   "extract_slm_policy",
				TokensIn:  estimateTokens(text),
				TokensOut: estimateTokens(best),
				LatencyMS: 0,
			},
			AdditionalRisk: []string{"extracted_focus_span"},
		}
	}
	return ExtractResult{
		Text: best,
		Invocation: core.ModelInvocation{
			Model:     cfg.Models.Extractor,
			Purpose:   "extract_slm_policy",
			TokensIn:  estimateTokens(text),
			TokensOut: estimateTokens(best),
			LatencyMS: 0,
		},
	}
}

func bestFocusSpan(cfg config.Config, semantic SemanticAligner, text, objective, profile string) string {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return ""
	}
	if len(sentences) == 1 || strings.TrimSpace(objective) == "" || semantic == nil {
		return sentences[0]
	}

	candidates := make([]SemanticCandidate, 0, len(sentences))
	for idx, sentence := range sentences {
		candidates = append(candidates, SemanticCandidate{
			ID:   sentenceID(idx),
			Text: sentence,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), semanticTimeout(cfg))
	defer cancel()
	scores, err := semantic.Score(ctx, objective, candidates)
	if err != nil || len(scores) == 0 {
		return sentences[0]
	}

	byID := make(map[string]float64, len(scores))
	bestID := ""
	bestScore := -1.0
	for _, score := range scores {
		byID[score.ID] = score.Similarity
		if score.Similarity > bestScore {
			bestID = score.ID
			bestScore = score.Similarity
		}
	}
	bestIdx := sentenceIndex(bestID)
	if bestIdx < 0 || bestIdx >= len(sentences) {
		return sentences[0]
	}

	maxSentences := 1
	if profile == core.ProfileDeep {
		maxSentences = 3
	}
	if maxSentences == 1 {
		return sentences[bestIdx]
	}

	selected := []int{bestIdx}
	for idx := range sentences {
		if idx == bestIdx || len(selected) == maxSentences {
			continue
		}
		score := byID[sentenceID(idx)]
		if score >= bestScore-0.08 && score >= 0.55 {
			selected = append(selected, idx)
		}
	}
	if len(selected) <= 1 {
		return sentences[bestIdx]
	}
	sortInts(selected)
	out := make([]string, 0, len(selected))
	for _, idx := range selected {
		out = append(out, sentences[idx])
	}
	return strings.Join(out, ". ")
}

func sortInts(items []int) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j] < items[i] {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

func splitSentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func sentenceID(idx int) string {
	return "sent_" + strconv.Itoa(idx)
}

func sentenceIndex(id string) int {
	if !strings.HasPrefix(id, "sent_") {
		return -1
	}
	idx, err := strconv.Atoi(strings.TrimPrefix(id, "sent_"))
	if err != nil {
		return -1
	}
	return idx
}

func semanticTimeout(cfg config.Config) time.Duration {
	timeout := cfg.Semantic.TimeoutMS
	if timeout <= 0 {
		timeout = 250
	}
	return time.Duration(timeout) * time.Millisecond
}
