package platform

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type staticResolver struct {
	mu      sync.Mutex
	answers map[string][]string
	lookups atomic.Int32
}

func (r *staticResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	r.lookups.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
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

// injectingDial forwards every validated dial to a local test server while
// recording the address the policy chose, so no unit test needs live internet.
func injectingDial(target string, seen *[]string, mu *sync.Mutex) EgressDialFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		mu.Lock()
		*seen = append(*seen, address)
		mu.Unlock()
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, target)
	}
}

func publicEgress(t *testing.T, resolver Resolver, dial EgressDialFunc) *Egress {
	t.Helper()
	egress := NewEgress(NetworkPolicyPublic, EgressOptions{Resolver: resolver, Dial: dial})
	if egress == nil {
		t.Fatal("public policy returned no egress")
	}
	return egress
}

func TestParseNetworkPolicy(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		want  NetworkPolicy
		error bool
	}{
		{"", NetworkPolicyAny, false},
		{"any", NetworkPolicyAny, false},
		{" PuBLic ", NetworkPolicyPublic, false},
		{"internet", "", true},
	} {
		got, err := ParseNetworkPolicy(tc.raw)
		if (err != nil) != tc.error || got != tc.want {
			t.Fatalf("ParseNetworkPolicy(%q) = %q, %v", tc.raw, got, err)
		}
	}
}

func TestNonPublicAddressClassification(t *testing.T) {
	denied := []string{
		"0.0.0.0", "0.1.2.3", "127.0.0.1", "127.10.0.1",
		"10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.1",
		"169.254.1.1", "224.0.0.1", "239.255.255.250",
		"100.64.0.1", "100.127.255.254",
		"192.0.0.1", "192.0.2.1", "198.18.0.1", "198.19.255.255",
		"198.51.100.1", "203.0.113.1", "240.0.0.1", "255.255.255.255",
		"192.88.99.1",
		"::", "::1", "fe80::1", "fc00::1", "fd12:3456::1", "ff02::1",
		"2001:db8::1", "2001::1", "100::1", "2002::1", "64:ff9b::1",
		"fec0::1", "3fff::1",
		"::ffff:10.0.0.1", "::ffff:127.0.0.1", "::ffff:192.168.0.1", "::10.0.0.1",
	}
	for _, raw := range denied {
		address := net.ParseIP(raw)
		if address == nil {
			t.Fatalf("bad fixture %q", raw)
		}
		if reason := nonPublicReason(mustAddr(t, raw)); reason == "" {
			t.Errorf("%s was accepted as public", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111", "2001:4860:4860::8888"} {
		if reason := nonPublicReason(mustAddr(t, raw)); reason != "" {
			t.Errorf("%s was rejected as %q", raw, reason)
		}
	}
}

func mustAddr(t *testing.T, raw string) netip.Addr {
	t.Helper()
	parsed, ok := parseIPAddress(raw)
	if !ok {
		t.Fatalf("parse %q as an IP address", raw)
	}
	return parsed
}

func TestEgressValidatesURLsBeforeDialing(t *testing.T) {
	egress := NewEgress(NetworkPolicyPublic)
	denied := map[string]string{
		"http://user:secret@example.com/": EgressCodeUserinfo,
		"http://127.0.0.1:8080/":          EgressCodeAddress,
		"http://[::1]/":                   EgressCodeAddress,
		"http://[::ffff:10.0.0.1]/":       EgressCodeAddress,
		"ftp://example.com/file":          EgressCodeScheme,
	}
	for raw, code := range denied {
		err := egress.ValidateURLString(raw)
		var failure *EgressDeniedError
		if !errors.As(err, &failure) || failure.Code != code || !errors.Is(err, ErrEgressDenied) {
			t.Errorf("%s: denied error = %v, want code %s", raw, err, code)
		}
		if strings.Contains(strings.ToLower(err.Error()), "disable") || strings.Contains(strings.ToLower(err.Error()), "try ") {
			t.Errorf("%s: denial suggests an alternative path: %v", raw, err)
		}
	}
	if err := egress.ValidateURLString("https://example.com/path"); err != nil {
		t.Fatalf("public URL rejected: %v", err)
	}
}

func TestEgressAnyKeepsHistoricalTransports(t *testing.T) {
	if egress := NewEgress(NetworkPolicyAny); egress != nil {
		t.Fatal("policy any created an enforcement object")
	}
	if client := NewEgress(NetworkPolicyAny).HTTPClient(nil); client != nil {
		t.Fatal("policy any replaced the client")
	}
	if err := NewEgress(NetworkPolicyAny).ValidateURLString("http://127.0.0.1/"); err != nil {
		t.Fatalf("policy any denied URL: %v", err)
	}
}

func TestEgressAllowsPublicDestinationThroughValidatedDial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/page" {
			t.Errorf("unexpected path %q", request.URL.Path)
		}
		_, _ = io.WriteString(w, "public-ok")
	}))
	defer server.Close()

	var seen []string
	var mu sync.Mutex
	egress := publicEgress(t,
		&staticResolver{answers: map[string][]string{"example.test": {"93.184.216.34"}}},
		injectingDial(server.Listener.Addr().String(), &seen, &mu),
	)
	response, err := egress.HTTPClient(nil).Get("http://example.test/page")
	if err != nil {
		t.Fatalf("public request failed: %v", err)
	}
	defer response.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(response.Body)
	if string(body) != "public-ok" {
		t.Fatalf("body = %q", body)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "93.184.216.34:80" {
		t.Fatalf("dialed addresses = %v, want the validated address", seen)
	}
}

func TestEgressRejectsMixedDNSAnswers(t *testing.T) {
	var dials atomic.Int32
	egress := publicEgress(t,
		&staticResolver{answers: map[string][]string{
			"mixed.test": {"93.184.216.34", "10.0.0.5"},
			"safe.test":  {"2606:4700:4700::1111", "93.184.216.34"},
		}},
		func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return nil, errors.New("unexpected dial")
		},
	)
	client := egress.HTTPClient(nil)
	if _, err := client.Get("http://mixed.test/"); !errors.Is(err, ErrEgressDenied) {
		t.Fatalf("mixed answers error = %v, want address denial", err)
	}
	if dials.Load() != 0 {
		t.Fatalf("mixed answers were dialed %d times", dials.Load())
	}
	// Multiple answers are allowed only when every answer is public; the
	// injected dial then fails deliberately so no live network is touched.
	if _, err := client.Get("http://safe.test/"); errors.Is(err, ErrEgressDenied) {
		t.Fatalf("public answers were denied: %v", err)
	}
	if dials.Load() != 1 {
		t.Fatalf("public answers dial attempts = %d", dials.Load())
	}
}

// TestEgressBindsValidationToDial defeats rebinding: the first resolution is
// validated and dialed directly, so a later private answer can never be used.
func TestEgressBindsValidationToDial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "bound")
	}))
	defer server.Close()

	resolver := &rebindingResolver{host: "rebind.test", answers: [][]string{{"93.184.216.34"}, {"10.0.0.5"}}}
	var seen []string
	var mu sync.Mutex
	egress := publicEgress(t, resolver, injectingDial(server.Listener.Addr().String(), &seen, &mu))
	response, err := egress.HTTPClient(nil).Get("http://rebind.test/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = response.Body.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "93.184.216.34:80" {
		t.Fatalf("dialed addresses = %v", seen)
	}
	if resolver.lookups.Load() != 1 {
		t.Fatalf("resolver consulted %d times, want exactly one validation", resolver.lookups.Load())
	}
}

type rebindingResolver struct {
	mu      sync.Mutex
	host    string
	answers [][]string
	lookups atomic.Int32
}

func (r *rebindingResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	r.lookups.Add(1)
	r.mu.Lock()
	defer r.mu.Unlock()
	if host != r.host {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	index := min(int(r.lookups.Load())-1, len(r.answers)-1)
	return []net.IPAddr{{IP: net.ParseIP(r.answers[index][0])}}, nil
}

func TestEgressRejectsRedirectToPrivateDestination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Redirect(w, request, "http://private.test/secret", http.StatusFound)
	}))
	defer server.Close()

	var dials atomic.Int32
	egress := publicEgress(t,
		&staticResolver{answers: map[string][]string{
			"public.test":  {"93.184.216.34"},
			"private.test": {"10.1.2.3"},
		}},
		func(context.Context, string, string) (net.Conn, error) {
			dials.Add(1)
			return net.Dial("tcp", server.Listener.Addr().String())
		},
	)
	_, err := egress.HTTPClient(nil).Get("http://public.test/")
	var denied *EgressDeniedError
	if !errors.As(err, &denied) || denied.Address != "10.1.2.3" {
		t.Fatalf("redirect error = %v, want private address denial", err)
	}
	if dials.Load() != 1 {
		t.Fatalf("dial attempts = %d, want only the initial public hop", dials.Load())
	}
}

func TestEgressRejectsRedirectCarryingCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Redirect(w, request, "http://user:secret@public.test/next", http.StatusFound)
	}))
	defer server.Close()

	egress := publicEgress(t,
		&staticResolver{answers: map[string][]string{"public.test": {"93.184.216.34"}}},
		func(context.Context, string, string) (net.Conn, error) {
			return net.Dial("tcp", server.Listener.Addr().String())
		},
	)
	_, err := egress.HTTPClient(nil).Get("http://public.test/")
	var denied *EgressDeniedError
	if !errors.As(err, &denied) || denied.Code != EgressCodeUserinfo {
		t.Fatalf("redirect error = %v, want userinfo denial", err)
	}
}

func TestEgressValidatesResolvedHostBeforeLaunch(t *testing.T) {
	egress := publicEgress(t, &staticResolver{answers: map[string][]string{
		"public.test":  {"93.184.216.34"},
		"private.test": {"10.0.0.1"},
	}}, nil)
	if err := egress.ValidateRenderURL(context.Background(), "http://public.test/"); err != nil {
		t.Fatalf("public render URL rejected: %v", err)
	}
	for _, raw := range []string{"http://private.test/", "http://127.0.0.1/"} {
		err := egress.ValidateRenderURL(context.Background(), raw)
		var denied *EgressDeniedError
		if !errors.As(err, &denied) || denied.Code != EgressCodeAddress {
			t.Fatalf("%s: render URL error = %v, want address denial", raw, err)
		}
	}
}

func TestEgressRejectsUnsupportedRedirectScheme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "ftp://example.com/file")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	egress := publicEgress(t, &staticResolver{answers: map[string][]string{"public.test": {"93.184.216.34"}}}, injectingDial(server.Listener.Addr().String(), new([]string), new(sync.Mutex)))
	client := egress.HTTPClient(nil)
	_, err := client.Get("http://public.test/")
	var denied *EgressDeniedError
	if !errors.As(err, &denied) || denied.Code != EgressCodeScheme {
		t.Fatalf("redirect scheme error = %v, want scheme denial", err)
	}
}
