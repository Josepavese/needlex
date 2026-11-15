package platform

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestSessionScopesHeadersAndStripsCrossOriginRedirect(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen["destination"] = r.Header.Get("Authorization")
		mu.Unlock()
		_, _ = io.WriteString(w, "ok")
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen["source"] = r.Header.Get("Authorization")
		mu.Unlock()
		http.Redirect(w, r, destination.URL+"/next", http.StatusFound)
	}))
	defer source.Close()

	sourceURL := mustParseURL(t, source.URL)
	targetURL := mustParseURL(t, source.URL+"/start")
	session, err := NewSession(SessionConfig{Headers: []SessionHeader{{
		Origin: Origin(sourceURL), Name: "Authorization", Value: "Bearer source-secret",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	client, err := session.HTTPClient(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(targetURL.String())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if seen["source"] != "Bearer source-secret" {
		t.Fatalf("source authorization=%q", seen["source"])
	}
	if seen["destination"] != "" {
		t.Fatalf("authorization leaked cross-origin: %q", seen["destination"])
	}
}

func TestSessionSeedsScopedCookies(t *testing.T) {
	var mu sync.Mutex
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r.Header.Get("Cookie")
		mu.Unlock()
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	target := mustParseURL(t, server.URL+"/page")
	session, err := NewSession(SessionConfig{Cookies: []SessionCookie{{
		Name: "sid", Value: "cookie-secret", Domain: target.Hostname(), Path: "/",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	client, err := session.HTTPClient(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(target.String())
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(got, "sid=cookie-secret") {
		t.Fatalf("cookie header=%q", got)
	}
}

func TestSessionHostOnlyCookieDoesNotLeakToSubdomain(t *testing.T) {
	session, err := NewSession(SessionConfig{Cookies: []SessionCookie{{
		Name: "sid", Value: "secret", Domain: "app.example.com", HostOnly: true, Path: "/",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if cookies := session.jar.Cookies(mustParseURL(t, "https://x.app.example.com/")); len(cookies) != 0 {
		t.Fatalf("host-only cookie leaked to subdomain: %+v", cookies)
	}
	if cookies := session.jar.Cookies(mustParseURL(t, "https://app.example.com/")); len(cookies) != 1 {
		t.Fatalf("host-only cookie missing from exact host: %+v", cookies)
	}
	browserCookies := session.BrowserCookies("https://app.example.com/")
	if len(browserCookies) != 1 || browserCookies[0]["url"] == nil || browserCookies[0]["domain"] != nil {
		t.Fatalf("browser cookies=%v", browserCookies)
	}
}

func TestBrowserCookiesStayOnTargetOrigin(t *testing.T) {
	session, err := NewSession(SessionConfig{Cookies: []SessionCookie{
		{Name: "target", Value: "yes", Domain: "a.example.com", HostOnly: true, Path: "/"},
		{Name: "other", Value: "no", Domain: "b.example.com", HostOnly: true, Path: "/"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	cookies := session.BrowserCookies("https://a.example.com/")
	if len(cookies) != 1 || cookies[0]["name"] != "target" {
		t.Fatalf("browser cookies for target=%v", cookies)
	}
}

func TestSessionRejectsPublicSuffixCookie(t *testing.T) {
	if _, err := NewSession(SessionConfig{Cookies: []SessionCookie{{
		Name: "sid", Value: "secret", Domain: "com", Path: "/",
	}}}); err == nil {
		t.Fatal("expected public suffix cookie rejection")
	}
}

type sessionRoundTripper func(*http.Request) (*http.Response, error)

func (f sessionRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSessionPreservesCustomBaseTransport(t *testing.T) {
	session, err := NewSession(SessionConfig{})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	base := &http.Client{Transport: sessionRoundTripper(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}}, nil
	})}
	client, err := session.HTTPClient(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get("https://public.test/")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if !called {
		t.Fatal("custom base transport was replaced")
	}
}

func TestSplitURLCredentialsSanitizesAndBuildsBasicAuth(t *testing.T) {
	sanitized, rule, ok := SplitURLCredentials("https://user:pass@example.com/path?q=1#frag")
	if !ok {
		t.Fatal("expected credentials")
	}
	if sanitized != "https://example.com/path?q=1" {
		t.Fatalf("sanitized=%q", sanitized)
	}
	if rule.Origin != "https://example.com" || rule.Name != "Authorization" {
		t.Fatalf("rule=%+v", rule)
	}
	token := strings.TrimPrefix(rule.Value, "Basic ")
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil || string(decoded) != "user:pass" {
		t.Fatalf("decoded=%q err=%v", decoded, err)
	}
}

func TestSessionProxyRequiresUnrestrictedPolicy(t *testing.T) {
	session, err := NewSession(SessionConfig{ProxyURL: "http://127.0.0.1:3128"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.HTTPClient(nil, NewEgress(NetworkPolicyPublic)); err == nil {
		t.Fatal("expected proxy/public-policy conflict")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	target, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return target
}
