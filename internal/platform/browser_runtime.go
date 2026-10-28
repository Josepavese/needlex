package platform

import (
	"context"
	"errors"
	"sync"
	"time"
)

// BrowserProcess is an owned native browser. Close waits for process/profile cleanup.
type BrowserProcess interface {
	Endpoint() string
	Alive() bool
	Close() error
}

type BrowserLimits struct {
	MaxConcurrency int
	IdleTimeout    time.Duration
	MaxUses        int
}

// BrowserLease borrows a process, never its profile or another caller's context.
// Release is terminal and idempotent. An unhealthy process retires after its
// existing leases finish; no new lease can borrow it.
type BrowserLease interface {
	Endpoint() string
	Reused() bool
	Release(healthy bool) error
}

// BrowserRuntime is thread-safe. It owns at most one native browser and bounds
// active leases. Close cancels startup/waiters and terminates the owned process.
type BrowserRuntime interface {
	Acquire(context.Context, string) (BrowserLease, error)
	CaptureDOM(context.Context, BrowserDOMRequest) ([]byte, error)
	Context() context.Context
	Close() error
}

var ErrBrowserRuntimeClosed = errors.New("browser runtime is closed")

type browserRuntime struct {
	mu       sync.Mutex
	limits   BrowserLimits
	launch   func(context.Context, string) (BrowserProcess, error)
	ctx      context.Context
	cancel   context.CancelFunc
	changed  chan struct{}
	process  BrowserProcess
	path     string
	active   int
	uses     int
	busy     bool // process startup/retirement is outside the lock
	retiring bool
	closed   bool
	idle     *time.Timer
	err      error
}

func (r *browserRuntime) Context() context.Context { return r.ctx }

func NewBrowserRuntime(limits BrowserLimits) (BrowserRuntime, error) {
	if limits.MaxConcurrency <= 0 || limits.IdleTimeout <= 0 || limits.MaxUses <= 0 {
		return nil, errors.New("browser concurrency, idle timeout and maximum uses must be positive")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &browserRuntime{limits: limits, launch: launchBrowser, ctx: ctx, cancel: cancel, changed: make(chan struct{})}, nil
}

func (r *browserRuntime) notify() { close(r.changed); r.changed = make(chan struct{}) }

func (r *browserRuntime) Acquire(ctx context.Context, path string) (BrowserLease, error) {
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil, ErrBrowserRuntimeClosed
		}
		if r.err != nil {
			err := r.err
			r.mu.Unlock()
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			r.mu.Unlock()
			return nil, err
		}
		if !r.busy && r.process != nil && (!r.process.Alive() || r.path != path || r.uses >= r.limits.MaxUses) {
			r.retiring = true
		}
		if !r.busy && r.retiring && r.active == 0 {
			p := r.detach()
			r.mu.Unlock()
			if err := r.finishRetirement(p); err != nil {
				return nil, err
			}
			continue
		}
		if !r.busy && !r.retiring && r.active < r.limits.MaxConcurrency {
			if r.idle != nil {
				r.idle.Stop()
				r.idle = nil
			}
			if r.process == nil {
				r.busy = true
				r.mu.Unlock()
				startup, cancel := context.WithCancel(ctx)
				stop := context.AfterFunc(r.ctx, cancel)
				p, err := r.launch(startup, path)
				stop()
				cancel()
				r.mu.Lock()
				if err == nil && r.closed {
					r.mu.Unlock()
					_ = p.Close()
					r.mu.Lock()
					err = ErrBrowserRuntimeClosed
				}
				r.busy = false
				if err != nil {
					r.notify()
					r.mu.Unlock()
					return nil, err
				}
				r.process, r.path, r.uses = p, path, 0
				r.notify()
			}
			r.active++
			r.uses++
			lease := &browserLease{owner: r, process: r.process, reused: r.uses > 1}
			r.mu.Unlock()
			return lease, nil
		}
		changed := r.changed
		r.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// detach reserves the single process slot until native retirement completes.
func (r *browserRuntime) detach() BrowserProcess {
	if r.idle != nil {
		r.idle.Stop()
		r.idle = nil
	}
	p := r.process
	r.process = nil
	r.busy = true
	r.retiring = false
	return p
}

func (r *browserRuntime) finishRetirement(p BrowserProcess) error {
	var err error
	if p != nil {
		err = p.Close()
	}
	r.mu.Lock()
	r.err = errors.Join(r.err, err)
	r.busy = false
	r.notify()
	r.mu.Unlock()
	return err
}

func (r *browserRuntime) expire(p BrowserProcess) {
	r.mu.Lock()
	if r.process != p || r.active != 0 || r.busy {
		r.mu.Unlock()
		return
	}
	p = r.detach()
	r.mu.Unlock()
	_ = r.finishRetirement(p)
}

func (r *browserRuntime) Close() error {
	r.mu.Lock()
	r.closed = true
	r.cancel()
	r.notify()
	for r.busy {
		changed := r.changed
		r.mu.Unlock()
		<-changed
		r.mu.Lock()
	}
	p := r.detach()
	r.mu.Unlock()
	_ = r.finishRetirement(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// CaptureDOM reserves the same single native-process slot exclusively. The
// degraded fallback cannot bypass the reuse pool's concurrency or shutdown.
func (r *browserRuntime) CaptureDOM(ctx context.Context, req BrowserDOMRequest) ([]byte, error) {
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return nil, ErrBrowserRuntimeClosed
		}
		if err := ctx.Err(); err != nil {
			r.mu.Unlock()
			return nil, err
		}
		if !r.busy && r.active == 0 {
			p := r.detach()
			r.mu.Unlock()
			var err error
			if p != nil {
				err = p.Close()
			}
			operation, cancel := context.WithCancel(ctx)
			stop := context.AfterFunc(r.ctx, cancel)
			var data []byte
			if err == nil {
				data, err = captureBrowserDOM(operation, req)
			}
			stop()
			cancel()
			r.mu.Lock()
			r.busy = false
			r.notify()
			r.mu.Unlock()
			return data, err
		}
		changed := r.changed
		r.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

type browserLease struct {
	owner   *browserRuntime
	process BrowserProcess
	reused  bool
	once    sync.Once
	err     error
}

func (l *browserLease) Endpoint() string { return l.process.Endpoint() }
func (l *browserLease) Reused() bool     { return l.reused }
func (l *browserLease) Release(healthy bool) error {
	l.once.Do(func() {
		r := l.owner
		r.mu.Lock()
		r.active--
		if r.process != l.process {
			r.notify()
			r.mu.Unlock()
			return
		}
		r.retiring = r.retiring || !healthy || r.uses >= r.limits.MaxUses
		if r.active == 0 && r.retiring {
			p := r.detach()
			r.mu.Unlock()
			l.err = r.finishRetirement(p)
			return
		}
		if r.active == 0 {
			r.idle = time.AfterFunc(r.limits.IdleTimeout, func() { r.expire(l.process) })
		}
		r.notify()
		r.mu.Unlock()
	})
	return l.err
}
