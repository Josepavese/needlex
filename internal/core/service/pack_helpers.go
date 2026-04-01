package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/intel"
)

func lanePath(maxLane int) []int {
	path := []int{0}
	if maxLane <= 0 {
		return path
	}
	for lane := 1; lane <= maxLane; lane++ {
		path = append(path, lane)
	}
	return path
}

func fingerprintSet(stable []string) map[string]struct{} {
	seen := make(map[string]struct{}, len(stable))
	for _, fp := range stable {
		seen[fp] = struct{}{}
	}
	return seen
}

func applyIntelTextResult(chunk *core.Chunk, decision *intel.Decision, text string, invocation core.ModelInvocation, additionalRisk []string) {
	if strings.TrimSpace(text) != "" {
		chunk.Text = text
	}
	if invocation.Model != "" {
		decision.ModelInvocations = append(decision.ModelInvocations, invocation)
	}
	decision.RiskFlags = append(decision.RiskFlags, additionalRisk...)
}

func prefixedHash(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + "_" + hex.EncodeToString(digest[:])[:16]
}
