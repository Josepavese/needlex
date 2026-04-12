package memory

import (
	"time"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/proof"
)

type Document struct {
	URL             string
	FinalURL        string
	Host            string
	Path            string
	Title           string
	SemanticSummary string
	Language        string
	LocalityHints   []string
	EntityHints     []string
	CategoryHints   []string
	ProofRefs       []string
	LastTraceID     string
	SourceKind      string
	StableRatio     float64
	NoveltyRatio    float64
	ChangedRecently bool
	ObservedAt      time.Time
	UpdatedAt       time.Time
}

type Edge struct {
	SourceURL  string
	TargetURL  string
	AnchorText string
	SameHost   bool
	TraceRef   string
	ObservedAt time.Time
}

type Embedding struct {
	EmbeddingRef string
	DocumentURL  string
	Model        string
	Backend      string
	InputText    string
	Dimension    int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Candidate struct {
	URL             string
	Title           string
	Score           float64
	Reasons         []string
	TraceRef        string
	ProofRef        string
	Host            string
	Distance        float64
	Source          string
	ObservedAt      time.Time
	StableRatio     float64
	NoveltyRatio    float64
	ChangedRecently bool
}

type Stats struct {
	DocumentCount  int
	EdgeCount      int
	EmbeddingCount int
	LastObservedAt time.Time
	LastRebuildAt  time.Time
	DBPath         string
}

type PrunePolicy struct {
	MaxDocuments  int
	MaxEdges      int
	MaxEmbeddings int
}

type SearchOptions struct {
	Limit         int
	ExpandLimit   int
	MinScore      float64
	DomainHints   []string
	QueryVariants []string
}

type Observation struct {
	Document        core.Document
	ResultPack      core.ResultPack
	ProofRecords    []proof.ProofRecord
	TraceID         string
	SourceKind      string
	ObservedAt      time.Time
	Language        string
	LocalityHints   []string
	EntityHints     []string
	CategoryHints   []string
	StableRatio     float64
	NoveltyRatio    float64
	ChangedRecently bool
}

type ExportStats struct {
	DocumentsPath  string `json:"documents_path"`
	EdgesPath      string `json:"edges_path"`
	EmbeddingsPath string `json:"embeddings_path"`
	DocumentCount  int    `json:"document_count"`
	EdgeCount      int    `json:"edge_count"`
	EmbeddingCount int    `json:"embedding_count"`
}

type ImportStats struct {
	DocumentCount  int `json:"document_count"`
	EdgeCount      int `json:"edge_count"`
	EmbeddingCount int `json:"embedding_count"`
}
