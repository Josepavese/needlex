package transport

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/josepavese/needlex/internal/store"
)

func writePruneUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  needlex prune (--all | --older-than-hours N) [--json]")
}

func (r Runner) runPrune(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var jsonOut bool
	var pruneAll bool
	var olderThanHours int

	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.BoolVar(&pruneAll, "all", false, "remove all local state")
	fs.IntVar(&olderThanHours, "older-than-hours", 0, "remove state older than N hours")

	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{
		"--older-than-hours": {},
		"-older-than-hours":  {},
	})); err != nil {
		return 2
	}
	if fs.NArg() != 0 || (!pruneAll && olderThanHours <= 0) {
		writePruneUsage(stderr)
		return 2
	}

	report, err := store.Prune(r.storeRoot, time.Duration(olderThanHours)*time.Hour, pruneAll, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(stderr, "prune failed: %v\n", err)
		return 1
	}

	if jsonOut {
		return writeJSON(stdout, stderr, report)
	}

	fmt.Fprintf(stdout, "Removed Files: %d\n", report.RemovedFiles)
	return 0
}
