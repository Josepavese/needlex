package pipeline

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

type pipelineResolver struct {
	answers map[string][]string
}

func (r pipelineResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	raw, ok := r.answers[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	answers := make([]net.IPAddr, 0, len(raw))
	for _, address := range raw {
		answers = append(answers, net.IPAddr{IP: net.ParseIP(address)})
	}
	return answers, nil
}

func pipelineEgress(t *testing.T, answers map[string][]string, dial platform.EgressDialFunc) *platform.Egress {
	t.Helper()
	egress := platform.NewEgress(platform.NetworkPolicyPublic, platform.EgressOptions{Resolver: pipelineResolver{answers: answers}, Dial: dial})
	if egress == nil {
		t.Fatal("public policy returned no egress")
	}
	return egress
}

func TestAcquirerPublicPolicyDeniesLoopbackLiteral(t *testing.T) {
	acquirer := Acquirer{Egress: platform.NewEgress(platform.NetworkPolicyPublic)}
	_, err := acquirer.Acquire(context.Background(), AcquireInput{
		URL: "http://127.0.0.1:9/private", Timeout: time.Second, MaxBytes: 1024, Profile: "standard",
	})
	var denied *platform.EgressDeniedError
	if !errors.As(err, &denied) || denied.Code != platform.EgressCodeAddress || !errors.Is(err, platform.ErrEgressDenied) {
		t.Fatalf("loopback acquire error = %v, want address denial", err)
	}
}

func TestAcquirerPublicPolicyAllowsValidatedDestination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "<html><body>public page</body></html>")
	}))
	defer server.Close()

	var seen []string
	var mu sync.Mutex
	egress := pipelineEgress(t, map[string][]string{"public.test": {"93.184.216.34"}}, func(ctx context.Context, network, address string) (net.Conn, error) {
		mu.Lock()
		seen = append(seen, address)
		mu.Unlock()
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, server.Listener.Addr().String())
	})
	page, err := (Acquirer{Egress: egress}).Acquire(context.Background(), AcquireInput{
		URL: "http://public.test/page", Timeout: 5 * time.Second, MaxBytes: 1 << 20, Profile: "standard",
	})
	if err != nil {
		t.Fatalf("public acquire failed: %v", err)
	}
	if page.FetchMode != "http" || page.HTML == "" {
		t.Fatalf("unexpected page: %+v", page)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "93.184.216.34:80" {
		t.Fatalf("dial addresses = %v", seen)
	}
}

func TestAcquirerReqPathDialsValidatedAddress(t *testing.T) {
	var seen []string
	var mu sync.Mutex
	egress := pipelineEgress(t, map[string][]string{"public.test": {"93.184.216.34"}}, func(_ context.Context, _, address string) (net.Conn, error) {
		mu.Lock()
		seen = append(seen, address)
		mu.Unlock()
		return nil, errors.New("deliberate dial stop")
	})
	_, err := (Acquirer{Egress: egress}).Acquire(context.Background(), AcquireInput{
		URL: "https://public.test/page", Timeout: 2 * time.Second, MaxBytes: 1 << 20, Profile: "browser_like",
	})
	if err == nil {
		t.Fatal("req path unexpectedly succeeded")
	}
	if errors.Is(err, platform.ErrEgressDenied) {
		t.Fatalf("public destination was denied: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 || seen[0] != "93.184.216.34:443" {
		t.Fatalf("req dial addresses = %v", seen)
	}
}

func TestDownloadDeniesAssetToPrivateDestination(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><script src="http://assets.test/app.js"></script></html>`)
	}))
	defer page.Close()

	egress := pipelineEgress(t, map[string][]string{
		"public.test": {"93.184.216.34"},
		"assets.test": {"10.9.9.9"},
	}, func(ctx context.Context, network, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, page.Listener.Addr().String())
	})
	parent := t.TempDir()
	_, err := Download(context.Background(), DownloadRequest{
		URL: "http://public.test/", Assets: true, AssetOrigins: []string{"http://assets.test"}, Egress: egress,
	}, func() (platform.DownloadDirectory, error) {
		return platform.NewDownloadDirectory(parent, "")
	})
	var denied *platform.EgressDeniedError
	if !errors.As(err, &denied) || denied.Address != "10.9.9.9" {
		t.Fatalf("asset download error = %v, want private address denial", err)
	}
	leftovers, readErr := os.ReadDir(parent)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(leftovers) != 0 {
		t.Fatalf("failed download left %d directories", len(leftovers))
	}
}

func TestDownloadPublicPolicyDeniesInitialPrivateURL(t *testing.T) {
	_, err := Download(context.Background(), DownloadRequest{
		URL: "http://127.0.0.1:9/file", Egress: platform.NewEgress(platform.NetworkPolicyPublic),
	}, func() (platform.DownloadDirectory, error) {
		t.Fatal("directory created for denied download")
		return nil, nil
	})
	if !errors.Is(err, platform.ErrEgressDenied) {
		t.Fatalf("initial private download error = %v", err)
	}
}

func TestAcquirerReqPathDeniesPrivateResolution(t *testing.T) {
	egress := pipelineEgress(t, map[string][]string{"private.test": {"10.0.0.1"}}, func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("denied destination was dialed")
		return nil, nil
	})
	_, err := (Acquirer{Egress: egress}).Acquire(context.Background(), AcquireInput{
		URL: "https://private.test/page", Timeout: time.Second, MaxBytes: 1024, Profile: "browser_like",
	})
	if !platform.IsEgressDenied(err) {
		t.Fatalf("req private acquire error = %v, want egress denial", err)
	}
}
