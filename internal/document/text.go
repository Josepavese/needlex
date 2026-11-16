package document

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	maxBlockRunes = 1000
)

var errOutputLimit = errors.New("document output limit reached")

func normalizePageText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	// Embedded fonts commonly emit presentation-form ligatures. The read
	// contract promises searchable text, not the font's glyph ids, so normalize
	// the common Latin forms without changing the raw PDF hash.
	text = strings.NewReplacer(
		"\ufb00", "ff",
		"\ufb01", "fi",
		"\ufb02", "fl",
		"\ufb03", "ffi",
		"\ufb04", "ffl",
	).Replace(text)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// pageBlocks preserves reading order while bounding one semantic node. It never
// invents text: an empty page remains empty and is reported as no_text.
func pageBlocks(text string, remaining int) ([]string, error) {
	if remaining <= 0 {
		return nil, errOutputLimit
	}
	paragraphs := strings.Split(text, "\n\n")
	blocks := make([]string, 0, len(paragraphs))
	used := 0
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		for _, block := range splitParagraph(paragraph, maxBlockRunes) {
			if used >= remaining {
				return blocks, errOutputLimit
			}
			block = truncateRunes(block, remaining-used)
			if strings.TrimSpace(block) == "" {
				continue
			}
			blocks = append(blocks, block)
			used += utf8Len(block)
		}
	}
	return blocks, nil
}

func splitParagraph(paragraph string, limit int) []string {
	if limit <= 0 || utf8Len(paragraph) <= limit {
		return []string{paragraph}
	}
	lines := strings.Split(paragraph, "\n")
	blocks := make([]string, 0, len(lines))
	var builder strings.Builder
	flush := func() {
		block := strings.TrimSpace(builder.String())
		if block != "" {
			blocks = append(blocks, block)
		}
		builder.Reset()
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		if builder.Len() > 0 && utf8Len(builder.String())+1+utf8Len(line) > limit {
			flush()
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(line)
	}
	flush()
	if len(blocks) == 0 {
		return []string{paragraph}
	}
	return blocks
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8Len(text) <= limit {
		return text
	}
	runes := []rune(text)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return strings.TrimSpace(string(runes))
}

func utf8Len(text string) int {
	return utf8.RuneCountInString(text)
}
