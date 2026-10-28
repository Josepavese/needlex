package memory

import (
	"context"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/intel"
)

type Store interface {
	UpsertDocument(ctx context.Context, doc Document) error
	UpsertEdges(ctx context.Context, edges []Edge) error
	ReusableEmbeddingVector(ctx context.Context, emb Embedding) ([]float32, bool, error)
	UpsertEmbedding(ctx context.Context, emb Embedding, vector []float32) error
	UpsertSemanticFamilyEvidence(ctx context.Context, doc Document, vector []float32, vectorSpace string) error
	RefreshTopicNodes(ctx context.Context, doc Document, vectorSpace string) error
	DocumentsForEmbeddingRefresh(ctx context.Context) ([]Document, error)
	SearchTopicNodes(ctx context.Context, vector []float32, vectorSpace string, limit int, domainHints []string) ([]Candidate, error)
	SearchByVector(ctx context.Context, vector []float32, vectorSpace string, limit int, domainHints []string) ([]Candidate, error)
	SearchSemanticFamilies(ctx context.Context, vector []float32, vectorSpace string, limit int, domainHints []string) ([]Candidate, error)
	ExpandAncestorRoots(ctx context.Context, urls []string, limit int) ([]Candidate, error)
	ExpandNeighbors(ctx context.Context, urls []string, limit int) ([]Candidate, error)
	ExpandHosts(ctx context.Context, hosts []string, limit int) ([]Candidate, error)
	GetStats(ctx context.Context) (Stats, error)
	Prune(ctx context.Context, policy PrunePolicy) error
	RebuildIndex(ctx context.Context) error
	ExportJSONL(ctx context.Context, dir string) (ExportStats, error)
	ImportJSONL(ctx context.Context, dir string) (ImportStats, error)
}

// LocalStore adds the location used by operator diagnostics to the storage
// contract. No native connection or database handle crosses this boundary.
type LocalStore interface {
	Store
	DBPath() string
}

type Searcher interface {
	Search(context.Context, string, SearchOptions) ([]Candidate, error)
}

// Runtime is the semantic memory capability. Its caller uses each borrowed
// instance synchronously; persistence and embedding resources stay in adapters.
type Runtime interface {
	Searcher
	Observe(context.Context, Observation) error
	RefreshEmbeddings(context.Context, bool) (EmbeddingRefreshStats, error)
}

var (
	_ LocalStore = SQLiteStore{}
	_ Runtime    = Service{}
)

type Service struct {
	cfg      config.MemoryConfig
	store    Store
	embedder intel.TextEmbedder
	now      func() time.Time
}

func NewService(cfg config.MemoryConfig, store Store, embedder intel.TextEmbedder) Service {
	return Service{cfg: cfg, store: store, embedder: embedder, now: time.Now}
}
