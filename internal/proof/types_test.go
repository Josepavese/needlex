package proof

import (
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
)

func TestBuildProofRecordFromSegment(t *testing.T) {
	record, err := BuildProofRecord(BuildInput{
		Chunk: core.Chunk{
			ID:          "chk_1",
			DocID:       "doc_1",
			Text:        "First paragraph.",
			Score:       0.9,
			Fingerprint: "fp_1",
			Confidence:  0.95,
		},
		Segment: pipeline.Segment{
			Kind:      "paragraph",
			Text:      "First paragraph.",
			NodePaths: []string{"/body[1]/article[1]/p[1]"},
		},
		Lane:           0,
		TransformChain: []string{"reduce:v1", "segment:v1"},
	})
	if err != nil {
		t.Fatalf("build proof record: %v", err)
	}
	if record.ID == "" {
		t.Fatal("expected proof record id")
	}
	if record.Proof.SourceSpan.Selector == "" {
		t.Fatal("expected selector to be populated")
	}
}

func TestRunTraceReplayAndDiff(t *testing.T) {
	startedAt := time.Unix(1700000000, 0).UTC()

	left := NewRecorder("run_1", "trace_1", startedAt)
	if err := left.StageStarted("acquire", map[string]string{"url": "https://example.com"}, startedAt); err != nil {
		t.Fatalf("stage started: %v", err)
	}
	if err := left.StageCompleted("acquire", map[string]string{"hash": "a"}, 1, map[string]string{"mode": "http"}, startedAt.Add(time.Millisecond)); err != nil {
		t.Fatalf("stage completed: %v", err)
	}
	traceA := left.Finish(startedAt.Add(2 * time.Millisecond))

	right := NewRecorder("run_2", "trace_2", startedAt)
	if err := right.StageStarted("acquire", map[string]string{"url": "https://example.com"}, startedAt); err != nil {
		t.Fatalf("stage started: %v", err)
	}
	if err := right.StageCompleted("acquire", map[string]string{"hash": "b"}, 1, map[string]string{"mode": "http"}, startedAt.Add(time.Millisecond)); err != nil {
		t.Fatalf("stage completed: %v", err)
	}
	traceB := right.Finish(startedAt.Add(2 * time.Millisecond))

	report, err := traceA.ReplayReport()
	if err != nil {
		t.Fatalf("replay report: %v", err)
	}
	if !report.Deterministic {
		t.Fatal("expected deterministic replay report")
	}

	diff, err := Diff(traceA, traceB)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if len(diff.ChangedStages) != 1 {
		t.Fatalf("expected one changed stage, got %d", len(diff.ChangedStages))
	}
	if diff.ChangedStages[0].Stage != "acquire" {
		t.Fatalf("expected changed acquire stage, got %#v", diff.ChangedStages[0])
	}
}

func TestRecorderRejectsDuplicateActiveStage(t *testing.T) {
	now := time.Now().UTC()
	rec := NewRecorder("run_1", "trace_1", now)
	if err := rec.StageStarted("reduce", nil, now); err != nil {
		t.Fatalf("unexpected start failure: %v", err)
	}
	if err := rec.StageStarted("reduce", nil, now); err == nil {
		t.Fatal("expected duplicate stage start to fail")
	}
}
