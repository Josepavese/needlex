package discovery

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

func WebSearchURL(baseURL, goal string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse web discover base url: %w", err)
	}
	query := parsed.Query()
	query.Set("q", goal)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func WebSearchProviders(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		value := strings.TrimSpace(part)
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

func ProviderName(baseURL string) string {
	switch strings.TrimSpace(baseURL) {
	case "brave://search":
		return "brave"
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return "web_search_bootstrap"
	}
	return parsed.Hostname()
}

func IsBraveProvider(baseURL string) bool {
	return strings.EqualFold(strings.TrimSpace(baseURL), "brave://search")
}

func IsDuckDuckGoProvider(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	return host == "duckduckgo.com" || host == "html.duckduckgo.com" || host == "lite.duckduckgo.com"
}

func LooksLikeDuckDuckGoAnomaly(rawHTML string) bool {
	lower := strings.ToLower(rawHTML)
	return strings.Contains(lower, "unfortunately, bots use duckduckgo too") ||
		strings.Contains(lower, "anomaly-modal") ||
		strings.Contains(lower, "/anomaly.js?") ||
		(strings.Contains(lower, "email us") && strings.Contains(lower, "duckduckgo"))
}

func ExtractSearchResults(rawHTML, baseURL string) []LinkCandidate {
	root, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	var out []LinkCandidate
	seen := map[string]struct{}{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "a") && looksLikeSearchResultAnchor(node) {
			href := attrValue(node, "href")
			label := nodeText(node)
			resolved, ok := resolveSearchResultURL(base, href)
			if ok && label != "" {
				if _, exists := seen[resolved]; !exists {
					seen[resolved] = struct{}{}
					out = append(out, LinkCandidate{URL: resolved, Label: label})
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return out
}

func hasClass(node *html.Node, className string) bool {
	classes := strings.Fields(attrValue(node, "class"))
	for _, class := range classes {
		if class == className {
			return true
		}
	}
	return false
}

func hasAncestorClass(node *html.Node, className string) bool {
	for current := node; current != nil; current = current.Parent {
		if hasClass(current, className) {
			return true
		}
	}
	return false
}

func looksLikeSearchResultAnchor(node *html.Node) bool {
	href := strings.TrimSpace(attrValue(node, "href"))
	if href == "" {
		return false
	}
	if hasClass(node, "result__a") {
		return true
	}
	lower := strings.ToLower(href)
	return strings.Contains(lower, "/l/?") ||
		strings.Contains(lower, "uddg=") ||
		hasAncestorClass(node, "b_algo")
}

func resolveSearchResultURL(base *url.URL, href string) (string, bool) {
	if strings.TrimSpace(href) == "" {
		return "", false
	}
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", false
	}
	resolved := base.ResolveReference(ref)
	if redirected, ok := searchRedirectTarget(resolved); ok {
		resolved = redirected
	}
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", false
	}
	if base.Host != "" && strings.EqualFold(resolved.Host, base.Host) {
		return "", false
	}
	return resolved.String(), true
}

func searchRedirectTarget(resolved *url.URL) (*url.URL, bool) {
	query := resolved.Query()
	for _, key := range []string{"uddg", "u", "url", "q"} {
		target, ok := decodeSearchRedirectValue(query.Get(key))
		if ok {
			return target, true
		}
	}
	return nil, false
}

func decodeSearchRedirectValue(raw string) (*url.URL, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false
	}
	if unescaped, err := url.QueryUnescape(value); err == nil {
		value = strings.TrimSpace(unescaped)
	}
	if target, ok := parseHTTPURL(value); ok {
		return target, true
	}
	candidates := []string{value}
	if len(value) > 2 {
		candidates = append(candidates, value[2:])
	}
	for _, candidate := range candidates {
		if target, ok := decodeBase64HTTPURL(candidate); ok {
			return target, true
		}
	}
	return nil, false
}

func decodeBase64HTTPURL(raw string) (*url.URL, bool) {
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		decoded, err := enc.DecodeString(raw)
		if err != nil {
			continue
		}
		if target, ok := parseHTTPURL(string(decoded)); ok {
			return target, true
		}
	}
	return nil, false
}

func parseHTTPURL(raw string) (*url.URL, bool) {
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		return nil, false
	}
	return target, true
}

func attrValue(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func nodeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return strings.TrimSpace(node.Data)
	}
	parts := make([]string, 0)
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			text := strings.TrimSpace(current.Data)
			if text != "" {
				parts = append(parts, text)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(parts, " ")
}
