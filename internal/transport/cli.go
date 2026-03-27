package transport

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/proof"
	"github.com/josepavese/needlex/internal/store"
)

type Runner struct {
	loadConfig func(path string) (config.Config, error)
	read       func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error)
	storeRoot  string
}

func NewRunner() Runner {
	return Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			svc, err := coreservice.New(cfg, nil)
			if err != nil {
				return coreservice.ReadResponse{}, err
			}
			return svc.Read(ctx, req)
		},
		storeRoot: ".needlex",
	}
}

func (r Runner) Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeRootUsage(stderr)
		return 2
	}

	switch args[0] {
	case "read":
		return r.runRead(args[1:], stdout, stderr)
	case "replay":
		return r.runReplay(args[1:], stdout, stderr)
	case "diff":
		return r.runDiff(args[1:], stdout, stderr)
	case "proof":
		return r.runProof(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		writeRootUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		writeRootUsage(stderr)
		return 2
	}
}

func (r Runner) runRead(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("read", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var configPath string
	var objective string
	var profile string
	var userAgent string
	var jsonOut bool

	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.StringVar(&objective, "objective", "", "optional read objective")
	fs.StringVar(&profile, "profile", "", "packing profile: tiny, standard, or deep")
	fs.StringVar(&userAgent, "user-agent", "", "override HTTP user agent")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")

	if err := fs.Parse(normalizeReadArgs(args)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		writeReadUsage(stderr)
		return 2
	}

	cfg, err := r.loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}

	resp, err := r.read(context.Background(), cfg, coreservice.ReadRequest{
		URL:       fs.Arg(0),
		Objective: objective,
		Profile:   profile,
		UserAgent: userAgent,
	})
	if err != nil {
		fmt.Fprintf(stderr, "read failed: %v\n", err)
		return 1
	}

	tracePath, err := store.NewTraceStore(r.storeRoot).SaveTrace(resp.Trace)
	if err != nil {
		fmt.Fprintf(stderr, "save trace: %v\n", err)
		return 1
	}
	proofPath, err := store.NewProofStore(r.storeRoot).SaveProofRecords(resp.Trace.TraceID, resp.ProofRecords)
	if err != nil {
		fmt.Fprintf(stderr, "save proofs: %v\n", err)
		return 1
	}

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintf(stderr, "encode output: %v\n", err)
			return 1
		}
		return 0
	}

	renderReadText(stdout, resp, tracePath, proofPath)
	return 0
}

func writeRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  needle read <url> [--json] [--config path] [--objective text] [--profile name] [--user-agent ua]")
	fmt.Fprintln(w, "  needle replay <trace-id> [--json]")
	fmt.Fprintln(w, "  needle diff <trace-a> <trace-b> [--json]")
	fmt.Fprintln(w, "  needle proof <trace-id|chunk-id> [--json]")
}

func writeReadUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  needle read <url> [--json] [--config path] [--objective text] [--profile name] [--user-agent ua]")
}

func writeReplayUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  needle replay <trace-id> [--json]")
}

func writeDiffUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  needle diff <trace-a> <trace-b> [--json]")
}

func normalizeReadArgs(args []string) []string {
	valueFlags := map[string]struct{}{
		"--config":     {},
		"-config":      {},
		"--objective":  {},
		"-objective":   {},
		"--profile":    {},
		"-profile":     {},
		"--user-agent": {},
		"-user-agent":  {},
	}

	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}

		flags = append(flags, arg)
		if _, ok := valueFlags[arg]; ok && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, positionals...)
}

func (r Runner) runReplay(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")

	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		writeReplayUsage(stderr)
		return 2
	}

	trace, err := store.NewTraceStore(r.storeRoot).LoadTrace(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "load trace: %v\n", err)
		return 1
	}
	report, err := trace.ReplayReport()
	if err != nil {
		fmt.Fprintf(stderr, "replay failed: %v\n", err)
		return 1
	}

	if jsonOut {
		return writeJSON(stdout, stderr, report)
	}

	renderReplayText(stdout, report)
	return 0
}

func (r Runner) runDiff(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")

	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		writeDiffUsage(stderr)
		return 2
	}

	traceA, err := store.NewTraceStore(r.storeRoot).LoadTrace(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "load trace a: %v\n", err)
		return 1
	}
	traceB, err := store.NewTraceStore(r.storeRoot).LoadTrace(fs.Arg(1))
	if err != nil {
		fmt.Fprintf(stderr, "load trace b: %v\n", err)
		return 1
	}
	report, err := proof.Diff(traceA, traceB)
	if err != nil {
		fmt.Fprintf(stderr, "diff failed: %v\n", err)
		return 1
	}

	if jsonOut {
		return writeJSON(stdout, stderr, report)
	}

	renderDiffText(stdout, report)
	return 0
}

func renderReadText(w io.Writer, resp coreservice.ReadResponse, tracePath, proofPath string) {
	title := strings.TrimSpace(resp.Document.Title)
	if title == "" {
		title = "(untitled)"
	}

	fmt.Fprintf(w, "Title: %s\n", title)
	fmt.Fprintf(w, "URL: %s\n", resp.Document.FinalURL)
	fmt.Fprintf(w, "Chunks: %d\n", len(resp.ResultPack.Chunks))
	if resp.ResultPack.Profile != "" {
		fmt.Fprintf(w, "Profile: %s\n", resp.ResultPack.Profile)
	}
	fmt.Fprintf(w, "Proof Records: %d\n", len(resp.ProofRecords))
	fmt.Fprintf(w, "Stages: %d\n", resp.Replay.StageCount)
	fmt.Fprintf(w, "Latency: %dms\n", resp.ResultPack.CostReport.LatencyMS)
	fmt.Fprintf(w, "Trace ID: %s\n", resp.Trace.TraceID)
	fmt.Fprintf(w, "Trace Path: %s\n", tracePath)
	fmt.Fprintf(w, "Proof Path: %s\n", proofPath)

	for i, chunk := range resp.ResultPack.Chunks {
		fmt.Fprintf(w, "\n[%d] ", i+1)
		if len(chunk.HeadingPath) > 0 {
			fmt.Fprintln(w, strings.Join(chunk.HeadingPath, " > "))
		} else {
			fmt.Fprintln(w, "(no heading)")
		}
		fmt.Fprintf(w, "%s\n", chunk.Text)
	}
}

func renderReplayText(w io.Writer, report proof.ReplayReport) {
	fmt.Fprintf(w, "Trace ID: %s\n", report.TraceID)
	fmt.Fprintf(w, "Run ID: %s\n", report.RunID)
	fmt.Fprintf(w, "Stages: %d\n", report.StageCount)
	fmt.Fprintf(w, "Events: %d\n", report.EventCount)
	fmt.Fprintf(w, "Deterministic: %t\n", report.Deterministic)
	if len(report.CompletedStages) > 0 {
		fmt.Fprintf(w, "Completed: %s\n", strings.Join(report.CompletedStages, ", "))
	}
}

func renderDiffText(w io.Writer, report proof.DiffReport) {
	fmt.Fprintf(w, "Trace A: %s\n", report.TraceA)
	fmt.Fprintf(w, "Trace B: %s\n", report.TraceB)
	fmt.Fprintf(w, "Changed Stages: %d\n", len(report.ChangedStages))
	for _, stage := range report.ChangedStages {
		fmt.Fprintf(w, "- %s: %s\n", stage.Stage, stage.Status)
	}
}

func writeJSON(stdout, stderr io.Writer, value any) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		fmt.Fprintf(stderr, "encode output: %v\n", err)
		return 1
	}
	return 0
}

func normalizeArgs(args []string, valueFlags map[string]struct{}) []string {
	if valueFlags == nil {
		valueFlags = map[string]struct{}{}
	}
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}

		flags = append(flags, arg)
		if _, ok := valueFlags[arg]; ok && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, positionals...)
}
