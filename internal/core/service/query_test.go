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

func TestQueryBuildsPlanAndResultPack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, testHTML)
	}))
	defer server.Close()

	svc, err := New(config.Defaults(), server.Client())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:    "proof replay deterministic",
		SeedURL: server.URL,
		Profile: core.ProfileTiny,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.Goal != "proof replay deterministic" {
		t.Fatalf("unexpected plan goal %q", resp.Plan.Goal)
	}
	if resp.ResultPack.Query != "proof replay deterministic" {
		t.Fatalf("expected result pack query to mirror goal, got %q", resp.ResultPack.Query)
	}
	if resp.Plan.SelectedURL != server.URL {
		t.Fatalf("expected selected url to default to seed, got %q", resp.Plan.SelectedURL)
	}
	if resp.TraceID == "" {
		t.Fatal("expected trace id")
	}
}

func TestQueryDiscoversHigherSignalCandidate(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Home</title></head><body><article><h1>Home</h1><p>Index page.</p><a href="%s/docs/replay-guide">Replay Guide</a></article></body></html>`, serverURL)
		case "/docs/replay-guide":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1><p>Proof replay deterministic context for operators.</p></article></body></html>`)
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
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:    "proof replay deterministic",
		SeedURL: server.URL,
		Profile: core.ProfileTiny,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.SelectedURL != server.URL+"/docs/replay-guide" {
		t.Fatalf("expected discovered docs page, got %q", resp.Plan.SelectedURL)
	}
	if resp.Document.FinalURL != server.URL+"/docs/replay-guide" {
		t.Fatalf("expected query to read selected page, got %q", resp.Document.FinalURL)
	}
	if len(resp.Plan.CandidateURLs) < 2 {
		t.Fatalf("expected discovery candidates, got %#v", resp.Plan.CandidateURLs)
	}
}

func TestQueryDiscoveryOffKeepsSeedURL(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(w, `<html><head><title>Home</title></head><body><article><h1>Home</h1><p>Seed page.</p><a href="%s/docs/replay-guide">Replay Guide</a></article></body></html>`, serverURL)
		case "/docs/replay-guide":
			_, _ = fmt.Fprint(w, `<html><head><title>Replay Guide</title></head><body><article><h1>Replay Guide</h1><p>Proof replay deterministic context for operators.</p></article></body></html>`)
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
	svc.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	resp, err := svc.Query(context.Background(), QueryRequest{
		Goal:          "proof replay deterministic",
		SeedURL:       server.URL,
		Profile:       core.ProfileTiny,
		DiscoveryMode: QueryDiscoveryOff,
	})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if resp.Plan.SelectedURL != server.URL {
		t.Fatalf("expected selected url to remain seed, got %q", resp.Plan.SelectedURL)
	}
	if len(resp.Plan.CandidateURLs) != 1 || resp.Plan.CandidateURLs[0] != server.URL {
		t.Fatalf("expected only seed candidate, got %#v", resp.Plan.CandidateURLs)
	}
}

func TestQueryRejectsMissingGoal(t *testing.T) {
	svc, err := New(config.Defaults(), nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Query(context.Background(), QueryRequest{SeedURL: "https://example.com"})
	if err == nil {
		t.Fatal("expected missing goal to fail")
	}
}
