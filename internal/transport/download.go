package transport

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/platform"
)

const (
	downloadUsage = "needlex fetch <url> [--config path] [--out-dir NEW_DIR] [--assets] [--asset-origin ORIGIN]... [--max-bytes N]"
	webFetchTool  = "web_fetch"
)

func (r Runner) runFetch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		writeUsage(fs.Output(), downloadUsage, "Save original HTTP(S) response bytes and print file paths as JSON.")
		fs.PrintDefaults()
	}
	var request pipeline.DownloadRequest
	var destination, configPath string
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.StringVar(&destination, "out-dir", "", "new output directory (default: a new directory under NEEDLEX_HOME/downloads)")
	fs.BoolVar(&request.Assets, "assets", false, "also download linked HTML stylesheets and scripts")
	fs.Func("asset-origin", "allow an additional asset origin; repeat for multiple origins", func(origin string) error {
		request.AssetOrigins = append(request.AssetOrigins, origin)
		return nil
	})
	fs.Int64Var(&request.MaxBytes, "max-bytes", pipeline.DefaultDownloadMaxBytes, "maximum bytes for the entire download")
	if err := fs.Parse(normalizeArgs(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		writeUsage(stderr, downloadUsage)
		return 2
	}
	if request.MaxBytes <= 0 {
		fmt.Fprintln(stderr, "fetch: --max-bytes must be positive")
		return 2
	}
	cfg, ok := r.loadConfigOrExit(configPath, stderr)
	if !ok {
		return 1
	}
	request.URL = fs.Arg(0)
	request.Egress = platform.NewEgress(cfg.EgressPolicy())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := platform.WatchShutdown(cancel)
	defer stop()
	r.operationContext = ctx
	result, err := r.download(request, destination)
	if err != nil {
		fmt.Fprintf(stderr, "fetch: %v\n", err)
		return 1
	}
	return r.writeJSON(stdout, stderr, "fetch", result)
}

func (r Runner) download(request pipeline.DownloadRequest, destination string) (pipeline.DownloadResult, error) {
	parent := r.operationContext
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, pipeline.DefaultDownloadTimeout)
	defer cancel()
	managedDirectory := platform.NewStateLayout(r.storeRoot).DownloadsDir
	return pipeline.Download(ctx, request, func() (platform.DownloadDirectory, error) {
		return platform.NewDownloadDirectory(managedDirectory, destination)
	})
}

func (r Runner) callMCPFetchTool(args map[string]any) (map[string]any, error) {
	if value := args["assets"]; value != nil {
		if _, ok := value.(bool); !ok {
			return nil, errors.New("assets must be a boolean")
		}
	}
	origins, err := stringListArg(args, "asset_origins")
	if err != nil {
		return nil, err
	}
	var maxBytes int
	if args["max_bytes"] != nil {
		var ok bool
		if maxBytes, ok = intArg(args, "max_bytes"); !ok || maxBytes <= 0 {
			return nil, errors.New("max_bytes must be a positive integer")
		}
	}
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	result, err := r.download(pipeline.DownloadRequest{
		URL: stringArg(args, "url"), Assets: boolArg(args, "assets"),
		AssetOrigins: origins, MaxBytes: int64(maxBytes),
		Egress: platform.NewEgress(cfg.EgressPolicy()),
	}, "")
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"kind": "raw_download", "download": result}
	return mcpToolResult(payload, nil), nil
}

func mcpFetchTool() mcpTool {
	return mcpTool{
		Name:        webFetchTool,
		Description: "Download the original HTTP response body, including binary files, to the local Needle-X downloads directory. Returns file paths and HTTP metadata. Use for exact bytes; use web_read for evidence.",
		InputSchema: schemaExamples(toolSchema(map[string]any{
			"url":           map[string]any{"type": "string", "description": "Absolute HTTP(S) URL of the page or file."},
			"assets":        map[string]any{"type": "boolean", "default": false, "description": "Also download directly linked CSS and JavaScript from HTML. Only the final page origin and explicitly allowed origins are fetched."},
			"asset_origins": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Extra asset origins (scheme, host and optional port), for example a trusted CDN. Requires assets=true."},
			"max_bytes":     map[string]any{"type": "integer", "minimum": 1, "default": pipeline.DefaultDownloadMaxBytes, "description": "Maximum total bytes for the page and all downloaded assets. A failed download discards its files."},
		}, "url"), map[string]any{"url": "https://example.com", "assets": true}),
	}
}
