package transport

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	coreservice "github.com/josepavese/needlex/internal/core/service"
)

func (r Runner) runQuery(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var configPath string
	var goal string
	var profile string
	var userAgent string
	var discovery string
	var jsonOut bool

	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.StringVar(&goal, "goal", "", "query goal")
	fs.StringVar(&profile, "profile", "", "packing profile: tiny, standard, or deep")
	fs.StringVar(&userAgent, "user-agent", "", "override HTTP user agent")
	fs.StringVar(&discovery, "discovery", "", "query discovery mode: same_site_links, web_search, or off")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")

	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{
		"--config":     {},
		"-config":      {},
		"--goal":       {},
		"-goal":        {},
		"--profile":    {},
		"-profile":     {},
		"--user-agent": {},
		"-user-agent":  {},
		"--discovery":  {},
		"-discovery":   {},
	})); err != nil {
		return 2
	}
	if fs.NArg() != 1 || goal == "" {
		writeQueryUsage(stderr)
		return 2
	}

	cfg, err := r.loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}

	resp, artifacts, err := r.executeQuery(cfg, coreservice.QueryRequest{
		Goal:          goal,
		SeedURL:       fs.Arg(0),
		Profile:       profile,
		UserAgent:     userAgent,
		DiscoveryMode: discovery,
	})
	if err != nil {
		fmt.Fprintf(stderr, "query failed: %v\n", err)
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

	renderQueryText(stdout, resp, artifacts)
	return 0
}

func renderQueryText(w io.Writer, resp coreservice.QueryResponse, artifacts queryArtifacts) {
	fmt.Fprintf(w, "Goal: %s\n", resp.Plan.Goal)
	fmt.Fprintf(w, "Seed URL: %s\n", resp.Plan.SeedURL)
	fmt.Fprintf(w, "Discovery: %s\n", resp.Plan.DiscoveryMode)
	if resp.Plan.DiscoveryProvider != "" {
		fmt.Fprintf(w, "Provider: %s\n", resp.Plan.DiscoveryProvider)
	}
	fmt.Fprintf(w, "Selected URL: %s\n", resp.Plan.SelectedURL)
	fmt.Fprintf(w, "Profile: %s\n", resp.ResultPack.Profile)
	fmt.Fprintf(w, "Trace ID: %s\n", resp.TraceID)
	fmt.Fprintf(w, "Trace Path: %s\n", artifacts.TracePath)
	fmt.Fprintf(w, "Proof Path: %s\n", artifacts.ProofPath)
	fmt.Fprintf(w, "Fingerprint Path: %s\n", artifacts.FingerprintPath)
	fmt.Fprintf(w, "Candidates: %d\n", len(resp.Plan.CandidateURLs))
	fmt.Fprintf(w, "Chunks: %d\n", len(resp.ResultPack.Chunks))
	for i, chunk := range resp.ResultPack.Chunks {
		fmt.Fprintf(w, "\n[%d] %s\n", i+1, chunk.Text)
	}
}
