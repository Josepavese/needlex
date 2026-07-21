package agentreadable

import (
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/josepavese/needlex/internal/pipeline"
	"golang.org/x/net/html"
)

const (
	KindMarkdownNegotiation = "markdown_content_negotiation"
	KindMarkdownVariant     = "markdown_variant"
	KindLLMSIndex           = "llms_txt"
	KindLLMSFull            = "llms_full_txt"
	KindAPICatalog          = "api_catalog"
	KindServiceDescription  = "service_description"
)

type Candidate struct {
	URL        string
	Kind       string
	DeclaredBy string
	Accept     string
	Priority   int
}

func Discover(page pipeline.RawPage, maxCandidates int) []Candidate {
	baseURL := strings.TrimSpace(page.FinalURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(page.URL)
	}
	if baseURL == "" {
		return nil
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil
	}
	out := []Candidate{}
	out = append(out, candidatesFromLinkHeaders(page.Headers, parsed)...)
	out = append(out, candidatesFromHTMLLinks(page.HTML, parsed)...)
	out = append(out, conventionalCandidates(parsed)...)
	return NormalizeCandidates(out, maxCandidates)
}

func candidatesFromLinkHeaders(headers map[string][]string, base *url.URL) []Candidate {
	values := headerValues(headers, "Link")
	out := []Candidate{}
	for _, value := range values {
		for _, link := range parseHTTPLinkHeader(value) {
			candidate, ok := candidateFromLinkedResource(link.URL, link.Rel, "link_header", base, 0)
			if ok {
				out = append(out, candidate)
			}
		}
	}
	return out
}

func candidatesFromHTMLLinks(rawHTML string, base *url.URL) []Candidate {
	if strings.TrimSpace(rawHTML) == "" {
		return nil
	}
	root, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return nil
	}
	out := []Candidate{}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && strings.EqualFold(node.Data, "link") {
			attrs := attrMap(node.Attr)
			if candidate, ok := candidateFromLinkedResource(attrs["href"], attrs["rel"], "html_link", base, 1); ok {
				out = append(out, candidate)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return out
}

func conventionalCandidates(base *url.URL) []Candidate {
	out := []Candidate{
		{
			URL:        base.String(),
			Kind:       KindMarkdownNegotiation,
			DeclaredBy: "accept_header",
			Accept:     "text/markdown, text/plain;q=0.9, text/html;q=0.3, */*;q=0.1",
			Priority:   10,
		},
	}
	if !hasExtension(base.Path, ".md", ".mdx", ".markdown") {
		out = append(out,
			Candidate{URL: withPathSuffix(base, ".md"), Kind: KindMarkdownVariant, DeclaredBy: "same_url_variant", Priority: 20},
			Candidate{URL: withPathSuffix(base, ".mdx"), Kind: KindMarkdownVariant, DeclaredBy: "same_url_variant", Priority: 22},
		)
	}
	root := *base
	root.Path = "/llms.txt"
	root.RawQuery = ""
	root.Fragment = ""
	out = append(out, Candidate{URL: root.String(), Kind: KindLLMSIndex, DeclaredBy: "well_known_path", Priority: 30})
	root.Path = "/llms-full.txt"
	out = append(out, Candidate{URL: root.String(), Kind: KindLLMSFull, DeclaredBy: "well_known_path", Priority: 35})
	for _, section := range sectionLLMSPaths(base.Path) {
		sectionURL := *base
		sectionURL.Path = section
		sectionURL.RawQuery = ""
		sectionURL.Fragment = ""
		out = append(out, Candidate{URL: sectionURL.String(), Kind: KindLLMSIndex, DeclaredBy: "section_path", Priority: 40})
	}
	apiCatalog := *base
	apiCatalog.Path = "/.well-known/api-catalog"
	apiCatalog.RawQuery = ""
	apiCatalog.Fragment = ""
	out = append(out, Candidate{URL: apiCatalog.String(), Kind: KindAPICatalog, DeclaredBy: "well_known_path", Priority: 80})
	for index, specPath := range []string{
		"/openapi.json",
		"/openapi.yaml",
		"/openapi.yml",
		"/swagger.json",
		"/swagger.yaml",
		"/swagger.yml",
		"/.well-known/openapi.json",
		"/.well-known/openapi.yaml",
	} {
		spec := *base
		spec.Path = specPath
		spec.RawQuery = ""
		spec.Fragment = ""
		out = append(out, Candidate{URL: spec.String(), Kind: KindServiceDescription, DeclaredBy: "well_known_path", Priority: 90 + index})
	}
	return out
}

func candidateFromLinkedResource(rawHref, rel, declaredBy string, base *url.URL, priorityOffset int) (Candidate, bool) {
	href := strings.TrimSpace(rawHref)
	if href == "" {
		return Candidate{}, false
	}
	resolved := resolveURL(base, href)
	if resolved == "" || !sameOrigin(base.String(), resolved) {
		return Candidate{}, false
	}
	rel = strings.ToLower(strings.TrimSpace(rel))
	switch {
	case strings.Contains(rel, "llms-full"):
		return Candidate{URL: resolved, Kind: KindLLMSFull, DeclaredBy: declaredBy, Priority: 3 + priorityOffset}, true
	case strings.Contains(rel, "llms") || strings.HasSuffix(strings.ToLower(resolved), "/llms.txt"):
		return Candidate{URL: resolved, Kind: KindLLMSIndex, DeclaredBy: declaredBy, Priority: 2 + priorityOffset}, true
	case strings.Contains(rel, "service-desc") || strings.Contains(rel, "service-doc") || strings.Contains(rel, "service-meta"):
		return Candidate{URL: resolved, Kind: KindServiceDescription, DeclaredBy: declaredBy, Priority: 60 + priorityOffset}, true
	case strings.Contains(rel, "api-catalog"):
		return Candidate{URL: resolved, Kind: KindAPICatalog, DeclaredBy: declaredBy, Priority: 50 + priorityOffset}, true
	case strings.Contains(rel, "alternate") && hasExtensionURL(resolved, ".md", ".mdx", ".markdown"):
		return Candidate{URL: resolved, Kind: KindMarkdownVariant, DeclaredBy: declaredBy, Priority: 12 + priorityOffset}, true
	}
	return Candidate{}, false
}

func headerValues(headers map[string][]string, key string) []string {
	if len(headers) == 0 {
		return nil
	}
	for candidate, values := range headers {
		if strings.EqualFold(candidate, key) {
			return append([]string{}, values...)
		}
	}
	return nil
}

type parsedLink struct {
	URL string
	Rel string
}

func parseHTTPLinkHeader(value string) []parsedLink {
	parts := splitLinkHeader(value)
	out := []parsedLink{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "<") {
			continue
		}
		end := strings.Index(part, ">")
		if end <= 1 {
			continue
		}
		link := parsedLink{URL: strings.TrimSpace(part[1:end])}
		params := strings.Split(part[end+1:], ";")
		for _, param := range params {
			key, rawValue, ok := strings.Cut(strings.TrimSpace(param), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "rel") {
				continue
			}
			link.Rel = strings.Trim(strings.TrimSpace(rawValue), `"`)
		}
		out = append(out, link)
	}
	return out
}

func NormalizeCandidates(candidates []Candidate, maxCandidates int) []Candidate {
	out := dedupeCandidates(candidates)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].URL < out[j].URL
		}
		return out[i].Priority < out[j].Priority
	})
	if maxCandidates > 0 && len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out
}

func splitLinkHeader(value string) []string {
	parts := []string{}
	start := 0
	inQuotes := false
	for i, r := range value {
		switch r {
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				parts = append(parts, value[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, value[start:])
	return parts
}

func attrMap(attrs []html.Attribute) map[string]string {
	out := map[string]string{}
	for _, attr := range attrs {
		out[strings.ToLower(attr.Key)] = strings.TrimSpace(attr.Val)
	}
	return out
}

func resolveURL(base *url.URL, raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return base.ResolveReference(parsed).String()
}

func withPathSuffix(base *url.URL, suffix string) string {
	next := *base
	next.Path += suffix
	next.RawQuery = ""
	next.Fragment = ""
	return next.String()
}

func sectionLLMSPaths(rawPath string) []string {
	clean := path.Clean("/" + strings.TrimSpace(rawPath))
	if clean == "/" {
		return nil
	}
	dir := path.Dir(clean)
	if dir == "/" || dir == "." {
		return nil
	}
	return []string{path.Join(dir, "llms.txt")}
}

func hasExtension(rawPath string, exts ...string) bool {
	ext := strings.ToLower(path.Ext(rawPath))
	for _, candidate := range exts {
		if ext == strings.ToLower(candidate) {
			return true
		}
	}
	return false
}

func hasExtensionURL(rawURL string, exts ...string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return hasExtension(parsed.Path, exts...)
}

func sameOrigin(a, b string) bool {
	left, err := url.Parse(a)
	if err != nil {
		return false
	}
	right, err := url.Parse(b)
	if err != nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

func dedupeCandidates(candidates []Candidate) []Candidate {
	seen := map[string]struct{}{}
	out := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := strings.ToLower(strings.TrimSpace(candidate.Kind + "\x00" + candidate.URL + "\x00" + candidate.Accept))
		if candidate.URL == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, candidate)
	}
	return out
}

var markdownLinkDetailPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)

type MarkdownLink struct {
	URL   string
	Label string
}

func MarkdownLinks(indexURL, markdown string) []string {
	links := MarkdownLinkDetails(indexURL, markdown)
	out := make([]string, 0, len(links))
	for _, link := range links {
		out = append(out, link.URL)
	}
	return out
}

func MarkdownLinkDetails(indexURL, markdown string) []MarkdownLink {
	base, err := url.Parse(strings.TrimSpace(indexURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := []MarkdownLink{}
	for _, match := range markdownLinkDetailPattern.FindAllStringSubmatch(markdown, -1) {
		if len(match) < 3 {
			continue
		}
		resolved := resolveURL(base, match[2])
		if resolved == "" || !sameOrigin(base.String(), resolved) || !hasExtensionURL(resolved, ".md", ".mdx", ".markdown") {
			continue
		}
		key := strings.ToLower(resolved)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, MarkdownLink{URL: resolved, Label: strings.TrimSpace(match[1])})
	}
	return out
}

func BestLinkedMarkdownFor(targetURL, indexURL, markdown string) string {
	target, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return ""
	}
	targetStem := strings.TrimSuffix(path.Clean(target.Path), path.Ext(target.Path))
	for _, link := range MarkdownLinks(indexURL, markdown) {
		parsed, err := url.Parse(link)
		if err != nil {
			continue
		}
		linkStem := strings.TrimSuffix(path.Clean(parsed.Path), path.Ext(parsed.Path))
		if strings.EqualFold(targetStem, linkStem) {
			return link
		}
	}
	return ""
}

func IsAgentReadablePage(page pipeline.RawPage) bool {
	contentType := strings.ToLower(strings.TrimSpace(page.ContentType))
	finalURL := strings.ToLower(strings.TrimSpace(page.FinalURL))
	sourceKind := strings.ToLower(strings.TrimSpace(page.SourceKind))
	if strings.Contains(contentType, "text/markdown") {
		return usefulText(page.HTML)
	}
	if strings.HasSuffix(finalURL, ".md") || strings.HasSuffix(finalURL, ".mdx") {
		if strings.Contains(contentType, "text/html") || strings.Contains(strings.ToLower(page.HTML), "<html") {
			return false
		}
		return usefulText(page.HTML) && looksLikeMarkdown(page.HTML)
	}
	if sourceKind == KindLLMSFull {
		if strings.Contains(contentType, "text/html") || strings.Contains(strings.ToLower(page.HTML), "<html") {
			return false
		}
		return usefulText(page.HTML)
	}
	if sourceKind == KindAPICatalog {
		if strings.Contains(contentType, "text/html") || strings.Contains(strings.ToLower(page.HTML), "<html") {
			return false
		}
		return usefulText(page.HTML) || looksLikeAPICatalog(page.HTML)
	}
	if sourceKind == KindServiceDescription || isServiceDescriptionURL(finalURL) {
		if strings.Contains(contentType, "text/html") || strings.Contains(strings.ToLower(page.HTML), "<html") {
			return false
		}
		return usefulText(page.HTML) || looksLikeServiceDescription(page.HTML)
	}
	return false
}

func usefulText(value string) bool {
	fields := strings.Fields(strings.TrimSpace(value))
	return len(fields) >= 12
}

func looksLikeMarkdown(value string) bool {
	signals := 0
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "# "):
			signals += 2
		case strings.HasPrefix(trimmed, "## "):
			signals++
		case strings.HasPrefix(trimmed, "- ") && strings.Contains(trimmed, "]("):
			signals++
		case strings.HasPrefix(trimmed, "```"):
			signals++
		}
		if signals >= 2 {
			return true
		}
	}
	return false
}

func RequestAccept(candidate Candidate) string {
	if strings.TrimSpace(candidate.Accept) != "" {
		return strings.TrimSpace(candidate.Accept)
	}
	if candidate.Kind == KindMarkdownVariant || candidate.Kind == KindLLMSIndex || candidate.Kind == KindLLMSFull {
		return "text/markdown, text/plain;q=0.9, */*;q=0.1"
	}
	if candidate.Kind == KindAPICatalog {
		return "application/linkset+json, application/json;q=0.9, application/yaml;q=0.8, text/yaml;q=0.7, text/plain;q=0.5, */*;q=0.1"
	}
	if candidate.Kind == KindServiceDescription {
		return "application/openapi+json, application/json;q=0.9, application/yaml;q=0.8, text/yaml;q=0.7, text/plain;q=0.5, */*;q=0.1"
	}
	return ""
}
