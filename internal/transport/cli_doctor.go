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
	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/platform/buildinfo"
)

type doctorReport struct {
	Version         string             `json:"version"`
	GOOS            string             `json:"goos"`
	GOARCH          string             `json:"goarch"`
	ExecutablePath  string             `json:"executable_path,omitempty"`
	PathCommand     string             `json:"path_command,omitempty"`
	NeedlexHomeEnv  string             `json:"needlex_home_env,omitempty"`
	StateRoot       string             `json:"state_root"`
	AnalyticsDBPath string             `json:"analytics_db_path"`
	DiscoveryDBPath string             `json:"discovery_db_path"`
	StorePaths      map[string]string  `json:"store_paths"`
	AnalyticsStats  analytics.Stats    `json:"analytics_stats,omitempty"`
	MemoryStats     memory.Stats       `json:"memory_stats,omitempty"`
	MCPProcesses    []doctorMCPProcess `json:"mcp_processes,omitempty"`
	Warnings        []string           `json:"warnings,omitempty"`
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
		return writeJSON(stdout, stderr, report)
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
	processes := detectMCPProcesses()
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
		StorePaths:      layout.Paths(),
		MCPProcesses:    processes,
	}
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
	fmt.Fprintf(w, "Analytics Runs: %d (%d successful)\n", report.AnalyticsStats.RunCount, report.AnalyticsStats.SuccessfulRuns)
	fmt.Fprintf(w, "Memory Docs: %d\n", report.MemoryStats.DocumentCount)
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
