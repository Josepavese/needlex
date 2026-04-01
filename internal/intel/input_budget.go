package intel

import "strings"

func estimateTokens(text string) int { return len(strings.Fields(strings.TrimSpace(text))) }

func trimTextTokens(text string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) <= maxTokens {
		return strings.TrimSpace(text)
	}
	return strings.Join(fields[:maxTokens], " ")
}

func clampHeadingPath(items []string) []string {
	if len(items) <= 2 {
		return append([]string{}, items...)
	}
	return append([]string{}, items[:2]...)
}

func boundedTextBudget(remaining, divisor, ceiling, floor int) int {
	value := remaining / divisor
	if value > ceiling {
		value = ceiling
	}
	if value < floor {
		value = floor
	}
	return value
}

func compactResolveAmbiguityInput(input ResolveAmbiguityInput, maxInputTokens int) ResolveAmbiguityInput {
	if maxInputTokens <= 0 {
		return input
	}
	out := ResolveAmbiguityInput{
		Objective:    strings.TrimSpace(input.Objective),
		FailureClass: strings.TrimSpace(input.FailureClass),
		Candidates:   make([]AmbiguityCandidate, 0, len(input.Candidates)),
	}
	remaining := maxInputTokens - estimateTokens(out.Objective) - estimateTokens(out.FailureClass)
	if remaining < 64 {
		remaining = 64
	}
	for idx, candidate := range input.Candidates {
		if idx == 4 {
			break
		}
		textBudget := boundedTextBudget(remaining, maxIntBudget(1, len(input.Candidates)-idx), 48, 16)
		text := trimTextTokens(candidate.Text, textBudget)
		if text == "" {
			text = trimTextTokens(candidate.Text, 16)
		}
		out.Candidates = append(out.Candidates, AmbiguityCandidate{
			ChunkID:     candidate.ChunkID,
			Fingerprint: candidate.Fingerprint,
			Text:        text,
			HeadingPath: clampHeadingPath(candidate.HeadingPath),
			Score:       candidate.Score,
			Confidence:  candidate.Confidence,
			RiskFlags:   append([]string{}, candidate.RiskFlags...),
			IRSignals:   append([]string{}, candidate.IRSignals...),
		})
		remaining -= estimateTokens(text)
	}
	if len(out.Candidates) < 2 {
		return input
	}
	return out
}

func maxIntBudget(left, right int) int {
	if left > right {
		return left
	}
	return right
}
