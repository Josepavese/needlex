package rendering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/josepavese/needlex/internal/platform"
)

func (r chromiumRenderer) renderWithCDP(ctx context.Context, req Request) (page Page, err error) {
	browserPath, err := platform.FindBrowserPath(r.cfg.BrowserPath)
	if err != nil {
		return Page{}, err
	}
	started := time.Now()
	lease, err := r.browser.Acquire(ctx, browserPath)
	if err != nil {
		return Page{}, err
	}
	healthy := false
	defer func() { err = errors.Join(err, lease.Release(healthy)) }()
	conn, _, err := websocket.Dial(ctx, lease.Endpoint(), nil)
	if err != nil {
		return Page{}, fmt.Errorf("connect renderer cdp: %w", err)
	}
	conn.SetReadLimit(req.NetworkMaxBytes + cdpEnvelopeHeadroomBytes)
	defer conn.CloseNow() //nolint:errcheck
	collector := newNetworkCollector(req.URL, req.NetworkMaxBytes, req.NetworkResourceMaxBytes, req.NetworkMaxResources, req.NetworkMaxMessages)
	cdp := cdpClient{conn: conn, onEvent: collector.handleEvent}
	contextResult, err := cdp.command(ctx, "", "Target.createBrowserContext", map[string]any{"disposeOnDetach": true})
	if err != nil {
		return Page{}, err
	}
	var isolated struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(contextResult, &isolated); err != nil || isolated.BrowserContextID == "" {
		return Page{}, errors.New("renderer did not create an isolated browser context")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), browserContextCleanupTimeout)
		defer cancel()
		if _, cleanupErr := cdp.command(cleanup, "", "Target.disposeBrowserContext", map[string]any{"browserContextId": isolated.BrowserContextID}); cleanupErr != nil {
			healthy = false
			err = errors.Join(err, cleanupErr)
		}
	}()
	createResult, err := cdp.command(ctx, "", "Target.createTarget", map[string]any{"url": "about:blank", "browserContextId": isolated.BrowserContextID})
	if err != nil {
		return Page{}, err
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(createResult, &created); err != nil || strings.TrimSpace(created.TargetID) == "" {
		return Page{}, fmt.Errorf("parse cdp target id: %w", err)
	}
	attachResult, err := cdp.command(ctx, "", "Target.attachToTarget", map[string]any{"targetId": created.TargetID, "flatten": true})
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

	_, _ = cdp.command(ctx, sessionID, "Network.enable", map[string]any{
		"maxTotalBufferSize":    req.NetworkMaxBytes,
		"maxResourceBufferSize": req.NetworkResourceMaxBytes,
	})
	_, _ = cdp.command(ctx, sessionID, "Page.enable", map[string]any{})
	if req.UserAgent != "" {
		if _, err := cdp.command(ctx, sessionID, "Network.setUserAgentOverride", map[string]any{"userAgent": req.UserAgent}); err != nil {
			return Page{}, err
		}
	}
	if _, err := cdp.command(ctx, sessionID, "Page.navigate", map[string]any{"url": req.URL}); err != nil {
		return Page{}, err
	}
	if err := waitForApplicationSettled(ctx, &cdp, sessionID, collector, req.NetworkIdle, renderSettleBudget(req.Timeout)); err != nil {
		collector.markSettleError(err)
	}
	snapshotCtx, cancelSnapshot := context.WithTimeout(ctx, renderSnapshotBudget(req.Timeout))
	defer cancelSnapshot()
	html, err := cdp.evaluateString(snapshotCtx, sessionID, `document.documentElement ? document.documentElement.outerHTML : ""`)
	if err != nil {
		return Page{}, err
	}
	finalURL, err := cdp.evaluateString(snapshotCtx, sessionID, `location.href`)
	if err != nil || strings.TrimSpace(finalURL) == "" {
		finalURL = req.URL
	}
	partial := int64(len([]byte(html))) > req.MaxBytes
	if partial {
		html = string([]byte(html)[:req.MaxBytes])
	}
	html = strings.TrimSpace(html)
	if html == "" {
		return Page{}, errors.New("renderer returned empty DOM")
	}
	networkResources, networkStats := collector.snapshot()
	healthy = true
	return Page{
		BrowserReused:    lease.Reused(),
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
