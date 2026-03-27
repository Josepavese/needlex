package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/core"
)

func TestCrawlVisitsLinkedPagesSameDomain(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Home</title></head><body><article><h1>Home</h1><p>Seed page.</p><a href="%s/docs">Docs</a></article></body></html>`, serverURL)
		case "/docs":
			_, _ = fmt.Fprint(w, `<html><head><title>Docs</title></head><body><article><h1>Docs</h1><p>Linked docs page.</p></article></body></html>`)
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
	svc.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	resp, err := svc.Crawl(context.Background(), CrawlRequest{
		SeedURL:    server.URL,
		Profile:    core.ProfileTiny,
		MaxPages:   2,
		MaxDepth:   1,
		SameDomain: true,
	})
	if err != nil {
		t.Fatalf("crawl failed: %v", err)
	}
	if resp.Summary.PagesVisited != 2 {
		t.Fatalf("expected 2 pages visited, got %d", resp.Summary.PagesVisited)
	}
	if resp.Summary.MaxDepthReached != 1 {
		t.Fatalf("expected max depth 1, got %d", resp.Summary.MaxDepthReached)
	}
}

func TestExtractLinksHonorsSameDomain(t *testing.T) {
	links := extractLinks(
		`<html><body><a href="/docs">Docs</a><a href="https://external.example/page">External</a></body></html>`,
		"https://example.com",
		true,
	)
	if len(links) != 1 {
		t.Fatalf("expected 1 same-domain link, got %d", len(links))
	}
	if links[0] != "https://example.com/docs" {
		t.Fatalf("unexpected link %q", links[0])
	}
}
