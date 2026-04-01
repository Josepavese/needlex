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
	writeUsage(w, "needlex crawl <seed-url> [--json] [--json-mode compact|full] [--config path] [--profile name] [--max-pages N] [--max-depth N] [--same-domain]")
}

func (r Runner) runCrawl(args []string, stdout, stderr io.Writer) int {
	configPath, profile, userAgent, seedURL, jsonMode, maxPages, maxDepth, sameDomain, jsonOut, ok := parseCrawlArgs(args, stderr)
	if !ok {
		writeCrawlUsage(stderr)
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

	resp, artifacts, err := r.executeCrawl(cfg, coreservice.CrawlRequest{
		SeedURL:    seedURL,
		Profile:    profile,
		UserAgent:  userAgent,
		MaxPages:   maxPages,
		MaxDepth:   maxDepth,
		SameDomain: sameDomain,
	})
	if err != nil {
		fmt.Fprintf(stderr, "crawl failed: %v\n", err)
		return 1
	}

	if jsonOut {
		if mode == jsonModeFull {
			return writeJSON(stdout, stderr, map[string]any{"documents": resp.Documents, "summary": resp.Summary, "pages": resp.Pages, "stored_runs": artifacts.StoredRuns})
		}
		return writeJSON(stdout, stderr, compactCrawlResponse(resp, artifacts))
	}

	renderCrawlText(stdout, resp, artifacts)
	return 0
}

func parseCrawlArgs(args []string, stderr io.Writer) (configPath, profile, userAgent, seedURL, jsonMode string, maxPages, maxDepth int, sameDomain, jsonOut, ok bool) {
	fs := flag.NewFlagSet("crawl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.StringVar(&profile, "profile", "", "packing profile: tiny, standard, or deep")
	fs.StringVar(&userAgent, "user-agent", "", "override HTTP user agent")
	fs.StringVar(&jsonMode, "json-mode", jsonModeCompact, "json output mode: compact or full")
	fs.IntVar(&maxPages, "max-pages", 0, "maximum pages to visit")
	fs.IntVar(&maxDepth, "max-depth", 0, "maximum crawl depth")
	fs.BoolVar(&sameDomain, "same-domain", false, "restrict crawl to the seed domain")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{
		"--config":     {},
		"-config":      {},
		"--json-mode":  {},
		"-json-mode":   {},
		"--profile":    {},
		"-profile":     {},
		"--max-pages":  {},
		"-max-pages":   {},
		"--max-depth":  {},
		"-max-depth":   {},
		"--user-agent": {},
		"-user-agent":  {},
	})); err != nil {
		return "", "", "", "", "", 0, 0, false, false, false
	}
	if fs.NArg() != 1 {
		return "", "", "", "", "", 0, 0, false, false, false
	}
	return configPath, profile, userAgent, fs.Arg(0), jsonMode, maxPages, maxDepth, sameDomain, jsonOut, true
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
