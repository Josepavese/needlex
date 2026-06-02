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
	URL       string
	UserAgent string
	Timeout   time.Duration
	MaxBytes  int64
}

type Page struct {
	URL       string
	FinalURL  string
	HTML      string
	Browser   string
	Duration  time.Duration
	Partial   bool
	FetchedAt time.Time
}

type Renderer interface {
	Render(ctx context.Context, req Request) (Page, error)
}

type NoopRenderer struct{}

func (NoopRenderer) Render(context.Context, Request) (Page, error) {
	return Page{}, errors.New("js rendering is disabled")
}

type ExecDumpDOMRenderer struct {
	BrowserPath string
	Timeout     time.Duration
	MaxBytes    int64
}

func New(cfg config.RenderConfig) Renderer {
	if !cfg.Enabled {
		return NoopRenderer{}
	}
	switch strings.TrimSpace(cfg.Provider) {
	case "", "exec-dump-dom":
		return ExecDumpDOMRenderer{
			BrowserPath: strings.TrimSpace(cfg.BrowserPath),
			Timeout:     time.Duration(cfg.TimeoutMS) * time.Millisecond,
		}
	default:
		return NoopRenderer{}
	}
}

func (r ExecDumpDOMRenderer) Render(ctx context.Context, req Request) (Page, error) {
	if strings.TrimSpace(req.URL) == "" {
		return Page{}, errors.New("render url must not be empty")
	}
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

func FindBrowserPath(configured string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		if _, err := os.Stat(strings.TrimSpace(configured)); err != nil {
			return "", fmt.Errorf("configured browser path unavailable: %w", err)
		}
		return strings.TrimSpace(configured), nil
	}
	for _, name := range []string{
		"chrome-headless-shell",
		"chromium",
		"chromium-browser",
		"google-chrome",
		"google-chrome-stable",
		"chrome",
		"msedge",
	} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("no Chrome/Chromium executable found for JS rendering")
}

func isHeadlessShell(browserPath string) bool {
	base := strings.ToLower(filepath.Base(browserPath))
	return strings.Contains(base, "chrome-headless-shell") || base == "headless_shell"
}

func firstPositiveDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
