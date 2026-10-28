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

type analyticsFailuresResult struct {
	Failures []analytics.FailureRollup `json:"failures"`
}

type analyticsDailyResult struct {
	Days []analytics.DailyRollup `json:"days"`
}

type analyticsExportResult struct {
	Export analytics.ExportStats `json:"export"`
}

func writeAnalyticsUsage(w io.Writer) {
	writeUsage(w, "needlex analytics <stats|recent|value-report|hosts|providers|failures|daily|export> [args]")
}

const (
	analyticsRecentLimit = 10
	analyticsRollupLimit = 20
	analyticsDailyLimit  = 30
)

type analyticsCLIOptions struct {
	jsonOut bool
	limit   int
	outDir  string
}

func (r Runner) runAnalytics(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeAnalyticsUsage(stderr)
		return 2
	}
	name := args[0]
	switch name {
	case "-h", "--help", "help":
		writeAnalyticsUsage(stdout)
		return 0
	}
	commands := map[string]struct {
		run       func(analyticsCLIOptions, io.Writer, io.Writer) int
		limit     int
		limitHelp string
	}{
		"stats":        {run: r.runAnalyticsStats},
		"recent":       {r.runAnalyticsRecent, analyticsRecentLimit, "number of recent runs"},
		"value-report": {run: r.runAnalyticsValueReport},
		"hosts":        {r.runAnalyticsHosts, analyticsRollupLimit, "number of hosts"},
		"providers":    {r.runAnalyticsProviders, analyticsRollupLimit, "number of providers"},
		"failures":     {r.runAnalyticsFailures, analyticsRollupLimit, "number of failure classes"},
		"daily":        {r.runAnalyticsDaily, analyticsDailyLimit, "number of days"},
		"export":       {run: r.runAnalyticsExport},
	}
	command, ok := commands[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown analytics subcommand %q\n\n", name)
		writeAnalyticsUsage(stderr)
		return 2
	}
	fs := flag.NewFlagSet("analytics "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	opts := analyticsCLIOptions{}
	fs.BoolVar(&opts.jsonOut, "json", false, "emit JSON output")
	usage := "needlex analytics " + name + " [--json]"
	if command.limitHelp != "" {
		fs.IntVar(&opts.limit, "limit", command.limit, command.limitHelp)
		usage = "needlex analytics " + name + " [--limit N] [--json]"
	}
	if name == "export" {
		fs.StringVar(&opts.outDir, "out", "", "export directory")
		usage = "needlex analytics export --out DIR [--json]"
	}
	if err := fs.Parse(normalizeArgs(fs, args[1:])); err != nil {
		return 2
	}
	if fs.NArg() != 0 || (name == "export" && strings.TrimSpace(opts.outDir) == "") {
		writeUsage(stderr, usage)
		return 2
	}
	return command.run(opts, stdout, stderr)
}

func (r Runner) runAnalyticsStats(opts analyticsCLIOptions, stdout, stderr io.Writer) int {
	stats, err := r.analyticsStore(r.storeRoot).Stats(context.Background())
	if err != nil {
		return r.reportCLIError(stderr, "analytics_stats", err, nil)
	}
	if opts.jsonOut {
		return r.writeJSON(stdout, stderr, "analytics_stats", analyticsStatsResult{Stats: stats})
	}
	fmt.Fprintf(stdout, "Runs: %d\n", stats.RunCount)
	fmt.Fprintf(stdout, "Successful Runs: %d\n", stats.SuccessfulRuns)
	fmt.Fprintf(stdout, "Queries: %d\n", stats.QueryRuns)
	fmt.Fprintf(stdout, "Reads: %d\n", stats.ReadRuns)
	fmt.Fprintf(stdout, "Crawls: %d\n", stats.CrawlRuns)
	fmt.Fprintf(stdout, "Stage Events: %d\n", stats.StageEventCount)
	fmt.Fprintf(stdout, "Embedding Cache Hits: %d\n", stats.EmbeddingCache.Hits)
	fmt.Fprintf(stdout, "Embedding Cache Misses: %d\n", stats.EmbeddingCache.Misses)
	fmt.Fprintf(stdout, "Embedding Cache Hit Rate: %.2f%%\n", stats.EmbeddingCache.HitRate*100)
	fmt.Fprintf(stdout, "Chars Saved for the Agent: %d\n", stats.TotalAgentCharsSaved)
	fmt.Fprintf(stdout, "Estimated Tokens Saved: %d\n", stats.TotalAgentTokensSaved)
	fmt.Fprintf(stdout, "Estimated Cost Saved: $%.2f @ $5/M input tokens\n", stats.EstimatedCostSavedUSD.At5USDPerMillionTokens)
	fmt.Fprintf(stdout, "Token Estimate Basis: %.1f chars/token (%s)\n", stats.CharsPerTokenEstimate, stats.TokenEstimateMethod)
	fmt.Fprintf(stdout, "DB Path: %s\n", stats.DBPath)
	fmt.Fprintf(stdout, "DB Size: %d bytes\n", stats.DBSizeBytes)
	if !stats.LastRunAt.IsZero() {
		fmt.Fprintf(stdout, "Last Run At: %s\n", stats.LastRunAt.Format(time.RFC3339))
	}
	return 0
}

func (r Runner) runAnalyticsRecent(opts analyticsCLIOptions, stdout, stderr io.Writer) int {
	runs, err := r.analyticsStore(r.storeRoot).RecentRuns(context.Background(), opts.limit)
	if err != nil {
		return r.reportCLIError(stderr, "analytics_recent", err, map[string]any{"limit": opts.limit})
	}
	if opts.jsonOut {
		return r.writeJSON(stdout, stderr, "analytics_recent", analyticsRecentResult{Runs: runs})
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
		fmt.Fprintf(stdout, "   Estimated Tokens Saved: %d\n", run.TokensSavedEstimated)
		fmt.Fprintf(stdout, "   Proof Usable: %t\n", run.ProofUsable)
		fmt.Fprintf(stdout, "   Local Memory Used: %t\n", run.LocalMemoryUsed)
		fmt.Fprintf(stdout, "   Public Bootstrap Used: %t\n", run.PublicBootstrapUsed)
	}
	return 0
}

func (r Runner) runAnalyticsValueReport(opts analyticsCLIOptions, stdout, stderr io.Writer) int {
	report, err := r.analyticsStore(r.storeRoot).ValueReport(context.Background())
	if err != nil {
		return r.reportCLIError(stderr, "analytics_value_report", err, nil)
	}
	if opts.jsonOut {
		return r.writeJSON(stdout, stderr, "analytics_value_report", analyticsValueReportResult{Report: report})
	}
	fmt.Fprintln(stdout, "Value")
	fmt.Fprintf(stdout, "  Chars Saved for the Agent: %d\n", report.TotalAgentCharsSaved)
	fmt.Fprintf(stdout, "  Estimated Tokens Saved: %d\n", report.TotalAgentTokensSaved)
	fmt.Fprintf(stdout, "  Estimated Cost Saved: $%.2f @ $5/M input tokens\n", report.EstimatedCostSavedUSD.At5USDPerMillionTokens)
	fmt.Fprintf(stdout, "  Token Estimate Basis: %.1f chars/token (%s)\n", report.CharsPerTokenEstimate, report.TokenEstimateMethod)
	fmt.Fprintf(stdout, "  Context Reduction: %.2f%%\n", report.ContextCompressionRatio*100)
	fmt.Fprintf(stdout, "  Context Compression Factor: %.2fx\n", report.ContextCompressionFactor)
	fmt.Fprintf(stdout, "  Bootstrap Avoided: %d\n", report.TotalPublicBootstrapsAvoided)
	fmt.Fprintf(stdout, "  Proof-Backed Answers Delivered: %d\n", report.TotalProofBackedPackets)
	fmt.Fprintf(stdout, "  Topic Roots Recovered: %d\n", report.TotalTopicRootCorrections)
	fmt.Fprintf(stdout, "  Warm-State Lift: %.2f%%\n", report.WarmLikeReuseRate*100)
	fmt.Fprintln(stdout, "\nTrust")
	fmt.Fprintf(stdout, "  Proof-Backed Rate: %.2f%%\n", report.ProofBackedRate*100)
	fmt.Fprintf(stdout, "  Successful Runs: %d / %d\n", report.SuccessfulRuns, report.TotalRuns)
	fmt.Fprintln(stdout, "\nEfficiency")
	fmt.Fprintf(stdout, "  Avg Latency: %dms\n", report.AvgLatencyMS)
	fmt.Fprintf(stdout, "  Sources Visited: %d\n", report.TotalSourcesVisited)
	fmt.Fprintf(stdout, "  Links Explored: %d\n", report.TotalLinksExplored)
	fmt.Fprintf(stdout, "  Web Work Avoided: %d memory reuse events\n", report.TotalMemoryReuseEvents)
	return 0
}

func (r Runner) runAnalyticsHosts(opts analyticsCLIOptions, stdout, stderr io.Writer) int {
	hosts, err := r.analyticsStore(r.storeRoot).Hosts(context.Background(), opts.limit)
	if err != nil {
		return r.reportCLIError(stderr, "analytics_hosts", err, map[string]any{"limit": opts.limit})
	}
	if opts.jsonOut {
		return r.writeJSON(stdout, stderr, "analytics_hosts", analyticsHostsResult{Hosts: hosts})
	}
	fmt.Fprintf(stdout, "Hosts: %d\n", len(hosts))
	for i, host := range hosts {
		fmt.Fprintf(stdout, "%d. %s\n", i+1, host.Host)
		renderAnalyticsRollup(stdout, host.RollupMetrics)
	}
	return 0
}

func (r Runner) runAnalyticsProviders(opts analyticsCLIOptions, stdout, stderr io.Writer) int {
	providers, err := r.analyticsStore(r.storeRoot).Providers(context.Background(), opts.limit)
	if err != nil {
		return r.reportCLIError(stderr, "analytics_providers", err, map[string]any{"limit": opts.limit})
	}
	if opts.jsonOut {
		return r.writeJSON(stdout, stderr, "analytics_providers", analyticsProvidersResult{Providers: providers})
	}
	fmt.Fprintf(stdout, "Providers: %d\n", len(providers))
	for i, provider := range providers {
		fmt.Fprintf(stdout, "%d. %s\n", i+1, provider.Provider)
		renderAnalyticsRollup(stdout, provider.RollupMetrics)
	}
	return 0
}

func (r Runner) runAnalyticsFailures(opts analyticsCLIOptions, stdout, stderr io.Writer) int {
	failures, err := r.analyticsStore(r.storeRoot).Failures(context.Background(), opts.limit)
	if err != nil {
		return r.reportCLIError(stderr, "analytics_failures", err, map[string]any{"limit": opts.limit})
	}
	if opts.jsonOut {
		return r.writeJSON(stdout, stderr, "analytics_failures", analyticsFailuresResult{Failures: failures})
	}
	fmt.Fprintf(stdout, "Failures: %d\n", len(failures))
	for i, failure := range failures {
		fmt.Fprintf(stdout, "%d. %s\n", i+1, failure.FailureClass)
		fmt.Fprintf(stdout, "   Runs: %d\n", failure.RunCount)
		fmt.Fprintf(stdout, "   Avg Latency: %dms\n", failure.AvgLatencyMS)
		if strings.TrimSpace(failure.LastSeenAt) != "" {
			fmt.Fprintf(stdout, "   Last Seen: %s\n", failure.LastSeenAt)
		}
	}
	return 0
}

func (r Runner) runAnalyticsDaily(opts analyticsCLIOptions, stdout, stderr io.Writer) int {
	days, err := r.analyticsStore(r.storeRoot).Daily(context.Background(), opts.limit)
	if err != nil {
		return r.reportCLIError(stderr, "analytics_daily", err, map[string]any{"limit": opts.limit})
	}
	if opts.jsonOut {
		return r.writeJSON(stdout, stderr, "analytics_daily", analyticsDailyResult{Days: days})
	}
	fmt.Fprintf(stdout, "Days: %d\n", len(days))
	for i, day := range days {
		fmt.Fprintf(stdout, "%d. %s\n", i+1, day.Day)
		renderAnalyticsRollup(stdout, day.RollupMetrics)
	}
	return 0
}

func (r Runner) runAnalyticsExport(opts analyticsCLIOptions, stdout, stderr io.Writer) int {
	exported, err := r.analyticsStore(r.storeRoot).ExportJSON(context.Background(), opts.outDir)
	if err != nil {
		return r.reportCLIError(stderr, "analytics_export", err, map[string]any{"out_dir": opts.outDir})
	}
	if opts.jsonOut {
		return r.writeJSON(stdout, stderr, "analytics_export", analyticsExportResult{Export: exported})
	}
	fmt.Fprintf(stdout, "Directory: %s\n", exported.Directory)
	fmt.Fprintf(stdout, "Runs: %s (%d)\n", exported.RunsPath, exported.RunCount)
	fmt.Fprintf(stdout, "Stages: %s (%d)\n", exported.StagesPath, exported.StageCount)
	fmt.Fprintf(stdout, "Hosts: %s (%d)\n", exported.HostsPath, exported.HostCount)
	fmt.Fprintf(stdout, "Providers: %s (%d)\n", exported.ProvidersPath, exported.ProviderCount)
	fmt.Fprintf(stdout, "Daily: %s (%d)\n", exported.DailyPath, exported.DailyCount)
	fmt.Fprintf(stdout, "Value Report: %s\n", exported.ValueReportPath)
	return 0
}

func renderAnalyticsRollup(w io.Writer, metrics analytics.RollupMetrics) {
	fmt.Fprintf(w, "   Runs: %d (%d successful)\n", metrics.RunCount, metrics.SuccessfulRuns)
	fmt.Fprintf(w, "   Avg Latency: %dms\n", metrics.AvgLatencyMS)
	fmt.Fprintf(w, "   Chars Saved: %d\n", metrics.TotalAgentCharsSaved)
	fmt.Fprintf(w, "   Estimated Tokens Saved: %d\n", metrics.TotalAgentTokensSaved)
	fmt.Fprintf(w, "   Proof-Backed Rate: %.2f%%\n", metrics.ProofBackedRate*100)
	fmt.Fprintf(w, "   Public Bootstrap Rate: %.2f%%\n", metrics.PublicBootstrapUsedRate*100)
	fmt.Fprintf(w, "   Local Memory Rate: %.2f%%\n", metrics.LocalMemoryUsedRate*100)
}
