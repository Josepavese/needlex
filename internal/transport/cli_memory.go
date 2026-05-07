package transport

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/memory"
)

type memoryStatsResult struct {
	Stats compactMemoryStats `json:"stats"`
}

type memorySearchResult struct {
	Query      string                   `json:"query"`
	Candidates []compactMemoryCandidate `json:"candidates"`
}

type memoryPruneResult struct {
	Before  compactMemoryStats `json:"before"`
	After   compactMemoryStats `json:"after"`
	Policy  memory.PrunePolicy `json:"policy"`
	Removed map[string]int     `json:"removed"`
}

type memoryExportResult struct {
	Export memory.ExportStats `json:"export"`
}

type memoryImportResult struct {
	Import memory.ImportStats `json:"import"`
}

type memoryRebuildResult struct {
	Stats compactMemoryStats `json:"stats"`
}

type compactMemoryStats struct {
	DocumentCount       int       `json:"document_count"`
	EdgeCount           int       `json:"edge_count"`
	EmbeddingCount      int       `json:"embedding_count"`
	TopicNodeCount      int       `json:"topic_node_count,omitempty"`
	SemanticFamilyCount int       `json:"semantic_family_count,omitempty"`
	SemanticMemberCount int       `json:"semantic_member_count,omitempty"`
	VectorEngine        string    `json:"vector_engine,omitempty"`
	VectorDimensions    []int     `json:"vector_dimensions,omitempty"`
	LastObservedAt      time.Time `json:"last_observed_at,omitempty"`
	LastRebuildAt       time.Time `json:"last_rebuild_at,omitempty"`
	DBPath              string    `json:"db_path"`
}

type compactMemoryCandidate struct {
	URL             string    `json:"url"`
	Title           string    `json:"title,omitempty"`
	Score           float64   `json:"score"`
	Reasons         []string  `json:"reasons,omitempty"`
	ProofRef        string    `json:"proof_ref,omitempty"`
	TraceRef        string    `json:"trace_ref,omitempty"`
	Source          string    `json:"source,omitempty"`
	ObservedAt      time.Time `json:"observed_at,omitempty"`
	StableRatio     float64   `json:"stable_ratio,omitempty"`
	NoveltyRatio    float64   `json:"novelty_ratio,omitempty"`
	ChangedRecently bool      `json:"changed_recently,omitempty"`
}

func writeMemoryUsage(w io.Writer) {
	writeUsage(w, "needlex memory <stats|search|prune|export|import|rebuild-index> [args]", "subcommands: stats, search, prune, export, import, rebuild-index")
}

func (r Runner) runMemory(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeMemoryUsage(stderr)
		return 2
	}
	switch args[0] {
	case "stats":
		return r.runMemoryStats(args[1:], stdout, stderr)
	case "search":
		return r.runMemorySearch(args[1:], stdout, stderr)
	case "prune":
		return r.runMemoryPrune(args[1:], stdout, stderr)
	case "export":
		return r.runMemoryExport(args[1:], stdout, stderr)
	case "import":
		return r.runMemoryImport(args[1:], stdout, stderr)
	case "rebuild-index":
		return r.runMemoryRebuildIndex(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		writeMemoryUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown memory subcommand %q\n\n", args[0])
		writeMemoryUsage(stderr)
		return 2
	}
}

func (r Runner) runMemoryStats(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory stats", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var configPath string
	var jsonOut bool
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex memory stats [--json] [--config path]")
		return 2
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	stats, err := r.loadMemoryStats(cfg)
	if err != nil {
		return r.reportCLIError(stderr, "memory_stats", err, nil)
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "memory_stats", memoryStatsResult{Stats: compactStats(stats)})
	}
	fmt.Fprintf(stdout, "Documents: %d\n", stats.DocumentCount)
	fmt.Fprintf(stdout, "Edges: %d\n", stats.EdgeCount)
	fmt.Fprintf(stdout, "Embeddings: %d\n", stats.EmbeddingCount)
	fmt.Fprintf(stdout, "Topic Nodes: %d\n", stats.TopicNodeCount)
	fmt.Fprintf(stdout, "Semantic Families: %d\n", stats.SemanticFamilyCount)
	fmt.Fprintf(stdout, "Semantic Family Members: %d\n", stats.SemanticMemberCount)
	if stats.VectorEngine != "" {
		fmt.Fprintf(stdout, "Vector Engine: %s\n", stats.VectorEngine)
	}
	fmt.Fprintf(stdout, "DB Path: %s\n", stats.DBPath)
	if !stats.LastObservedAt.IsZero() {
		fmt.Fprintf(stdout, "Last Observed At: %s\n", stats.LastObservedAt.Format(time.RFC3339))
	}
	if !stats.LastRebuildAt.IsZero() {
		fmt.Fprintf(stdout, "Last Rebuild At: %s\n", stats.LastRebuildAt.Format(time.RFC3339))
	}
	return 0
}

func (r Runner) runMemorySearch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var configPath string
	var jsonOut bool
	var limit int
	var domainHints string
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.IntVar(&limit, "limit", 5, "candidate limit")
	fs.StringVar(&domainHints, "domain-hints", "", "comma-separated domain hints")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		writeUsage(stderr, "needlex memory search <query> [--json] [--config path] [--limit N] [--domain-hints host1,host2]")
		return 2
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	query := strings.TrimSpace(fs.Arg(0))
	candidates, err := r.searchMemory(cfg, query, limit, splitCSV(domainHints))
	if err != nil {
		return r.reportCLIError(stderr, "memory_search", err, map[string]any{"query": query, "limit": limit})
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "memory_search", memorySearchResult{Query: query, Candidates: compactMemoryCandidates(candidates)})
	}
	fmt.Fprintf(stdout, "Query: %s\n", query)
	fmt.Fprintf(stdout, "Candidates: %d\n", len(candidates))
	for i, candidate := range candidates {
		fmt.Fprintf(stdout, "%d. %s\n", i+1, candidate.URL)
		if strings.TrimSpace(candidate.Title) != "" {
			fmt.Fprintf(stdout, "   Title: %s\n", candidate.Title)
		}
		fmt.Fprintf(stdout, "   Score: %.4f\n", candidate.Score)
		if len(candidate.Reasons) > 0 {
			fmt.Fprintf(stdout, "   Reasons: %s\n", strings.Join(candidate.Reasons, ", "))
		}
		if strings.TrimSpace(candidate.ProofRef) != "" {
			fmt.Fprintf(stdout, "   Proof Ref: %s\n", candidate.ProofRef)
		}
		if !candidate.ObservedAt.IsZero() {
			fmt.Fprintf(stdout, "   Observed At: %s\n", candidate.ObservedAt.Format(time.RFC3339))
		}
	}
	return 0
}

func (r Runner) runMemoryPrune(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory prune", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var configPath string
	var jsonOut bool
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex memory prune [--json] [--config path]")
		return 2
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	before, after, policy, err := r.pruneMemory(cfg)
	if err != nil {
		return r.reportCLIError(stderr, "memory_prune", err, nil)
	}
	removed := map[string]int{
		"documents":  before.DocumentCount - after.DocumentCount,
		"edges":      before.EdgeCount - after.EdgeCount,
		"embeddings": before.EmbeddingCount - after.EmbeddingCount,
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "memory_prune", memoryPruneResult{Before: compactStats(before), After: compactStats(after), Policy: policy, Removed: removed})
	}
	fmt.Fprintf(stdout, "Documents: %d -> %d\n", before.DocumentCount, after.DocumentCount)
	fmt.Fprintf(stdout, "Edges: %d -> %d\n", before.EdgeCount, after.EdgeCount)
	fmt.Fprintf(stdout, "Embeddings: %d -> %d\n", before.EmbeddingCount, after.EmbeddingCount)
	return 0
}

func (r Runner) runMemoryExport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var configPath string
	var jsonOut bool
	var outDir string
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.StringVar(&outDir, "out", "", "export directory")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{"--out": {}, "-out": {}, "--config": {}, "-config": {}})); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(outDir) == "" {
		writeUsage(stderr, "needlex memory export --out DIR [--json] [--config path]")
		return 2
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	result, err := r.exportMemory(cfg, outDir)
	if err != nil {
		return r.reportCLIError(stderr, "memory_export", err, map[string]any{"out_dir": outDir})
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "memory_export", memoryExportResult{Export: result})
	}
	fmt.Fprintf(stdout, "Documents: %s (%d)\n", result.DocumentsPath, result.DocumentCount)
	fmt.Fprintf(stdout, "Edges: %s (%d)\n", result.EdgesPath, result.EdgeCount)
	fmt.Fprintf(stdout, "Embeddings: %s (%d)\n", result.EmbeddingsPath, result.EmbeddingCount)
	return 0
}

func (r Runner) runMemoryImport(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var configPath string
	var jsonOut bool
	var inDir string
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	fs.StringVar(&inDir, "in", "", "import directory")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{"--in": {}, "-in": {}, "--config": {}, "-config": {}})); err != nil {
		return 2
	}
	if fs.NArg() != 0 || strings.TrimSpace(inDir) == "" {
		writeUsage(stderr, "needlex memory import --in DIR [--json] [--config path]")
		return 2
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	result, err := r.importMemory(cfg, inDir)
	if err != nil {
		return r.reportCLIError(stderr, "memory_import", err, map[string]any{"in_dir": inDir})
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "memory_import", memoryImportResult{Import: result})
	}
	fmt.Fprintf(stdout, "Imported documents: %d\n", result.DocumentCount)
	fmt.Fprintf(stdout, "Imported edges: %d\n", result.EdgeCount)
	fmt.Fprintf(stdout, "Imported embeddings: %d\n", result.EmbeddingCount)
	return 0
}

func (r Runner) runMemoryRebuildIndex(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("memory rebuild-index", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var configPath string
	var jsonOut bool
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex memory rebuild-index [--json] [--config path]")
		return 2
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	stats, err := r.rebuildMemoryIndex(cfg)
	if err != nil {
		return r.reportCLIError(stderr, "memory_rebuild_index", err, nil)
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "memory_rebuild_index", memoryRebuildResult{Stats: compactStats(stats)})
	}
	fmt.Fprintf(stdout, "Rebuilt discovery memory acceleration state.\n")
	if !stats.LastRebuildAt.IsZero() {
		fmt.Fprintf(stdout, "Last Rebuild At: %s\n", stats.LastRebuildAt.Format(time.RFC3339))
	}
	fmt.Fprintf(stdout, "DB Path: %s\n", stats.DBPath)
	return 0
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (r Runner) loadMemoryStats(cfg config.Config) (memory.Stats, error) {
	store := memory.NewSQLiteStore(r.storeRoot, cfg.Memory.Path)
	return store.GetStats(context.Background())
}

func (r Runner) searchMemory(cfg config.Config, query string, limit int, domainHints []string) ([]memory.Candidate, error) {
	store := memory.NewSQLiteStore(r.storeRoot, cfg.Memory.Path)
	service := memory.NewService(cfg.Memory, store, intel.NewTextEmbedder(cfg, nil))
	return service.Search(context.Background(), query, memory.SearchOptions{
		Limit:       limit,
		ExpandLimit: 2,
		MinScore:    0.15,
		DomainHints: domainHints,
	})
}

func (r Runner) pruneMemory(cfg config.Config) (memory.Stats, memory.Stats, memory.PrunePolicy, error) {
	store := memory.NewSQLiteStore(r.storeRoot, cfg.Memory.Path)
	before, err := store.GetStats(context.Background())
	if err != nil {
		return memory.Stats{}, memory.Stats{}, memory.PrunePolicy{}, err
	}
	policy := memory.PrunePolicy{
		MaxDocuments:  cfg.Memory.MaxDocuments,
		MaxEdges:      cfg.Memory.MaxEdges,
		MaxEmbeddings: cfg.Memory.MaxEmbeddings,
	}
	if err := store.Prune(context.Background(), policy); err != nil {
		return memory.Stats{}, memory.Stats{}, memory.PrunePolicy{}, err
	}
	after, err := store.GetStats(context.Background())
	if err != nil {
		return memory.Stats{}, memory.Stats{}, memory.PrunePolicy{}, err
	}
	return before, after, policy, nil
}

func (r Runner) exportMemory(cfg config.Config, outDir string) (memory.ExportStats, error) {
	store := memory.NewSQLiteStore(r.storeRoot, cfg.Memory.Path)
	return store.ExportJSONL(context.Background(), outDir)
}

func (r Runner) importMemory(cfg config.Config, inDir string) (memory.ImportStats, error) {
	store := memory.NewSQLiteStore(r.storeRoot, cfg.Memory.Path)
	return store.ImportJSONL(context.Background(), inDir)
}

func (r Runner) rebuildMemoryIndex(cfg config.Config) (memory.Stats, error) {
	store := memory.NewSQLiteStore(r.storeRoot, cfg.Memory.Path)
	if err := store.RebuildIndex(context.Background()); err != nil {
		return memory.Stats{}, err
	}
	return store.GetStats(context.Background())
}

func compactStats(stats memory.Stats) compactMemoryStats {
	return compactMemoryStats{
		DocumentCount:       stats.DocumentCount,
		EdgeCount:           stats.EdgeCount,
		EmbeddingCount:      stats.EmbeddingCount,
		TopicNodeCount:      stats.TopicNodeCount,
		SemanticFamilyCount: stats.SemanticFamilyCount,
		SemanticMemberCount: stats.SemanticMemberCount,
		VectorEngine:        stats.VectorEngine,
		VectorDimensions:    append([]int{}, stats.VectorDimensions...),
		LastObservedAt:      stats.LastObservedAt,
		LastRebuildAt:       stats.LastRebuildAt,
		DBPath:              stats.DBPath,
	}
}

func compactMemoryCandidates(candidates []memory.Candidate) []compactMemoryCandidate {
	out := make([]compactMemoryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, compactMemoryCandidate{
			URL:             candidate.URL,
			Title:           candidate.Title,
			Score:           candidate.Score,
			Reasons:         append([]string{}, candidate.Reasons...),
			ProofRef:        candidate.ProofRef,
			TraceRef:        candidate.TraceRef,
			Source:          candidate.Source,
			ObservedAt:      candidate.ObservedAt,
			StableRatio:     candidate.StableRatio,
			NoveltyRatio:    candidate.NoveltyRatio,
			ChangedRecently: candidate.ChangedRecently,
		})
	}
	return out
}
