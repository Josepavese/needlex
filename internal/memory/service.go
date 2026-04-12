package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/intel"
)

type Store interface {
	UpsertDocument(ctx context.Context, doc Document) error
	UpsertEdges(ctx context.Context, edges []Edge) error
	UpsertEmbedding(ctx context.Context, emb Embedding, vector []float32) error
	RefreshTopicNodes(ctx context.Context, doc Document) error
	SearchTopicNodes(ctx context.Context, vector []float32, limit int, domainHints []string) ([]Candidate, error)
	SearchByVector(ctx context.Context, vector []float32, limit int, domainHints []string) ([]Candidate, error)
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

func (s Service) Observe(ctx context.Context, obs Observation) error {
	if s.store == nil {
		return fmt.Errorf("memory store is required")
	}
	observedAt := obs.ObservedAt
	if observedAt.IsZero() {
		observedAt = s.now().UTC()
	}
	host, path := hostPath(firstNonEmpty(obs.Document.FinalURL, obs.Document.URL))
	proofRefs := proofRefsFromObservation(obs)
	summary := buildSemanticSummary(obs)
	doc := Document{
		URL:             firstNonEmpty(obs.Document.FinalURL, obs.Document.URL),
		FinalURL:        firstNonEmpty(obs.Document.FinalURL, obs.Document.URL),
		Host:            host,
		Path:            path,
		Title:           strings.TrimSpace(obs.Document.Title),
		SemanticSummary: summary,
		Language:        strings.TrimSpace(obs.Language),
		LocalityHints:   compactStrings(obs.LocalityHints),
		EntityHints:     compactStrings(obs.EntityHints),
		CategoryHints:   compactStrings(obs.CategoryHints),
		ProofRefs:       proofRefs,
		LastTraceID:     strings.TrimSpace(obs.TraceID),
		SourceKind:      firstNonEmpty(obs.SourceKind, "read"),
		StableRatio:     clampUnitInterval(obs.StableRatio),
		NoveltyRatio:    clampUnitInterval(obs.NoveltyRatio),
		ChangedRecently: obs.ChangedRecently,
		ObservedAt:      observedAt,
		UpdatedAt:       observedAt,
	}
	if err := s.store.UpsertDocument(ctx, doc); err != nil {
		return err
	}
	edges := buildEdges(doc.FinalURL, obs.ResultPack.Links, obs.TraceID, observedAt)
	if err := s.store.UpsertEdges(ctx, edges); err != nil {
		return err
	}
	input := buildEmbeddingInput(doc)
	vectors, err := s.embedder.Embed(ctx, []string{input})
	if err != nil {
		return err
	}
	if len(vectors) == 0 {
		return nil
	}
	emb := Embedding{
		EmbeddingRef: embeddingRef(doc.FinalURL, s.cfg.EmbeddingModel, s.cfg.EmbeddingBackend),
		DocumentURL:  doc.URL,
		Model:        s.cfg.EmbeddingModel,
		Backend:      s.cfg.EmbeddingBackend,
		InputText:    input,
		Dimension:    len(vectors[0]),
		CreatedAt:    observedAt,
		UpdatedAt:    observedAt,
	}
	if err := s.store.UpsertEmbedding(ctx, emb, vectors[0]); err != nil {
		return err
	}
	return s.store.RefreshTopicNodes(ctx, doc)
}

func (s Service) Search(ctx context.Context, goal string, opts SearchOptions) ([]Candidate, error) {
	if s.store == nil || s.embedder == nil {
		return nil, nil
	}
	inputs := []string{strings.TrimSpace(goal)}
	for _, variant := range opts.QueryVariants {
		variant = strings.TrimSpace(variant)
		if variant != "" && variant != strings.TrimSpace(goal) {
			inputs = append(inputs, variant)
		}
	}
	vectors, err := s.embedder.Embed(ctx, inputs)
	if err != nil {
		return nil, err
	}
	merged := map[string]Candidate{}
	for _, vector := range vectors {
		topicMatches, err := s.store.SearchTopicNodes(ctx, vector, opts.Limit, opts.DomainHints)
		if err != nil {
			return nil, err
		}
		for _, match := range topicMatches {
			mergeCandidate(merged, match)
		}
		matches, err := s.store.SearchByVector(ctx, vector, opts.Limit, opts.DomainHints)
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			mergeCandidate(merged, match)
		}
	}
	neighborLimit := opts.ExpandLimit
	if neighborLimit <= 0 {
		neighborLimit = minInt(3, opts.Limit)
	}
	ancestorRoots, err := s.store.ExpandAncestorRoots(ctx, candidateURLs(merged), neighborLimit)
	if err != nil {
		return nil, err
	}
	for _, candidate := range ancestorRoots {
		mergeCandidate(merged, candidate)
	}
	neighbors, err := s.store.ExpandNeighbors(ctx, candidateURLs(merged), neighborLimit)
	if err != nil {
		return nil, err
	}
	for _, neighbor := range neighbors {
		mergeCandidate(merged, neighbor)
	}
	hostCandidates, err := s.store.ExpandHosts(ctx, candidateHosts(merged), neighborLimit)
	if err != nil {
		return nil, err
	}
	for _, candidate := range hostCandidates {
		mergeCandidate(merged, candidate)
	}
	out := make([]Candidate, 0, len(merged))
	for _, candidate := range merged {
		if candidate.Score < opts.MinScore {
			continue
		}
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].URL < out[j].URL
		}
		return out[i].Score > out[j].Score
	})
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

func buildSemanticSummary(obs Observation) string {
	parts := make([]string, 0, 3)
	for _, chunk := range obs.ResultPack.Chunks {
		text := strings.TrimSpace(chunk.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
		if len(parts) == 3 {
			break
		}
	}
	joined := strings.Join(parts, " ")
	joined = strings.Join(strings.Fields(joined), " ")
	if len(joined) > 600 {
		joined = strings.TrimSpace(joined[:600])
	}
	if joined == "" {
		joined = strings.TrimSpace(obs.Document.Title)
	}
	return joined
}

func buildEmbeddingInput(doc Document) string {
	parts := []string{strings.TrimSpace(doc.Title), strings.TrimSpace(doc.SemanticSummary)}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func buildEdges(sourceURL string, links []string, traceID string, observedAt time.Time) []Edge {
	sourceHost, _ := hostPath(sourceURL)
	out := make([]Edge, 0, len(links))
	for _, targetURL := range compactStrings(links) {
		targetHost, _ := hostPath(targetURL)
		out = append(out, Edge{SourceURL: sourceURL, TargetURL: targetURL, AnchorText: targetURL, SameHost: sourceHost != "" && sourceHost == targetHost, TraceRef: traceID, ObservedAt: observedAt})
	}
	return out
}

func proofRefsFromObservation(obs Observation) []string {
	if len(obs.ResultPack.ProofRefs) > 0 {
		return compactStrings(obs.ResultPack.ProofRefs)
	}
	out := make([]string, 0, len(obs.ProofRecords))
	for _, record := range obs.ProofRecords {
		if strings.TrimSpace(record.ID) != "" {
			out = append(out, record.ID)
		}
	}
	return compactStrings(out)
}

func embeddingRef(documentURL, model, backend string) string {
	return prefixedHash("emb", documentURL, model, backend)
}

func candidateURLs(items map[string]Candidate) []string {
	out := make([]string, 0, len(items))
	for url := range items {
		out = append(out, url)
	}
	sort.Strings(out)
	return out
}

func candidateHosts(items map[string]Candidate) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		host := strings.TrimSpace(item.Host)
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	sort.Strings(out)
	return out
}

func mergeCandidate(target map[string]Candidate, candidate Candidate) {
	existing, ok := target[candidate.URL]
	if !ok || candidate.Score > existing.Score {
		target[candidate.URL] = candidate
		return
	}
	existing.Reasons = compactStrings(append(existing.Reasons, candidate.Reasons...))
	if existing.ProofRef == "" {
		existing.ProofRef = candidate.ProofRef
	}
	if existing.TraceRef == "" {
		existing.TraceRef = candidate.TraceRef
	}
	target[candidate.URL] = existing
}

func prefixedHash(prefix string, parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return prefix + "_" + hex.EncodeToString(sum[:8])
}

func minInt(left, right int) int {
	if left == 0 {
		return right
	}
	if right == 0 {
		return left
	}
	if left < right {
		return left
	}
	return right
}

func clampUnitInterval(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
