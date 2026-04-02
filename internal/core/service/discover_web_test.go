package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

func TestRefineWebCandidatePrefersFirstPartyHostCoherence(t *testing.T) {
	official := refineWebCandidate(
		"OpenAI API pricing",
		DiscoverCandidate{URL: "https://developers.openai.com/api/pricing", Label: "OpenAI API pricing"},
		"https://developers.openai.com/api/pricing",
		"OpenAI API pricing",
		core.WebIR{},
		nil,
	)
	thirdParty := refineWebCandidate(
		"OpenAI API pricing",
		DiscoverCandidate{URL: "https://curlscape.com/blog/openai-api-pricing-guide", Label: "OpenAI API pricing"},
		"https://curlscape.com/blog/openai-api-pricing-guide",
		"OpenAI API pricing",
		core.WebIR{},
		nil,
	)
	if official.Score <= thirdParty.Score {
		t.Fatalf("expected first-party host coherence to win, official=%f third_party=%f", official.Score, thirdParty.Score)
	}
}

func TestDiscoverWebHostRootIdentityPrefersFirstPartyDocs(t *testing.T) {
	officialServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<html><head><title>OpenAI Platform</title></head><body><h1>OpenAI</h1></body></html>`)
		case "/api/pricing":
			_, _ = fmt.Fprint(w, `<html><head><title>API pricing</title></head><body><h1>Pricing</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer officialServer.Close()

	thirdPartyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<html><head><title>Curlscape</title></head><body><h1>Curlscape</h1></body></html>`)
		case "/blog/openai-api-pricing-guide":
			_, _ = fmt.Fprint(w, `<html><head><title>OpenAI API pricing</title></head><body><h1>Guide</h1></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer thirdPartyServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(
			w,
			`<html><body><a class="result__a" href="%s/blog/openai-api-pricing-guide">OpenAI API pricing</a><a class="result__a" href="%s/api/pricing">OpenAI API pricing</a></body></html>`,
			thirdPartyServer.URL,
			officialServer.URL,
		)
	}))
	defer searchServer.Close()

	cfg := config.Defaults()
	svc, err := New(cfg, searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.SetWebDiscoverBaseURL(searchServer.URL)

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "OpenAI API pricing",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != officialServer.URL+"/api/pricing" {
		t.Fatalf("expected first-party docs candidate to win, got %q", resp.SelectedURL)
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Metadata["host_root_title"] != "OpenAI Platform" {
		t.Fatalf("expected host root identity metadata on top candidate, got %#v", resp.Candidates)
	}
}

func TestCanonicalizeCandidateFamiliesPrefersSameHostRootWhenScoresClose(t *testing.T) {
	candidates := []DiscoverCandidate{
		{URL: "https://playwright.dev/community/welcome", Score: 1.20},
		{URL: "https://playwright.dev/", Score: 1.05},
	}
	got := canonicalizeCandidateFamilies(candidates)
	if got[0].URL != "https://playwright.dev/" {
		t.Fatalf("expected host root to win after canonicalization, got %q", got[0].URL)
	}
}

func TestSemanticDisambiguateCandidateFamiliesBoostsBrandAlignedFamily(t *testing.T) {
	semantic := newDiscoverSemanticServer()
	defer semantic.Close()

	cfg := config.Defaults()
	enableDiscoverSemantic(&cfg, semantic.URL)
	svc := newTestService(t, cfg, nil)

	candidates := []DiscoverCandidate{
		{
			URL:   "https://packaging-guide.openastronomy.org/en/latest/",
			Score: 1.00,
			Label: "Packaging guide",
			Metadata: map[string]string{
				"host_root_title": "OpenAstronomy packaging guide",
				"page_title":      "Packaging guide",
			},
		},
		{
			URL:   "https://packaging.python.org/en/latest/",
			Score: 0.92,
			Label: "Python Packaging User Guide",
			Metadata: map[string]string{
				"host_root_title": "Python Packaging User Guide",
				"page_title":      "Python Packaging User Guide",
			},
		},
	}

	got := svc.semanticDisambiguateCandidateFamilies(context.Background(), "python packaging", candidates)
	if got[0].URL != "https://packaging.python.org/en/latest/" {
		t.Fatalf("expected semantic family scorer to prefer python packaging family, got %q", got[0].URL)
	}
}
