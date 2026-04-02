package discovery

import "testing"

func TestScoreCandidatesPrefersHostGoalCoherenceOverGenericThirdPartyLabel(t *testing.T) {
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
	if candidates[0].URL != "https://developers.openai.com/api/pricing" {
		t.Fatalf("expected first-party host to win, got %q", candidates[0].URL)
	}
	if !contains(candidates[0].Reason, "host_goal_coherence") {
		t.Fatalf("expected host_goal_coherence reason, got %#v", candidates[0].Reason)
	}
}

func TestHostTitleCoherenceBoostPrefersBrandAlignedHost(t *testing.T) {
	official := HostTitleCoherenceBoost("Playwright", "https://playwright.dev/docs/intro")
	thirdParty := HostTitleCoherenceBoost("Playwright", "https://browserstack.com/guide/playwright-tutorial")
	if official <= thirdParty {
		t.Fatalf("expected official host-title coherence to win, official=%f third_party=%f", official, thirdParty)
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

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
