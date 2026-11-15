package pipeline

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/platform"
)

func TestAcquirerUsesSessionCredentialsOnHTTPPath(t *testing.T) {
	var seenAuth, seenCookie string
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seenAuth = req.Header.Get("Authorization")
		seenCookie = req.Header.Get("Cookie")
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body>private page</body></html>")
	}))
	defer site.Close()
	target, err := url.Parse(site.URL + "/profile")
	if err != nil {
		t.Fatal(err)
	}
	session, err := platform.NewSession(platform.SessionConfig{
		Headers: []platform.SessionHeader{{Origin: platform.Origin(target), Name: "Authorization", Value: "Bearer token"}},
		Cookies: []platform.SessionCookie{{Name: "sid", Value: "cookie", Domain: target.Hostname(), Path: "/"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := (Acquirer{}).Acquire(context.Background(), AcquireInput{
		URL: target.String(), MaxBytes: 1 << 20, Session: session,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.FetchMode != "http" || !strings.Contains(page.HTML, "private page") {
		t.Fatalf("page=%+v", page)
	}
	if seenAuth != "Bearer token" || !strings.Contains(seenCookie, "sid=cookie") {
		t.Fatalf("auth=%q cookie=%q", seenAuth, seenCookie)
	}
}
