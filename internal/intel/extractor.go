package intel

import (
	"strings"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

type ExtractResult struct {
	Text           string
	Invocation     core.ModelInvocation
	AdditionalRisk []string
}

func Extract(cfg config.Config, decision Decision, chunk core.Chunk, objective string) ExtractResult {
	if decision.Lane < 2 {
		return ExtractResult{Text: chunk.Text}
	}

	text := strings.TrimSpace(chunk.Text)
	best := bestSentence(text, objective)
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

func bestSentence(text, objective string) string {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return ""
	}
	tokens := objectiveTokens(objective)
	if len(tokens) == 0 {
		return sentences[0]
	}

	best := sentences[0]
	bestScore := -1
	for _, sentence := range sentences {
		score := 0
		haystack := strings.ToLower(sentence)
		for _, token := range tokens {
			if strings.Contains(haystack, token) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			best = sentence
		}
	}
	return best
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

func estimateTokens(text string) int {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return 0
	}
	return len(fields)
}
