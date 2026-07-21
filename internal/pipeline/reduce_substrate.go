package pipeline

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

func inferSubstrateClass(page RawPage) string {
	if sourceKind := strings.ToLower(strings.TrimSpace(page.SourceKind)); sourceKind != "" {
		switch {
		case strings.Contains(sourceKind, "render"):
			return "rendered_html"
		case strings.Contains(sourceKind, "markdown") || strings.Contains(sourceKind, "llms"):
			return "agent_markdown"
		}
	}
	haystack := strings.ToLower(strings.TrimSpace(page.HTML))
	if haystack == "" {
		return "generic_content"
	}
	stats := analyzeHTMLSubstrate(haystack)
	if stats.looksClientRendered() {
		return "client_rendered_app"
	}
	if stats.looksThemeHeavy() {
		return "theme_heavy_site"
	}
	return "generic_content"
}

type htmlSubstrateStats struct {
	bodyElements             int
	bodyVisibleTextChars     int
	emptyMountContainers     int
	customElementCount       int
	scriptCount              int
	stylesheetCount          int
	semanticContainerCount   int
	decorativeContainerCount int
	mediaAssetCount          int
}

func analyzeHTMLSubstrate(rawHTML string) htmlSubstrateStats {
	root, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return htmlSubstrateStats{bodyVisibleTextChars: utf8.RuneCountInString(stripRoughMarkupText(rawHTML))}
	}
	stats := htmlSubstrateStats{}
	walkSubstrate(root, false, false, &stats)
	return stats
}

func walkSubstrate(node *html.Node, hidden, inBody bool, stats *htmlSubstrateStats) {
	if node == nil {
		return
	}
	nextHidden := hidden
	nextInBody := inBody
	if node.Type == html.ElementNode {
		tag := strings.ToLower(strings.TrimSpace(node.Data))
		if tag == "body" {
			nextInBody = true
		}
		switch tag {
		case "script", "style", "noscript", "template", "svg", "canvas":
			nextHidden = true
		}
		switch tag {
		case "script":
			stats.scriptCount++
		case "link":
			if attrHasToken(node.Attr, "rel", "stylesheet") {
				stats.stylesheetCount++
			}
		case "article", "section", "main", "p", "li", "table", "pre", "blockquote":
			stats.semanticContainerCount++
		case "div", "span", "i":
			stats.decorativeContainerCount++
		case "img", "picture", "video", "source":
			stats.mediaAssetCount++
		}
		if inBody && node.Parent != nil && node.Parent.Type == html.ElementNode && strings.EqualFold(node.Parent.Data, "body") {
			stats.bodyElements++
			if isPotentialMountContainer(node) && visibleTextChars(node) == 0 {
				stats.emptyMountContainers++
			}
		}
		if isCustomElementTag(tag) {
			stats.customElementCount++
			if visibleTextChars(node) == 0 {
				stats.emptyMountContainers++
			}
		}
	}
	if node.Type == html.TextNode && inBody && !hidden {
		stats.bodyVisibleTextChars += utf8.RuneCountInString(strings.TrimSpace(node.Data))
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkSubstrate(child, nextHidden, nextInBody, stats)
	}
}

func (s htmlSubstrateStats) looksClientRendered() bool {
	if s.bodyVisibleTextChars > 260 {
		if s.emptyMountContainers > 0 && s.scriptCount >= 2 && s.semanticContainerCount <= 2 {
			return true
		}
		if s.scriptCount >= 6 && s.semanticContainerCount <= 1 && s.bodyElements <= 12 {
			return true
		}
		return false
	}
	if s.scriptCount == 0 && s.customElementCount == 0 {
		return false
	}
	if s.emptyMountContainers > 0 && s.semanticContainerCount <= 1 {
		return true
	}
	if s.bodyVisibleTextChars < 90 && s.scriptCount >= 2 && s.semanticContainerCount <= 1 {
		return true
	}
	if s.bodyVisibleTextChars < 160 && s.scriptCount >= 4 && s.bodyElements <= 10 {
		return true
	}
	return false
}

func (s htmlSubstrateStats) looksThemeHeavy() bool {
	if s.bodyVisibleTextChars < 180 || s.semanticContainerCount == 0 {
		return false
	}
	assetWeight := s.scriptCount + s.stylesheetCount + s.mediaAssetCount
	if assetWeight < 6 {
		return false
	}
	if s.decorativeContainerCount >= 3 {
		return true
	}
	if s.decorativeContainerCount >= max(6, s.semanticContainerCount*2) {
		return true
	}
	return assetWeight >= 10
}

func isPotentialMountContainer(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}
	tag := strings.ToLower(strings.TrimSpace(node.Data))
	if isCustomElementTag(tag) {
		return true
	}
	switch tag {
	case "div", "main", "section":
		return true
	default:
		return false
	}
}

func visibleTextChars(node *html.Node) int {
	total := 0
	var walk func(*html.Node, bool)
	walk = func(current *html.Node, hidden bool) {
		if current == nil {
			return
		}
		nextHidden := hidden
		if current.Type == html.ElementNode {
			switch strings.ToLower(strings.TrimSpace(current.Data)) {
			case "script", "style", "noscript", "template", "svg", "canvas":
				nextHidden = true
			}
		}
		if current.Type == html.TextNode && !hidden {
			total += utf8.RuneCountInString(strings.TrimSpace(current.Data))
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child, nextHidden)
		}
	}
	walk(node, false)
	return total
}

func isCustomElementTag(tag string) bool {
	return strings.Contains(tag, "-")
}

func attrHasToken(attrs []html.Attribute, key, token string) bool {
	for _, attr := range attrs {
		if !strings.EqualFold(attr.Key, key) {
			continue
		}
		for _, field := range strings.Fields(strings.ToLower(attr.Val)) {
			if field == strings.ToLower(token) {
				return true
			}
		}
	}
	return false
}

func stripRoughMarkupText(rawHTML string) string {
	var out strings.Builder
	inTag := false
	for _, r := range rawHTML {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(out.String())
}
