package analytics

import (
	"context"
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
}
