package store

import (
	"errors"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/proof"
)

func TestTraceStoreSaveAndLoad(t *testing.T) {
	root := t.TempDir()
	store := NewTraceStore(root)

	trace := proof.RunTrace{
		RunID:      "run_1",
		TraceID:    "trace_1",
		StartedAt:  time.Unix(1700000000, 0).UTC(),
		FinishedAt: time.Unix(1700000001, 0).UTC(),
		Stages: []proof.StageSnapshot{
			{
				Stage:       "acquire",
				StartedAt:   time.Unix(1700000000, 0).UTC(),
				CompletedAt: time.Unix(1700000001, 0).UTC(),
				OutputHash:  "hash",
			},
		},
		Events: []proof.TraceEvent{
			{
				Type:      proof.EventStageStarted,
				Stage:     "acquire",
				Timestamp: time.Unix(1700000000, 0).UTC(),
			},
		},
	}

	if _, err := store.SaveTrace(trace); err != nil {
		t.Fatalf("save trace: %v", err)
	}

	loaded, err := store.LoadTrace("trace_1")
	if err != nil {
		t.Fatalf("load trace: %v", err)
	}
	if loaded.TraceID != trace.TraceID {
		t.Fatalf("expected trace id %q, got %q", trace.TraceID, loaded.TraceID)
	}
}

func TestTraceStoreMissingTrace(t *testing.T) {
	store := NewTraceStore(t.TempDir())
	_, err := store.LoadTrace("missing")
	if !errors.Is(err, ErrTraceNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}
