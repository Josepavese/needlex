package transport

import (
	"flag"
	"fmt"
	"io"
	"strings"

	coreservice "github.com/josepavese/needlex/internal/core/service"
)

func (r Runner) runQuery(args []string, stdout, stderr io.Writer) int {
	configPath, goal, profile, userAgent, discovery, seedURL, jsonMode, jsonOut, ok := parseQueryArgs(args, stderr)
	if !ok {
		writeQueryUsage(stderr)
		return 2
	}
	mode, err := normalizeJSONMode(jsonMode)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}

	resp, artifacts, err := r.executeQuery(cfg, coreservice.QueryRequest{
		Goal:          goal,
		SeedURL:       seedURL,
		Profile:       profile,
		UserAgent:     userAgent,
		DiscoveryMode: discovery,
	})
	if err != nil {
		fmt.Fprintf(stderr, "query failed: %v\n", err)
		return 1
	}

	if jsonOut {
		if mode == jsonModeFull {
			return writeJSON(stdout, stderr, resp)
		}
		return writeJSON(stdout, stderr, compactQueryResponse(resp))
	}

	renderQueryText(stdout, resp, artifacts)
	return 0
}

func parseQueryArgs(args []string, stderr io.Writer) (configPath, goal, profile, userAgent, discovery, seedURL, jsonMode string, jsonOut, ok bool) {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.StringVar(&goal, "goal", "", "query goal")
	fs.StringVar(&profile, "profile", "", "packing profile: tiny, standard, or deep")
	fs.StringVar(&userAgent, "user-agent", "", "override HTTP user agent")
	fs.StringVar(&discovery, "discovery", "", "query discovery mode: same_site_links, web_search, or off")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.StringVar(&jsonMode, "json-mode", jsonModeCompact, "json output mode: compact or full")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{
		"--config":     {},
		"-config":      {},
		"--json-mode":  {},
		"-json-mode":   {},
		"--goal":       {},
		"-goal":        {},
		"--profile":    {},
		"-profile":     {},
		"--user-agent": {},
		"-user-agent":  {},
		"--discovery":  {},
		"-discovery":   {},
	})); err != nil {
		return "", "", "", "", "", "", "", false, false
	}
	if fs.NArg() > 1 || goal == "" {
		return "", "", "", "", "", "", "", false, false
	}
	if fs.NArg() == 1 {
		seedURL = fs.Arg(0)
	}
	return configPath, goal, profile, userAgent, discovery, seedURL, jsonMode, jsonOut, true
}

func renderQueryText(w io.Writer, resp coreservice.QueryResponse, artifacts artifactPaths) {
	compact := compactQueryResponse(resp)
	fmt.Fprintf(w, "Kind: %s\n", compact.Kind)
	fmt.Fprintf(w, "Goal: %s\n", compact.Goal)
	if compact.SeedURL != "" {
		fmt.Fprintf(w, "Seed URL: %s\n", compact.SeedURL)
	}
	if resp.Plan.DiscoveryMode != "" {
		fmt.Fprintf(w, "Discovery: %s\n", resp.Plan.DiscoveryMode)
	}
	if compact.Provider != "" {
		fmt.Fprintf(w, "Provider: %s\n", compact.Provider)
	}
	fmt.Fprintf(w, "Selected URL: %s\n", compact.SelectedURL)
	if compact.Summary != "" {
		fmt.Fprintf(w, "Summary: %s\n", compact.Summary)
	}
	fmt.Fprintf(w, "Uncertainty: %s\n", compact.Uncertainty.Level)
	if len(compact.SelectionWhy) > 0 {
		fmt.Fprintf(w, "Selection Why: %s\n", strings.Join(compact.SelectionWhy, ", "))
	}
	fmt.Fprintf(w, "Profile: %s\n", compact.Profile)
	fmt.Fprintf(w, "Trace ID: %s\n", compact.TraceID)
	renderArtifactPaths(w, artifacts)
	fmt.Fprintf(w, "Candidates: %d\n", len(compact.Candidates))
	fmt.Fprintf(w, "Chunks: %d\n", len(compact.Chunks))
	renderWebIRSummary(w, resp.WebIR.NodeCount, resp.WebIR.Signals)
	if pack := tracePackMetadata(resp.Trace); len(pack) > 0 {
		renderPackMetadata(w, pack, false)
	}
	renderChunkTexts(w, resp.ResultPack.Chunks, false)
}
