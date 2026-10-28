package vectorindex

import (
	"context"
	"math"
	"testing"
)

func TestCosineContract(t *testing.T) {
	for _, tc := range []struct {
		name        string
		left, right []float32
		want        float64
	}{
		{"empty", nil, nil, 0},
		{"dimensions", []float32{1}, []float32{1, 0}, 0},
		{"zero_left", []float32{0, 0}, []float32{1, 1}, 0},
		{"zero_right", []float32{1, 1}, []float32{0, 0}, 0},
		{"scaled", []float32{3, 4}, []float32{6, 8}, 1},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0},
		{"opposite", []float32{3, 4}, []float32{-6, -8}, -1},
		{"diagonal", []float32{1, 0}, []float32{1, 1}, 1 / math.Sqrt2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Cosine(tc.left, tc.right); math.Abs(got-tc.want) > 1e-12 {
				t.Fatalf("cosine: got %v want %v", got, tc.want)
			}
		})
	}
}

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
