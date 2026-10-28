package platform

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeBrowserCrashRecoveryAndProfileCleanup(t *testing.T) {
	path, err := FindBrowserPath("")
	if err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	host, err := NewBrowserRuntime(BrowserLimits{MaxConcurrency: 1, IdleTimeout: time.Minute, MaxUses: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close() //nolint:errcheck
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first, err := host.Acquire(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	process := first.(*browserLease).process.(*browserProcess)
	_ = first.Release(true)
	if err := process.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-process.done:
	case <-ctx.Done():
		t.Fatal("native process/children not reaped")
	}
	second, err := host.Acquire(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if second.Reused() {
		t.Fatal("crashed native process was reused")
	}
	_ = second.Release(true)
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	profiles, err := filepath.Glob(filepath.Join(root, "needlex-render-*"))
	if err != nil || len(profiles) != 0 {
		t.Fatalf("profiles=%v err=%v", profiles, err)
	}
}
