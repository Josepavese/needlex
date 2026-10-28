package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	browserDiagnosticBytes = 500
	browserStartupPoll     = 100 * time.Millisecond
	browserShutdownGrace   = time.Second
	devToolsActivePortFile = "DevToolsActivePort"
	cdpLoopbackHost        = "127.0.0.1"
)

type browserOutput struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *browserOutput) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, data[:min(len(data), max(0, b.limit-len(b.data)))]...)
	return len(data), nil
}

func (b *browserOutput) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...)
}

type browserProcess struct {
	cmd        *exec.Cmd
	endpoint   string
	done       chan struct{}
	cleanupErr error
	waitErr    error
	once       sync.Once
}

func (p *browserProcess) Endpoint() string { return p.endpoint }
func (p *browserProcess) Alive() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (p *browserProcess) Close() error {
	p.once.Do(func() { _ = killBrowserProcess(p.cmd) })
	<-p.done
	return p.cleanupErr
}

func browserArgs(path, profile string) []string {
	args := []string{}
	if !IsHeadlessShell(path) {
		args = append(args, "--headless=new")
	}
	return append(args, "--no-sandbox", "--disable-gpu", "--no-first-run",
		"--no-default-browser-check", "--disable-dev-shm-usage",
		"--disable-background-networking", "--disable-sync", "--hide-scrollbars",
		"--mute-audio", "--user-data-dir="+profile)
}

func launchBrowser(ctx context.Context, path string) (BrowserProcess, error) {
	path, err := FindBrowserPath(path)
	if err != nil {
		return nil, err
	}
	profile, err := os.MkdirTemp("", "needlex-render-cdp-*")
	if err != nil {
		return nil, fmt.Errorf("create render profile: %w", err)
	}
	args := append(browserArgs(path, profile), "--remote-debugging-port=0",
		"--remote-debugging-address="+cdpLoopbackHost, "about:blank")
	cmd := exec.Command(path, args...)
	configureBrowserProcess(cmd)
	cmd.WaitDelay = browserShutdownGrace
	stderr := &browserOutput{limit: browserDiagnosticBytes}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(profile)
		return nil, fmt.Errorf("start renderer %s: %w", filepath.Base(path), err)
	}
	p := &browserProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		p.waitErr = cmd.Wait()
		_ = killBrowserProcess(cmd)
		p.cleanupErr = os.RemoveAll(profile)
		close(p.done)
	}()
	tick := time.NewTicker(browserStartupPoll)
	defer tick.Stop()
	for {
		if data, err := os.ReadFile(filepath.Join(profile, devToolsActivePortFile)); err == nil {
			lines := strings.Fields(string(data))
			if len(lines) == 2 {
				port, parseErr := strconv.ParseUint(lines[0], 10, 16)
				if parseErr == nil && port != 0 && strings.HasPrefix(lines[1], "/devtools/browser/") {
					p.endpoint = "ws://" + cdpLoopbackHost + ":" + lines[0] + lines[1]
					return p, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, errors.Join(ctx.Err(), p.Close())
		case <-p.done:
			cause := p.waitErr
			if cause == nil {
				cause = errors.New("browser exited without a CDP endpoint")
			}
			return nil, fmt.Errorf("renderer stopped before CDP was ready: %s: %w", strings.TrimSpace(string(stderr.bytes())), cause)
		case <-tick.C:
		}
	}
}

// BrowserDOMRequest is the degraded dump-DOM transport, not a second CDP path.
type BrowserDOMRequest struct {
	Path, URL, UserAgent string
	MaxBytes             int64
	VirtualTime          time.Duration
}

func captureBrowserDOM(ctx context.Context, req BrowserDOMRequest) (data []byte, err error) {
	profile, err := os.MkdirTemp("", "needlex-render-*")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(profile)) }()
	args := append(browserArgs(req.Path, profile),
		"--virtual-time-budget="+strconv.FormatInt(req.VirtualTime.Milliseconds(), 10), "--dump-dom")
	if req.UserAgent != "" {
		args = append(args, "--user-agent="+req.UserAgent)
	}
	cmd := exec.CommandContext(ctx, req.Path, append(args, req.URL)...)
	cmd.Cancel = func() error { return killBrowserProcess(cmd) }
	configureBrowserProcess(cmd)
	cmd.WaitDelay = browserShutdownGrace
	stdout := &browserOutput{limit: int(req.MaxBytes) + 1}
	stderr := &browserOutput{limit: browserDiagnosticBytes}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err = cmd.Run()
	if cmd.Process != nil {
		_ = killBrowserProcess(cmd)
	}
	if err != nil {
		return nil, fmt.Errorf("renderer failed: %s: %w", strings.TrimSpace(string(stderr.bytes())), err)
	}
	return stdout.bytes(), nil
}

func FindBrowserPath(configured string) (string, error) {
	if strings.TrimSpace(configured) != "" {
		if _, err := os.Stat(strings.TrimSpace(configured)); err != nil {
			return "", fmt.Errorf("configured browser path unavailable: %w", err)
		}
		return strings.TrimSpace(configured), nil
	}
	for _, name := range []string{"chrome-headless-shell", "chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome", "msedge"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("no Chrome/Chromium executable found for JS rendering")
}

func IsHeadlessShell(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.Contains(base, "chrome-headless-shell") || base == "headless_shell"
}
