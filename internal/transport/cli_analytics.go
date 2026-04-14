package transport

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/analytics"
)

type analyticsStatsResult struct {
	Stats analytics.Stats `json:"stats"`
}

type analyticsRecentResult struct {
	Runs []analytics.RecentRun `json:"runs"`
}

type analyticsValueReportResult struct {
	Report analytics.ValueReport `json:"report"`
}

type analyticsHostsResult struct {
	Hosts []analytics.HostRollup `json:"hosts"`
}

type analyticsProvidersResult struct {
	Providers []analytics.ProviderRollup `json:"providers"`
}

func writeAnalyticsUsage(w io.Writer) {
	writeUsage(w, "needlex analytics <stats|recent|value-report|hosts|providers> [args]")
}

func (r Runner) runAnalytics(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeAnalyticsUsage(stderr)
		return 2
	}
	switch args[0] {
	case "stats":
		return r.runAnalyticsStats(args[1:], stdout, stderr)
	case "recent":
		return r.runAnalyticsRecent(args[1:], stdout, stderr)
	case "value-report":
		return r.runAnalyticsValueReport(args[1:], stdout, stderr)
	case "hosts":
		return r.runAnalyticsHosts(args[1:], stdout, stderr)
	case "providers":
		return r.runAnalyticsProviders(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		writeAnalyticsUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown analytics subcommand %q\n\n", args[0])
		writeAnalyticsUsage(stderr)
		return 2
	}
}

func (r Runner) runAnalyticsStats(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("analytics stats", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex analytics stats [--json]")
		return 2
	}
	stats, err := analytics.NewSQLiteStore(r.storeRoot).Stats(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "analytics stats failed: %v\n", err)
		return 1
	}
	if jsonOut {
		return writeJSON(stdout, stderr, analyticsStatsResult{Stats: stats})
	}
	fmt.Fprintf(stdout, "Runs: %d\n", stats.RunCount)
	fmt.Fprintf(stdout, "Successful Runs: %d\n", stats.SuccessfulRuns)
	fmt.Fprintf(stdout, "Queries: %d\n", stats.QueryRuns)
	fmt.Fprintf(stdout, "Reads: %d\n", stats.ReadRuns)
	fmt.Fprintf(stdout, "Crawls: %d\n", stats.CrawlRuns)
	fmt.Fprintf(stdout, "Stage Events: %d\n", stats.StageEventCount)
	fmt.Fprintf(stdout, "DB Path: %s\n", stats.DBPath)
	fmt.Fprintf(stdout, "DB Size: %d bytes\n", stats.DBSizeBytes)
	if !stats.LastRunAt.IsZero() {
		fmt.Fprintf(stdout, "Last Run At: %s\n", stats.LastRunAt.Format(time.RFC3339))
	}
	return 0
}

func (r Runner) runAnalyticsRecent(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("analytics recent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	var limit int
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.IntVar(&limit, "limit", 10, "number of recent runs")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex analytics recent [--limit N] [--json]")
		return 2
	}
	runs, err := analytics.NewSQLiteStore(r.storeRoot).RecentRuns(context.Background(), limit)
	if err != nil {
		fmt.Fprintf(stderr, "analytics recent failed: %v\n", err)
		return 1
	}
	if jsonOut {
		return writeJSON(stdout, stderr, analyticsRecentResult{Runs: runs})
	}
	fmt.Fprintf(stdout, "Recent Runs: %d\n", len(runs))
	for i, run := range runs {
		fmt.Fprintf(stdout, "%d. %s %s\n", i+1, run.Operation, run.CompletedAt.Format(time.RFC3339))
		if strings.TrimSpace(run.SelectedURL) != "" {
			fmt.Fprintf(stdout, "   URL: %s\n", run.SelectedURL)
		}
		if strings.TrimSpace(run.Provider) != "" {
			fmt.Fprintf(stdout, "   Provider: %s\n", run.Provider)
		}
		fmt.Fprintf(stdout, "   Latency: %dms\n", run.LatencyMS)
		fmt.Fprintf(stdout, "   Chars Saved: %d\n", run.CharsSaved)
		fmt.Fprintf(stdout, "   Proof Usable: %t\n", run.ProofUsable)
		fmt.Fprintf(stdout, "   Local Memory Used: %t\n", run.LocalMemoryUsed)
		fmt.Fprintf(stdout, "   Public Bootstrap Used: %t\n", run.PublicBootstrapUsed)
	}
	return 0
}

func (r Runner) runAnalyticsValueReport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("analytics value-report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex analytics value-report [--json]")
		return 2
	}
	report, err := analytics.NewSQLiteStore(r.storeRoot).ValueReport(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "analytics value-report failed: %v\n", err)
		return 1
	}
	if jsonOut {
		return writeJSON(stdout, stderr, analyticsValueReportResult{Report: report})
	}
	fmt.Fprintf(stdout, "Chars Saved for the Agent: %d\n", report.TotalAgentCharsSaved)
	fmt.Fprintf(stdout, "Context Compression Ratio: %.2fx\n", report.ContextCompressionRatio)
	fmt.Fprintf(stdout, "Bootstrap Avoided: %d\n", report.TotalPublicBootstrapsAvoided)
	fmt.Fprintf(stdout, "Proof-Backed Answers Delivered: %d\n", report.TotalProofBackedPackets)
	fmt.Fprintf(stdout, "Web Work Avoided: %d memory reuse events\n", report.TotalMemoryReuseEvents)
	fmt.Fprintf(stdout, "Topic Roots Recovered: %d\n", report.TotalTopicRootCorrections)
	fmt.Fprintf(stdout, "Warm-State Lift: %.2f%%\n", report.WarmLikeReuseRate*100)
	fmt.Fprintf(stdout, "\nRuns: %d successful / %d total\n", report.SuccessfulRuns, report.TotalRuns)
	fmt.Fprintf(stdout, "Avg Latency: %dms\n", report.AvgLatencyMS)
	fmt.Fprintf(stdout, "Sources Visited: %d\n", report.TotalSourcesVisited)
	fmt.Fprintf(stdout, "Links Explored: %d\n", report.TotalLinksExplored)
	fmt.Fprintf(stdout, "Proof-Backed Rate: %.2f%%\n", report.ProofBackedRate*100)
	return 0
}

func (r Runner) runAnalyticsHosts(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("analytics hosts", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	var limit int
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.IntVar(&limit, "limit", 20, "number of hosts")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex analytics hosts [--limit N] [--json]")
		return 2
	}
	hosts, err := analytics.NewSQLiteStore(r.storeRoot).Hosts(context.Background(), limit)
	if err != nil {
		fmt.Fprintf(stderr, "analytics hosts failed: %v\n", err)
		return 1
	}
	if jsonOut {
		return writeJSON(stdout, stderr, analyticsHostsResult{Hosts: hosts})
	}
	fmt.Fprintf(stdout, "Hosts: %d\n", len(hosts))
	for i, host := range hosts {
		fmt.Fprintf(stdout, "%d. %s\n", i+1, host.Host)
		fmt.Fprintf(stdout, "   Runs: %d (%d successful)\n", host.RunCount, host.SuccessfulRuns)
		fmt.Fprintf(stdout, "   Avg Latency: %dms\n", host.AvgLatencyMS)
		fmt.Fprintf(stdout, "   Chars Saved: %d\n", host.TotalAgentCharsSaved)
		fmt.Fprintf(stdout, "   Proof-Backed Rate: %.2f%%\n", host.ProofBackedRate*100)
		fmt.Fprintf(stdout, "   Public Bootstrap Rate: %.2f%%\n", host.PublicBootstrapUsedRate*100)
		fmt.Fprintf(stdout, "   Local Memory Rate: %.2f%%\n", host.LocalMemoryUsedRate*100)
	}
	return 0
}

func (r Runner) runAnalyticsProviders(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("analytics providers", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	var limit int
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.IntVar(&limit, "limit", 20, "number of providers")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex analytics providers [--limit N] [--json]")
		return 2
	}
	providers, err := analytics.NewSQLiteStore(r.storeRoot).Providers(context.Background(), limit)
	if err != nil {
		fmt.Fprintf(stderr, "analytics providers failed: %v\n", err)
		return 1
	}
	if jsonOut {
		return writeJSON(stdout, stderr, analyticsProvidersResult{Providers: providers})
	}
	fmt.Fprintf(stdout, "Providers: %d\n", len(providers))
	for i, provider := range providers {
		fmt.Fprintf(stdout, "%d. %s\n", i+1, provider.Provider)
		fmt.Fprintf(stdout, "   Runs: %d (%d successful)\n", provider.RunCount, provider.SuccessfulRuns)
		fmt.Fprintf(stdout, "   Avg Latency: %dms\n", provider.AvgLatencyMS)
		fmt.Fprintf(stdout, "   Chars Saved: %d\n", provider.TotalAgentCharsSaved)
		fmt.Fprintf(stdout, "   Proof-Backed Rate: %.2f%%\n", provider.ProofBackedRate*100)
		fmt.Fprintf(stdout, "   Public Bootstrap Rate: %.2f%%\n", provider.PublicBootstrapUsedRate*100)
		fmt.Fprintf(stdout, "   Local Memory Rate: %.2f%%\n", provider.LocalMemoryUsedRate*100)
	}
	return 0
}
