package rendering

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
)

type Request struct {
	URL                     string
	UserAgent               string
	Timeout                 time.Duration
	MaxBytes                int64
	NetworkIdle             time.Duration
	NetworkMaxBytes         int64
	NetworkResourceMaxBytes int64
	NetworkMaxResources     int
	NetworkMaxMessages      int
}

type Page struct {
	URL              string
	FinalURL         string
	HTML             string
	Browser          string
	Duration         time.Duration
	Partial          bool
	FetchedAt        time.Time
	NetworkResources []NetworkResource
	NetworkStats     NetworkStats
}

type NetworkResource struct {
	URL          string
	Type         string
	ContentType  string
	Status       int
	Source       string
	Body         string
	BodyBytes    int64
	Truncated    bool
	MessageCount int
	Finished     bool
}

type NetworkStats struct {
	ResourceCount       int
	EventSourceMessages int
	WebSocketMessages   int
	BodyBytes           int64
	Truncated           bool
	IdleReason          string
}

type Renderer interface {
	Render(ctx context.Context, req Request) (Page, error)
}

type NoopRenderer struct{}

func (NoopRenderer) Render(context.Context, Request) (Page, error) {
	return Page{}, errors.New("js rendering is disabled")
}

type ExecDumpDOMRenderer struct {
	BrowserPath             string
	Timeout                 time.Duration
	MaxBytes                int64
	NetworkIdle             time.Duration
	NetworkMaxBytes         int64
	NetworkResourceMaxBytes int64
	NetworkMaxResources     int
	NetworkMaxMessages      int
}

func New(cfg config.RenderConfig) Renderer {
	if !cfg.Enabled {
		return NoopRenderer{}
	}
	var renderer Renderer
	switch strings.TrimSpace(cfg.Provider) {
	case "", "exec-dump-dom":
		renderer = ExecDumpDOMRenderer{
			BrowserPath:             strings.TrimSpace(cfg.BrowserPath),
			Timeout:                 time.Duration(cfg.TimeoutMS) * time.Millisecond,
			NetworkIdle:             time.Duration(cfg.NetworkIdleMS) * time.Millisecond,
			NetworkMaxBytes:         cfg.NetworkMaxBytes,
			NetworkResourceMaxBytes: cfg.NetworkResourceMaxBytes,
			NetworkMaxResources:     cfg.NetworkMaxResources,
			NetworkMaxMessages:      cfg.NetworkMaxMessages,
		}
	default:
		return NoopRenderer{}
	}
	if cfg.MaxConcurrency > 0 {
		return limitedRenderer{next: renderer, sem: make(chan struct{}, cfg.MaxConcurrency)}
	}
	return renderer
}

type limitedRenderer struct {
	next Renderer
	sem  chan struct{}
}

func (r limitedRenderer) Render(ctx context.Context, req Request) (Page, error) {
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return Page{}, ctx.Err()
	}
	return r.next.Render(ctx, req)
}

func (r ExecDumpDOMRenderer) Render(ctx context.Context, req Request) (Page, error) {
	if strings.TrimSpace(req.URL) == "" {
		return Page{}, errors.New("render url must not be empty")
	}
	if page, err := r.renderWithCDP(ctx, req); err == nil {
		return page, nil
	}
	return r.renderWithDumpDOM(ctx, req)
}

func (r ExecDumpDOMRenderer) renderWithDumpDOM(ctx context.Context, req Request) (Page, error) {
	browserPath, err := FindBrowserPath(r.BrowserPath)
	if err != nil {
		return Page{}, err
	}
	timeout := firstPositiveDuration(req.Timeout, r.Timeout, 5*time.Second)
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = r.MaxBytes
	}
	if maxBytes <= 0 {
		maxBytes = 4_000_000
	}
	renderCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	profileDir, err := os.MkdirTemp("", "needlex-render-*")
	if err != nil {
		return Page{}, fmt.Errorf("create render profile: %w", err)
	}
	defer os.RemoveAll(profileDir) //nolint:errcheck

	args := []string{}
	if !isHeadlessShell(browserPath) {
		args = append(args, "--headless=new")
	}
	args = append(args,
		"--no-sandbox",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-dev-shm-usage",
		"--disable-background-networking",
		"--disable-sync",
		"--hide-scrollbars",
		"--mute-audio",
		"--user-data-dir="+profileDir,
		"--virtual-time-budget=3000",
		"--dump-dom",
	)
	if strings.TrimSpace(req.UserAgent) != "" {
		args = append(args, "--user-agent="+strings.TrimSpace(req.UserAgent))
	}
	args = append(args, req.URL)

	started := time.Now()
	cmd := exec.CommandContext(renderCtx, browserPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Page{}, fmt.Errorf("open renderer stdout: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Page{}, fmt.Errorf("start renderer %s: %w", filepath.Base(browserPath), err)
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, maxBytes+1))
	waitErr := cmd.Wait()
	if readErr != nil {
		return Page{}, fmt.Errorf("read rendered dom: %w", readErr)
	}
	if waitErr != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > 500 {
			message = message[:500]
		}
		if message == "" {
			message = waitErr.Error()
		}
		return Page{}, fmt.Errorf("renderer failed: %s", message)
	}
	partial := int64(len(data)) > maxBytes
	if partial {
		data = data[:maxBytes]
	}
	html := strings.TrimSpace(string(data))
	if html == "" {
		return Page{}, errors.New("renderer returned empty DOM")
	}
	return Page{
		URL:       req.URL,
		FinalURL:  req.URL,
		HTML:      html,
		Browser:   browserPath,
		Duration:  time.Since(started),
		Partial:   partial,
		FetchedAt: time.Now().UTC(),
	}, nil
}
