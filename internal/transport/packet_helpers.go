package transport

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/josepavese/needlex/internal/core"
	coreservice "github.com/josepavese/needlex/internal/core/service"
)

const compactChunkLimit = 5

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		value = cleanDisplayString(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func topChunkConfidence(chunks []coreservice.AgentChunk) float64 {
	for _, chunk := range chunks {
		if chunk.Confidence > 0 {
			return chunk.Confidence
		}
	}
	return 0
}

func topCandidateReasons(selectedURL string, candidates []coreservice.AgentCandidate) []string {
	selectedURL = strings.TrimSpace(selectedURL)
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.URL) == selectedURL {
			return append([]string{}, candidate.Reason...)
		}
	}
	if len(candidates) > 0 {
		return append([]string{}, candidates[0].Reason...)
	}
	return nil
}

func deriveSummary(title string, chunks []coreservice.AgentChunk) string {
	cleanTitle := cleanDisplayString(title)
	for _, chunk := range chunks {
		text := cleanDisplayString(chunk.Text)
		if text == "" {
			continue
		}
		summary := summarizeChunkHead(text)
		if shouldPrefixTitleInSummary(cleanTitle, summary) {
			return summarizeChunkHead(cleanTitle + ". " + summary)
		}
		return summary
	}
	return cleanTitle
}

func summarizeChunkHead(text string) string {
	text = cleanDisplayString(text)
	if text == "" {
		return ""
	}
	if idx := strings.Index(text, "\n\n"); idx >= 0 {
		text = text[:idx]
	}
	text = strings.Join(strings.Fields(text), " ")
	const maxChars = 220
	if len(text) <= maxChars {
		return text
	}
	cut := strings.LastIndex(text[:maxChars], " ")
	if cut < 80 {
		cut = maxChars
	}
	return strings.TrimSpace(text[:cut]) + "..."
}

func compactChunkSelection(chunks []coreservice.AgentChunk) []coreservice.AgentChunk {
	if len(chunks) == 0 {
		return nil
	}
	selected := make([]coreservice.AgentChunk, 0, minInt(len(chunks), compactChunkLimit))
	anchorStrong := false
	for _, chunk := range chunks {
		if len(selected) >= compactChunkLimit {
			break
		}
		if isRedundantCompactChunk(chunk, selected) {
			continue
		}
		if anchorStrong && isLowUtilityCompactChunk(chunk) {
			continue
		}
		selected = append(selected, chunk)
		if compactChunkUtility(chunk) >= 0.68 {
			anchorStrong = true
		}
	}
	if len(selected) == 0 {
		return append([]coreservice.AgentChunk{}, chunks[:minInt(len(chunks), compactChunkLimit)]...)
	}
	return selected
}

func isRedundantCompactChunk(candidate coreservice.AgentChunk, selected []coreservice.AgentChunk) bool {
	text := cleanDisplayString(candidate.Text)
	if text == "" {
		return false
	}
	path := strings.Join(cleanDisplayPath(candidate.HeadingPath), " > ")
	for _, existing := range selected {
		existingText := cleanDisplayString(existing.Text)
		if existingText == "" {
			continue
		}
		if text == existingText {
			return true
		}
		if containmentRatio(text, existingText) >= 0.82 {
			return true
		}
		if commonPrefixRatio(text, existingText) >= 0.74 {
			if path == strings.Join(cleanDisplayPath(existing.HeadingPath), " > ") || path == "" {
				return true
			}
		}
	}
	return false
}

func containmentRatio(a, b string) float64 {
	shorter, longer := a, b
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if shorter == "" {
		return 0
	}
	if strings.Contains(longer, shorter) {
		return float64(len(shorter)) / float64(len(longer))
	}
	return 0
}

func commonPrefixRatio(a, b string) float64 {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 || len(br) == 0 {
		return 0
	}
	limit := minInt(len(ar), len(br))
	count := 0
	for count < limit && ar[count] == br[count] {
		count++
	}
	return float64(count) / float64(limit)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isLowUtilityCompactChunk(chunk coreservice.AgentChunk) bool {
	return compactChunkUtility(chunk) < 0.42
}

func compactChunkUtility(chunk coreservice.AgentChunk) float64 {
	text := cleanDisplayString(chunk.Text)
	if text == "" {
		return 0
	}
	weight := 0.0
	runeLen := len([]rune(text))
	switch {
	case runeLen >= 180 && runeLen <= 700:
		weight += 0.42
	case runeLen >= 100:
		weight += 0.28
	case runeLen >= 60:
		weight += 0.16
	default:
		weight += 0.06
	}
	switch len(cleanDisplayPath(chunk.HeadingPath)) {
	case 0:
		weight += 0.02
	case 1:
		weight += 0.14
	case 2:
		weight += 0.08
	default:
		weight += 0.02
	}
	punctuation := sentencePunctuationCountForDisplay(text)
	switch {
	case punctuation >= 2:
		weight += 0.20
	case punctuation == 1:
		weight += 0.12
	}
	if averageLineLengthForDisplay(text) >= 48 {
		weight += 0.10
	}
	if len(cleanDisplayPath(chunk.HeadingPath)) >= 3 && runeLen <= 180 {
		weight -= 0.22
	}
	if isNavLikeTokenLine(text, chunk.HeadingPath) {
		weight -= 0.26
	}
	if weight < 0 {
		return 0
	}
	if weight > 1 {
		return 1
	}
	return weight
}

func shouldPrefixTitleInSummary(title, summary string) bool {
	if title == "" || summary == "" {
		return false
	}
	if len([]rune(summary)) > 110 {
		return false
	}
	lowerTitle := strings.ToLower(title)
	lowerSummary := strings.ToLower(summary)
	if strings.Contains(lowerSummary, lowerTitle) || strings.Contains(lowerTitle, lowerSummary) {
		return false
	}
	return true
}

func sentencePunctuationCountForDisplay(text string) int {
	return strings.Count(text, ".") + strings.Count(text, "!") + strings.Count(text, "?") + strings.Count(text, ";") + strings.Count(text, ":")
}

func averageLineLengthForDisplay(text string) float64 {
	lines := strings.Split(text, "\n")
	count := 0
	total := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		count++
		total += len([]rune(line))
	}
	if count == 0 {
		return 0
	}
	return float64(total) / float64(count)
}

func isNavLikeTokenLine(text string, headingPath []string) bool {
	if sentencePunctuationCountForDisplay(text) != 0 {
		return false
	}
	if len(cleanDisplayPath(headingPath)) > 1 {
		return false
	}
	runeLen := len([]rune(text))
	if runeLen < 60 || runeLen > 180 {
		return false
	}
	tokens := strings.Fields(text)
	if len(tokens) < 8 {
		return false
	}
	if averageTokenLengthForDisplay(tokens) > 10 {
		return false
	}
	return true
}

func averageTokenLengthForDisplay(tokens []string) float64 {
	if len(tokens) == 0 {
		return 0
	}
	total := 0
	for _, token := range tokens {
		total += len([]rune(strings.TrimSpace(token)))
	}
	return float64(total) / float64(len(tokens))
}

func cleanDisplayPath(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if cleaned := cleanDisplayString(value); cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return out
}

func cleanDisplayString(value string) string {
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	prevSpace := false
	for _, r := range value {
		switch {
		case unicode.IsSpace(r):
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		case unicode.IsControl(r) || unicode.In(r, unicode.Cf):
			continue
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	cleaned := strings.TrimSpace(b.String())
	cleaned = trimSpaceBeforePunctuation(cleaned)
	return cleaned
}

func trimSpaceBeforePunctuation(value string) string {
	if value == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if isTightPunctuation(r) && b.Len() > 0 {
			s := b.String()
			if strings.HasSuffix(s, " ") {
				_, size := utf8.DecodeLastRuneInString(s)
				b.Reset()
				b.WriteString(s[:len(s)-size])
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isTightPunctuation(r rune) bool {
	switch r {
	case ',', '.', ':', ';', '!', '?':
		return true
	default:
		return false
	}
}

func deriveReadUncertainty(chunks []coreservice.AgentChunk, webIR core.WebIR) compactUncertainty {
	reasons := make([]string, 0, 4)
	confidence := topChunkConfidence(chunks)
	switch {
	case confidence > 0 && confidence < 0.72:
		reasons = append(reasons, "low_top_chunk_confidence")
	case confidence > 0 && confidence < 0.84:
		reasons = append(reasons, "moderate_top_chunk_confidence")
	}
	if webIR.Signals.HeadingRatio > 0 && webIR.Signals.HeadingRatio < 0.12 {
		reasons = append(reasons, "weak_heading_structure")
	}
	if webIR.Signals.ShortTextRatio >= 0.88 {
		reasons = append(reasons, "fragmented_page_surface")
	}
	if webIR.Signals.EmbeddedNodeCount >= 12 {
		reasons = append(reasons, "embedded_heavy_surface")
	}
	return compactUncertainty{
		Level:   uncertaintyLevel(reasons),
		Reasons: reasons,
	}
}

func deriveQueryUncertainty(chunks []coreservice.AgentChunk, candidates []coreservice.AgentCandidate, selectedURL string, webIR core.WebIR) compactUncertainty {
	reasons := deriveReadUncertainty(chunks, webIR).Reasons
	if delta, ok := selectedCandidateDelta(candidates, selectedURL); ok {
		switch {
		case delta <= 0.08:
			reasons = appendUniqueStrings(reasons, "ambiguous_candidate_selection")
		case delta <= 0.18:
			reasons = appendUniqueStrings(reasons, "narrow_candidate_margin")
		}
	}
	if len(candidates) == 1 {
		reasons = appendUniqueStrings(reasons, "single_candidate_path")
	}
	return compactUncertainty{
		Level:   uncertaintyLevel(reasons),
		Reasons: reasons,
	}
}

func uncertaintyLevel(reasons []string) string {
	switch len(reasons) {
	case 0:
		return "low"
	case 1, 2:
		return "medium"
	default:
		return "high"
	}
}

func selectedCandidateDelta(candidates []coreservice.AgentCandidate, selectedURL string) (float64, bool) {
	selectedURL = strings.TrimSpace(selectedURL)
	if len(candidates) == 0 {
		return 0, false
	}
	selectedScore := candidates[0].Score
	nextScore := 0.0
	foundSelected := false
	for i, candidate := range candidates {
		if strings.TrimSpace(candidate.URL) == selectedURL {
			selectedScore = candidate.Score
			foundSelected = true
			if i == 0 && len(candidates) > 1 {
				nextScore = candidates[1].Score
			}
			break
		}
	}
	if !foundSelected {
		selectedScore = candidates[0].Score
		foundSelected = true
		if len(candidates) > 1 {
			nextScore = candidates[1].Score
		}
	}
	if !foundSelected || len(candidates) < 2 {
		return 0, false
	}
	if nextScore == 0 && len(candidates) > 1 && strings.TrimSpace(candidates[0].URL) != selectedURL {
		nextScore = candidates[0].Score
	}
	return selectedScore - nextScore, true
}

func appendUniqueStrings(existing []string, incoming ...string) []string {
	seen := make(map[string]struct{}, len(existing))
	out := append([]string{}, existing...)
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	for _, value := range incoming {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
