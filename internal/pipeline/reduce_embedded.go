package pipeline

import (
	"fmt"
	htmlstd "html"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

func extractEmbeddedPayloadNodes(root *html.Node) []SimplifiedNode {
	scripts := collectScriptContent(root)
	if len(scripts) == 0 {
		return nil
	}

	nodes := make([]SimplifiedNode, 0, 8)
	for scriptIndex, script := range scripts {
		if !isEmbeddedPayloadCandidate(script) {
			continue
		}

		values := extractJSONFieldValues(script)
		for valueIndex, value := range values {
			text := normalizeWhitespace(htmlText(value))
			if len(text) < 30 || isLikelyJunk(text) {
				continue
			}
			nodes = append(nodes, SimplifiedNode{
				Path:         fmt.Sprintf("/embedded/script[%d]/text[%d]", scriptIndex+1, valueIndex+1),
				Tag:          "script",
				Kind:         "paragraph",
				Text:         text,
				Depth:        3,
				HeadingLevel: 0,
			})
			if len(nodes) >= 8 {
				return nodes
			}
		}
	}
	return nodes
}

func collectScriptContent(root *html.Node) []string {
	out := []string{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "script") {
			if text := normalizeWhitespace(scriptText(node)); text != "" {
				out = append(out, text)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return out
}

func scriptText(node *html.Node) string {
	parts := []string{}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			text := strings.TrimSpace(child.Data)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, " ")
}

func isEmbeddedPayloadCandidate(value string) bool {
	lower := strings.ToLower(value)
	markers := []string{
		"window._a2s",
		"window.__next_data__",
		"application/ld+json",
		"\"description\":\"",
		"\"subtitle\":\"",
		"\"title\":\"",
		"\"business_name\":\"",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func extractJSONFieldValues(raw string) []string {
	keys := []string{
		`"description":"`,
		`"subtitle":"`,
		`"title":"`,
		`"name":"`,
		`"business_name":"`,
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, key := range keys {
		start := 0
		for len(out) < 24 {
			index := strings.Index(raw[start:], key)
			if index < 0 {
				break
			}
			begin := start + index + len(key)
			value, next, ok := readJSONStringValue(raw, begin)
			if !ok {
				start = begin
				continue
			}
			start = next
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
	}
	return out
}

func readJSONStringValue(raw string, start int) (string, int, bool) {
	if start >= len(raw) {
		return "", start, false
	}
	builder := strings.Builder{}
	escape := false
	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if escape {
			builder.WriteByte(ch)
			escape = false
			continue
		}
		if ch == '\\' {
			builder.WriteByte(ch)
			escape = true
			continue
		}
		if ch == '"' {
			rawValue := builder.String()
			unquoted, err := strconv.Unquote(`"` + rawValue + `"`)
			if err == nil {
				return unquoted, i + 1, true
			}
			return rawValue, i + 1, true
		}
		builder.WriteByte(ch)
	}
	return "", len(raw), false
}

func htmlText(value string) string {
	decoded := strings.ReplaceAll(htmlstd.UnescapeString(value), `\/`, "/")
	if !strings.Contains(decoded, "<") {
		return decoded
	}
	root, err := html.Parse(strings.NewReader("<div>" + decoded + "</div>"))
	if err != nil {
		return decoded
	}
	return extractText(root)
}

func isLikelyJunk(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "{") || strings.Contains(lower, "function(") || strings.Count(lower, "http") > 4
}
