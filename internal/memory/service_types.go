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
	UpsertEmbedding(ctx context.Context, emb Embedding, vector []float32) error
	UpsertSemanticFamilyEvidence(ctx context.Context, doc Document, vector []float32, vectorSpace string) error
	RefreshTopicNodes(ctx context.Context, doc Document, vectorSpace string) error
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

type Service struct {
	cfg      config.MemoryConfig
	store    Store
	embedder intel.TextEmbedder
	now      func() time.Time
}

func NewService(cfg config.MemoryConfig, store Store, embedder intel.TextEmbedder) Service {
	return Service{cfg: cfg, store: store, embedder: embedder, now: time.Now}
}
