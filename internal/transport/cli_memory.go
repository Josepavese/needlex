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

type compactMemoryStats struct {
	DocumentCount  int       `json:"document_count"`
	EdgeCount      int       `json:"edge_count"`
	EmbeddingCount int       `json:"embedding_count"`
	LastObservedAt time.Time `json:"last_observed_at,omitempty"`
	DBPath         string    `json:"db_path"`
}

type compactMemoryCandidate struct {
	URL      string   `json:"url"`
	Title    string   `json:"title,omitempty"`
	Score    float64  `json:"score"`
	Reasons  []string `json:"reasons,omitempty"`
	ProofRef string   `json:"proof_ref,omitempty"`
	TraceRef string   `json:"trace_ref,omitempty"`
	Source   string   `json:"source,omitempty"`
}

func writeMemoryUsage(w io.Writer) {
	writeUsage(w, "needle memory <stats|search|prune> [args]", "subcommands: stats, search, prune")
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
		writeUsage(stderr, "needle memory stats [--json] [--config path]")
		return 2
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	stats, err := r.loadMemoryStats(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "memory stats failed: %v\n", err)
		return 1
	}
	if jsonOut {
		return writeJSON(stdout, stderr, memoryStatsResult{Stats: compactStats(stats)})
	}
	fmt.Fprintf(stdout, "Documents: %d\n", stats.DocumentCount)
	fmt.Fprintf(stdout, "Edges: %d\n", stats.EdgeCount)
	fmt.Fprintf(stdout, "Embeddings: %d\n", stats.EmbeddingCount)
	fmt.Fprintf(stdout, "DB Path: %s\n", stats.DBPath)
	if !stats.LastObservedAt.IsZero() {
		fmt.Fprintf(stdout, "Last Observed At: %s\n", stats.LastObservedAt.Format(time.RFC3339))
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
		writeUsage(stderr, "needle memory search <query> [--json] [--config path] [--limit N] [--domain-hints host1,host2]")
		return 2
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	query := strings.TrimSpace(fs.Arg(0))
	candidates, err := r.searchMemory(cfg, query, limit, splitCSV(domainHints))
	if err != nil {
		fmt.Fprintf(stderr, "memory search failed: %v\n", err)
		return 1
	}
	if jsonOut {
		return writeJSON(stdout, stderr, memorySearchResult{Query: query, Candidates: compactMemoryCandidates(candidates)})
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
		writeUsage(stderr, "needle memory prune [--json] [--config path]")
		return 2
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	before, after, policy, err := r.pruneMemory(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "memory prune failed: %v\n", err)
		return 1
	}
	removed := map[string]int{
		"documents":  before.DocumentCount - after.DocumentCount,
		"edges":      before.EdgeCount - after.EdgeCount,
		"embeddings": before.EmbeddingCount - after.EmbeddingCount,
	}
	if jsonOut {
		return writeJSON(stdout, stderr, memoryPruneResult{Before: compactStats(before), After: compactStats(after), Policy: policy, Removed: removed})
	}
	fmt.Fprintf(stdout, "Documents: %d -> %d\n", before.DocumentCount, after.DocumentCount)
	fmt.Fprintf(stdout, "Edges: %d -> %d\n", before.EdgeCount, after.EdgeCount)
	fmt.Fprintf(stdout, "Embeddings: %d -> %d\n", before.EmbeddingCount, after.EmbeddingCount)
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

func compactStats(stats memory.Stats) compactMemoryStats {
	return compactMemoryStats{
		DocumentCount:  stats.DocumentCount,
		EdgeCount:      stats.EdgeCount,
		EmbeddingCount: stats.EmbeddingCount,
		LastObservedAt: stats.LastObservedAt,
		DBPath:         stats.DBPath,
	}
}

func compactMemoryCandidates(candidates []memory.Candidate) []compactMemoryCandidate {
	out := make([]compactMemoryCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, compactMemoryCandidate{
			URL:      candidate.URL,
			Title:    candidate.Title,
			Score:    candidate.Score,
			Reasons:  append([]string{}, candidate.Reasons...),
			ProofRef: candidate.ProofRef,
			TraceRef: candidate.TraceRef,
			Source:   candidate.Source,
		})
	}
	return out
}
