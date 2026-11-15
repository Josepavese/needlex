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
	// Session injects caller-provided cookies and origin-scoped request headers
	// into the isolated browser context. Headers are added by CDP Fetch
	// interception only for requests whose origin matches a session rule.
	Session *platform.Session
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
	// egress validates the render URL before the browser is launched; autonomous
	// browser requests are filtered by the runtime's loopback proxy.
	egress *platform.Egress
}

func (r chromiumRenderer) Render(ctx context.Context, req Request) (page Page, err error) {
	req = r.resolveRequest(req)
	if err := r.egress.ValidateRenderURL(ctx, req.URL); err != nil {
		return Page{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	if strings.TrimSpace(req.URL) == "" {
		return Page{}, errors.New("render url must not be empty")
	}
	if sessionNeedsDedicatedBrowser(req.Session) {
		return r.renderWithSessionBrowser(ctx, req)
	}
	stop := context.AfterFunc(r.browser.Context(), cancel)
	defer stop()
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
	if req.Session != nil {
		// The degraded dump-DOM path cannot apply session headers or cookies;
		// returning an unauthenticated DOM would silently change identity.
		return Page{}, cdpErr
	}
	fallback, err := r.renderWithDumpDOM(ctx, req)
	if err != nil {
		return Page{}, err
	}
	fallback.Degraded = true
	fallback.DegradeReason = boundedRenderReason("cdp_unavailable: " + cdpErr.Error())
	return fallback, nil
}

func sessionNeedsDedicatedBrowser(session *platform.Session) bool {
	if session == nil {
		return false
	}
	return session.Proxy() != nil || session.TLSConfig() != nil
}

func (r chromiumRenderer) renderWithSessionBrowser(ctx context.Context, req Request) (Page, error) {
	proxyURL := ""
	if proxy := req.Session.Proxy(); proxy != nil {
		if r.egress.Policy() == platform.NetworkPolicyPublic {
			return Page{}, errors.New("session browser proxy requires network.policy=any")
		}
		if proxy.User != nil {
			return Page{}, errors.New("browser proxy URL credentials are not supported; use HTTP acquisition or an unauthenticated proxy")
		}
		proxyURL = proxy.String()
	}
	insecure := false
	if tlsConfig := req.Session.TLSConfig(); tlsConfig != nil {
		if !tlsConfig.InsecureSkipVerify {
			return Page{}, errors.New("browser rendering supports only the explicit insecure TLS opt-in; custom CA and client certificates apply to HTTP acquisition")
		}
		insecure = true
	}
	defaults := config.DefaultRenderConfig()
	host, err := platform.NewBrowserRuntimeWithOptions(platform.BrowserLimits{
		MaxConcurrency: firstPositiveInt(r.cfg.MaxConcurrency, defaults.MaxConcurrency),
		IdleTimeout:    firstPositiveDuration(time.Duration(r.cfg.BrowserIdleTimeoutMS)*time.Millisecond, time.Duration(defaults.BrowserIdleTimeoutMS)*time.Millisecond),
		MaxUses:        firstPositiveInt(r.cfg.BrowserMaxUses, defaults.BrowserMaxUses),
	}, r.egress.Policy(), platform.BrowserRuntimeOptions{ProxyURL: proxyURL, InsecureTLS: insecure})
	if err != nil {
		return Page{}, err
	}
	defer host.Close() //nolint:errcheck
	renderer := chromiumRenderer{browser: host, cfg: r.cfg, egress: r.egress}
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()
	stop := context.AfterFunc(host.Context(), cancel)
	defer stop()
	page, err := renderer.renderWithCDP(ctx, req)
	if err == nil {
		return page, nil
	}
	if host.Context().Err() != nil || errors.Is(err, platform.ErrBrowserRuntimeClosed) {
		return Page{}, platform.ErrBrowserRuntimeClosed
	}
	if ctx.Err() != nil {
		return Page{}, ctx.Err()
	}
	return Page{}, err
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
