package discovery

import "testing"

func TestScoreStructuralCandidatesRemainStructureFirstWithoutSurfaceFormPromotion(t *testing.T) {
	candidates := ScoreStructuralCandidates(
		"",
		"",
		[]LinkCandidate{
			{URL: "https://curlscape.com/blog/openai-api-pricing-guide", Label: "OpenAI API pricing"},
			{URL: "https://developers.openai.com/api/pricing", Label: "OpenAI API pricing"},
		},
		nil,
	)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if contains(candidates[0].Reason, "semantic_goal_alignment") {
		t.Fatalf("expected surface-form promotion reason to be absent, got %#v", candidates[0].Reason)
	}
}

func TestScoreStructuralCandidatesPreservesSourceContextForSemanticLayers(t *testing.T) {
	candidates := ScoreStructuralCandidates(
		"",
		"",
		[]LinkCandidate{{
			URL:     "https://source.example/record",
			Label:   "Source record",
			Context: "  maintained context\nfrom the search result container\twith provenance details  ",
		}},
		nil,
	)
	if got := candidates[0].Metadata["source_context"]; got != "maintained context from the search result container with provenance details" {
		t.Fatalf("unexpected source context %q", got)
	}
}

func TestURLStructureBoostPenalizesArticleLikeDeepPaths(t *testing.T) {
	deepArticle := urlStructureBoost("https://example.com/2025/04/playwright-automation-tutorial.html")
	shallowRoot := urlStructureBoost("https://playwright.dev/")
	if deepArticle >= shallowRoot {
		t.Fatalf("expected deep article-like path to score below shallow root, article=%f root=%f", deepArticle, shallowRoot)
	}
}

func TestCanonicalURLKeyTreatsIndexHomeAsRoot(t *testing.T) {
	if !SameCanonicalURL("https://www.sqlite.org/", "https://sqlite.org/index.html") {
		t.Fatalf("expected root and index page to share canonical URL key")
	}
	if SameCanonicalURL("https://sqlite.org/", "https://sqlite.org/download.html") {
		t.Fatalf("expected internal page to remain distinct from root")
	}
}

func TestResourceClassClassification(t *testing.T) {
	if got := ResourceClass("https://example.com/logo.png"); got != ResourceClassMediaAsset {
		t.Fatalf("expected media asset, got %q", got)
	}
	if got := ResourceClass("https://developers.openai.com/api"); got != ResourceClassHTMLLike {
		t.Fatalf("expected html-like, got %q", got)
	}
	if got := ResourceClass("https://api.example.com/openapi.json"); got != ResourceClassStructured {
		t.Fatalf("expected structured data, got %q", got)
	}
	if got := ResourceClass("https://example.com/assets/app.css"); got != ResourceClassTextAsset {
		t.Fatalf("expected text asset, got %q", got)
	}
}

func TestScoreStructuralCandidatesPrefersHTMLDocsOverStructuredFeedWithoutGoalTextMatch(t *testing.T) {
	candidates := ScoreStructuralCandidates(
		"",
		"",
		[]LinkCandidate{
			{URL: "https://www.scribd.com/opensearch.xml", Label: "MDN JavaScript guide"},
			{URL: "https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide", Label: "MDN JavaScript guide"},
		},
		nil,
	)
	if candidates[0].URL != "https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide" {
		t.Fatalf("expected html docs candidate to beat structured feed, got %q", candidates[0].URL)
	}
}

func TestSameSiteContextPriorPromotesSpecificRouteWithoutSurfaceFormAlignment(t *testing.T) {
	candidates := ScoreStructuralCandidates(
		"https://sqlite.org",
		"SQLite",
		[]LinkCandidate{
			{URL: "https://sqlite.org/chronology.html", Label: "Release History"},
			{URL: "https://sqlite.org/download.html", Label: "Download"},
		},
		[]string{"sqlite.org"},
	)
	reranked := ApplySameSiteContextPrior("https://sqlite.org", candidates)
	if reranked[0].URL != "https://sqlite.org/chronology.html" {
		t.Fatalf("expected first same-site route to beat seed without surface-form alignment, got %q", reranked[0].URL)
	}
	if contains(reranked[0].Reason, "semantic_goal_alignment") {
		t.Fatalf("expected surface-form promotion reason to remain absent, got %#v", reranked[0].Reason)
	}
	if !contains(reranked[0].Reason, "same_site_specific_route") {
		t.Fatalf("expected same-site structural route reason, got %#v", reranked[0].Reason)
	}
}

func TestSameSiteContextPriorKeepsDeepSeedScope(t *testing.T) {
	candidates := ScoreStructuralCandidates(
		"https://docs.python.org/3/tutorial/index.html",
		"Python Tutorial",
		[]LinkCandidate{
			{URL: "https://docs.python.org/3/index.html", Label: "Python documentation"},
			{URL: "https://docs.python.org/3/tutorial/venv.html", Label: "Virtual environments"},
		},
		[]string{"docs.python.org"},
	)
	reranked := ApplySameSiteContextPrior("https://docs.python.org/3/tutorial/index.html", candidates)
	if reranked[0].URL != "https://docs.python.org/3/tutorial/venv.html" {
		t.Fatalf("expected same-scope sibling route, got %q", reranked[0].URL)
	}
	if !contains(reranked[0].Reason, "same_site_sibling_route") {
		t.Fatalf("expected same-site sibling reason, got %#v", reranked[0].Reason)
	}
}

func TestSameSiteContextPriorPromotesDominantFamilyRepresentative(t *testing.T) {
	candidates := ScoreStructuralCandidates(
		"https://playwright.dev",
		"Playwright",
		[]LinkCandidate{
			{URL: "https://playwright.dev/mcp/introduction", Label: "MCP"},
			{URL: "https://playwright.dev/agent-cli/introduction", Label: "CLI"},
			{URL: "https://playwright.dev/docs/getting-started-cli", Label: "CLI documentation"},
			{URL: "https://playwright.dev/docs/getting-started-mcp", Label: "MCP documentation"},
			{URL: "https://playwright.dev/docs/intro", Label: "Docs"},
		},
		nil,
	)
	reranked := ApplySameSiteContextPrior("https://playwright.dev", candidates)
	if reranked[0].URL != "https://playwright.dev/docs/intro" {
		t.Fatalf("expected dominant path family representative, got %q", reranked[0].URL)
	}
	if !contains(reranked[0].Reason, "same_site_family_representative") {
		t.Fatalf("expected family representative reason, got %#v", reranked[0].Reason)
	}
}

func TestURLStructureBoostPenalizesNumericVersionLeaf(t *testing.T) {
	versionLeaf := urlStructureBoost("https://sqlite.org/releaselog/3_53_0.html")
	indexPage := urlStructureBoost("https://sqlite.org/chronology.html")
	if versionLeaf >= indexPage {
		t.Fatalf("expected numeric version leaf to score below index page, version=%f index=%f", versionLeaf, indexPage)
	}
}

func TestURLStructureBoostPenalizesOpaqueFragmentSchemaPath(t *testing.T) {
	opaque := urlStructureBoost("https://portaleimpresa24.it/#/schema/person/58759422aedb08e769f435f4bb1631cc")
	root := urlStructureBoost("https://www.coni.it/")
	if opaque >= root {
		t.Fatalf("expected opaque fragment schema path to score below root, opaque=%f root=%f", opaque, root)
	}
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
