package vectorindex

import (
	"context"
	"testing"
)

func TestExactSearchOrdersByCosineSimilarity(t *testing.T) {
	idx, err := NewExact([]Item{{ID: "a", Vector: []float32{1, 0}}, {ID: "b", Vector: []float32{0, 1}}})
	if err != nil {
		t.Fatalf("new exact: %v", err)
	}
	hits, err := idx.Search(context.Background(), []float32{0.9, 0.1}, SearchOptions{Limit: 2})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 || hits[0].ID != "a" {
		t.Fatalf("unexpected hits: %#v", hits)
	}
	if idx.Stats().Engine != "exact" || idx.Stats().ItemCount != 2 || idx.Dimension() != 2 {
		t.Fatalf("unexpected stats: %#v", idx.Stats())
	}
}

func TestExactRejectsMixedDimensions(t *testing.T) {
	if _, err := NewExact([]Item{{ID: "a", Vector: []float32{1}}, {ID: "b", Vector: []float32{1, 0}}}); err == nil {
		t.Fatal("expected mixed dimension error")
	}
}
