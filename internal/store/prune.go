package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type PruneReport struct {
	RemovedFiles int      `json:"removed_files"`
	RemovedPaths []string `json:"removed_paths,omitempty"`
}

func Prune(root string, olderThan time.Duration, pruneAll bool, now time.Time) (PruneReport, error) {
	if !pruneAll && olderThan <= 0 {
		return PruneReport{}, fmt.Errorf("prune requires --all or positive older_than")
	}
	if root == "" {
		root = ".needlex"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	report := PruneReport{}
	for _, dir := range []string{"traces", "proofs", "fingerprints", "genome"} {
		pattern := filepath.Join(root, dir, "*.json")
		paths, err := filepath.Glob(pattern)
		if err != nil {
			return PruneReport{}, fmt.Errorf("glob %s: %w", dir, err)
		}
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil {
				return PruneReport{}, fmt.Errorf("stat %s: %w", path, err)
			}
			if !pruneAll && now.Sub(info.ModTime()) < olderThan {
				continue
			}
			if err := os.Remove(path); err != nil {
				return PruneReport{}, fmt.Errorf("remove %s: %w", path, err)
			}
			report.RemovedFiles++
			report.RemovedPaths = append(report.RemovedPaths, path)
		}
	}
	return report, nil
}
