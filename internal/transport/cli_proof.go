package transport

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/josepavese/needlex/internal/proof"
)

type proofLookupResult struct {
	Lookup  string              `json:"lookup"`
	TraceID string              `json:"trace_id"`
	Records []proof.ProofRecord `json:"proof_records"`
}

func writeProofUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  needle proof <trace-id|chunk-id> [--json]")
}

func (r Runner) runProof(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("proof", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")

	if err := fs.Parse(normalizeArgs(args, nil)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		writeProofUsage(stderr)
		return 2
	}

	lookup := strings.TrimSpace(fs.Arg(0))
	result, err := r.loadProof(lookup)
	if err != nil {
		fmt.Fprintf(stderr, "load proof: %v\n", err)
		return 1
	}

	if jsonOut {
		return writeJSON(stdout, stderr, result)
	}

	renderProofText(stdout, result)
	return 0
}

func renderProofText(w io.Writer, result proofLookupResult) {
	fmt.Fprintf(w, "Lookup: %s\n", result.Lookup)
	fmt.Fprintf(w, "Trace ID: %s\n", result.TraceID)
	fmt.Fprintf(w, "Proof Records: %d\n", len(result.Records))
	for i, record := range result.Records {
		fmt.Fprintf(w, "\n[%d] %s\n", i+1, record.ID)
		fmt.Fprintf(w, "Chunk ID: %s\n", record.Proof.ChunkID)
		fmt.Fprintf(w, "Selector: %s\n", record.Proof.SourceSpan.Selector)
		fmt.Fprintf(w, "Lane: %d\n", record.Proof.Lane)
		if len(record.Proof.TransformChain) > 0 {
			fmt.Fprintf(w, "Transform Chain: %s\n", strings.Join(record.Proof.TransformChain, " -> "))
		}
	}
}
