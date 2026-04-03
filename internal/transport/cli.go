package transport

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/josepavese/needlex/internal/buildinfo"
	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/proof"
)

type Runner struct {
	loadConfig func(path string) (config.Config, error)
	read       func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error)
	query      func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error)
	crawl      func(ctx context.Context, cfg config.Config, req coreservice.CrawlRequest) (coreservice.CrawlResponse, error)
	stdin      io.Reader
	storeRoot  string
}

func NewRunner() Runner {
	newService := func(cfg config.Config) (*coreservice.Service, error) {
		return coreservice.New(cfg, nil)
	}
	return Runner{
		loadConfig: config.Load,
		read: func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error) {
			svc, err := newService(cfg)
			if err != nil {
				return coreservice.ReadResponse{}, err
			}
			return svc.Read(ctx, req)
		},
		query: func(ctx context.Context, cfg config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
			svc, err := newService(cfg)
			if err != nil {
				return coreservice.QueryResponse{}, err
			}
			return svc.Query(ctx, req)
		},
		crawl: func(ctx context.Context, cfg config.Config, req coreservice.CrawlRequest) (coreservice.CrawlResponse, error) {
			svc, err := newService(cfg)
			if err != nil {
				return coreservice.CrawlResponse{}, err
			}
			return svc.Crawl(ctx, req)
		},
		stdin:     os.Stdin,
		storeRoot: platform.DefaultStateRoot(),
	}
}

func (r Runner) Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeRootUsage(stderr)
		return 2
	}

	switch args[0] {
	case "crawl":
		return r.runCrawl(args[1:], stdout, stderr)
	case "query":
		return r.runQuery(args[1:], stdout, stderr)
	case "read":
		return r.runRead(args[1:], stdout, stderr)
	case "replay":
		return r.runReplay(args[1:], stdout, stderr)
	case "diff":
		return r.runDiff(args[1:], stdout, stderr)
	case "proof":
		return r.runProof(args[1:], stdout, stderr)
	case "memory":
		return r.runMemory(args[1:], stdout, stderr)
	case "prune":
		return r.runPrune(args[1:], stdout, stderr)
	case "mcp":
		return r.runMCP(args[1:], stdout, stderr)
	case "tool-catalog":
		return r.runToolCatalog(args[1:], stdout, stderr)
	case "version":
		return r.runVersion(args[1:], stdout, stderr)
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
	var jsonMode string

	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.StringVar(&objective, "objective", "", "optional read objective")
	fs.StringVar(&profile, "profile", "", "packing profile: tiny, standard, or deep")
	fs.StringVar(&userAgent, "user-agent", "", "override HTTP user agent")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.StringVar(&jsonMode, "json-mode", jsonModeCompact, "json output mode: compact or full")

	if err := fs.Parse(normalizeArgs(args, readValueFlags)); err != nil {
		return 2
	}
	mode, err := normalizeJSONMode(jsonMode)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	if fs.NArg() != 1 {
		writeReadUsage(stderr)
		return 2
	}

	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}

	resp, artifacts, err := r.executeRead(cfg, coreservice.ReadRequest{
		URL:       fs.Arg(0),
		Objective: objective,
		Profile:   profile,
		UserAgent: userAgent,
	})
	if err != nil {
		fmt.Fprintf(stderr, "read failed: %v\n", err)
		return 1
	}

	if jsonOut {
		if mode == jsonModeFull {
			return writeJSON(stdout, stderr, resp)
		}
		return writeJSON(stdout, stderr, compactReadResponse(resp))
	}

	renderReadText(stdout, resp, artifacts)
	return 0
}

func writeRootUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  needlex crawl <seed-url> [--json] [--json-mode compact|full] [--config path] [--profile name] [--max-pages N] [--max-depth N] [--same-domain]
  needlex query [seed-url] --goal text [--json] [--json-mode compact|full] [--config path] [--profile name] [--user-agent ua] [--discovery mode]
  needlex read <url> [--json] [--json-mode compact|full] [--config path] [--objective text] [--profile name] [--user-agent ua]
  needlex replay <trace-id> [--json]
  needlex diff <trace-a> <trace-b> [--json]
  needlex proof <trace-id|proof-id|chunk-id> [--json]
  needlex memory <stats|search|prune> [args]
  needlex prune (--all | --older-than-hours N) [--json]
  needlex mcp
  needlex tool-catalog --provider openai|anthropic [--strict]
  needlex version
`)
}

func (r Runner) runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "version does not accept positional arguments")
		return 2
	}
	_, _ = fmt.Fprintln(stdout, buildinfo.Version)
	return 0
}

func writeQueryUsage(w io.Writer) {
	writeUsage(w, "needlex query [seed-url] --goal text [--json] [--json-mode compact|full] [--config path] [--profile name] [--user-agent ua] [--discovery mode]", "note: when seed-url is omitted, discovery defaults to web_search")
}

func writeReadUsage(w io.Writer) {
	writeUsage(w, "needlex read <url> [--json] [--json-mode compact|full] [--config path] [--objective text] [--profile name] [--user-agent ua]")
}

func writeReplayUsage(w io.Writer) {
	writeUsage(w, "needlex replay <trace-id> [--json]")
}

func writeDiffUsage(w io.Writer) {
	writeUsage(w, "needlex diff <trace-a> <trace-b> [--json]")
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

	report, err := r.loadReplay(fs.Arg(0))
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

	report, err := r.loadDiff(fs.Arg(0), fs.Arg(1))
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

func renderReadText(w io.Writer, resp coreservice.ReadResponse, artifacts artifactPaths) {
	compact := compactReadResponse(resp)
	title := strings.TrimSpace(compact.Title)
	if title == "" {
		title = "(untitled)"
	}

	fmt.Fprintf(w, "Kind: %s\n", compact.Kind)
	fmt.Fprintf(w, "Title: %s\n", title)
	fmt.Fprintf(w, "URL: %s\n", compact.URL)
	if compact.Summary != "" {
		fmt.Fprintf(w, "Summary: %s\n", compact.Summary)
	}
	fmt.Fprintf(w, "Uncertainty: %s\n", compact.Uncertainty.Level)
	fmt.Fprintf(w, "Chunks: %d\n", len(compact.Chunks))
	renderWebIRSummary(w, resp.WebIR.NodeCount, resp.WebIR.Signals)
	if compact.Profile != "" {
		fmt.Fprintf(w, "Profile: %s\n", compact.Profile)
	}
	fmt.Fprintf(w, "Latency: %dms\n", compact.CostReport.LatencyMS)
	fmt.Fprintf(w, "Trace ID: %s\n", compact.TraceID)
	renderArtifactPaths(w, artifacts)
	if pack := tracePackMetadata(resp.Trace); len(pack) > 0 {
		renderPackMetadata(w, pack, true)
	}
	renderChunkTexts(w, resp.ResultPack.Chunks, true)
}

func tracePackMetadata(trace proof.RunTrace) map[string]string {
	for _, stage := range trace.Stages {
		if stage.Stage != "pack" || len(stage.Metadata) == 0 {
			continue
		}
		return stage.Metadata
	}
	return nil
}

func firstNonEmptyValue(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func renderReplayText(w io.Writer, report proof.ReplayReport) {
	fmt.Fprintf(w, "Trace ID: %s\nRun ID: %s\nStages: %d\nEvents: %d\nDeterministic: %t\n", report.TraceID, report.RunID, report.StageCount, report.EventCount, report.Deterministic)
	if len(report.CompletedStages) > 0 {
		fmt.Fprintf(w, "Completed: %s\n", strings.Join(report.CompletedStages, ", "))
	}
}

func renderDiffText(w io.Writer, report proof.DiffReport) {
	fmt.Fprintf(w, "Trace A: %s\nTrace B: %s\nChanged Stages: %d\n", report.TraceA, report.TraceB, len(report.ChangedStages))
	for _, stage := range report.ChangedStages {
		fmt.Fprintf(w, "- %s: %s\n", stage.Stage, stage.Status)
	}
}

func renderPackMetadata(w io.Writer, pack map[string]string, includePolicy bool) {
	fmt.Fprintf(w, "IR Selection: embedded=%s heading=%s shallow=%s\n", firstNonEmptyValue(pack["selected_ir_embedded_hits"], "0"), firstNonEmptyValue(pack["selected_ir_heading_hits"], "0"), firstNonEmptyValue(pack["selected_ir_shallow_hits"], "0"))
	if includePolicy {
		fmt.Fprintf(w, "IR Policy: embedded_required=%s embedded_applied=%s heading_required=%s heading_applied=%s noise_swap=%s\n",
			firstNonEmptyValue(pack["web_ir_policy_embedded_required"], "false"),
			firstNonEmptyValue(pack["web_ir_policy_embedded_applied"], "false"),
			firstNonEmptyValue(pack["web_ir_policy_heading_required"], "false"),
			firstNonEmptyValue(pack["web_ir_policy_heading_applied"], "false"),
			firstNonEmptyValue(pack["web_ir_policy_noise_swap"], "false"),
		)
	}
}

func writeJSON(stdout, stderr io.Writer, value any) int {
	if err := writeIndentedJSON(stdout, value); err != nil {
		fmt.Fprintf(stderr, "encode output: %v\n", err)
		return 1
	}
	return 0
}

func writeIndentedJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func writeUsage(w io.Writer, lines ...string) {
	fmt.Fprintln(w, "Usage:")
	for _, line := range lines {
		fmt.Fprintf(w, "  %s\n", line)
	}
}

func (r Runner) loadConfigOrExit(path string, stderr io.Writer) (config.Config, bool) {
	cfg, err := r.loadConfig(path)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return config.Config{}, false
	}
	return cfg, true
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

var readValueFlags = map[string]struct{}{
	"--config":     {},
	"-config":      {},
	"--json-mode":  {},
	"-json-mode":   {},
	"--objective":  {},
	"-objective":   {},
	"--profile":    {},
	"-profile":     {},
	"--user-agent": {},
	"-user-agent":  {},
}
