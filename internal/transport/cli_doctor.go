package transport

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/josepavese/needlex/internal/analytics"
	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/observability"
	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/platform/buildinfo"
)

type doctorReport struct {
	Version         string                 `json:"version"`
	GOOS            string                 `json:"goos"`
	GOARCH          string                 `json:"goarch"`
	ExecutablePath  string                 `json:"executable_path,omitempty"`
	PathCommand     string                 `json:"path_command,omitempty"`
	NeedlexHomeEnv  string                 `json:"needlex_home_env,omitempty"`
	StateRoot       string                 `json:"state_root"`
	AnalyticsDBPath string                 `json:"analytics_db_path"`
	DiscoveryDBPath string                 `json:"discovery_db_path"`
	LogsDir         string                 `json:"logs_dir"`
	RuntimeLogPath  string                 `json:"runtime_log_path"`
	StorePaths      map[string]string      `json:"store_paths"`
	AnalyticsStats  analytics.Stats        `json:"analytics_stats,omitempty"`
	MemoryStats     memory.Stats           `json:"memory_stats,omitempty"`
	LogStats        observability.LogStats `json:"log_stats,omitempty"`
	Diagnostics     doctorDiagnostics      `json:"diagnostics,omitempty"`
	MCPProcesses    []doctorMCPProcess     `json:"mcp_processes,omitempty"`
	Warnings        []string               `json:"warnings,omitempty"`
}

type doctorDiagnostics struct {
	RecentLogEvents     int                        `json:"recent_log_events"`
	RecentErrors        int                        `json:"recent_errors"`
	RecentWarnings      int                        `json:"recent_warnings"`
	LastDiagnosticID    string                     `json:"last_diagnostic_id,omitempty"`
	LastFailureClass    string                     `json:"last_failure_class,omitempty"`
	LastRuntimeEvent    string                     `json:"last_runtime_event,omitempty"`
	AnalyticsFailures   []analytics.FailureRollup  `json:"analytics_failures,omitempty"`
	AnalyticsProviders  []analytics.ProviderRollup `json:"analytics_providers,omitempty"`
	RuntimeEventCounts  map[string]int             `json:"runtime_event_counts,omitempty"`
	RuntimeLevelCounts  map[string]int             `json:"runtime_level_counts,omitempty"`
	FailureClassCounts  map[string]int             `json:"failure_class_counts,omitempty"`
	LastRuntimeLogEvent observability.Event        `json:"last_runtime_log_event,omitempty"`
}

type doctorMCPProcess struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
}

func (r Runner) runDoctor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	var configPath string
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex doctor [--json] [--config path]")
		return 2
	}
	report := r.buildDoctorReport(configPath)
	if jsonOut {
		return r.writeJSON(stdout, stderr, "doctor", report)
	}
	renderDoctorText(stdout, report)
	return 0
}

func (r Runner) buildDoctorReport(configPath string) doctorReport {
	executable, _ := os.Executable()
	pathCommand, _ := exec.LookPath("needlex")
	stateRoot := strings.TrimSpace(r.storeRoot)
	if stateRoot == "" {
		stateRoot = platform.DefaultStateRoot()
	}
	cfg, cfgErr := r.loadConfig(configPath)
	if cfgErr != nil {
		cfg = config.Defaults()
	}
	analyticsStore := analytics.NewSQLiteStore(stateRoot)
	memoryStore := memory.NewSQLiteStore(stateRoot, cfg.Memory.Path)
	layout := platform.NewStateLayout(stateRoot)
	stats, statsErr := analyticsStore.Stats(context.Background())
	memoryStats, memoryErr := memoryStore.GetStats(context.Background())
	logger := observability.NewLogger(stateRoot)
	logStats, logErr := logger.Stats()
	processes := detectMCPProcesses()
	diagnostics, diagnosticsWarnings := buildDoctorDiagnostics(context.Background(), analyticsStore, logger)
	report := doctorReport{
		Version:         buildinfo.Version,
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
		ExecutablePath:  executable,
		PathCommand:     pathCommand,
		NeedlexHomeEnv:  os.Getenv(platform.EnvHome),
		StateRoot:       stateRoot,
		AnalyticsDBPath: analyticsStore.DBPath(),
		DiscoveryDBPath: memoryStore.DBPath(),
		LogsDir:         layout.LogsDir,
		RuntimeLogPath:  layout.RuntimeLog,
		StorePaths:      layout.Paths(),
		Diagnostics:     diagnostics,
		MCPProcesses:    processes,
	}
	report.Warnings = append(report.Warnings, diagnosticsWarnings...)
	if statsErr == nil {
		report.AnalyticsStats = stats
	} else {
		report.Warnings = append(report.Warnings, "analytics stats unavailable: "+statsErr.Error())
	}
	if memoryErr == nil {
		report.MemoryStats = memoryStats
	} else {
		report.Warnings = append(report.Warnings, "memory stats unavailable: "+memoryErr.Error())
	}
	if logErr == nil {
		report.LogStats = logStats
	} else {
		report.Warnings = append(report.Warnings, "runtime log stats unavailable: "+logErr.Error())
	}
	if cfgErr != nil {
		report.Warnings = append(report.Warnings, "config load failed: "+cfgErr.Error())
	}
	if pathCommand != "" && executable != "" && !sameCleanPath(pathCommand, executable) {
		report.Warnings = append(report.Warnings, "PATH needlex differs from current executable")
	}
	if len(processes) > 0 {
		report.Warnings = append(report.Warnings, "active MCP processes detected; restart clients after local upgrades")
	}
	return report
}

func renderDoctorText(w io.Writer, report doctorReport) {
	fmt.Fprintf(w, "Needle-X Doctor\n")
	fmt.Fprintf(w, "Version: %s\n", report.Version)
	fmt.Fprintf(w, "Platform: %s/%s\n", report.GOOS, report.GOARCH)
	fmt.Fprintf(w, "Executable: %s\n", report.ExecutablePath)
	fmt.Fprintf(w, "PATH command: %s\n", report.PathCommand)
	fmt.Fprintf(w, "NEEDLEX_HOME: %s\n", doctorFirstNonEmpty(report.NeedlexHomeEnv, "<unset>"))
	fmt.Fprintf(w, "State Root: %s\n", report.StateRoot)
	fmt.Fprintf(w, "Analytics DB: %s\n", report.AnalyticsDBPath)
	fmt.Fprintf(w, "Discovery DB: %s\n", report.DiscoveryDBPath)
	fmt.Fprintf(w, "Runtime Log: %s\n", report.RuntimeLogPath)
	fmt.Fprintf(w, "Analytics Runs: %d (%d successful)\n", report.AnalyticsStats.RunCount, report.AnalyticsStats.SuccessfulRuns)
	fmt.Fprintf(w, "Memory Docs: %d\n", report.MemoryStats.DocumentCount)
	fmt.Fprintf(w, "Log Events: %d\n", report.LogStats.LineCount)
	fmt.Fprintf(w, "Recent Runtime Errors: %d\n", report.Diagnostics.RecentErrors)
	fmt.Fprintf(w, "Recent Runtime Warnings: %d\n", report.Diagnostics.RecentWarnings)
	if report.Diagnostics.LastDiagnosticID != "" {
		fmt.Fprintf(w, "Last Diagnostic ID: %s\n", report.Diagnostics.LastDiagnosticID)
	}
	if len(report.Diagnostics.AnalyticsFailures) > 0 {
		fmt.Fprintln(w, "Analytics Failures:")
		for _, item := range report.Diagnostics.AnalyticsFailures {
			fmt.Fprintf(w, "  - %s: %d\n", item.FailureClass, item.RunCount)
		}
	}
	fmt.Fprintf(w, "MCP Processes: %d\n", len(report.MCPProcesses))
	for _, proc := range report.MCPProcesses {
		fmt.Fprintf(w, "  %d %s\n", proc.PID, proc.Command)
	}
	if len(report.Warnings) > 0 {
		fmt.Fprintln(w, "Warnings:")
		for _, warning := range report.Warnings {
			fmt.Fprintf(w, "  - %s\n", warning)
		}
	}
}

func buildDoctorDiagnostics(ctx context.Context, analyticsStore analytics.SQLiteStore, logger observability.Logger) (doctorDiagnostics, []string) {
	warnings := []string{}
	events, err := logger.Tail(100)
	if err != nil {
		warnings = append(warnings, "runtime log tail unavailable: "+err.Error())
	}
	failures, err := analyticsStore.Failures(ctx, 5)
	if err != nil {
		warnings = append(warnings, "analytics failures unavailable: "+err.Error())
	}
	providers, err := analyticsStore.Providers(ctx, 5)
	if err != nil {
		warnings = append(warnings, "analytics providers unavailable: "+err.Error())
	}
	out := doctorDiagnostics{
		RecentLogEvents:    len(events),
		AnalyticsFailures:  failures,
		AnalyticsProviders: providers,
		RuntimeEventCounts: map[string]int{},
		RuntimeLevelCounts: map[string]int{},
		FailureClassCounts: map[string]int{},
	}
	for _, event := range events {
		out.RuntimeEventCounts[event.Event]++
		out.RuntimeLevelCounts[event.Level]++
		if event.FailureClass != "" {
			out.FailureClassCounts[event.FailureClass]++
		}
		switch event.Level {
		case observability.LevelError:
			out.RecentErrors++
		case observability.LevelWarn:
			out.RecentWarnings++
		}
		if event.ID != "" {
			out.LastRuntimeLogEvent = event
			out.LastDiagnosticID = event.ID
			out.LastFailureClass = event.FailureClass
			out.LastRuntimeEvent = event.Event
		}
	}
	return out, warnings
}

func detectMCPProcesses() []doctorMCPProcess {
	if runtime.GOOS != "linux" {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	out := []doctorMCPProcess{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || len(data) == 0 {
			continue
		}
		cmd := strings.TrimSpace(strings.ReplaceAll(string(data), "\x00", " "))
		lower := strings.ToLower(cmd)
		if strings.Contains(lower, "needlex") && strings.Contains(lower, " mcp") {
			out = append(out, doctorMCPProcess{PID: pid, Command: cmd})
		}
	}
	return out
}

func sameCleanPath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil {
		left = leftAbs
	}
	if rightErr == nil {
		right = rightAbs
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func doctorFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
