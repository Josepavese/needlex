package pipeline

import (
	"fmt"
	"strings"
)

func markdownNodes(text string) ([]SimplifiedNode, string) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	nodes := make([]SimplifiedNode, 0, len(lines)/3)
	counts := map[string]int{}
	title := ""
	inFence := false
	fence := []string{}
	paragraph := []string{}
	list := []string{}
	table := []string{}

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		appendMarkdownNode(&nodes, counts, "p", "paragraph", normalizeWhitespace(strings.Join(paragraph, " ")), 0)
		paragraph = paragraph[:0]
	}
	flushList := func() {
		if len(list) == 0 {
			return
		}
		appendMarkdownNode(&nodes, counts, "li", "list_item", normalizeWhitespace(strings.Join(list, "\n")), 0)
		list = list[:0]
	}
	flushTable := func() {
		if len(table) == 0 {
			return
		}
		appendMarkdownNode(&nodes, counts, "table", "table_cell", strings.TrimSpace(strings.Join(table, "\n")), 0)
		table = table[:0]
	}
	flushFence := func() {
		if len(fence) == 0 {
			return
		}
		appendMarkdownNode(&nodes, counts, "pre", "code", strings.TrimSpace(strings.Join(fence, "\n")), 0)
		fence = fence[:0]
	}

	frontmatter := false
	for i, rawLine := range lines {
		line := strings.TrimRight(rawLine, " \t")
		trimmed := strings.TrimSpace(line)
		if i == 0 && trimmed == "---" {
			frontmatter = true
			continue
		}
		if frontmatter {
			if trimmed == "---" {
				frontmatter = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if inFence {
				inFence = false
				flushFence()
				continue
			}
			flushParagraph()
			flushList()
			flushTable()
			inFence = true
			continue
		}
		if inFence {
			fence = append(fence, line)
			continue
		}
		if trimmed == "" {
			flushParagraph()
			flushList()
			flushTable()
			continue
		}
		if level, heading := markdownHeading(trimmed); level > 0 {
			flushParagraph()
			flushList()
			flushTable()
			if title == "" && level == 1 {
				title = heading
			}
			appendMarkdownNode(&nodes, counts, "h"+fmt.Sprint(level), "heading", heading, level)
			continue
		}
		if markdownTableLine(trimmed) {
			flushParagraph()
			flushList()
			table = append(table, trimmed)
			continue
		}
		if markdownListLine(trimmed) {
			flushParagraph()
			flushTable()
			list = append(list, normalizeMarkdownListItem(trimmed))
			continue
		}
		flushList()
		flushTable()
		paragraph = append(paragraph, trimmed)
	}
	flushFence()
	flushParagraph()
	flushList()
	flushTable()
	return nodes, title
}

func appendMarkdownNode(nodes *[]SimplifiedNode, counts map[string]int, tag, kind, text string, headingLevel int) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	counts[tag]++
	*nodes = append(*nodes, SimplifiedNode{
		Path:         fmt.Sprintf("/markdown/%s[%d]", tag, counts[tag]),
		Tag:          tag,
		Kind:         kind,
		Text:         text,
		Depth:        1,
		HeadingLevel: headingLevel,
	})
}

func markdownHeading(line string) (int, string) {
	hashes := 0
	for hashes < len(line) && line[hashes] == '#' {
		hashes++
	}
	if hashes == 0 || hashes > 6 || hashes >= len(line) || line[hashes] != ' ' {
		return 0, ""
	}
	return hashes, strings.TrimSpace(line[hashes+1:])
}

func markdownTableLine(line string) bool {
	return strings.Count(line, "|") >= 2
}

func markdownListLine(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return true
	}
	if len(line) < 4 {
		return false
	}
	dot := strings.Index(line, ". ")
	if dot <= 0 || dot > 3 {
		return false
	}
	for _, r := range line[:dot] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizeMarkdownListItem(line string) string {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	if dot := strings.Index(line, ". "); dot > 0 {
		return strings.TrimSpace(line[dot+2:])
	}
	return line
}
