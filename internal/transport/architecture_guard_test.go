package transport

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestRuntimeConsumersUseContractsAndOneCompositionPoint(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate transport sources")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(current), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	// true marks a factory; false marks a concrete adapter type.
	watched := map[string]map[string]bool{
		"analytics": {"NewSQLiteStore": true, "SQLiteStore": false},
		"memory":    {"NewSQLiteStore": true, "NewService": true, "SQLiteStore": false, "Service": false},
		"store":     {"NewLocalState": true, "CandidateStore": false, "DomainGraphStore": false, "GenomeStore": false, "FingerprintGraphStore": false},
		"rendering": {"NewRuntime": true},
	}
	bindings := map[string]int{}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			module := strings.TrimPrefix(strings.Trim(spec.Path.Value, `"`), "github.com/josepavese/needlex/internal/")
			symbols, watchedModule := watched[module]
			if !watchedModule {
				continue
			}
			alias := module
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			ast.Inspect(file, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				owner, ok := selector.X.(*ast.Ident)
				if !ok || owner.Name != alias {
					return true
				}
				factory, tracked := symbols[selector.Sel.Name]
				if !tracked {
					return true
				}
				if !factory {
					t.Errorf("%s consumes concrete adapter %s.%s", filepath.Base(path), module, selector.Sel.Name)
					return true
				}
				bindings[module+"."+selector.Sel.Name]++
				if filepath.Base(path) != "cli.go" {
					t.Errorf("%s bypasses the composition point for %s", filepath.Base(path), module)
				}
				return true
			})
		}
	}
	for module, symbols := range watched {
		for symbol, factory := range symbols {
			if factory && bindings[module+"."+symbol] != 1 {
				t.Errorf("expected one %s.%s binding, found %d", module, symbol, bindings[module+"."+symbol])
			}
		}
	}
}

func TestLocalStatePoliciesDoNotConstructAdapters(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate core sources")
	}
	for _, name := range []string{"query_state.go", "state_helpers.go", "read_crawl_state.go"} {
		path := filepath.Join(filepath.Dir(current), "..", "core", "service", name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ports := map[string]bool{}
		for _, spec := range file.Imports {
			pkg := strings.Trim(spec.Path.Value, `"`)
			switch pkg {
			case "os", "path/filepath", "database/sql", "os/exec":
				t.Errorf("%s imports native backend %s", name, pkg)
			}
			if strings.HasPrefix(pkg, "github.com/josepavese/needlex/internal/") {
				alias := filepath.Base(pkg)
				if spec.Name != nil {
					alias = spec.Name.Name
				}
				ports[alias] = true
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			owner, ok := selector.X.(*ast.Ident)
			if ok && ports[owner.Name] && strings.HasPrefix(selector.Sel.Name, "New") {
				t.Errorf("%s constructs %s.%s inside state policy", name, owner.Name, selector.Sel.Name)
			}
			return true
		})
	}
}

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
		"cli.go":         {}, // composition and public repository contracts
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

func TestRenderingNativeProcessEffectsStayInPAL(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "rendering", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{"os": true, "os/exec": true, "syscall": true, "net": true, "os/signal": true}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range source.Imports {
			if forbidden[strings.Trim(spec.Path.Value, `"`)] {
				t.Errorf("native process/platform import %s in %s", spec.Path.Value, path)
			}
		}
	}
}
