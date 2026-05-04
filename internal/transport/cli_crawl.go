package transport

import (
	"flag"
	"fmt"
	"io"

	coreservice "github.com/josepavese/needlex/internal/core/service"
)

type crawlArtifacts struct {
	StoredRuns int `json:"stored_runs"`
}

func writeCrawlUsage(w io.Writer) {
	writeUsage(w, "needlex crawl <seed-url> [--json] [--json-mode compact|full] [--config path] [--profile name] [--max-pages N] [--max-depth N] [--same-domain] [--retrieval-effort minimal|light|balanced|standard|exhaustive]")
}

func (r Runner) runCrawl(args []string, stdout, stderr io.Writer) int {
	configPath, profile, userAgent, seedURL, retrievalEffort, jsonMode, maxPages, maxDepth, sameDomain, jsonOut, ok := parseCrawlArgs(args, stderr)
	if !ok {
		writeCrawlUsage(stderr)
		return 2
	}
	mode, err := normalizeJSONMode(jsonMode)
	if err != nil {
		return r.reportCLIError(stderr, "crawl", err, map[string]any{"phase": "parse_json_mode"})
	}

	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	if err := applyRetrievalEffort(retrievalEffort, &cfg); err != nil {
		return r.reportCLIErrorCode(stderr, "crawl", err, map[string]any{"retrieval_effort": retrievalEffort}, 2)
	}

	resp, artifacts, err := r.executeCrawlWithSurface(cfg, coreservice.CrawlRequest{
		SeedURL:    seedURL,
		Profile:    profile,
		UserAgent:  userAgent,
		MaxPages:   maxPages,
		MaxDepth:   maxDepth,
		SameDomain: sameDomain,
	}, "cli")
	if err != nil {
		return r.reportCLIError(stderr, "crawl", err, map[string]any{"seed_url": seedURL, "same_domain": sameDomain})
	}

	if jsonOut {
		if mode == jsonModeFull {
			return r.writeJSON(stdout, stderr, "crawl", map[string]any{"documents": resp.Documents, "summary": resp.Summary, "pages": resp.Pages, "stored_runs": artifacts.StoredRuns})
		}
		return r.writeJSON(stdout, stderr, "crawl", compactCrawlResponse(resp, artifacts))
	}

	renderCrawlText(stdout, resp, artifacts)
	return 0
}

func parseCrawlArgs(args []string, stderr io.Writer) (configPath, profile, userAgent, seedURL, retrievalEffort, jsonMode string, maxPages, maxDepth int, sameDomain, jsonOut, ok bool) {
	fs := flag.NewFlagSet("crawl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.StringVar(&profile, "profile", "", "packing profile: tiny, standard, or deep")
	fs.StringVar(&userAgent, "user-agent", "", "override HTTP user agent")
	fs.StringVar(&retrievalEffort, "retrieval-effort", "", "retrieval effort: minimal, light, balanced, standard, or exhaustive")
	fs.StringVar(&jsonMode, "json-mode", jsonModeCompact, "json output mode: compact or full")
	fs.IntVar(&maxPages, "max-pages", 0, "maximum pages to visit")
	fs.IntVar(&maxDepth, "max-depth", 0, "maximum crawl depth")
	fs.BoolVar(&sameDomain, "same-domain", false, "restrict crawl to the seed domain")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{
		"--config":           {},
		"-config":            {},
		"--json-mode":        {},
		"-json-mode":         {},
		"--profile":          {},
		"-profile":           {},
		"--max-pages":        {},
		"-max-pages":         {},
		"--max-depth":        {},
		"-max-depth":         {},
		"--user-agent":       {},
		"-user-agent":        {},
		"--retrieval-effort": {},
		"-retrieval-effort":  {},
	})); err != nil {
		return "", "", "", "", "", "", 0, 0, false, false, false
	}
	if fs.NArg() != 1 {
		return "", "", "", "", "", "", 0, 0, false, false, false
	}
	return configPath, profile, userAgent, fs.Arg(0), retrievalEffort, jsonMode, maxPages, maxDepth, sameDomain, jsonOut, true
}

func renderCrawlText(w io.Writer, resp coreservice.CrawlResponse, artifacts crawlArtifacts) {
	fmt.Fprintf(w, "Seed URL: %s\n", resp.Summary.SeedURL)
	fmt.Fprintf(w, "Pages Visited: %d\n", resp.Summary.PagesVisited)
	fmt.Fprintf(w, "Max Depth: %d\n", resp.Summary.MaxDepthReached)
	fmt.Fprintf(w, "Chunks: %d\n", resp.Summary.ChunkCount)
	fmt.Fprintf(w, "Stored Runs: %d\n", artifacts.StoredRuns)
	for i, doc := range resp.Documents {
		fmt.Fprintf(w, "\n[%d] %s\n", i+1, doc.FinalURL)
		if doc.Title != "" {
			fmt.Fprintf(w, "Title: %s\n", doc.Title)
		}
	}
}
