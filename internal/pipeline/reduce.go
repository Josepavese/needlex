package pipeline

import (
	"fmt"
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
	walker.walk(body, pathState{})

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

func (w *domWalker) walk(node *html.Node, state pathState) {
	siblingCounts := map[string]int{}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}

		tag := strings.ToLower(child.Data)
		if shouldSkipNode(child, tag) {
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

		w.walk(child, nextState)
	}
}

func (s pathState) clone() pathState {
	return pathState{
		path: append([]string{}, s.path...),
	}
}

func shouldSkipNode(node *html.Node, tag string) bool {
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
		if (key == "class" || key == "id" || key == "role") && isNoiseHint(value) {
			return true
		}
	}
	return false
}

func isNoiseHint(value string) bool {
	hints := []string{
		"cookie", "consent", "banner", "promo", "advert", "ads",
		"nav", "footer", "header", "sidebar", "comment", "modal", "popup",
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

func headingLevel(tag string) int {
	if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
		return int(tag[1] - '0')
	}
	return 0
}
