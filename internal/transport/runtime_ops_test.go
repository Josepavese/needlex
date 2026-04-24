package transport

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/analytics"
	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
)

func TestExecuteReadFailureRecordsAnalyticsSurface(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner()
	runner.storeRoot = root
	runner.read = func(context.Context, config.Config, coreservice.ReadRequest) (coreservice.ReadResponse, error) {
		return coreservice.ReadResponse{}, fmt.Errorf("unexpected status code 404")
	}

	_, _, err := runner.executeReadWithSurface(config.Defaults(), coreservice.ReadRequest{
		URL:       "https://example.com/missing",
		Objective: "analytics failure smoke",
		Profile:   "standard",
	}, "mcp")
	if err == nil {
		t.Fatal("expected read error")
	}

	store := analytics.NewSQLiteStore(root)
	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.RunCount != 1 || stats.SuccessfulRuns != 0 || stats.ReadRuns != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	recent, err := store.RecentRuns(context.Background(), 1)
	if err != nil {
		t.Fatalf("RecentRuns() error = %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected one recent run, got %d", len(recent))
	}
	if recent[0].Surface != "mcp" || recent[0].Provider != "error:upstream_not_found" || recent[0].Success {
		t.Fatalf("unexpected recent run: %+v", recent[0])
	}
}

func TestSeedlessAmbiguousCandidatesWriteRuntimeWarning(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner()
	runner.storeRoot = root

	runner.observeRuntimeQueryDiagnostics("cli", coreservice.QueryRequest{Goal: "find canonical docs"}, coreservice.QueryResponse{
		Plan: coreservice.QueryPlan{
			DiscoveryMode:     coreservice.QueryDiscoveryWeb,
			DiscoveryProvider: "provider",
			SelectedURL:       "https://example.com/a",
		},
		AgentContext: coreservice.AgentContext{
			Candidates: []coreservice.AgentCandidate{
				{URL: "https://example.com/a", Score: 0.51},
				{URL: "https://example.com/b", Score: 0.49},
				{URL: "https://example.com/c", Score: 0.48},
			},
		},
		TraceID: "trace_1",
	})

	events, err := runner.runtimeLogger().Tail(1)
	if err != nil {
		t.Fatalf("tail logs: %v", err)
	}
	if len(events) != 1 || events[0].Level != "warn" || events[0].Event != "seedless.ambiguous_candidates" {
		t.Fatalf("unexpected warning event: %+v", events)
	}
	if !strings.Contains(fmt.Sprint(events[0].Fields["candidate_urls"]), "https://example.com/b") {
		t.Fatalf("expected candidate urls in warning event: %+v", events[0].Fields)
	}
}

func TestExecuteReadSuccessWritesFetchRuntimeEvent(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner()
	runner.storeRoot = root
	runner.read = func(context.Context, config.Config, coreservice.ReadRequest) (coreservice.ReadResponse, error) {
		resp := fakeResponse()
		resp.Trace.Stages[0].Metadata = map[string]string{
			"final_url":      "https://example.com/final",
			"fetch_mode":     "http",
			"fetch_profile":  "browser_like",
			"retry_profile":  "hardened",
			"retry_count":    "1",
			"host_pacing_ms": "25",
			"raw_chars":      "100",
			"raw_bytes":      "120",
			"content_type":   "text/html",
		}
		return resp, nil
	}

	_, _, err := runner.executeReadWithSurface(config.Defaults(), coreservice.ReadRequest{
		URL:     "https://example.com",
		Profile: "standard",
	}, "cli")
	if err != nil {
		t.Fatalf("execute read: %v", err)
	}

	events, err := runner.runtimeLogger().Tail(1)
	if err != nil {
		t.Fatalf("tail logs: %v", err)
	}
	if len(events) != 1 || events[0].Level != "info" || events[0].Event != "fetch.completed" {
		t.Fatalf("unexpected fetch event: %+v", events)
	}
	if events[0].Fields["fetch_profile"] != "browser_like" || events[0].Fields["retry_count"] != float64(1) {
		t.Fatalf("unexpected fetch fields: %+v", events[0].Fields)
	}
}
