package transport

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/store"
)

func writePruneUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  needlex prune (--all | --older-than-hours N | --embedding-cache) [--dry-run] [--json]")
}

func (r Runner) runPrune(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var jsonOut bool
	var pruneAll bool
	var dryRun bool
	var embeddingCache bool
	var olderThanHours int

	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.BoolVar(&pruneAll, "all", false, "remove all local state")
	fs.BoolVar(&dryRun, "dry-run", false, "report removable state without deleting it")
	fs.BoolVar(&embeddingCache, "embedding-cache", false, "target PAL embedding cache files")
	fs.IntVar(&olderThanHours, "older-than-hours", 0, "remove state older than N hours")

	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{
		"--older-than-hours": {},
		"-older-than-hours":  {},
	})); err != nil {
		return 2
	}
	if fs.NArg() != 0 || (!pruneAll && olderThanHours <= 0 && !embeddingCache) || (dryRun && !embeddingCache) {
		writePruneUsage(stderr)
		return 2
	}
	if embeddingCache {
		return r.runEmbeddingCachePrune(stdout, stderr, jsonOut, dryRun)
	}

	report, err := store.Prune(r.storeRoot, time.Duration(olderThanHours)*time.Hour, pruneAll, time.Now().UTC())
	if err != nil {
		return r.reportCLIError(stderr, "prune", err, map[string]any{"all": pruneAll, "older_than_hours": olderThanHours})
	}

	if jsonOut {
		return r.writeJSON(stdout, stderr, "prune", report)
	}

	fmt.Fprintf(stdout, "Removed Files: %d\n", report.RemovedFiles)
	return 0
}

func (r Runner) runEmbeddingCachePrune(stdout, stderr io.Writer, jsonOut, dryRun bool) int {
	cfg, err := config.Load("")
	if err != nil {
		cfg = config.Defaults()
	}
	layout := platform.NewStateLayout(r.storeRoot)
	policy := intel.EmbeddingCachePolicyFromConfigEnv(cfg.Semantic.EmbeddingURL, layout.EmbeddingCacheDir, cfg.Semantic.EmbeddingCache)
	report, err := intel.PruneEmbeddingCache(policy.Dir, dryRun, time.Now().UTC(), policy)
	if err != nil {
		return r.reportCLIError(stderr, "prune_embedding_cache", err, map[string]any{"dry_run": dryRun})
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "prune_embedding_cache", report)
	}
	if dryRun {
		fmt.Fprintf(stdout, "Matched Files: %d\n", report.MatchedFiles)
		fmt.Fprintf(stdout, "Reclaimable Bytes: %d\n", report.RemovedBytes)
		return 0
	}
	fmt.Fprintf(stdout, "Removed Files: %d\n", report.RemovedFiles)
	fmt.Fprintf(stdout, "Removed Bytes: %d\n", report.RemovedBytes)
	return 0
}
