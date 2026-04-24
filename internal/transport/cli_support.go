package transport

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/josepavese/needlex/internal/analytics"
	"github.com/josepavese/needlex/internal/observability"
	"github.com/josepavese/needlex/internal/platform/buildinfo"
)

type supportBundleResult struct {
	Directory        string                `json:"directory"`
	ManifestPath     string                `json:"manifest_path"`
	DoctorPath       string                `json:"doctor_path"`
	LogStatsPath     string                `json:"log_stats_path"`
	LogTailPath      string                `json:"log_tail_path"`
	RuntimeLogCopies []string              `json:"runtime_log_copies,omitempty"`
	AnalyticsExport  analytics.ExportStats `json:"analytics_export"`
}

type supportBundleManifest struct {
	Version          string                `json:"version"`
	GeneratedAtUTC   time.Time             `json:"generated_at_utc"`
	StateRoot        string                `json:"state_root"`
	RuntimeLogPath   string                `json:"runtime_log_path"`
	LogLimit         int                   `json:"log_limit"`
	DoctorPath       string                `json:"doctor_path"`
	LogStatsPath     string                `json:"log_stats_path"`
	LogTailPath      string                `json:"log_tail_path"`
	RuntimeLogCopies []string              `json:"runtime_log_copies,omitempty"`
	AnalyticsExport  analytics.ExportStats `json:"analytics_export"`
	Notes            []string              `json:"notes"`
}

type supportLogTailArtifact struct {
	Events []observability.Event `json:"events"`
}

func writeSupportUsage(w io.Writer) {
	writeUsage(w, "needlex support bundle --out DIR [--config path] [--log-limit N] [--json]")
}

func (r Runner) runSupport(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeSupportUsage(stderr)
		return 2
	}
	switch args[0] {
	case "bundle":
		return r.runSupportBundle(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		writeSupportUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown support subcommand %q\n\n", args[0])
		writeSupportUsage(stderr)
		return 2
	}
}

func (r Runner) runSupportBundle(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("support bundle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var outDir string
	var configPath string
	var logLimit int
	var jsonOut bool
	fs.StringVar(&outDir, "out", "", "output directory")
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.IntVar(&logLimit, "log-limit", 200, "runtime log events to include")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{"--out": {}, "-out": {}, "--config": {}, "-config": {}, "--log-limit": {}, "-log-limit": {}})); err != nil {
		return 2
	}
	if fs.NArg() != 0 || outDir == "" {
		writeUsage(stderr, "needlex support bundle --out DIR [--config path] [--log-limit N] [--json]")
		return 2
	}
	result, err := r.buildSupportBundle(context.Background(), outDir, configPath, logLimit)
	if err != nil {
		return r.reportCLIError(stderr, "support_bundle", err, map[string]any{"out_dir": outDir})
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "support_bundle", result)
	}
	fmt.Fprintf(stdout, "Support Bundle: %s\n", result.Directory)
	fmt.Fprintf(stdout, "Manifest: %s\n", result.ManifestPath)
	fmt.Fprintf(stdout, "Doctor: %s\n", result.DoctorPath)
	fmt.Fprintf(stdout, "Runtime Log Tail: %s\n", result.LogTailPath)
	fmt.Fprintf(stdout, "Analytics Export: %s\n", result.AnalyticsExport.Directory)
	return 0
}

func (r Runner) buildSupportBundle(ctx context.Context, outDir, configPath string, logLimit int) (supportBundleResult, error) {
	if logLimit <= 0 {
		logLimit = 200
	}
	outDir = filepath.Clean(outDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return supportBundleResult{}, fmt.Errorf("create support bundle dir: %w", err)
	}

	doctorPath := filepath.Join(outDir, "doctor.json")
	logStatsPath := filepath.Join(outDir, "runtime_log_stats.json")
	logTailPath := filepath.Join(outDir, "runtime_log_tail.json")
	manifestPath := filepath.Join(outDir, "manifest.json")

	doctor := r.buildDoctorReport(configPath)
	if err := writeJSONArtifact(doctorPath, doctor); err != nil {
		return supportBundleResult{}, err
	}

	logger := r.runtimeLogger()
	logStats, err := logger.Stats()
	if err != nil {
		return supportBundleResult{}, fmt.Errorf("runtime log stats: %w", err)
	}
	if err := writeJSONArtifact(logStatsPath, logStats); err != nil {
		return supportBundleResult{}, err
	}
	logTail, err := logger.Tail(logLimit)
	if err != nil {
		return supportBundleResult{}, fmt.Errorf("runtime log tail: %w", err)
	}
	if err := writeJSONArtifact(logTailPath, supportLogTailArtifact{Events: logTail}); err != nil {
		return supportBundleResult{}, err
	}

	copiedLogs, err := copyRuntimeLogs(outDir, logStats)
	if err != nil {
		return supportBundleResult{}, err
	}
	analyticsExport, err := analytics.NewSQLiteStore(r.storeRoot).ExportJSON(ctx, filepath.Join(outDir, "analytics"))
	if err != nil {
		return supportBundleResult{}, fmt.Errorf("analytics export: %w", err)
	}

	result := supportBundleResult{
		Directory:        outDir,
		ManifestPath:     manifestPath,
		DoctorPath:       doctorPath,
		LogStatsPath:     logStatsPath,
		LogTailPath:      logTailPath,
		RuntimeLogCopies: copiedLogs,
		AnalyticsExport:  analyticsExport,
	}
	manifest := buildSupportBundleManifest(r.storeRoot, logger.Path(), logLimit, result)
	if err := writeJSONArtifact(manifestPath, manifest); err != nil {
		return supportBundleResult{}, err
	}
	return result, nil
}

func buildSupportBundleManifest(stateRoot, runtimeLogPath string, logLimit int, result supportBundleResult) supportBundleManifest {
	return supportBundleManifest{
		Version:          buildinfo.Version,
		GeneratedAtUTC:   time.Now().UTC(),
		StateRoot:        stateRoot,
		RuntimeLogPath:   runtimeLogPath,
		LogLimit:         logLimit,
		DoctorPath:       result.DoctorPath,
		LogStatsPath:     result.LogStatsPath,
		LogTailPath:      result.LogTailPath,
		RuntimeLogCopies: result.RuntimeLogCopies,
		AnalyticsExport:  result.AnalyticsExport,
		Notes: []string{
			"Runtime logs are redacted by Needle-X before persistence, but may still contain target URLs and operational metadata.",
			"The bundle excludes traces, proofs, fingerprints, and source files by default.",
		},
	}
}

func copyRuntimeLogs(outDir string, stats observability.LogStats) ([]string, error) {
	logDir := filepath.Join(outDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create support logs dir: %w", err)
	}
	paths := append([]string{stats.Path}, stats.Rotated...)
	copied := []string{}
	for _, source := range paths {
		if source == "" {
			continue
		}
		destination := filepath.Join(logDir, filepath.Base(source))
		ok, err := copyFileIfExists(source, destination)
		if err != nil {
			return nil, err
		}
		if ok {
			copied = append(copied, destination)
		}
	}
	return copied, nil
}

func copyFileIfExists(source, destination string) (bool, error) {
	in, err := os.Open(source)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("open %s: %w", source, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return false, fmt.Errorf("create %s: %w", destination, err)
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return false, fmt.Errorf("copy %s: %w", source, err)
	}
	return true, nil
}

func writeJSONArtifact(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()
	if err := writeIndentedJSON(file, value); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
