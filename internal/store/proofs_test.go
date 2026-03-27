package store

import (
	"errors"
	"testing"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
)

func TestProofStoreSaveAndLoad(t *testing.T) {
	root := t.TempDir()
	store := NewProofStore(root)
	records := fakeProofRecords()

	if _, err := store.SaveProofRecords("trace_1", records); err != nil {
		t.Fatalf("save proof records: %v", err)
	}

	loaded, err := store.LoadProofRecords("trace_1")
	if err != nil {
		t.Fatalf("load proof records: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 proof record, got %d", len(loaded))
	}
	if loaded[0].Proof.ChunkID != "chk_1" {
		t.Fatalf("expected chunk id chk_1, got %q", loaded[0].Proof.ChunkID)
	}
}

func TestProofStoreFindByChunkID(t *testing.T) {
	root := t.TempDir()
	store := NewProofStore(root)
	if _, err := store.SaveProofRecords("trace_a", fakeProofRecords()); err != nil {
		t.Fatalf("seed proof records: %v", err)
	}

	record, traceID, err := store.FindProofByChunkID("chk_1")
	if err != nil {
		t.Fatalf("find by chunk id: %v", err)
	}
	if traceID != "trace_a" {
		t.Fatalf("expected trace id trace_a, got %q", traceID)
	}
	if record.ID != "proof_1" {
		t.Fatalf("expected proof id proof_1, got %q", record.ID)
	}
}

func TestProofStoreMissingProof(t *testing.T) {
	store := NewProofStore(t.TempDir())
	_, err := store.LoadProofRecords("missing")
	if !errors.Is(err, ErrProofNotFound) {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func fakeProofRecords() []proof.ProofRecord {
	return []proof.ProofRecord{
		{
			ID: "proof_1",
			Proof: core.Proof{
				ChunkID:        "chk_1",
				SourceSpan:     core.SourceSpan{Selector: "/article/p[1]", CharStart: 0, CharEnd: 16},
				TransformChain: []string{"reduce:v1", "segment:v1", "pack:v1"},
				Lane:           0,
			},
		},
	}
}
