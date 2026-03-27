package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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
