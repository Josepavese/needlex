package discovery

import "testing"

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
