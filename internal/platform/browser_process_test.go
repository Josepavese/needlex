package platform

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserEndpointAnnouncementSurvivesChunksAndDiagnosticLimit(t *testing.T) {
	const endpoint = "ws://127.0.0.1:9223/devtools/browser/test-session"
	out := &browserOutput{limit: browserDiagnosticBytes, watchEndpoint: true}
	for _, part := range []string{strings.Repeat("diagnostic noise\n", 100), "DevTools lis", "tening on " + endpoint + "\n"} {
		_, _ = out.Write([]byte(part))
	}
	if out.browserEndpoint() != endpoint || len(out.bytes()) > browserDiagnosticBytes || len(out.pending) > browserDiagnosticBytes {
		t.Fatalf("endpoint=%q diagnostics=%d pending=%d", out.browserEndpoint(), len(out.bytes()), len(out.pending))
	}
	for _, invalid := range []string{"ws://example.com:9223/devtools/browser/session", "ws://127.0.0.1:0/devtools/browser/session", "http://127.0.0.1:9223/devtools/browser/session"} {
		out := &browserOutput{limit: browserDiagnosticBytes, watchEndpoint: true}
		_, _ = out.Write([]byte(devToolsAnnouncement + invalid + "\n"))
		if out.browserEndpoint() != "" {
			t.Fatalf("accepted invalid browser endpoint %q", invalid)
		}
	}
}

func TestNativeBrowserCrashRecoveryAndProfileCleanup(t *testing.T) {
	path, err := FindBrowserPath("")
	if err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	probe, probeErr := captureBrowserDOM(probeCtx, BrowserDOMRequest{
		Path: path, URL: "data:text/html,<main>Needle-X%20browser%20capability%20probe</main>",
		MaxBytes: 8192, VirtualTime: time.Second,
	})
	probeCancel()
	if probeErr != nil || !strings.Contains(string(probe), "Needle-X browser capability probe") {
		t.Skipf("browser installed but unavailable for native qualification: %s: %v", path, probeErr)
	}
	host, err := NewBrowserRuntime(BrowserLimits{MaxConcurrency: 1, IdleTimeout: time.Minute, MaxUses: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close() //nolint:errcheck
	ctx := t.Context()
	if deadline, ok := t.Deadline(); ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
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
