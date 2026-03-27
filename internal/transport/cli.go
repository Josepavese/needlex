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
)

type Runner struct {
	loadConfig func(path string) (config.Config, error)
	read       func(ctx context.Context, cfg config.Config, req coreservice.ReadRequest) (coreservice.ReadResponse, error)
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
	var userAgent string
	var jsonOut bool

	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.StringVar(&objective, "objective", "", "optional read objective")
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
		UserAgent: userAgent,
	})
	if err != nil {
		fmt.Fprintf(stderr, "read failed: %v\n", err)
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

	renderReadText(stdout, resp)
	return 0
}

func writeRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  needle read <url> [--json] [--config path] [--objective text] [--user-agent ua]")
}

func writeReadUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  needle read <url> [--json] [--config path] [--objective text] [--user-agent ua]")
}

func normalizeReadArgs(args []string) []string {
	valueFlags := map[string]struct{}{
		"--config":     {},
		"-config":      {},
		"--objective":  {},
		"-objective":   {},
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

func renderReadText(w io.Writer, resp coreservice.ReadResponse) {
	title := strings.TrimSpace(resp.Document.Title)
	if title == "" {
		title = "(untitled)"
	}

	fmt.Fprintf(w, "Title: %s\n", title)
	fmt.Fprintf(w, "URL: %s\n", resp.Document.FinalURL)
	fmt.Fprintf(w, "Chunks: %d\n", len(resp.ResultPack.Chunks))
	fmt.Fprintf(w, "Proof Records: %d\n", len(resp.ProofRecords))
	fmt.Fprintf(w, "Stages: %d\n", resp.Replay.StageCount)
	fmt.Fprintf(w, "Latency: %dms\n", resp.ResultPack.CostReport.LatencyMS)

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
