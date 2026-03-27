package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
)

func TestExtractLinkCandidatesPreservesLabels(t *testing.T) {
	links := extractLinkCandidates(
		`<html><body><a href="/docs/replay">Replay Guide</a><a href="https://external.example/page">External</a></body></html>`,
		"https://example.com",
		true,
	)
	if len(links) != 1 {
		t.Fatalf("expected 1 same-domain candidate, got %d", len(links))
	}
	if links[0].URL != "https://example.com/docs/replay" {
		t.Fatalf("unexpected candidate url %q", links[0].URL)
	}
	if links[0].Label != "Replay Guide" {
		t.Fatalf("unexpected candidate label %q", links[0].Label)
	}
}

func TestDiscoverChoosesBestGoalMatch(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Portal</title></head><body><article><h1>Portal</h1><a href="%s/blog">Blog</a><a href="%s/docs/replay">Replay Guide</a></article></body></html>`, serverURL, serverURL)
		case "/docs/replay":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay</h1></article></body></html>`)
		case "/blog":
			_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	resp, err := svc.Discover(context.Background(), DiscoverRequest{
		Goal:       "proof replay deterministic",
		SeedURL:    server.URL,
		SameDomain: true,
	})
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if resp.SelectedURL != server.URL+"/docs/replay" {
		t.Fatalf("expected replay guide to win, got %q", resp.SelectedURL)
	}
	if len(resp.Candidates) < 3 {
		t.Fatalf("expected seed plus links, got %#v", resp.Candidates)
	}
}

func TestExtractSearchResultsResolvesRedirectTargets(t *testing.T) {
	results := extractSearchResults(
		`<html><body><a class="result__a" href="/l/?uddg=https%3A%2F%2Fdocs.example.com%2Freplay">Replay Docs</a></body></html>`,
		"https://html.duckduckgo.com/html/?q=replay",
	)
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}
	if results[0].URL != "https://docs.example.com/replay" {
		t.Fatalf("unexpected resolved url %q", results[0].URL)
	}
	if results[0].Label != "Replay Docs" {
		t.Fatalf("unexpected label %q", results[0].Label)
	}
}

func TestDiscoverWebChoosesBestCrossSiteCandidate(t *testing.T) {
	docsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Replay Proof Guide</h1></article></body></html>`)
	}))
	defer docsServer.Close()

	blogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1></article></body></html>`)
	}))
	defer blogServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Company Blog</a><a class="result__a" href="%s">Replay Proof Guide</a></body></html>`, blogServer.URL, docsServer.URL)
	}))
	defer searchServer.Close()

	svc, err := New(config.Defaults(), searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	svc.webDiscoverBaseURL = searchServer.URL

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != docsServer.URL {
		t.Fatalf("expected docs candidate to win, got %q", resp.SelectedURL)
	}
	if resp.Provider == "" {
		t.Fatal("expected provider name")
	}
}

func TestDiscoverWebReranksUsingFetchedPageTitle(t *testing.T) {
	docsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Guide</h1></article></body></html>`)
	}))
	defer docsServer.Close()

	blogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Company Updates</title></head><body><article><h1>Blog</h1></article></body></html>`)
	}))
	defer blogServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Official Site</a><a class="result__a" href="%s">Official Site</a></body></html>`, blogServer.URL, docsServer.URL)
	}))
	defer searchServer.Close()

	svc, err := New(config.Defaults(), searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	svc.webDiscoverBaseURL = searchServer.URL

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != docsServer.URL {
		t.Fatalf("expected fetched page title rerank to prefer docs candidate, got %q", resp.SelectedURL)
	}
	if len(resp.Candidates) == 0 || !containsReason(resp.Candidates[0].Reason, "page_title_probe") {
		t.Fatalf("expected top candidate to include page_title_probe reason, got %#v", resp.Candidates)
	}
}

func TestDiscoverWebExpandsLandingPageToBetterChild(t *testing.T) {
	var portalURL string
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Portal</title></head><body><article><h1>Portal</h1><a href="%s/docs/replay-proof">Replay Proof Guide</a></article></body></html>`, portalURL)
		case "/docs/replay-proof":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Replay Proof Guide</h1></article></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer portalServer.Close()
	portalURL = portalServer.URL

	blogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1></article></body></html>`)
	}))
	defer blogServer.Close()

	searchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Official Site</a><a class="result__a" href="%s">News</a></body></html>`, portalServer.URL, blogServer.URL)
	}))
	defer searchServer.Close()

	svc, err := New(config.Defaults(), searchServer.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	svc.webDiscoverBaseURL = searchServer.URL

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != portalServer.URL+"/docs/replay-proof" {
		t.Fatalf("expected expansion to choose child docs page, got %q", resp.SelectedURL)
	}
	if len(resp.Candidates) == 0 || !containsReason(resp.Candidates[0].Reason, "page_expand") {
		t.Fatalf("expected top candidate to include page_expand reason, got %#v", resp.Candidates)
	}
}

func TestDiscoverWebMergesMultipleProviders(t *testing.T) {
	docsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Replay Proof Guide</title></head><body><article><h1>Replay Proof Guide</h1></article></body></html>`)
	}))
	defer docsServer.Close()

	blogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Blog</title></head><body><article><h1>Blog</h1></article></body></html>`)
	}))
	defer blogServer.Close()

	searchOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Company Blog</a></body></html>`, blogServer.URL)
	}))
	defer searchOne.Close()

	searchTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<html><body><a class="result__a" href="%s">Replay Proof Guide</a></body></html>`, docsServer.URL)
	}))
	defer searchTwo.Close()

	svc, err := New(config.Defaults(), searchOne.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	svc.webDiscoverBaseURL = searchOne.URL + "," + searchTwo.URL

	resp, err := svc.DiscoverWeb(context.Background(), DiscoverWebRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       "https://seed.example/root",
		MaxCandidates: 5,
	})
	if err != nil {
		t.Fatalf("discover web failed: %v", err)
	}
	if resp.SelectedURL != docsServer.URL {
		t.Fatalf("expected merged providers to surface docs candidate, got %q", resp.SelectedURL)
	}
	if !strings.Contains(resp.Provider, discoverProviderName(searchOne.URL)) || !strings.Contains(resp.Provider, discoverProviderName(searchTwo.URL)) {
		t.Fatalf("expected combined provider names, got %q", resp.Provider)
	}
}

func containsReason(reasons []string, needle string) bool {
	for _, reason := range reasons {
		if reason == needle {
			return true
		}
	}
	return false
}
