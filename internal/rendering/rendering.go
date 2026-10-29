package rendering

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/platform"
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
	BrowserReused    bool
	URL              string
	FinalURL         string
	HTML             string
	Browser          string
	Duration         time.Duration
	Partial          bool
	Degraded         bool
	DegradeReason    string
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
	// HTTPRequests counts CDP request events through the snapshot, including
	// redirects/cache events. It is independent of application-body filtering.
	HTTPRequests        int
	ResourceCount       int
	ObservedResources   int
	BodyUnavailable     int
	BodyUnavailableURLs []string
	StreamsOpen         int
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

type chromiumRenderer struct {
	browser platform.BrowserRuntime
	cfg     config.RenderConfig
}

func (r chromiumRenderer) Render(ctx context.Context, req Request) (page Page, err error) {
	req = r.resolveRequest(req)
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	stop := context.AfterFunc(r.browser.Context(), cancel)
	defer stop()
	if strings.TrimSpace(req.URL) == "" {
		return Page{}, errors.New("render url must not be empty")
	}
	page, cdpErr := r.renderWithCDP(ctx, req)
	if cdpErr == nil {
		return page, nil
	}
	if r.browser.Context().Err() != nil || errors.Is(cdpErr, platform.ErrBrowserRuntimeClosed) {
		return Page{}, platform.ErrBrowserRuntimeClosed
	}
	if ctx.Err() != nil {
		return Page{}, ctx.Err()
	}
	fallback, err := r.renderWithDumpDOM(ctx, req)
	if err != nil {
		return Page{}, err
	}
	fallback.Degraded = true
	fallback.DegradeReason = boundedRenderReason("cdp_unavailable: " + cdpErr.Error())
	return fallback, nil
}

func boundedRenderReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if len(reason) > 240 {
		return reason[:240]
	}
	return reason
}

func (r chromiumRenderer) renderWithDumpDOM(ctx context.Context, req Request) (Page, error) {
	browserPath, err := platform.FindBrowserPath(r.cfg.BrowserPath)
	if err != nil {
		return Page{}, err
	}
	started := time.Now()
	data, err := r.browser.CaptureDOM(ctx, platform.BrowserDOMRequest{
		Path: browserPath, URL: req.URL, UserAgent: strings.TrimSpace(req.UserAgent),
		MaxBytes: req.MaxBytes, VirtualTime: dumpDOMVirtualTime,
	})
	if err != nil {
		return Page{}, err
	}
	partial := int64(len(data)) > req.MaxBytes
	if partial {
		data = data[:req.MaxBytes]
	}
	html := strings.TrimSpace(string(data))
	if html == "" {
		return Page{}, errors.New("renderer returned empty DOM")
	}
	return Page{
		URL: req.URL, FinalURL: req.URL, HTML: html, Browser: browserPath,
		Duration: time.Since(started), Partial: partial, FetchedAt: time.Now().UTC(),
	}, nil
}
