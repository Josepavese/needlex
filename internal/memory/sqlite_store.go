package memory

import (
	"path/filepath"
	"strings"

	"github.com/josepavese/needlex/internal/platform"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	root   string
	dbPath string
}

type topicNodeRow struct {
	TopicKey            string
	Host                string
	RootPath            string
	RepresentativeURL   string
	RepresentativeTitle string
	SemanticSummary     string
	Language            string
	SupportCount        int
	ChildCount          int
	TopicDepth          int
	ObservedAt          string
	UpdatedAt           string
	Vector              []byte
}

type topicDoc struct {
	URL        string
	Title      string
	Path       string
	Summary    string
	Language   string
	ObservedAt string
	Vector     []float32
}

type rowScanner interface {
	Scan(dest ...any) error
}

type memoryDocumentRow struct {
	URL             string
	Title           string
	Host            string
	RawProofRefs    string
	TraceRef        string
	SourceKind      string
	ObservedAtRaw   string
	StableRatio     float64
	NoveltyRatio    float64
	ChangedRecently int
}

type embeddingCandidateRow struct {
	memoryDocumentRow
	RawVector []byte
}

func NewSQLiteStore(root, relativePath string) SQLiteStore {
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		cleanRoot = platform.DefaultStateRoot()
	}
	cleanPath := strings.TrimSpace(relativePath)
	if cleanPath == "" {
		cleanPath = platform.DefaultDiscoveryDBRelativePath
	}
	if filepath.IsAbs(cleanPath) {
		return SQLiteStore{root: cleanRoot, dbPath: cleanPath}
	}
	return SQLiteStore{root: cleanRoot, dbPath: filepath.Join(cleanRoot, cleanPath)}
}

func (s SQLiteStore) DBPath() string {
	return s.dbPath
}
