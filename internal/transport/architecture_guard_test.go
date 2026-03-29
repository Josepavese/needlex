package transport

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestRuntimeOpsStaysAdapterOnlyForStateOrchestration(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller not available")
	}
	baseDir := filepath.Dir(currentFile)
	runtimeOpsPath := filepath.Join(baseDir, "runtime_ops.go")
	data, err := os.ReadFile(runtimeOpsPath)
	if err != nil {
		t.Fatalf("read runtime_ops.go: %v", err)
	}
	text := string(data)

	forbidden := []string{
		"NewCandidateStore(",
		"NewDomainGraphStore(",
		"NewGenomeStore(",
	}
	for _, token := range forbidden {
		if strings.Contains(text, token) {
			t.Fatalf("runtime_ops.go contains forbidden state-orchestration token %q", token)
		}
	}
}

func TestCLIAndMCPEntryPointsDoNotOwnStateOrchestration(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller not available")
	}
	baseDir := filepath.Dir(currentFile)

	checkNoTokensInFile(t, filepath.Join(baseDir, "cli_query.go"), []string{
		"NewCandidateStore(",
		"NewDomainGraphStore(",
		"NewGenomeStore(",
		"PrepareQueryRequestWithLocalState(",
	})
	checkNoTokensInFile(t, filepath.Join(baseDir, "mcp_tools.go"), []string{
		"NewCandidateStore(",
		"NewDomainGraphStore(",
		"NewGenomeStore(",
		"PrepareQueryRequestWithLocalState(",
		"PrepareReadRequestWithLocalState(",
		"PrepareCrawlRequestWithLocalState(",
	})
}

func TestTransportPackageFilesDoNotReintroduceStateOrchestration(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller not available")
	}
	baseDir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		t.Fatalf("read transport dir: %v", err)
	}

	excluded := map[string]struct{}{
		"architecture_guard_test.go": {},
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, skip := excluded[name]; skip {
			continue
		}
		path := filepath.Join(baseDir, name)
		checkNoTokensInFile(t, path, transportStateOrchestrationTokens())
	}
}

func TestTransportStoreImportsAreExplicitlyScoped(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller not available")
	}
	baseDir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		t.Fatalf("read transport dir: %v", err)
	}

	allowStoreImport := map[string]struct{}{
		"runtime_ops.go": {},
		"mcp_tools.go":   {},
		"cli_prune.go":   {},
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(baseDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), `"github.com/josepavese/needlex/internal/store"`) {
			continue
		}
		if _, ok := allowStoreImport[name]; !ok {
			t.Fatalf("%s imports internal/store but is not in allowlist", name)
		}
	}
}

func TestTransportFilesDoNotDefineStateLogicSymbols(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller not available")
	}
	baseDir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		t.Fatalf("read transport dir: %v", err)
	}
	forbidden := []string{
		"func applyGenomeTo",
		"func mergeDomainHints(",
		"func domainHintsFrom",
		"func observeCandidate",
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(baseDir, name)
		checkNoTokensInFile(t, path, forbidden)
	}
}

func transportStateOrchestrationTokens() []string {
	tokens := []string{
		"NewCandidateStore(",
		"NewDomainGraphStore(",
		"NewGenomeStore(",
	}
	slices.Sort(tokens)
	return tokens
}

func checkNoTokensInFile(t *testing.T, path string, forbidden []string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	text := string(data)
	for _, token := range forbidden {
		if strings.Contains(text, token) {
			t.Fatalf("%s contains forbidden token %q", filepath.Base(path), token)
		}
	}
}
