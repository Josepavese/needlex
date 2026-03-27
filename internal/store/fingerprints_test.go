package store

import (
	"errors"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/core"
)

func TestFingerprintStoreSaveAndLoad(t *testing.T) {
	root := t.TempDir()
	store := NewFingerprintStore(root)
	store.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }

	if _, err := store.SaveChunks("trace_1", []core.Chunk{fakeChunk()}); err != nil {
		t.Fatalf("save fingerprints: %v", err)
	}

	records, err := store.Load("trace_1")
	if err != nil {
		t.Fatalf("load fingerprints: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Fingerprint != "fp_1" {
		t.Fatalf("expected fp_1, got %q", records[0].Fingerprint)
	}
}

func TestFingerprintStoreMissing(t *testing.T) {
	store := NewFingerprintStore(t.TempDir())
	_, err := store.Load("missing")
	if !errors.Is(err, ErrFingerprintNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func fakeChunk() core.Chunk {
	return core.Chunk{
		ID:          "chk_1",
		DocID:       "doc_1",
		Text:        "Compact context.",
		HeadingPath: []string{"Needle Runtime"},
		Score:       0.9,
		Fingerprint: "fp_1",
		Confidence:  0.95,
	}
}
