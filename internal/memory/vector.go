package memory

import (
	"bytes"
	"encoding/binary"
	"fmt"
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
