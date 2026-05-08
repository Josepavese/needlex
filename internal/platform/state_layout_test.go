package platform

import (
	"path/filepath"
	"testing"
)

func TestNewStateLayoutBuildsCanonicalPaths(t *testing.T) {
	root := filepath.Join("tmp", "needlex")
	layout := NewStateLayout(root)
	if layout.Root != root {
		t.Fatalf("root got %q want %q", layout.Root, root)
	}
	if layout.AnalyticsDB != filepath.Join(root, "analytics", "analytics.db") {
		t.Fatalf("unexpected analytics db: %q", layout.AnalyticsDB)
	}
	if layout.DiscoveryDB != filepath.Join(root, DefaultDiscoveryDBRelativePath) {
		t.Fatalf("unexpected discovery db: %q", layout.DiscoveryDB)
	}
	if layout.EmbeddingCacheDir != filepath.Join(root, "data", "embeddings", "cache") {
		t.Fatalf("unexpected embedding cache dir: %q", layout.EmbeddingCacheDir)
	}
	if got := layout.Paths()["proofs"]; got != filepath.Join(root, "proofs") {
		t.Fatalf("unexpected proofs path: %q", got)
	}
	if got := layout.Paths()["embedding_cache"]; got != layout.EmbeddingCacheDir {
		t.Fatalf("unexpected embedding cache path: %q", got)
	}
	if got := layout.Paths()["logs"]; got != filepath.Join(root, "logs") {
		t.Fatalf("unexpected logs path: %q", got)
	}
	if layout.ConfigPath != filepath.Join(root, "configs", "needlex.json") {
		t.Fatalf("unexpected config path: %q", layout.ConfigPath)
	}
	if layout.RuntimeLog != filepath.Join(root, "logs", "needlex.jsonl") {
		t.Fatalf("unexpected runtime log: %q", layout.RuntimeLog)
	}
}
