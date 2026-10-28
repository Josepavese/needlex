package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSharedRollupMetricsPreserveGroupingOrderAndLimits(t *testing.T) {
	store := NewSQLiteStore(t.TempDir())
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, row := range []struct {
		host, provider                   string
		day, latency, raw, final         int
		success, proof, bootstrap, local bool
	}{
		{"a", "p", 0, 10, 100, 20, true, true, false, true},
		{"a", "q", 0, 30, 20, 30, false, false, true, false},
		{"b", "p", 1, 20, 120, 40, true, true, true, true},
		{"c", "q", 1, 50, 0, 0, false, false, true, false},
	} {
		at := start.Add(time.Duration(row.day) * 24 * time.Hour)
		if err := store.AppendRun(context.Background(), RunRecord{
			RunID: fmt.Sprintf("rollup_%d", i), StartedAt: at, CompletedAt: at.Add(time.Second),
			Operation: "read", Surface: "cli", Host: row.host, Provider: row.provider,
			Success: row.success, LatencyMS: int64(row.latency), RawFetchChars: row.raw,
			FinalContextChars: row.final, ProofUsable: row.proof,
			PublicBootstrapUsed: row.bootstrap, LocalMemoryUsed: row.local,
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	hosts, err := store.Hosts(context.Background(), 1)
	if err != nil || len(hosts) != 1 || hosts[0].Host != "a" {
		t.Fatalf("host grouping/limit: %+v, %v", hosts, err)
	}
	want := RollupMetrics{
		RunCount: 2, SuccessfulRuns: 1, AvgLatencyMS: 20,
		TotalAgentCharsSaved: 80, TotalAgentTokensSaved: 20,
		ProofBackedRate: 0.5, PublicBootstrapUsedRate: 0.5, LocalMemoryUsedRate: 0.5,
	}
	if hosts[0].RollupMetrics != want {
		t.Fatalf("host metrics: %+v want %+v", hosts[0].RollupMetrics, want)
	}
	providers, err := store.Providers(context.Background(), 0)
	if err != nil || len(providers) != 2 || providers[0].Provider != "p" || providers[1].Provider != "q" {
		t.Fatalf("provider tie order/default limit: %+v, %v", providers, err)
	}
	wantProvider := RollupMetrics{
		RunCount: 2, SuccessfulRuns: 2, AvgLatencyMS: 15,
		TotalAgentCharsSaved: 160, TotalAgentTokensSaved: 40,
		ProofBackedRate: 1, PublicBootstrapUsedRate: 0.5, LocalMemoryUsedRate: 1,
	}
	if providers[0].RollupMetrics != wantProvider {
		t.Fatalf("provider metrics: %+v want %+v", providers[0].RollupMetrics, wantProvider)
	}
	days, err := store.Daily(context.Background(), -1)
	if err != nil || len(days) != 2 || days[0].Day != "2026-01-02" || days[1].Day != "2026-01-01" || days[1].RollupMetrics != want {
		t.Fatalf("daily grouping/order/metrics: %+v, %v", days, err)
	}
	data, err := json.Marshal(hosts[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil || len(fields) != 9 || fields["host"] != "a" || fields["run_count"] != float64(2) {
		t.Fatalf("rollup JSON is no longer flat: %s, %v", data, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Providers(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("query lost cancellation: %v", err)
	}
}
