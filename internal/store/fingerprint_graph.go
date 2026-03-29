package store

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core"
)

type FingerprintGraphNode struct {
	Fingerprint string   `json:"fingerprint"`
	ChunkID     string   `json:"chunk_id"`
	HeadingPath []string `json:"heading_path,omitempty"`
}

type FingerprintGraphRelation struct {
	Fingerprint     string `json:"fingerprint"`
	PreviousChunkID string `json:"previous_chunk_id"`
	CurrentChunkID  string `json:"current_chunk_id"`
}

type FingerprintGraphDelta struct {
	PreviousTraceID string                     `json:"previous_trace_id,omitempty"`
	CurrentTraceID  string                     `json:"current_trace_id"`
	Retained        []FingerprintGraphRelation `json:"retained,omitempty"`
	Added           []string                   `json:"added,omitempty"`
	Removed         []string                   `json:"removed,omitempty"`
	ObservedAt      time.Time                  `json:"observed_at"`
}

type FingerprintGraph struct {
	URL           string                  `json:"url"`
	LatestTraceID string                  `json:"latest_trace_id"`
	LatestNodes   []FingerprintGraphNode  `json:"latest_nodes"`
	History       []FingerprintGraphDelta `json:"history,omitempty"`
	UpdatedAt     time.Time               `json:"updated_at"`
}

type FingerprintGraphStore struct {
	root string
	now  func() time.Time
}

func NewFingerprintGraphStore(root string) FingerprintGraphStore {
	if strings.TrimSpace(root) == "" {
		root = ".needlex"
	}
	return FingerprintGraphStore{root: root, now: time.Now}
}

func (s FingerprintGraphStore) Observe(url, traceID string, chunks []core.Chunk) (FingerprintGraphDelta, string, error) {
	cleanURL := strings.TrimSpace(url)
	cleanTraceID := sanitizeID(traceID)
	if cleanURL == "" {
		return FingerprintGraphDelta{}, "", fmt.Errorf("url must not be empty")
	}
	if cleanTraceID == "" {
		return FingerprintGraphDelta{}, "", fmt.Errorf("trace id must not be empty")
	}
	nodes, err := fingerprintGraphNodes(chunks)
	if err != nil {
		return FingerprintGraphDelta{}, "", err
	}
	graph, _ := s.Load(cleanURL)
	now := s.now().UTC()
	delta := fingerprintGraphDelta(graph.LatestTraceID, cleanTraceID, graph.LatestNodes, nodes, now)
	graph.URL = cleanURL
	graph.LatestTraceID = cleanTraceID
	graph.LatestNodes = nodes
	graph.UpdatedAt = now
	if graph.History == nil {
		graph.History = []FingerprintGraphDelta{}
	}
	graph.History = append(graph.History, delta)

	dir := filepath.Join(s.root, "fingerprint_graph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return FingerprintGraphDelta{}, "", fmt.Errorf("create fingerprint graph dir: %w", err)
	}
	path := filepath.Join(dir, fingerprintGraphID(cleanURL)+".json")
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return FingerprintGraphDelta{}, "", fmt.Errorf("encode fingerprint graph: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return FingerprintGraphDelta{}, "", fmt.Errorf("write fingerprint graph: %w", err)
	}
	return delta, path, nil
}

func (s FingerprintGraphStore) Load(url string) (FingerprintGraph, error) {
	path := filepath.Join(s.root, "fingerprint_graph", fingerprintGraphID(strings.TrimSpace(url))+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FingerprintGraph{}, err
		}
		return FingerprintGraph{}, fmt.Errorf("read fingerprint graph: %w", err)
	}
	var graph FingerprintGraph
	if err := json.Unmarshal(data, &graph); err != nil {
		return FingerprintGraph{}, fmt.Errorf("decode fingerprint graph: %w", err)
	}
	if err := graph.Validate(); err != nil {
		return FingerprintGraph{}, err
	}
	return graph, nil
}

func (g FingerprintGraph) Validate() error {
	if strings.TrimSpace(g.URL) == "" {
		return fmt.Errorf("url is required")
	}
	if strings.TrimSpace(g.LatestTraceID) == "" {
		return fmt.Errorf("latest_trace_id is required")
	}
	if g.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at is required")
	}
	if len(g.LatestNodes) == 0 {
		return fmt.Errorf("latest_nodes must not be empty")
	}
	for i, node := range g.LatestNodes {
		if err := node.Validate(); err != nil {
			return fmt.Errorf("latest_nodes[%d]: %w", i, err)
		}
	}
	for i, delta := range g.History {
		if err := delta.Validate(); err != nil {
			return fmt.Errorf("history[%d]: %w", i, err)
		}
	}
	return nil
}

func (n FingerprintGraphNode) Validate() error {
	if strings.TrimSpace(n.Fingerprint) == "" {
		return fmt.Errorf("fingerprint is required")
	}
	if strings.TrimSpace(n.ChunkID) == "" {
		return fmt.Errorf("chunk_id is required")
	}
	for i, part := range n.HeadingPath {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("heading_path[%d] must not be empty", i)
		}
	}
	return nil
}

func (d FingerprintGraphDelta) Validate() error {
	if strings.TrimSpace(d.CurrentTraceID) == "" {
		return fmt.Errorf("current_trace_id is required")
	}
	if d.ObservedAt.IsZero() {
		return fmt.Errorf("observed_at is required")
	}
	for i, rel := range d.Retained {
		if err := rel.Validate(); err != nil {
			return fmt.Errorf("retained[%d]: %w", i, err)
		}
	}
	for i, fp := range d.Added {
		if strings.TrimSpace(fp) == "" {
			return fmt.Errorf("added[%d] must not be empty", i)
		}
	}
	for i, fp := range d.Removed {
		if strings.TrimSpace(fp) == "" {
			return fmt.Errorf("removed[%d] must not be empty", i)
		}
	}
	return nil
}

func (r FingerprintGraphRelation) Validate() error {
	if strings.TrimSpace(r.Fingerprint) == "" {
		return fmt.Errorf("fingerprint is required")
	}
	if strings.TrimSpace(r.PreviousChunkID) == "" {
		return fmt.Errorf("previous_chunk_id is required")
	}
	if strings.TrimSpace(r.CurrentChunkID) == "" {
		return fmt.Errorf("current_chunk_id is required")
	}
	return nil
}

func fingerprintGraphNodes(chunks []core.Chunk) ([]FingerprintGraphNode, error) {
	nodes := make([]FingerprintGraphNode, 0, len(chunks))
	for i, chunk := range chunks {
		if err := chunk.Validate(); err != nil {
			return nil, fmt.Errorf("chunks[%d]: %w", i, err)
		}
		nodes = append(nodes, FingerprintGraphNode{
			Fingerprint: chunk.Fingerprint,
			ChunkID:     chunk.ID,
			HeadingPath: append([]string{}, chunk.HeadingPath...),
		})
	}
	return nodes, nil
}

func fingerprintGraphDelta(previousTraceID, currentTraceID string, previous, current []FingerprintGraphNode, observedAt time.Time) FingerprintGraphDelta {
	prevByFingerprint := make(map[string]FingerprintGraphNode, len(previous))
	for _, node := range previous {
		prevByFingerprint[node.Fingerprint] = node
	}
	currentByFingerprint := make(map[string]FingerprintGraphNode, len(current))
	for _, node := range current {
		currentByFingerprint[node.Fingerprint] = node
	}
	delta := FingerprintGraphDelta{
		PreviousTraceID: strings.TrimSpace(previousTraceID),
		CurrentTraceID:  strings.TrimSpace(currentTraceID),
		ObservedAt:      observedAt,
	}
	for fp, node := range currentByFingerprint {
		if prev, ok := prevByFingerprint[fp]; ok {
			delta.Retained = append(delta.Retained, FingerprintGraphRelation{
				Fingerprint:     fp,
				PreviousChunkID: prev.ChunkID,
				CurrentChunkID:  node.ChunkID,
			})
			continue
		}
		delta.Added = append(delta.Added, fp)
	}
	for fp := range prevByFingerprint {
		if _, ok := currentByFingerprint[fp]; ok {
			continue
		}
		delta.Removed = append(delta.Removed, fp)
	}
	return delta
}

func fingerprintGraphID(url string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(url)))
	return fmt.Sprintf("fpgraph_%x", sum[:6])
}
