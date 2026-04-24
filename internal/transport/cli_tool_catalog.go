package transport

import (
	"flag"
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
		return r.reportCLIErrorCode(stderr, "tool_catalog", err, map[string]any{"provider": provider, "strict": strict}, 2)
	}
	return r.writeJSON(stdout, stderr, "tool_catalog", catalog)
}
