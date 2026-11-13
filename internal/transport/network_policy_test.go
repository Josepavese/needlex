package transport

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func publicPolicyConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "needlex.json")
	if err := os.WriteFile(path, []byte(`{"network":{"policy":"public"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFetchCLIAppliesPublicNetworkPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("policy denied a loopback fetch but the request reached the server")
	}))
	defer server.Close()

	runner := NewRunner()
	runner.storeRoot = t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runner.Run([]string{"fetch", "--config", publicPolicyConfig(t), server.URL + "/file"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "egress denied by network policy") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
}

func TestMCPFetchAppliesPublicNetworkPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("policy denied a loopback fetch but the request reached the server")
	}))
	defer server.Close()

	t.Setenv("NEEDLEX_CONFIG", publicPolicyConfig(t))
	runner := NewRunner()
	runner.storeRoot = t.TempDir()
	runner.stdin = strings.NewReader(fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"web_fetch","arguments":{"url":%q}}}`,
		server.URL+"/file",
	))
	var stdout, stderr bytes.Buffer
	runner.Run([]string{"mcp"}, &stdout, &stderr)
	if !strings.Contains(stdout.String(), "egress denied by network policy") {
		t.Fatalf("stdout=%s stderr=%s", &stdout, &stderr)
	}
}

func TestReadCLIAppliesPublicNetworkPolicy(t *testing.T) {
	runner := NewRunner()
	runner.storeRoot = t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runner.Run([]string{"read", "http://127.0.0.1:9471/private", "--config", publicPolicyConfig(t), "--json"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "class=network_denied") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
}
