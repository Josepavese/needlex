package analytics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStoreAppendRunAndReports(t *testing.T) {
	root := t.TempDir()
	store := NewSQLiteStore(root)
	startedAt := time.Now().UTC().Add(-2 * time.Second)
	completedAt := startedAt.Add(1500 * time.Millisecond)
	err := store.AppendRun(context.Background(), RunRecord{
		RunID:                "run_1",
		StartedAt:            startedAt,
		CompletedAt:          completedAt,
		Operation:            "query",
		Surface:              "cli",
		Profile:              "standard",
		GoalHash:             "goal_hash",
		GoalLengthChars:      21,
		DiscoveryMode:        "web_search",
		SelectedURL:          "https://example.com/docs",
		Provider:             "discovery_memory_same_site",
		Success:              true,
		TraceID:              "trace_1",
		LatencyMS:            1500,
		PacketBytes:          640,
		FinalContextChars:    400,
		ChunkCount:           2,
		SourceCount:          1,
		LinkCount:            3,
		ProofRefCount:        2,
		ProofUsable:          true,
		PublicBootstrapUsed:  false,
		LocalMemoryUsed:      true,
		TopicNodeUsed:        true,
		SameSiteRecoveryUsed: true,
		CandidateCount:       5,
		RawFetchChars:        2400,
		RawFetchBytes:        2400,
		ReducedChars:         900,
		ReducedNodeCount:     18,
		MemoryDocumentCount:  12,
		MemoryEmbeddingCount: 10,
		MemoryTopicNodeCount: 4,
	}, []StageEvent{
		{
			RunID:       "run_1",
			Stage:       "acquire",
			StartedAt:   startedAt,
			CompletedAt: startedAt.Add(100 * time.Millisecond),
			LatencyMS:   100,
			ItemCount:   1,
			Status:      "completed",
			Metadata:    map[string]string{"raw_chars": "2400"},
		},
	})
	if err != nil {
		t.Fatalf("append run: %v", err)
	}

	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.RunCount != 1 || stats.QueryRuns != 1 || stats.StageEventCount != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	report, err := store.ValueReport(context.Background())
	if err != nil {
		t.Fatalf("value report: %v", err)
	}
	if report.TotalRuns != 1 || report.TotalAgentCharsSaved <= 0 || report.TotalPublicBootstrapsAvoided != 1 || report.TotalTopicRootCorrections != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}

	recent, err := store.RecentRuns(context.Background(), 5)
	if err != nil {
		t.Fatalf("recent runs: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected one recent run, got %d", len(recent))
	}
	if recent[0].CharsSaved <= 0 || !recent[0].LocalMemoryUsed || recent[0].PublicBootstrapUsed {
		t.Fatalf("unexpected recent run: %+v", recent[0])
	}

	hosts, err := store.Hosts(context.Background(), 10)
	if err != nil {
		t.Fatalf("hosts: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Host != "example.com" {
		t.Fatalf("unexpected hosts: %+v", hosts)
	}

	providers, err := store.Providers(context.Background(), 10)
	if err != nil {
		t.Fatalf("providers: %v", err)
	}
	if len(providers) != 1 || providers[0].Provider != "discovery_memory_same_site" {
		t.Fatalf("unexpected providers: %+v", providers)
	}

	daily, err := store.Daily(context.Background(), 10)
	if err != nil {
		t.Fatalf("daily: %v", err)
	}
	if len(daily) != 1 || daily[0].RunCount != 1 {
		t.Fatalf("unexpected daily: %+v", daily)
	}

	exportDir := t.TempDir()
	exported, err := store.ExportJSON(context.Background(), exportDir)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, path := range []string{exported.RunsPath, exported.StagesPath, exported.HostsPath, exported.ProvidersPath, exported.DailyPath, exported.ValueReportPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected export file %s: %v", filepath.Base(path), err)
		}
	}
}
