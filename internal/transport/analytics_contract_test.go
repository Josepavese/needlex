package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/josepavese/needlex/internal/analytics"
	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
)

type analyticsCall struct {
	action string
	limit  int
	out    string
}

type analyticsProbe struct {
	calls   []analyticsCall
	failure error
	run     analytics.RunRecord
}

func (p *analyticsProbe) record(action string, limit int, out string) error {
	p.calls = append(p.calls, analyticsCall{action, limit, out})
	return p.failure
}

func (p *analyticsProbe) DBPath() string { return "in-memory-analytics" }

func (p *analyticsProbe) Stats(context.Context) (analytics.Stats, error) {
	return analytics.Stats{RunCount: 7}, p.record("stats", 0, "")
}

func (p *analyticsProbe) RecentRuns(_ context.Context, limit int) ([]analytics.RecentRun, error) {
	return []analytics.RecentRun{{RunID: "probe-run"}}, p.record("recent_runs", limit, "")
}

func (p *analyticsProbe) ValueReport(context.Context) (analytics.ValueReport, error) {
	return analytics.ValueReport{TotalRuns: 7}, p.record("value_report", 0, "")
}

func (p *analyticsProbe) Hosts(_ context.Context, limit int) ([]analytics.HostRollup, error) {
	return []analytics.HostRollup{{Host: "probe-host"}}, p.record("hosts", limit, "")
}

func (p *analyticsProbe) Providers(_ context.Context, limit int) ([]analytics.ProviderRollup, error) {
	return []analytics.ProviderRollup{{Provider: "probe-provider"}}, p.record("providers", limit, "")
}

func (p *analyticsProbe) Failures(_ context.Context, limit int) ([]analytics.FailureRollup, error) {
	return []analytics.FailureRollup{{FailureClass: "probe-failure"}}, p.record("failures", limit, "")
}

func (p *analyticsProbe) Daily(_ context.Context, limit int) ([]analytics.DailyRollup, error) {
	return []analytics.DailyRollup{{Day: "2026-01-01"}}, p.record("daily", limit, "")
}

func (p *analyticsProbe) ExportJSON(_ context.Context, out string) (analytics.ExportStats, error) {
	return analytics.ExportStats{Directory: out}, p.record("export", 0, out)
}

func (p *analyticsProbe) AppendRun(_ context.Context, run analytics.RunRecord, _ []analytics.StageEvent) error {
	p.run = run
	return p.record("append", 0, "")
}

func TestAnalyticsCLIAndMCPUseTheSameProviderContract(t *testing.T) {
	for _, tc := range []struct {
		cli, mcp, key string
		limit         int
	}{
		{"stats", "stats", "stats", 0},
		{"recent", "recent_runs", "runs", 10},
		{"value-report", "value_report", "report", 0},
		{"hosts", "hosts", "hosts", 20},
		{"providers", "providers", "providers", 20},
		{"failures", "failures", "failures", 20},
		{"daily", "daily", "days", 30},
		{"export", "export", "export", 0},
	} {
		t.Run(tc.cli, func(t *testing.T) {
			probe := &analyticsProbe{}
			runner := NewRunner()
			runner.storeRoot = t.TempDir()
			runner.newAnalyticsStore = func(root string) analytics.Store {
				if root != runner.storeRoot {
					t.Fatalf("wrong state owner: %s", root)
				}
				return probe
			}
			args := []string{"analytics", tc.cli, "--json"}
			mcpArgs := map[string]any{"action": tc.mcp}
			out := ""
			if tc.cli == "export" {
				out = filepath.Join(runner.storeRoot, "export")
				args = append(args, "--out", out)
				mcpArgs["out_dir"] = out
			}
			var stdout, stderr bytes.Buffer
			if code := runner.Run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("CLI: exit=%d %s", code, stderr.String())
			}
			var cli map[string]json.RawMessage
			if err := json.Unmarshal(stdout.Bytes(), &cli); err != nil || len(cli) != 1 {
				t.Fatalf("CLI shape changed: %s, %v", stdout.String(), err)
			}
			result, err := runner.callMCPAnalyticsTool(mcpArgs)
			if err != nil {
				t.Fatal(err)
			}
			compact := result["structuredContent"].(map[string]any)["compact"].(map[string]any)
			if compact["kind"] != "analytics_"+tc.mcp {
				t.Fatalf("MCP kind changed: %v", compact)
			}
			var cliValue any
			if err := json.Unmarshal(cli[tc.key], &cliValue); err != nil {
				t.Fatal(err)
			}
			mcpBytes, err := json.Marshal(compact[tc.key])
			if err != nil {
				t.Fatal(err)
			}
			var mcpValue any
			if err := json.Unmarshal(mcpBytes, &mcpValue); err != nil || !reflect.DeepEqual(cliValue, mcpValue) {
				t.Fatalf("CLI/MCP data diverged: %s, %s, %v", cli[tc.key], mcpBytes, err)
			}
			wantCall := analyticsCall{tc.mcp, tc.limit, out}
			if !reflect.DeepEqual(probe.calls, []analyticsCall{wantCall, wantCall}) {
				t.Fatalf("provider/defaults bypassed: %v", probe.calls)
			}
		})
	}
}

func TestAnalyticsProviderFailureDoesNotFallBackToSQLite(t *testing.T) {
	failure := errors.New("analytics unavailable")
	probe := &analyticsProbe{failure: failure}
	runner := NewRunner()
	runner.storeRoot = t.TempDir()
	runner.newAnalyticsStore = func(string) analytics.Store { return probe }
	if _, err := runner.callMCPAnalyticsTool(map[string]any{"action": "stats"}); !errors.Is(err, failure) {
		t.Fatalf("MCP lost provider failure: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if code := runner.Run([]string{"analytics", "stats", "--json"}, &stdout, &stderr); code != 1 || stdout.Len() != 0 {
		t.Fatalf("CLI masked provider failure: %d, %s", code, stdout.String())
	}
	if len(probe.calls) != 2 {
		t.Fatalf("provider call count: %v", probe.calls)
	}
}

func TestRuntimeObservationUsesInjectedRecorder(t *testing.T) {
	probe := &analyticsProbe{}
	runner := NewRunner()
	runner.storeRoot = t.TempDir()
	runner.newAnalyticsStore = func(string) analytics.Store { return probe }
	cfg := config.Defaults()
	cfg.Memory.Enabled = false
	runner.observeAnalyticsRead(cfg, "mcp", coreservice.ReadRequest{URL: "https://example.com"}, fakeResponse(), analytics.EmbeddingCacheCounters{})
	if len(probe.calls) != 1 || probe.calls[0].action != "append" || probe.run.Surface != "mcp" || probe.run.Operation != "read" {
		t.Fatalf("recorder bypassed: %v, %+v", probe.calls, probe.run)
	}
}
