package vectorindex

import (
	"context"
	"fmt"
	"math"
	"sort"
)

type Item struct {
	ID     string
	Vector []float32
}

type Hit struct {
	ID         string
	Similarity float64
	Distance   float64
}

type SearchOptions struct {
	Limit int
}

type Stats struct {
	Engine    string
	ItemCount int
	Dimension int
}

type Index interface {
	Name() string
	Dimension() int
	Search(ctx context.Context, query []float32, opts SearchOptions) ([]Hit, error)
	Stats() Stats
}

type Exact struct {
	dimension int
	items     []Item
}

func NewExact(items []Item) (Exact, error) {
	idx := Exact{items: make([]Item, 0, len(items))}
	for _, item := range items {
		if item.ID == "" || len(item.Vector) == 0 {
			continue
		}
		if idx.dimension == 0 {
			idx.dimension = len(item.Vector)
		}
		if len(item.Vector) != idx.dimension {
			return Exact{}, fmt.Errorf("vector dimension mismatch for %s: got %d want %d", item.ID, len(item.Vector), idx.dimension)
		}
		idx.items = append(idx.items, item)
	}
	return idx, nil
}

func (e Exact) Name() string { return "exact" }

func (e Exact) Dimension() int { return e.dimension }

func (e Exact) Stats() Stats {
	return Stats{Engine: e.Name(), ItemCount: len(e.items), Dimension: e.dimension}
}

func (e Exact) Search(ctx context.Context, query []float32, opts SearchOptions) ([]Hit, error) {
	if len(query) == 0 || len(e.items) == 0 {
		return nil, nil
	}
	if e.dimension != 0 && len(query) != e.dimension {
		return nil, fmt.Errorf("query vector dimension mismatch: got %d want %d", len(query), e.dimension)
	}
	hits := make([]Hit, 0, len(e.items))
	for _, item := range e.items {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		similarity := Cosine(query, item.Vector)
		if similarity <= 0 {
			continue
		}
		hits = append(hits, Hit{ID: item.ID, Similarity: similarity, Distance: 1 - similarity})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Similarity == hits[j].Similarity {
			return hits[i].ID < hits[j].ID
		}
		return hits[i].Similarity > hits[j].Similarity
	})
	if opts.Limit > 0 && len(hits) > opts.Limit {
		hits = hits[:opts.Limit]
	}
	return hits, nil
}

func Cosine(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		l := float64(left[i])
		r := float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}
