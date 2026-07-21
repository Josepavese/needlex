package rendering

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
)

func (r ExecDumpDOMRenderer) renderWithCDP(ctx context.Context, req Request) (Page, error) {
	browserPath, err := FindBrowserPath(r.BrowserPath)
	if err != nil {
		return Page{}, err
	}
	timeout := firstPositiveDuration(req.Timeout, r.Timeout, 5*time.Second)
	networkIdle := firstPositiveDuration(req.NetworkIdle, r.NetworkIdle, 1500*time.Millisecond)
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = r.MaxBytes
	}
	if maxBytes <= 0 {
		maxBytes = 4_000_000
	}
	networkMaxBytes := firstPositiveInt64(req.NetworkMaxBytes, r.NetworkMaxBytes, 64_000_000)
	networkResourceMaxBytes := firstPositiveInt64(req.NetworkResourceMaxBytes, r.NetworkResourceMaxBytes, networkMaxBytes)
	networkMaxResources := firstPositiveInt(req.NetworkMaxResources, r.NetworkMaxResources, 32)
	networkMaxMessages := firstPositiveInt(req.NetworkMaxMessages, r.NetworkMaxMessages, 4096)
	renderCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	profileDir, err := os.MkdirTemp("", "needlex-render-cdp-*")
	if err != nil {
		return Page{}, fmt.Errorf("create render profile: %w", err)
	}
	defer os.RemoveAll(profileDir) //nolint:errcheck

	port, err := freeLocalPort()
	if err != nil {
		return Page{}, err
	}
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
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		"about:blank",
	)
	if strings.TrimSpace(req.UserAgent) != "" {
		args = append(args, "--user-agent="+strings.TrimSpace(req.UserAgent))
	}

	started := time.Now()
	cmd := exec.CommandContext(renderCtx, browserPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return Page{}, fmt.Errorf("start renderer %s: %w", filepath.Base(browserPath), err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	wsURL, err := waitForCDPWebSocketURL(renderCtx, port)
	if err != nil {
		return Page{}, err
	}
	conn, _, err := websocket.Dial(renderCtx, wsURL, nil)
	if err != nil {
		return Page{}, fmt.Errorf("connect renderer cdp: %w", err)
	}
	conn.SetReadLimit(networkMaxBytes + 8_000_000)
	defer conn.Close(websocket.StatusNormalClosure, "done") //nolint:errcheck

	collector := newNetworkCollector(req.URL, networkMaxBytes, networkResourceMaxBytes, networkMaxResources, networkMaxMessages)
	cdp := cdpClient{conn: conn, onEvent: collector.handleEvent}
	createResult, err := cdp.command(renderCtx, "", "Target.createTarget", map[string]any{"url": "about:blank"})
	if err != nil {
		return Page{}, err
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(createResult, &created); err != nil || strings.TrimSpace(created.TargetID) == "" {
		return Page{}, fmt.Errorf("parse cdp target id: %w", err)
	}
	attachResult, err := cdp.command(renderCtx, "", "Target.attachToTarget", map[string]any{"targetId": created.TargetID, "flatten": true})
	if err != nil {
		return Page{}, err
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attachResult, &attached); err != nil || strings.TrimSpace(attached.SessionID) == "" {
		return Page{}, fmt.Errorf("parse cdp session id: %w", err)
	}
	sessionID := attached.SessionID
	defer cdp.command(context.Background(), "", "Target.closeTarget", map[string]any{"targetId": created.TargetID}) //nolint:errcheck

	_, _ = cdp.command(renderCtx, sessionID, "Network.enable", map[string]any{
		"maxTotalBufferSize":    networkMaxBytes,
		"maxResourceBufferSize": networkResourceMaxBytes,
	})
	_, _ = cdp.command(renderCtx, sessionID, "Page.enable", map[string]any{})
	if _, err := cdp.command(renderCtx, sessionID, "Page.navigate", map[string]any{"url": req.URL}); err != nil {
		return Page{}, err
	}
	if err := waitForApplicationSettled(renderCtx, &cdp, sessionID, collector, networkIdle, renderSettleBudget(timeout)); err != nil {
		collector.markSettleError(err)
	}
	snapshotCtx, cancelSnapshot := context.WithTimeout(renderCtx, renderSnapshotBudget(timeout))
	defer cancelSnapshot()
	html, err := cdp.evaluateString(snapshotCtx, sessionID, `document.documentElement ? document.documentElement.outerHTML : ""`)
	if err != nil {
		return Page{}, err
	}
	finalURL, err := cdp.evaluateString(snapshotCtx, sessionID, `location.href`)
	if err != nil || strings.TrimSpace(finalURL) == "" {
		finalURL = req.URL
	}
	partial := int64(len([]byte(html))) > maxBytes
	if partial {
		html = string([]byte(html)[:maxBytes])
	}
	html = strings.TrimSpace(html)
	if html == "" {
		return Page{}, errors.New("renderer returned empty DOM")
	}
	networkResources, networkStats := collector.snapshot()
	return Page{
		URL:              req.URL,
		FinalURL:         strings.TrimSpace(finalURL),
		HTML:             html,
		Browser:          browserPath + " cdp",
		Duration:         time.Since(started),
		Partial:          partial,
		FetchedAt:        time.Now().UTC(),
		NetworkResources: networkResources,
		NetworkStats:     networkStats,
	}, nil
}
