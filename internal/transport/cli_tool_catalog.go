package transport

import (
	"flag"
	"fmt"
	"io"
)

func writeToolCatalogUsage(w io.Writer) {
	writeUsage(w, "needlex tool-catalog --provider openai|anthropic [--strict]")
}

func (r Runner) runToolCatalog(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tool-catalog", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var provider string
	var strict bool

	fs.StringVar(&provider, "provider", "openai", "tool catalog provider: openai or anthropic")
	fs.BoolVar(&strict, "strict", false, "emit OpenAI strict function definitions")

	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{
		"--provider": {},
		"-provider":  {},
	})); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeToolCatalogUsage(stderr)
		return 2
	}

	catalog, err := toolCatalog(provider, strict)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}
	return writeJSON(stdout, stderr, catalog)
}
