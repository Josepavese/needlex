package evalutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func WriteJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func Ratio(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func MinInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
