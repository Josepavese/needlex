package pipeline

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
)

func extractStructuredDataNodes(root *xhtml.Node) []SimplifiedNode {
	scripts := collectTypedScriptContent(root, "application/ld+json")
	if len(scripts) == 0 {
		return nil
	}
	nodes := make([]SimplifiedNode, 0, 8)
	for scriptIndex, script := range scripts {
		values := jsonLDTextValues(script)
		for valueIndex, value := range values {
			text := normalizeWhitespace(html.UnescapeString(value))
			if len(text) < 30 || structuredValueLooksJunk(text) {
				continue
			}
			nodes = append(nodes, SimplifiedNode{
				Path:  fmt.Sprintf("/structured/jsonld[%d]/text[%d]", scriptIndex+1, valueIndex+1),
				Tag:   "script",
				Kind:  "paragraph",
				Text:  text,
				Depth: 3,
			})
			if len(nodes) >= 8 {
				return nodes
			}
		}
	}
	return nodes
}

func collectTypedScriptContent(root *xhtml.Node, scriptType string) []string {
	out := []string{}
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, "script") && scriptHasType(node, scriptType) {
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

func scriptHasType(node *xhtml.Node, scriptType string) bool {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, "type") && strings.EqualFold(strings.TrimSpace(attr.Val), scriptType) {
			return true
		}
	}
	return false
}

func scriptText(node *xhtml.Node) string {
	parts := []string{}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.TextNode {
			text := strings.TrimSpace(child.Data)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, " ")
}

func jsonLDTextValues(raw string) []string {
	var value any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil
	}
	out := []string{}
	seen := map[string]struct{}{}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if !looksLikeJSONLDObject(typed) {
				return
			}
			for _, key := range jsonLDTextKeys() {
				if text, ok := typed[key].(string); ok {
					addStructuredValue(&out, seen, text)
				}
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return out
}

func looksLikeJSONLDObject(value map[string]any) bool {
	if _, ok := value["@context"]; ok {
		return true
	}
	if _, ok := value["@type"]; ok {
		return true
	}
	return false
}

func jsonLDTextKeys() []string {
	return []string{
		"headline",
		"name",
		"description",
		"articleBody",
		"text",
		"abstract",
	}
}

func addStructuredValue(out *[]string, seen map[string]struct{}, value string) {
	value = strings.TrimSpace(strings.ReplaceAll(value, `\/`, "/"))
	if value == "" {
		return
	}
	if _, ok := seen[value]; ok {
		return
	}
	seen[value] = struct{}{}
	*out = append(*out, value)
}

func structuredValueLooksJunk(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "{") || strings.Contains(lower, "function(") || strings.Count(lower, "http") > 4
}
