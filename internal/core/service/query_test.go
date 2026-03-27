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
	if resp.TraceID == "" {
		t.Fatal("expected trace id")
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
