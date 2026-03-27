package intel

import (
	"strings"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

type FormatResult struct {
	Text           string
	Invocation     core.ModelInvocation
	AdditionalRisk []string
}

func Format(cfg config.Config, decision Decision, chunk core.Chunk, profile string) FormatResult {
	if decision.Lane < 3 {
		return FormatResult{Text: chunk.Text}
	}

	text := strings.Join(strings.Fields(strings.TrimSpace(chunk.Text)), " ")
	if profile == core.ProfileTiny && len(text) > 160 {
		text = text[:160]
		text = strings.TrimSpace(text)
	}
	if text != "" && !strings.HasSuffix(text, ".") {
		text += "."
	}

	result := FormatResult{
		Text: text,
		Invocation: core.ModelInvocation{
			Model:     cfg.Models.Formatter,
			Purpose:   "formatter_policy",
			TokensIn:  estimateTokens(chunk.Text),
			TokensOut: estimateTokens(text),
			LatencyMS: 0,
		},
	}
	if len(text) < len(strings.TrimSpace(chunk.Text)) {
		result.AdditionalRisk = []string{"formatter_compaction"}
	}
	return result
}
