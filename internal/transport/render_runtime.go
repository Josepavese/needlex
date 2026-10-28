package transport

import (
	"sync"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/rendering"
)

// The MCP dispatch loop borrows one render runtime. Configuration replacement
// happens between operations; shutdown may close it concurrently.
type renderRuntime struct {
	mu      sync.Mutex
	config  config.RenderConfig
	runtime rendering.Runtime
	new     func(config.RenderConfig) (rendering.Runtime, error)
	closed  bool
}

func (r *renderRuntime) get(cfg config.RenderConfig) (rendering.Renderer, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, platform.ErrBrowserRuntimeClosed
	}
	if r.runtime != nil && r.config == cfg {
		out := r.runtime
		r.mu.Unlock()
		return out, nil
	}
	next, err := r.new(cfg)
	if err != nil {
		r.mu.Unlock()
		return nil, err
	}
	previous := r.runtime
	r.runtime, r.config = next, cfg
	r.mu.Unlock()
	if previous != nil {
		if err := previous.Close(); err != nil {
			return nil, err
		}
	}
	return next, nil
}

func (r *renderRuntime) Close() error {
	r.mu.Lock()
	r.closed = true
	previous := r.runtime
	r.runtime = nil
	r.mu.Unlock()
	if previous != nil {
		return previous.Close()
	}
	return nil
}
