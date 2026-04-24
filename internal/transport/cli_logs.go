package transport

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/josepavese/needlex/internal/observability"
)

type logsStatsResult struct {
	Stats observability.LogStats `json:"stats"`
}

type logsTailResult struct {
	Events []observability.Event `json:"events"`
}

func writeLogsUsage(w io.Writer) {
	writeUsage(w, "needlex logs <path|stats|tail> [--limit N] [--json]")
}

func (r Runner) runLogs(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return r.runLogsStats(nil, stdout, stderr)
	}
	switch args[0] {
	case "path":
		return r.runLogsPath(args[1:], stdout, stderr)
	case "stats":
		return r.runLogsStats(args[1:], stdout, stderr)
	case "tail":
		return r.runLogsTail(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		writeLogsUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown logs subcommand %q\n\n", args[0])
		writeLogsUsage(stderr)
		return 2
	}
}

func (r Runner) runLogsPath(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("logs path", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex logs path")
		return 2
	}
	fmt.Fprintln(stdout, r.runtimeLogger().Path())
	return 0
}

func (r Runner) runLogsStats(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("logs stats", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex logs stats [--json]")
		return 2
	}
	stats, err := r.runtimeLogger().Stats()
	if err != nil {
		return r.reportCLIError(stderr, "logs_stats", err, nil)
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "logs_stats", logsStatsResult{Stats: stats})
	}
	fmt.Fprintf(stdout, "Runtime Log: %s\n", stats.Path)
	fmt.Fprintf(stdout, "Exists: %t\n", stats.Exists)
	fmt.Fprintf(stdout, "Size: %d bytes\n", stats.SizeBytes)
	fmt.Fprintf(stdout, "Events: %d\n", stats.LineCount)
	fmt.Fprintf(stdout, "Rotated Files: %d\n", len(stats.Rotated))
	if !stats.LastSeenAt.IsZero() {
		fmt.Fprintf(stdout, "Last Seen At: %s\n", stats.LastSeenAt.Format(time.RFC3339))
	}
	if stats.LastEvent.ID != "" {
		fmt.Fprintf(stdout, "Last Event: %s %s %s\n", stats.LastEvent.ID, stats.LastEvent.Level, stats.LastEvent.Event)
	}
	return 0
}

func (r Runner) runLogsTail(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("logs tail", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	var limit int
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.IntVar(&limit, "limit", 20, "number of events")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{"--limit": {}, "-limit": {}})); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex logs tail [--limit N] [--json]")
		return 2
	}
	events, err := r.runtimeLogger().Tail(limit)
	if err != nil {
		return r.reportCLIError(stderr, "logs_tail", err, map[string]any{"limit": limit})
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "logs_tail", logsTailResult{Events: events})
	}
	fmt.Fprintf(stdout, "Events: %d\n", len(events))
	for _, event := range events {
		fmt.Fprintf(stdout, "%s %s %s %s %s\n", event.TimestampUTC, event.Level, event.ID, event.Operation, event.Event)
		if event.FailureClass != "" {
			fmt.Fprintf(stdout, "  Class: %s\n", event.FailureClass)
		}
		if event.Message != "" {
			fmt.Fprintf(stdout, "  Message: %s\n", event.Message)
		}
	}
	return 0
}
