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
