package transport

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/platform"
)

type configShowResult struct {
	Path   string        `json:"path"`
	Config config.Config `json:"config"`
}

func writeConfigUsage(w io.Writer) {
	writeUsage(w,
		"needlex config path [--path path]",
		"needlex config show [--json] [--path path]",
		"needlex config init [--force] [--path path]",
		"needlex config set <key> <value> [--path path]",
		"keys: semantic.embedding_url, semantic.provider_model, semantic.vector_space, semantic.timeout_ms, semantic.max_candidates, models.base_url, models.backend",
	)
}

func (r Runner) runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeConfigUsage(stderr)
		return 2
	}
	switch args[0] {
	case "path":
		return r.runConfigPath(args[1:], stdout, stderr)
	case "show":
		return r.runConfigShow(args[1:], stdout, stderr)
	case "init":
		return r.runConfigInit(args[1:], stdout, stderr)
	case "set":
		return r.runConfigSet(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		writeConfigUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown config subcommand %q\n\n", args[0])
		writeConfigUsage(stderr)
		return 2
	}
}

func (r Runner) runConfigPath(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config path", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var path string
	fs.StringVar(&path, "path", "", "override config path")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{"--path": {}, "-path": {}})); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex config path [--path path]")
		return 2
	}
	fmt.Fprintln(stdout, effectiveConfigPath(path))
	return 0
}

func (r Runner) runConfigShow(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var path string
	var jsonOut bool
	fs.StringVar(&path, "path", "", "override config path")
	fs.BoolVar(&jsonOut, "json", false, "emit JSON output")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{"--path": {}, "-path": {}})); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex config show [--json] [--path path]")
		return 2
	}
	resolved := effectiveConfigPath(path)
	cfg, err := loadConfigForConfigCommand(resolved)
	if err != nil {
		return r.reportCLIError(stderr, "config_show", err, map[string]any{"config_path": resolved})
	}
	if jsonOut {
		return r.writeJSON(stdout, stderr, "config_show", configShowResult{Path: resolved, Config: cfg})
	}
	renderConfigText(stdout, resolved, cfg)
	return 0
}

func (r Runner) runConfigInit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var path string
	var force bool
	fs.StringVar(&path, "path", "", "override config path")
	fs.BoolVar(&force, "force", false, "overwrite existing config")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{"--path": {}, "-path": {}})); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		writeUsage(stderr, "needlex config init [--force] [--path path]")
		return 2
	}
	resolved := effectiveConfigPath(path)
	if _, err := os.Stat(resolved); err == nil && !force {
		fmt.Fprintf(stdout, "Config already exists: %s\n", resolved)
		return 0
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return r.reportCLIError(stderr, "config_init", err, map[string]any{"config_path": resolved})
	}
	cfg, err := config.DefaultsWithEnv()
	if err != nil {
		return r.reportCLIError(stderr, "config_init", err, map[string]any{"config_path": resolved})
	}
	if err := config.Write(resolved, cfg); err != nil {
		return r.reportCLIError(stderr, "config_init", err, map[string]any{"config_path": resolved})
	}
	fmt.Fprintf(stdout, "Initialized config: %s\n", resolved)
	return 0
}

func (r Runner) runConfigSet(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config set", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var path string
	fs.StringVar(&path, "path", "", "override config path")
	if err := fs.Parse(normalizeArgs(args, map[string]struct{}{"--path": {}, "-path": {}})); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		writeUsage(stderr, "needlex config set <key> <value> [--path path]")
		return 2
	}
	resolved := effectiveConfigPath(path)
	cfg, err := loadConfigForConfigCommand(resolved)
	if err != nil {
		return r.reportCLIError(stderr, "config_set", err, map[string]any{"config_path": resolved})
	}
	if err := setConfigValue(&cfg, fs.Arg(0), fs.Arg(1)); err != nil {
		return r.reportCLIErrorCode(stderr, "config_set", err, map[string]any{"key": fs.Arg(0)}, 2)
	}
	if err := config.Write(resolved, cfg); err != nil {
		return r.reportCLIError(stderr, "config_set", err, map[string]any{"config_path": resolved, "key": fs.Arg(0)})
	}
	fmt.Fprintf(stdout, "Updated %s in %s\n", fs.Arg(0), resolved)
	return 0
}

func effectiveConfigPath(path string) string {
	if resolved := config.ResolvePath(path); resolved != "" {
		return resolved
	}
	return config.DefaultPath()
}

func loadConfigForConfigCommand(path string) (config.Config, error) {
	if strings.TrimSpace(path) == "" {
		return config.Load("")
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return config.Defaults(), nil
	}
	return config.Load(path)
}

func setConfigValue(cfg *config.Config, key, value string) error {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	switch key {
	case "semantic.embedding_url":
		cfg.Semantic.EmbeddingURL = value
	case "semantic.provider_model":
		cfg.Semantic.ProviderModel = value
	case "semantic.vector_space":
		cfg.Semantic.VectorSpace = value
	case "semantic.timeout_ms":
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("%s must be an integer", key)
		}
		cfg.Semantic.TimeoutMS = parsed
	case "semantic.max_candidates":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer", key)
		}
		cfg.Semantic.MaxCandidates = parsed
	case "models.backend":
		cfg.Models.Backend = value
	case "models.base_url":
		cfg.Models.BaseURL = value
	case "models.router":
		cfg.Models.Router = value
	case "models.judge":
		cfg.Models.Judge = value
	case "models.extractor":
		cfg.Models.Extractor = value
	case "models.formatter":
		cfg.Models.Formatter = value
	default:
		return fmt.Errorf("unsupported config key %q", key)
	}
	return cfg.Validate()
}

func renderConfigText(w io.Writer, path string, cfg config.Config) {
	fmt.Fprintf(w, "Needle-X Config\n")
	fmt.Fprintf(w, "Path: %s\n", path)
	fmt.Fprintf(w, "Embedding URL: %s\n", cfg.Semantic.EmbeddingURL)
	fmt.Fprintf(w, "Embedding Model: %s\n", cfg.Semantic.ProviderModel)
	fmt.Fprintf(w, "Vector Space: %s\n", cfg.Semantic.VectorSpace)
	fmt.Fprintf(w, "Model Backend: %s\n", cfg.Models.Backend)
	fmt.Fprintf(w, "Model Base URL: %s\n", cfg.Models.BaseURL)
	fmt.Fprintf(w, "State Config Env: %s\n", os.Getenv(platform.EnvConfig))
}
