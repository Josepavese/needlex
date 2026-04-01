package memory

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

func encodeVector(vector []float32) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, len(vector)*4))
	for _, value := range vector {
		if err := binary.Write(buf, binary.LittleEndian, value); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func decodeVector(raw []byte) ([]float32, error) {
	if len(raw)%4 != 0 {
		return nil, fmt.Errorf("invalid vector blob length %d", len(raw))
	}
	out := make([]float32, len(raw)/4)
	reader := bytes.NewReader(raw)
	for i := range out {
		if err := binary.Read(reader, binary.LittleEndian, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func cosineSimilarity(left, right []float32) float64 {
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
