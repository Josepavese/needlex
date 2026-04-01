package transport

import (
	"fmt"
	"io"
	"strings"

	"github.com/josepavese/needlex/internal/core"
)

type artifactPaths struct {
	TracePath       string `json:"trace_path"`
	ProofPath       string `json:"proof_path"`
	FingerprintPath string `json:"fingerprint_path"`
}

func renderWebIRSummary(w io.Writer, nodeCount int, signals core.WebIRSignals) {
	fmt.Fprintf(w, "Web IR Nodes: %d\n", nodeCount)
	fmt.Fprintf(w, "Web IR Signals: heading=%.2f short_text=%.2f embedded=%d\n", signals.HeadingRatio, signals.ShortTextRatio, signals.EmbeddedNodeCount)
}

func renderArtifactPaths(w io.Writer, artifacts artifactPaths) {
	fmt.Fprintf(w, "Trace Path: %s\nProof Path: %s\nFingerprint Path: %s\n", artifacts.TracePath, artifacts.ProofPath, artifacts.FingerprintPath)
}

func renderChunkTexts(w io.Writer, chunks []core.Chunk, headings bool) {
	for i, chunk := range chunks {
		fmt.Fprintf(w, "\n[%d] ", i+1)
		if headings {
			fmt.Fprintf(w, "%s\n%s\n", firstNonEmptyValue(strings.Join(chunk.HeadingPath, " > "), "(no heading)"), chunk.Text)
			continue
		}
		fmt.Fprintf(w, "%s\n", chunk.Text)
	}
}
