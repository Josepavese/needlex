package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/intel"
)

var tinyStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "before": {},
	"by": {}, "for": {}, "from": {}, "in": {}, "into": {}, "is": {}, "it": {}, "of": {},
	"on": {}, "or": {}, "that": {}, "the": {}, "their": {}, "to": {}, "when": {}, "with": {},
	"without": {},
}

func (s *Service) compactTinyText(text, objective string) (string, bool) {
	normalized := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if normalized == "" {
		return "", false
	}

	candidate := s.bestCompactSentence(normalized, objective)
	words := strings.Fields(candidate)
	if len(words) == 0 {
		return normalized, false
	}

	compactedWords := make([]string, 0, len(words))
	for _, word := range words {
		clean := trimWord(word)
		if clean == "" {
			continue
		}
		if _, ok := tinyStopwords[strings.ToLower(clean)]; ok && len(words) > 8 {
			continue
		}
		compactedWords = append(compactedWords, clean)
		if len(compactedWords) == 14 {
			break
		}
	}

	if len(compactedWords) < 6 {
		compactedWords = fallbackWords(words, 10)
	}

	compacted := strings.Join(compactedWords, " ")
	if compacted == "" {
		compacted = normalized
	}
	if !strings.HasSuffix(compacted, ".") {
		compacted += "."
	}
	return compacted, compacted != normalized
}

func (s *Service) bestCompactSentence(text, objective string) string {
	sentences := splitTinySentences(text)
	if len(sentences) == 0 {
		return text
	}
	if strings.TrimSpace(objective) == "" {
		return sentences[0]
	}
	candidates := make([]intel.SemanticCandidate, 0, len(sentences))
	for idx, sentence := range sentences {
		candidates = append(candidates, intel.SemanticCandidate{
			ID:   fmt.Sprintf("sent_%d", idx),
			Text: sentence,
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.cfg.Semantic.TimeoutMS)*time.Millisecond)
	defer cancel()
	scores, err := s.semantic.Score(ctx, objective, candidates)
	if err != nil || len(scores) == 0 {
		return sentences[0]
	}
	bestID := ""
	bestScore := -1.0
	for _, score := range scores {
		if score.Similarity > bestScore {
			bestID = score.ID
			bestScore = score.Similarity
		}
	}
	for idx, sentence := range sentences {
		if bestID == fmt.Sprintf("sent_%d", idx) {
			return sentence
		}
	}
	return sentences[0]
}

func splitTinySentences(text string) []string {
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

func trimWord(word string) string {
	return strings.Trim(word, " \t\r\n,;:!?()[]{}\"'")
}

func fallbackWords(words []string, limit int) []string {
	out := make([]string, 0, min(len(words), limit))
	for _, word := range words {
		clean := trimWord(word)
		if clean == "" {
			continue
		}
		out = append(out, clean)
		if len(out) == limit {
			break
		}
	}
	return out
}
