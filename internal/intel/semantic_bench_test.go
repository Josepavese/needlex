package intel

import (
	"testing"

	"github.com/josepavese/needlex/internal/core/vectorindex"
)

func BenchmarkCosineSimilarity(b *testing.B) {
	left := make([]float32, 384)
	right := make([]float32, 384)
	for i := range left {
		left[i] = float32((i%7)+1) / 10
		right[i] = float32((i%5)+1) / 10
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = vectorindex.Cosine(left, right)
	}
}
