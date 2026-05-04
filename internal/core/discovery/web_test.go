package discovery

import (
	"strings"
	"testing"
)

func TestLooksLikeDuckDuckGoAnomaly(t *testing.T) {
	if !LooksLikeDuckDuckGoAnomaly(`<html><body><div class="anomaly-modal__title">Unfortunately, bots use DuckDuckGo too.</div><form action="/anomaly.js?sv=lite"></form></body></html>`) {
		t.Fatal("expected anomaly page to be detected")
	}
	if LooksLikeDuckDuckGoAnomaly(`<html><body><a class="result__a" href="https://playwright.dev">Playwright</a></body></html>`) {
		t.Fatal("did not expect normal result page to be flagged as anomaly")
	}
}

func TestIsDuckDuckGoProvider(t *testing.T) {
	if !IsDuckDuckGoProvider("https://lite.duckduckgo.com/lite/") || !IsDuckDuckGoProvider("https://html.duckduckgo.com/html/") {
		t.Fatal("expected duckduckgo providers to be recognized")
	}
	if IsDuckDuckGoProvider("https://example.com/search") {
		t.Fatal("did not expect non-duckduckgo provider")
	}
}

func TestProviderNameRecognizesAPIProviders(t *testing.T) {
	if ProviderName("brave://search") != "brave" {
		t.Fatalf("unexpected brave provider name")
	}
}

func TestExtractSearchResultsDuckDuckGoRedirect(t *testing.T) {
	results := ExtractSearchResults(`
<html><body>
<div class="result"><a class="result__a" href="/l/?uddg=https%3A%2F%2Fdeveloper.mozilla.org%2Fdocs">MDN</a><span class="result__snippet">Maintained web platform reference.</span></div>
</body></html>`, "https://lite.duckduckgo.com/lite/")
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].URL != "https://developer.mozilla.org/docs" {
		t.Fatalf("unexpected result URL %q", results[0].URL)
	}
	if !strings.Contains(results[0].Context, "Maintained web platform reference.") {
		t.Fatalf("expected DDG search result context, got %q", results[0].Context)
	}
}

func TestExtractSearchResultsBingDirectAnchor(t *testing.T) {
	results := ExtractSearchResults(`
<html><body>
<nav><a href="https://www.bing.com/images">Images</a></nav>
<ol><li class="b_algo"><h2><a href="https://sqlite.org/index.html">SQLite Home Page</a></h2><p>Maintained project source context.</p></li></ol>
</body></html>`, "https://www.bing.com/search")
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].URL != "https://sqlite.org/index.html" {
		t.Fatalf("unexpected result URL %q", results[0].URL)
	}
	if !strings.Contains(results[0].Context, "Maintained project source context.") {
		t.Fatalf("expected search result context, got %q", results[0].Context)
	}
}

func TestExtractSearchResultsBingRedirectAnchor(t *testing.T) {
	results := ExtractSearchResults(`
<html><body>
<ol><li class="b_algo"><h2><a href="/ck/a?u=a1aHR0cHM6Ly9zcWxpdGUub3JnLw">SQLite</a></h2></li></ol>
</body></html>`, "https://www.bing.com/search")
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].URL != "https://sqlite.org/" {
		t.Fatalf("unexpected result URL %q", results[0].URL)
	}
}
