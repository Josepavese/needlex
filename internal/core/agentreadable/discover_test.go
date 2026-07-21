package agentreadable

import (
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/pipeline"
)

func TestDiscoverIncludesDeclaredAndConventionalProtocolCandidates(t *testing.T) {
	page := pipeline.RawPage{
		URL:      "https://example.com/docs/reference",
		FinalURL: "https://example.com/docs/reference",
		Headers: map[string][]string{
			"Link": {`</.well-known/api-catalog>; rel="api-catalog", </llms.txt>; rel="llms"`},
		},
		HTML: `<html><head><link rel="alternate" href="/docs/reference.md"></head><body></body></html>`,
	}
	candidates := Discover(page, 24)

	if !hasCandidate(candidates, KindAPICatalog, "link_header", "https://example.com/.well-known/api-catalog") {
		t.Fatalf("expected declared api catalog candidate, got %#v", candidates)
	}
	if !hasCandidate(candidates, KindLLMSIndex, "link_header", "https://example.com/llms.txt") {
		t.Fatalf("expected declared llms candidate, got %#v", candidates)
	}
	if !hasCandidate(candidates, KindMarkdownVariant, "html_link", "https://example.com/docs/reference.md") {
		t.Fatalf("expected declared markdown alternate, got %#v", candidates)
	}
	if !hasCandidate(candidates, KindServiceDescription, "well_known_path", "https://example.com/openapi.json") {
		t.Fatalf("expected conventional OpenAPI candidate, got %#v", candidates)
	}
}

func TestRobotsSitemapsAreSameOriginOnly(t *testing.T) {
	robots := `
User-agent: *
Allow: /
Sitemap: https://example.com/sitemap.xml
Sitemap: https://other.example.net/sitemap.xml
Sitemap: /nested-sitemap.xml
`
	got := SitemapURLsFromRobots("https://example.com/docs/reference", robots)
	want := []string{"https://example.com/sitemap.xml", "https://example.com/nested-sitemap.xml"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected sitemap URLs\nwant=%#v\ngot=%#v", want, got)
	}
}

func TestCandidatesFromSitemapKeepsProtocolResources(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<urlset>
  <url><loc>https://example.com/docs/reference-agent.md</loc></url>
  <url><loc>https://example.com/openapi.json</loc></url>
  <url><loc>https://other.example.net/docs/reference-agent.md</loc></url>
  <url><loc>https://example.com/blog/post</loc></url>
</urlset>`
	candidates := CandidatesFromSitemap("https://example.com/docs/reference", "https://example.com/sitemap.xml", body, 12)

	if !hasCandidate(candidates, KindMarkdownVariant, "sitemap", "https://example.com/docs/reference-agent.md") {
		t.Fatalf("expected sitemap markdown candidate, got %#v", candidates)
	}
	if !hasCandidate(candidates, KindServiceDescription, "sitemap", "https://example.com/openapi.json") {
		t.Fatalf("expected sitemap OpenAPI candidate, got %#v", candidates)
	}
	if hasURL(candidates, "https://other.example.net/docs/reference-agent.md") || hasURL(candidates, "https://example.com/blog/post") {
		t.Fatalf("expected only same-origin protocol resources, got %#v", candidates)
	}
}

func TestCandidatesFromAPICatalogExtractsLinksetResources(t *testing.T) {
	body := `{
  "linkset": [{
    "anchor": "https://example.com/",
    "service-desc": [{"href": "/openapi.json", "type": "application/openapi+json"}],
    "service-doc": [{"href": "/docs/api.md", "type": "text/markdown"}],
    "api-catalog": [{"href": "https://other.example.net/.well-known/api-catalog"}]
  }]
}`
	candidates := CandidatesFromAPICatalog("https://example.com/.well-known/api-catalog", body, 12)

	if !hasCandidate(candidates, KindServiceDescription, "api_catalog", "https://example.com/openapi.json") {
		t.Fatalf("expected OpenAPI service description, got %#v", candidates)
	}
	if !hasCandidate(candidates, KindMarkdownVariant, "api_catalog", "https://example.com/docs/api.md") {
		t.Fatalf("expected markdown service doc, got %#v", candidates)
	}
	if hasURL(candidates, "https://other.example.net/.well-known/api-catalog") {
		t.Fatalf("expected cross-origin catalog link to be ignored, got %#v", candidates)
	}
}

func TestCandidatesFromAPICatalogExtractsYAMLLinksetResources(t *testing.T) {
	body := `
linkset:
  - anchor: https://example.com/
    service-desc:
      - href: /openapi.yaml
        type: application/openapi+yaml
    service-doc:
      - href: /docs/api.md
        type: text/markdown
`
	candidates := CandidatesFromAPICatalog("https://example.com/.well-known/api-catalog", body, 12)

	if !hasCandidate(candidates, KindServiceDescription, "api_catalog", "https://example.com/openapi.yaml") {
		t.Fatalf("expected YAML OpenAPI service description, got %#v", candidates)
	}
	if !hasCandidate(candidates, KindMarkdownVariant, "api_catalog", "https://example.com/docs/api.md") {
		t.Fatalf("expected YAML markdown service doc, got %#v", candidates)
	}
}

func TestRobotsPolicyAppliesLongestAllowDisallowMatch(t *testing.T) {
	policy := ParseRobots("https://example.com/docs/reference", `
User-agent: *
Disallow: /docs/
Allow: /docs/public/
`)
	if policy.Allows("needle-x", "https://example.com/docs/private.md") {
		t.Fatalf("expected private docs path to be disallowed")
	}
	if !policy.Allows("needle-x", "https://example.com/docs/public/reference.md") {
		t.Fatalf("expected longest allow rule to permit public docs path")
	}
}

func TestIsAgentReadablePageAcceptsOpenAPIServiceDescription(t *testing.T) {
	page := pipeline.RawPage{
		FinalURL:    "https://example.com/openapi.json",
		ContentType: "application/json",
		SourceKind:  KindServiceDescription,
		HTML:        `{"openapi":"3.1.0","info":{"title":"Example API","version":"1.0.0"},"paths":{"/widgets":{"get":{"description":"List widgets"}}}}`,
	}
	if !IsAgentReadablePage(page) {
		t.Fatalf("expected OpenAPI description to be accepted")
	}
}

func hasCandidate(candidates []Candidate, kind, declaredBy, rawURL string) bool {
	for _, candidate := range candidates {
		if candidate.Kind == kind && candidate.DeclaredBy == declaredBy && candidate.URL == rawURL {
			return true
		}
	}
	return false
}

func hasURL(candidates []Candidate, rawURL string) bool {
	for _, candidate := range candidates {
		if candidate.URL == rawURL {
			return true
		}
	}
	return false
}
