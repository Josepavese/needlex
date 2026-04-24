package discovery

import "testing"

func TestScoreCandidatesRemainStructureFirstWithoutLexicalPromotion(t *testing.T) {
	candidates := ScoreCandidates(
		"OpenAI API pricing",
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
	if contains(candidates[0].Reason, "goal_label_alignment") {
		t.Fatalf("expected lexical goal-label reason to be absent, got %#v", candidates[0].Reason)
	}
}

func TestURLStructureBoostPenalizesArticleLikeDeepPaths(t *testing.T) {
	deepArticle := urlStructureBoost("https://example.com/2025/04/playwright-automation-tutorial.html")
	shallowRoot := urlStructureBoost("https://playwright.dev/")
	if deepArticle >= shallowRoot {
		t.Fatalf("expected deep article-like path to score below shallow root, article=%f root=%f", deepArticle, shallowRoot)
	}
}

func TestHostCompactnessBoostPrefersCanonicalHostOverSubdomain(t *testing.T) {
	canonical := HostCompactnessBoost("https://playwright.dev/")
	subdomain := HostCompactnessBoost("https://try.playwright.tech/")
	if canonical <= subdomain {
		t.Fatalf("expected canonical host compactness to beat subdomain, canonical=%f subdomain=%f", canonical, subdomain)
	}
}

func TestProbableNonHTMLURLDetectsAssets(t *testing.T) {
	if !ProbableNonHTMLURL("https://example.com/logo.png") {
		t.Fatal("expected png asset url to be detected as non-html")
	}
	if ProbableNonHTMLURL("https://developers.openai.com/api") {
		t.Fatal("expected docs page not to be detected as non-html")
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

func TestScoreCandidatesPrefersHTMLDocsOverStructuredFeedWhenGoalMatches(t *testing.T) {
	candidates := ScoreCandidates(
		"MDN JavaScript guide",
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

func TestSameSiteContextPriorPromotesSpecificRouteWithoutLexicalAlignment(t *testing.T) {
	candidates := ScoreCandidates(
		"opaque multilingual retrieval intent",
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
		t.Fatalf("expected first same-site route to beat seed without lexical alignment, got %q", reranked[0].URL)
	}
	if contains(reranked[0].Reason, "goal_label_alignment") {
		t.Fatalf("expected lexical goal-label reason to remain absent, got %#v", reranked[0].Reason)
	}
	if !contains(reranked[0].Reason, "same_site_specific_route") {
		t.Fatalf("expected same-site structural route reason, got %#v", reranked[0].Reason)
	}
}

func TestSameSiteContextPriorKeepsDeepSeedScope(t *testing.T) {
	candidates := ScoreCandidates(
		"cross-language contextual route",
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
	candidates := ScoreCandidates(
		"broad routing intent",
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
