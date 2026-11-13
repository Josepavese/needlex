package platform

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func proxyTestEgress(t *testing.T, resolver Resolver, dial EgressDialFunc) (*Egress, *egressProxy) {
	t.Helper()
	egress := publicEgress(t, resolver, dial)
	proxy, err := startEgressProxy(egress)
	if err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	return egress, proxy
}

func proxyRequest(t *testing.T, proxy *egressProxy, raw string) (int, string) {
	t.Helper()
	conn, err := net.Dial("tcp", proxy.listener.Addr().String())
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close() //nolint:errcheck
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, raw); err != nil {
		t.Fatalf("write proxy request: %v", err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read proxy response: %v", err)
	}
	defer response.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(body)
}

func TestEgressProxyDeniesConnectToNonPublicDestination(t *testing.T) {
	var dials atomic.Int32
	_, proxy := proxyTestEgress(t,
		&staticResolver{answers: map[string][]string{"private.test": {"10.0.0.7"}}},
		func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return nil, io.EOF
		},
	)
	for _, raw := range []string{
		"CONNECT private.test:443 HTTP/1.1\r\nHost: private.test:443\r\n\r\n",
		"CONNECT 127.0.0.1:443 HTTP/1.1\r\nHost: 127.0.0.1:443\r\n\r\n",
	} {
		status, body := proxyRequest(t, proxy, raw)
		if status != http.StatusForbidden || !strings.Contains(body, "egress denied by network policy") || !strings.Contains(body, EgressCodeAddress) {
			t.Fatalf("CONNECT denial status=%d body=%q", status, body)
		}
	}
	if dials.Load() != 0 {
		t.Fatalf("denied CONNECT dialed %d times", dials.Load())
	}
}

func TestEgressProxyAllowsValidatedForwarding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/asset" {
			t.Errorf("unexpected path %q", request.URL.Path)
		}
		_, _ = io.WriteString(w, "forwarded")
	}))
	defer server.Close()

	var seen []string
	var mu sync.Mutex
	_, proxy := proxyTestEgress(t,
		&staticResolver{answers: map[string][]string{"public.test": {"93.184.216.34"}}},
		injectingDial(server.Listener.Addr().String(), &seen, &mu),
	)
	status, body := proxyRequest(t, proxy, "GET http://public.test/asset HTTP/1.1\r\nHost: public.test\r\nConnection: close\r\n\r\n")
	if status != http.StatusOK || body != "forwarded" {
		t.Fatalf("forward status=%d body=%q", status, body)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "93.184.216.34:80" {
		t.Fatalf("forwarded dial addresses = %v", seen)
	}
}

func TestEgressProxyDeniesPlainHTTPPrivateDestination(t *testing.T) {
	_, proxy := proxyTestEgress(t,
		&staticResolver{},
		func(context.Context, string, string) (net.Conn, error) {
			return nil, io.EOF
		},
	)
	status, body := proxyRequest(t, proxy, "GET http://127.0.0.1:8080/secret HTTP/1.1\r\nHost: 127.0.0.1:8080\r\n\r\n")
	if status != http.StatusForbidden || !strings.Contains(body, EgressCodeAddress) {
		t.Fatalf("private plain HTTP status=%d body=%q", status, body)
	}
}

func TestEgressProxyTunnelsConnectToValidatedDestination(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close() //nolint:errcheck
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_, _ = io.Copy(conn, conn)
		_ = conn.Close()
	}()

	var seen []string
	var mu sync.Mutex
	_, proxy := proxyTestEgress(t,
		&staticResolver{answers: map[string][]string{"public.test": {"93.184.216.34"}}},
		injectingDial(listener.Addr().String(), &seen, &mu),
	)
	conn, err := net.Dial("tcp", proxy.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close() //nolint:errcheck
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, "CONNECT public.test:443 HTTP/1.1\r\nHost: public.test:443\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status=%v err=%v", response, err)
	}
	if _, err := io.WriteString(conn, "ping"); err != nil {
		t.Fatal(err)
	}
	echo := make([]byte, 4)
	if _, err := io.ReadFull(reader, echo); err != nil || string(echo) != "ping" {
		t.Fatalf("tunnel echo = %q, %v", echo, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "93.184.216.34:443" {
		t.Fatalf("CONNECT dial addresses = %v", seen)
	}
}

func TestEgressProxyCloseReleasesOpenTunnels(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close() //nolint:errcheck
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, conn)
		_ = conn.Close()
	}()
	egress := publicEgress(t,
		&staticResolver{answers: map[string][]string{"public.test": {"93.184.216.34"}}},
		func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, listener.Addr().String())
		},
	)
	proxy, err := startEgressProxy(egress)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", proxy.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(conn, "CONNECT public.test:443 HTTP/1.1\r\nHost: public.test:443\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT status=%v err=%v", response, err)
	}
	closed := make(chan error, 1)
	go func() { closed <- proxy.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("proxy Close blocked on an open tunnel")
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("open tunnel survived proxy Close")
	}
	_ = conn.Close()
}

func TestEgressProxyTunnelsUpgradedPlainHTTP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close() //nolint:errcheck
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close() //nolint:errcheck
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_, _ = io.Copy(conn, conn)
	}()

	var seen []string
	var mu sync.Mutex
	_, proxy := proxyTestEgress(t,
		&staticResolver{answers: map[string][]string{"public.test": {"93.184.216.34"}}},
		injectingDial(listener.Addr().String(), &seen, &mu),
	)
	conn, err := net.Dial("tcp", proxy.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close() //nolint:errcheck
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	request := "GET http://public.test/socket HTTP/1.1\r\nHost: public.test\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n"
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status=%v err=%v", response, err)
	}
	if _, err := io.WriteString(conn, "ping"); err != nil {
		t.Fatal(err)
	}
	echo := make([]byte, 4)
	if _, err := io.ReadFull(reader, echo); err != nil || string(echo) != "ping" {
		t.Fatalf("upgraded echo = %q, %v", echo, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "93.184.216.34:80" {
		t.Fatalf("upgrade dial addresses = %v", seen)
	}
}

func TestBrowserRuntimePublicOwnsFilteredProxy(t *testing.T) {
	host, err := NewBrowserRuntime(BrowserLimits{MaxConcurrency: 1, IdleTimeout: time.Minute, MaxUses: 2}, NetworkPolicyPublic)
	if err != nil {
		t.Fatal(err)
	}
	runtime := host.(*browserRuntime)
	defer runtime.Close() //nolint:errcheck
	if !strings.HasPrefix(runtime.proxyURL, "http://127.0.0.1:") {
		t.Fatalf("proxy url = %q", runtime.proxyURL)
	}
	var launchProxy atomic.Value
	runtime.launch = func(_ context.Context, _, proxyURL string) (BrowserProcess, error) {
		launchProxy.Store(proxyURL)
		return newFakeBrowser(), nil
	}
	lease, err := runtime.Acquire(context.Background(), "browser")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := launchProxy.Load().(string); got != runtime.proxyURL {
		t.Fatalf("launch proxy = %q, want %q", got, runtime.proxyURL)
	}
	_ = lease.Release(true)
}

func TestBrowserRuntimeAnyPolicyHasNoProxy(t *testing.T) {
	host, err := NewBrowserRuntime(BrowserLimits{MaxConcurrency: 1, IdleTimeout: time.Minute, MaxUses: 2}, NetworkPolicyAny)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close() //nolint:errcheck
	if runtime := host.(*browserRuntime); runtime.proxyURL != "" || runtime.proxy != nil {
		t.Fatalf("policy any started a proxy: %q", runtime.proxyURL)
	}
}

func TestBrowserProxyArgsRouteLoopbackThroughFilter(t *testing.T) {
	args := browserArgs("/usr/bin/chrome-headless-shell", "/tmp/profile", "http://127.0.0.1:4321")
	joined := strings.Join(args, " ")
	for _, want := range []string{"--proxy-server=http://127.0.0.1:4321", "--proxy-bypass-list=<-loopback>", "--disable-quic", "--force-webrtc-ip-handling-policy=disable_non_proxied_udp"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("browser args %q missing %q", joined, want)
		}
	}
	if len(browserProxyArgs("")) != 0 {
		t.Fatal("policy any added proxy arguments")
	}
	dom := BrowserDOMRequest{Path: "/usr/bin/chrome-headless-shell", URL: "https://example.com", ProxyURL: "http://127.0.0.1:4321"}
	if !strings.Contains(strings.Join(browserArgs(dom.Path, "/tmp/profile", dom.ProxyURL), " "), "--proxy-server=http://127.0.0.1:4321") {
		t.Fatal("dump-dom transport does not route through the policy proxy")
	}
}
