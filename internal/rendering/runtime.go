package rendering

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/platform"
)

// Runtime owns browser reuse. Borrowers may call Render concurrently; the owner
// must Close it on shutdown. Every render creates a fresh CDP browser context.
type Runtime interface {
	Renderer
	Close() error
}

type managedRenderer struct{ chromiumRenderer }

func (r *managedRenderer) Close() error { return r.browser.Close() }

type oneShotRenderer struct {
	cfg   config.RenderConfig
	slots chan struct{}
}

func (r oneShotRenderer) Render(ctx context.Context, req Request) (page Page, err error) {
	runtime, err := NewRuntime(r.cfg)
	if err != nil {
		return Page{}, err
	}
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	case <-ctx.Done():
		return Page{}, errors.Join(ctx.Err(), runtime.Close())
	}
	defer func() { err = errors.Join(err, runtime.Close()) }()
	return runtime.Render(ctx, req)
}

func New(cfg config.RenderConfig) Renderer {
	if !cfg.Enabled {
		return NoopRenderer{}
	}
	var slots chan struct{}
	if cfg.MaxConcurrency > 0 {
		slots = make(chan struct{}, cfg.MaxConcurrency)
	}
	return oneShotRenderer{cfg: cfg, slots: slots}
}

func NewRuntime(cfg config.RenderConfig) (Runtime, error) {
	if !cfg.Enabled {
		return disabledRuntime{}, nil
	}
	if provider := strings.TrimSpace(cfg.Provider); provider != "" && provider != "exec-dump-dom" {
		return nil, errors.New("unsupported render provider")
	}
	host, err := platform.NewBrowserRuntime(platform.BrowserLimits{
		MaxConcurrency: cfg.MaxConcurrency,
		IdleTimeout:    time.Duration(cfg.BrowserIdleTimeoutMS) * time.Millisecond,
		MaxUses:        cfg.BrowserMaxUses,
	})
	if err != nil {
		return nil, err
	}
	return &managedRenderer{chromiumRenderer{browser: host, cfg: cfg}}, nil
}

type disabledRuntime struct{ NoopRenderer }

func (disabledRuntime) Close() error { return nil }

func (r chromiumRenderer) resolveRequest(req Request) Request {
	defaults := config.DefaultRenderConfig()
	req.Timeout = firstPositiveDuration(req.Timeout, time.Duration(r.cfg.TimeoutMS)*time.Millisecond, time.Duration(defaults.TimeoutMS)*time.Millisecond)
	req.MaxBytes = firstPositiveInt64(req.MaxBytes, config.Defaults().Runtime.MaxBytes)
	req.NetworkIdle = firstPositiveDuration(req.NetworkIdle, time.Duration(r.cfg.NetworkIdleMS)*time.Millisecond, time.Duration(defaults.NetworkIdleMS)*time.Millisecond)
	req.NetworkMaxBytes = firstPositiveInt64(req.NetworkMaxBytes, r.cfg.NetworkMaxBytes, defaults.NetworkMaxBytes)
	req.NetworkResourceMaxBytes = firstPositiveInt64(req.NetworkResourceMaxBytes, r.cfg.NetworkResourceMaxBytes, req.NetworkMaxBytes)
	req.NetworkMaxResources = firstPositiveInt(req.NetworkMaxResources, r.cfg.NetworkMaxResources, defaults.NetworkMaxResources)
	req.NetworkMaxMessages = firstPositiveInt(req.NetworkMaxMessages, r.cfg.NetworkMaxMessages, defaults.NetworkMaxMessages)
	return req
}
