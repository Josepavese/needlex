package pipeline

import (
	"fmt"
	htmlstd "html"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var noiseTags = map[string]struct{}{
	"aside":    {},
	"button":   {},
	"footer":   {},
	"form":     {},
	"header":   {},
	"iframe":   {},
	"input":    {},
	"label":    {},
	"nav":      {},
	"noscript": {},
	"script":   {},
	"style":    {},
	"svg":      {},
}

var textTags = map[string]string{
	"h1":         "heading",
	"h2":         "heading",
	"h3":         "heading",
	"h4":         "heading",
	"h5":         "heading",
	"h6":         "heading",
	"p":          "paragraph",
	"li":         "list_item",
	"blockquote": "paragraph",
	"pre":        "code",
	"code":       "code",
	"td":         "table_cell",
	"th":         "table_cell",
}

type Reducer struct{}

func (Reducer) Reduce(page RawPage) (SimplifiedDOM, error) {
	return Reducer{}.ReduceProfile(page, "standard")
}

func (Reducer) ReduceProfile(page RawPage, profile string) (SimplifiedDOM, error) {
	root, err := html.Parse(strings.NewReader(page.HTML))
	if err != nil {
		return SimplifiedDOM{}, fmt.Errorf("parse html: %w", err)
	}

	body := findElement(root, "body")
	if body == nil {
		body = root
	}

	walker := domWalker{
		title: extractTitle(root),
	}
	walker.walk(body, pathState{}, profile)
	if len(walker.nodes) < 2 {
		walker.nodes = append(walker.nodes, extractEmbeddedPayloadNodes(root)...)
	}

	return SimplifiedDOM{
		URL:   page.FinalURL,
		Title: walker.title,
		Nodes: walker.nodes,
	}, nil
}

type domWalker struct {
	title string
	nodes []SimplifiedNode
}

type pathState struct {
	path []string
}

func (w *domWalker) walk(node *html.Node, state pathState, profile string) {
	siblingCounts := map[string]int{}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}

		tag := strings.ToLower(child.Data)
		if shouldSkipNode(child, tag, profile) {
			continue
		}

		nextState := state.clone()
		siblingCounts[tag]++
		path := append(append([]string{}, nextState.path...), fmt.Sprintf("%s[%d]", tag, siblingCounts[tag]))
		nextState.path = path

		if kind, ok := textTags[tag]; ok {
			text := normalizeWhitespace(extractText(child))
			if text != "" {
				w.nodes = append(w.nodes, SimplifiedNode{
					Path:         "/" + strings.Join(path, "/"),
					Tag:          tag,
					Kind:         kind,
					Text:         text,
					Depth:        len(path),
					HeadingLevel: headingLevel(tag),
				})
			}
		}

		w.walk(child, nextState, profile)
	}
}

func (s pathState) clone() pathState {
	return pathState{
		path: append([]string{}, s.path...),
	}
}

func shouldSkipNode(node *html.Node, tag, profile string) bool {
	if _, blocked := noiseTags[tag]; blocked {
		return true
	}
	for _, attr := range node.Attr {
		key := strings.ToLower(attr.Key)
		value := strings.ToLower(attr.Val)
		if key == "hidden" {
			return true
		}
		if key == "aria-hidden" && value == "true" {
			return true
		}
		if (key == "class" || key == "id" || key == "role") && isNoiseHint(value, profile) {
			return true
		}
	}
	return false
}

func isNoiseHint(value, profile string) bool {
	hints := []string{
		"cookie", "consent", "banner", "promo", "advert", "ads",
		"nav", "footer", "header", "sidebar", "comment", "modal", "popup",
	}
	switch strings.TrimSpace(strings.ToLower(profile)) {
	case "aggressive":
		hints = append(hints, "related", "share", "newsletter", "social", "hero", "toolbar")
	case "forum":
		hints = append(hints, "trending", "reaction", "avatar", "signature", "profile-card")
	}
	for _, hint := range hints {
		if strings.Contains(value, hint) {
			return true
		}
	}
	return false
}

func extractTitle(root *html.Node) string {
	titleNode := findElement(root, "title")
	if titleNode == nil {
		return ""
	}
	return normalizeWhitespace(extractText(titleNode))
}

func findElement(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && strings.EqualFold(node.Data, name) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		found := findElement(child, name)
		if found != nil {
			return found
		}
	}
	return nil
}

func extractText(node *html.Node) string {
	var parts []string
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			if text := normalizeWhitespace(current.Data); text != "" {
				parts = append(parts, text)
			}
		}
		if current.Type == html.ElementNode {
			tag := strings.ToLower(current.Data)
			if _, blocked := noiseTags[tag]; blocked {
				return
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(parts, " ")
}

func normalizeWhitespace(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

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
			text := normalizeWhitespace(scriptText(node))
			if text != "" {
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
	if strings.Contains(lower, "{") || strings.Contains(lower, "function(") {
		return true
	}
	if strings.Count(lower, "http") > 4 {
		return true
	}
	return false
}

func headingLevel(tag string) int {
	if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
		return int(tag[1] - '0')
	}
	return 0
}
