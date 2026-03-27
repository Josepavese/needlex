package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneAllRemovesStateFiles(t *testing.T) {
	root := t.TempDir()
	writeStateFile(t, filepath.Join(root, "traces", "a.json"))
	writeStateFile(t, filepath.Join(root, "proofs", "a.json"))
	writeStateFile(t, filepath.Join(root, "fingerprints", "a.json"))

	report, err := Prune(root, 0, true, time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatalf("prune all: %v", err)
	}
	if report.RemovedFiles != 3 {
		t.Fatalf("expected 3 files removed, got %d", report.RemovedFiles)
	}
}

func TestPruneOlderThan(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "traces", "old.json")
	newPath := filepath.Join(root, "proofs", "new.json")
	writeStateFile(t, oldPath)
	writeStateFile(t, newPath)

	old := time.Unix(1700000000, 0).UTC()
	newer := old.Add(2 * time.Hour)
	if err := os.Chtimes(oldPath, old, old); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}
	if err := os.Chtimes(newPath, newer, newer); err != nil {
		t.Fatalf("chtimes new: %v", err)
	}

	report, err := Prune(root, time.Hour, false, newer)
	if err != nil {
		t.Fatalf("prune older than: %v", err)
	}
	if report.RemovedFiles != 1 {
		t.Fatalf("expected 1 file removed, got %d", report.RemovedFiles)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected newer file to remain: %v", err)
	}
}

func writeStateFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
