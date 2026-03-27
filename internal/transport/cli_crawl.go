package transport

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	coreservice "github.com/josepavese/needlex/internal/core/service"
)

type crawlArtifacts struct {
	StoredRuns int `json:"stored_runs"`
}

func writeCrawlUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  needle crawl <seed-url> [--json] [--config path] [--profile name] [--max-pages N] [--max-depth N] [--same-domain]")
}

func (r Runner) runCrawl(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("crawl", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var configPath string
	var profile string
	var maxPages int
	var maxDepth int
	var userAgent string
	var sameDomain bool
	var jsonOut bool

	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.StringVar(&profile, "profile", "", "packing profile: tiny, standard, or deep")
	fs.StringVar(&userAgent, "user-agent", "", "override HTTP user agent")
	fs.IntVar(&maxPages, "max-pages", 0, "maximum pages to visit")
	fs.IntVar(&maxDepth, "max-depth", 0, "maximum crawl depth")
	fs.BoolVar(&sameDomain, "same-domain", false, "restrict crawl to the seed domain")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")

	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{
		"--config":     {},
		"-config":      {},
		"--profile":    {},
		"-profile":     {},
		"--max-pages":  {},
		"-max-pages":   {},
		"--max-depth":  {},
		"-max-depth":   {},
		"--user-agent": {},
		"-user-agent":  {},
	})); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		writeCrawlUsage(stderr)
		return 2
	}

	cfg, err := r.loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 1
	}

	resp, artifacts, err := r.executeCrawl(cfg, coreservice.CrawlRequest{
		SeedURL:    fs.Arg(0),
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
		payload := map[string]any{
			"documents":   resp.Documents,
			"summary":     resp.Summary,
			"stored_runs": artifacts.StoredRuns,
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintf(stderr, "encode output: %v\n", err)
			return 1
		}
		return 0
	}

	renderCrawlText(stdout, resp, artifacts)
	return 0
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
