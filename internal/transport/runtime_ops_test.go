package transport

import (
	"context"
	"fmt"
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
