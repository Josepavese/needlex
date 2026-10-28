package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/josepavese/needlex/internal/analytics"
	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/memory"
	"github.com/josepavese/needlex/internal/store"
)

type memoryStoreProbe struct {
	memory.LocalStore
	failure error
	reads   int
}

func (p *memoryStoreProbe) DBPath() string { return "memory-probe" }

func (p *memoryStoreProbe) GetStats(context.Context) (memory.Stats, error) {
	p.reads++
	return memory.Stats{DocumentCount: 13, DBPath: p.DBPath()}, p.failure
}

type memoryRuntimeProbe struct {
	memory.Runtime
	searches, observations, refreshes int
	force                             bool
	observed                          memory.Observation
}

func (p *memoryRuntimeProbe) Search(context.Context, string, memory.SearchOptions) ([]memory.Candidate, error) {
	p.searches++
	return nil, nil
}

func (p *memoryRuntimeProbe) Observe(_ context.Context, observation memory.Observation) error {
	p.observations++
	p.observed = observation
	return nil
}

func (p *memoryRuntimeProbe) RefreshEmbeddings(_ context.Context, force bool) (memory.EmbeddingRefreshStats, error) {
	p.refreshes++
	p.force = force
	return memory.EmbeddingRefreshStats{DocumentCount: 13}, nil
}

func TestMemoryStatsUseInjectedStorageAcrossCLIAndMCP(t *testing.T) {
	runner := NewRunner()
	runner.storeRoot = t.TempDir()
	cfgPath := filepath.Join(runner.storeRoot, "config.json")
	if err := config.Write(cfgPath, config.Defaults()); err != nil {
		t.Fatal(err)
	}
	backend := &memoryStoreProbe{}
	runner.newMemoryStore = func(root, path string) memory.LocalStore {
		if root != runner.storeRoot || path != config.Defaults().Memory.Path {
			t.Fatalf("storage owner/configuration changed: %q %q", root, path)
		}
		return backend
	}
	var stdout, stderr bytes.Buffer
	if code := runner.Run([]string{"memory", "stats", "--config", cfgPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("CLI stats: %d %s", code, stderr.String())
	}
	var output memoryStatsResult
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil || output.Stats.DocumentCount != 13 || output.Stats.DBPath != "memory-probe" {
		t.Fatalf("CLI bypassed storage contract: %s %v", stdout.String(), err)
	}
	result, err := runner.callMCPMemoryStatsTool(map[string]any{"config_path": cfgPath})
	if err != nil {
		t.Fatal(err)
	}
	stats := result["structuredContent"].(map[string]any)["stats"].(compactMemoryStats)
	if stats.DocumentCount != 13 || stats.DBPath != "memory-probe" || backend.reads != 2 {
		t.Fatalf("MCP bypassed storage contract: %+v reads=%d", stats, backend.reads)
	}
	failure := errors.New("memory unavailable")
	backend.failure = failure
	if _, err := runner.callMCPMemoryStatsTool(map[string]any{"config_path": cfgPath}); !errors.Is(err, failure) {
		t.Fatalf("provider error hidden by fallback: %v", err)
	}
}

func TestQueryStateRecallAndObservationUseInjectedRuntime(t *testing.T) {
	runner := NewRunner()
	runner.storeRoot = t.TempDir()
	state := store.NewLocalState(runner.storeRoot)
	stateUsed := false
	runner.newLocalState = func(root string) store.LocalState {
		if root != runner.storeRoot {
			t.Fatalf("wrong local-state owner: %s", root)
		}
		stateUsed = true
		return state
	}
	runtime := &memoryRuntimeProbe{}
	runner.newMemoryRuntime = func(config.Config) memory.Runtime { return runtime }
	runner.newMemoryStore = func(string, string) memory.LocalStore { return &memoryStoreProbe{} }
	runner.newAnalyticsStore = func(string) analytics.Store { return &analyticsProbe{} }
	runner.newSemanticAligner = func(config.Config) intel.SemanticAligner { return stubSemanticAligner{} }
	runner.query = func(_ context.Context, _ config.Config, req coreservice.QueryRequest) (coreservice.QueryResponse, error) {
		if req.FingerprintEvidenceLoader == nil {
			t.Fatal("fingerprint provider not supplied")
		}
		return fakeQueryResponse(req), nil
	}
	cfg := config.Defaults()
	if _, _, err := runner.executeQueryWithSurface(cfg, coreservice.QueryRequest{
		Goal: "retention policy", SeedURL: "https://example.com", DiscoveryMode: coreservice.QueryDiscoverySameSite,
	}, "mcp"); err != nil {
		t.Fatal(err)
	}
	if !stateUsed || runtime.searches != 1 || runtime.observations != 1 || runtime.observed.SourceKind != "query" || runtime.observed.TraceID == "" {
		t.Fatalf("state/runtime contract bypassed: state=%t runtime=%+v", stateUsed, runtime)
	}
	if _, err := runner.refreshMemoryEmbeddings(cfg, true); err != nil || runtime.refreshes != 1 || !runtime.force {
		t.Fatalf("maintenance bypassed runtime: %+v %v", runtime, err)
	}
}
