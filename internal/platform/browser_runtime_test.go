package platform

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeBrowser struct {
	alive  atomic.Bool
	closed chan struct{}
	once   atomic.Bool
}

func newFakeBrowser() *fakeBrowser {
	p := &fakeBrowser{closed: make(chan struct{})}
	p.alive.Store(true)
	return p
}
func (p *fakeBrowser) Endpoint() string { return "ws://fixture" }
func (p *fakeBrowser) Alive() bool      { return p.alive.Load() }
func (p *fakeBrowser) Close() error {
	if p.once.CompareAndSwap(false, true) {
		p.alive.Store(false)
		close(p.closed)
	}
	return nil
}

func fakeRuntime(t *testing.T, limits BrowserLimits) (*browserRuntime, *atomic.Int32) {
	t.Helper()
	host, err := NewBrowserRuntime(limits)
	if err != nil {
		t.Fatal(err)
	}
	r := host.(*browserRuntime)
	var starts atomic.Int32
	r.launch = func(context.Context, string) (BrowserProcess, error) { starts.Add(1); return newFakeBrowser(), nil }
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	return r, &starts
}

func acquireBrowser(t *testing.T, r BrowserRuntime, path string) BrowserLease {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	l, err := r.Acquire(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestBrowserRuntimeReusesAndRetiresAtUseLimit(t *testing.T) {
	r, starts := fakeRuntime(t, BrowserLimits{MaxConcurrency: 1, IdleTimeout: time.Minute, MaxUses: 2})
	a := acquireBrowser(t, r, "browser")
	if a.Reused() {
		t.Fatal("first lease reused")
	}
	_ = a.Release(true)
	b := acquireBrowser(t, r, "browser")
	if !b.Reused() {
		t.Fatal("second lease not reused")
	}
	_ = b.Release(true)
	if starts.Load() != 1 {
		t.Fatal("did not share a process")
	}
	c := acquireBrowser(t, r, "browser")
	if c.Reused() {
		t.Fatal("use budget did not recycle")
	}
	_ = c.Release(true)
	d := acquireBrowser(t, r, "different-browser")
	if d.Reused() {
		t.Fatal("path change reused wrong executable")
	}
	_ = d.Release(true)
	if starts.Load() != 3 {
		t.Fatalf("starts=%d", starts.Load())
	}
}

func TestBrowserRuntimeBoundsConcurrencyAndWaitersCancel(t *testing.T) {
	r, starts := fakeRuntime(t, BrowserLimits{MaxConcurrency: 2, IdleTimeout: time.Minute, MaxUses: 20})
	a, b := acquireBrowser(t, r, "browser"), acquireBrowser(t, r, "browser")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := r.Acquire(ctx, "browser"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter: %v", err)
	}
	if starts.Load() != 1 {
		t.Fatal("concurrency spawned another process")
	}
	_ = a.Release(false)
	if !b.(*browserLease).process.Alive() {
		t.Fatal("unhealthy lease killed another active borrower")
	}
	_ = b.Release(true)
	c := acquireBrowser(t, r, "browser")
	if c.Reused() {
		t.Fatal("unhealthy process reused")
	}
	_ = c.Release(true)
}

func TestBrowserRuntimeIdleCleanupAndCrashRecovery(t *testing.T) {
	r, starts := fakeRuntime(t, BrowserLimits{MaxConcurrency: 1, IdleTimeout: 20 * time.Millisecond, MaxUses: 20})
	a := acquireBrowser(t, r, "browser")
	p := a.(*browserLease).process.(*fakeBrowser)
	_ = a.Release(true)
	select {
	case <-p.closed:
	case <-time.After(time.Second):
		t.Fatal("idle process not retired")
	}
	b := acquireBrowser(t, r, "browser")
	crashed := b.(*browserLease).process.(*fakeBrowser)
	_ = b.Release(true)
	crashed.alive.Store(false)
	c := acquireBrowser(t, r, "browser")
	if c.Reused() {
		t.Fatal("crashed process reused")
	}
	_ = c.Release(true)
	if starts.Load() != 3 {
		t.Fatal("idle/crash did not restart")
	}
}

func TestBrowserRuntimeCloseCancelsStartupAndRejectsWork(t *testing.T) {
	r, _ := fakeRuntime(t, BrowserLimits{MaxConcurrency: 1, IdleTimeout: time.Minute, MaxUses: 2})
	started := make(chan struct{})
	r.launch = func(ctx context.Context, _ string) (BrowserProcess, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	result := make(chan error, 1)
	go func() { _, err := r.Acquire(context.Background(), "browser"); result <- err }()
	<-started
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("startup=%v", err)
	}
	if _, err := r.Acquire(context.Background(), "browser"); !errors.Is(err, ErrBrowserRuntimeClosed) {
		t.Fatalf("closed acquire=%v", err)
	}
	if _, err := r.CaptureDOM(context.Background(), BrowserDOMRequest{}); !errors.Is(err, ErrBrowserRuntimeClosed) {
		t.Fatalf("closed fallback=%v", err)
	}
}

func TestBrowserRuntimeCloseTerminatesActiveLease(t *testing.T) {
	r, _ := fakeRuntime(t, BrowserLimits{MaxConcurrency: 1, IdleTimeout: time.Minute, MaxUses: 2})
	a := acquireBrowser(t, r, "browser")
	p := a.(*browserLease).process
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if p.Alive() {
		t.Fatal("close left process alive")
	}
	if err := a.Release(true); err != nil {
		t.Fatal(err)
	}
	if err := a.Release(true); err != nil {
		t.Fatal(err)
	}
}
