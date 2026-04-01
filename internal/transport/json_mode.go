package transport

import (
	"fmt"
	"strings"
)

const (
	jsonModeCompact = "compact"
	jsonModeFull    = "full"
)

func normalizeJSONMode(mode string) (string, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		return jsonModeCompact, nil
	}
	switch mode {
	case jsonModeCompact, jsonModeFull:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid json mode %q", mode)
	}
}
