package service

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

func TestReadUsesOperationSession(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer service-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<!doctype html><html><head><title>Private profile</title></head><body><h1>Private profile</h1><p>The authenticated profile contains account settings and security details for the signed-in user.</p></body></html>`)
	}))
	defer site.Close()
	target, err := url.Parse(site.URL + "/profile")
	if err != nil {
		t.Fatal(err)
	}
	session, err := platform.NewSession(platform.SessionConfig{Headers: []platform.SessionHeader{{
		Origin: platform.Origin(target), Name: "Authorization", Value: "Bearer service-token",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	svc := newTestService(t, testConfig(), nil)
	response, err := svc.Read(platform.WithSession(context.Background(), session), ReadRequest{
		URL: target.String(), Objective: "account settings and security details",
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Document.URL != target.String() || !strings.Contains(response.Document.Title, "Private profile") {
		t.Fatalf("document=%+v", response.Document)
	}
}
