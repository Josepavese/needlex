package intel

import "testing"

func BenchmarkCosineSimilarity(b *testing.B) {
	left := make([]float64, 384)
	right := make([]float64, 384)
	for i := range left {
		left[i] = float64((i%7)+1) / 10
		right[i] = float64((i%5)+1) / 10
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cosineSimilarity(left, right)
	}
}
