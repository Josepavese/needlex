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
	if got := layout.Paths()["proofs"]; got != filepath.Join(root, "proofs") {
		t.Fatalf("unexpected proofs path: %q", got)
	}
}
