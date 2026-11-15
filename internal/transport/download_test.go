package transport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/josepavese/needlex/internal/pipeline"
)

func TestFetchCLIOriginalHTMLAndLinkedAssets(t *testing.T) {
	const css = "/* untouched */\r\n.a { color: red; }\n"
	const js = "\xef\xbb\xbf/* untouched */\r\nalert(1);"
	var cdnRequests atomic.Int32
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		cdnRequests.Add(1)
		if req.URL.Path != "/app.js" || req.Header.Get("Referer") != "" {
			t.Errorf("unexpected CDN request %s with Referer %q", req.URL, req.Header.Get("Referer"))
		}
		_, _ = io.WriteString(w, js)
	}))
	defer cdn.Close()

	page := "<!doctype html>\r\n<meta http-equiv=refresh content='0;url=/never'>" +
		"<link rel=stylesheet href='theme?v=2&amp;x=1'>" +
		"<link rel=stylesheet href='theme?v=2&amp;x=1#duplicate'>" +
		"<base href='/static/'><script src='" + cdn.URL + "/app.js'></script>\xff\r\n"
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Accept-Encoding") != "identity" {
			t.Errorf("Accept-Encoding = %q", req.Header.Get("Accept-Encoding"))
		}
		switch req.URL.Path {
		case "/entry":
			http.Redirect(w, req, "/nested/page", http.StatusFound)
		case "/nested/page":
			w.Header().Set("Content-Type", "text/html; charset=windows-1252")
			_, _ = io.WriteString(w, page)
		case "/static/theme":
			if req.URL.RawQuery != "v=2&x=1" {
				t.Errorf("query = %q", req.URL.RawQuery)
			}
			_, _ = io.WriteString(w, css)
		default:
			t.Errorf("unexpected request %s", req.URL)
			http.NotFound(w, req)
		}
	}))
	defer site.Close()

	r := NewRunner()
	r.storeRoot = t.TempDir()
	out := filepath.Join(t.TempDir(), "source")
	result := runFetchTestCLI(t, r, site.URL+"/entry", "--assets", "--out-dir", out)
	if result.Directory != out || result.Primary.FinalURL != site.URL+"/nested/page" ||
		len(result.Assets) != 1 || !reflect.DeepEqual(result.SkippedAssetOrigins, []string{cdn.URL}) {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertDownloadedBytes(t, result.Primary, []byte(page))
	assertDownloadedBytes(t, result.Assets[0], []byte(css))
	assertDownloadModes(t, result.Directory, result.Primary.Path)
	result = runFetchTestCLI(t, r, site.URL+"/entry", "--assets", "--asset-origin", cdn.URL)
	if len(result.Assets) != 2 || len(result.SkippedAssetOrigins) != 0 || cdnRequests.Load() != 1 {
		t.Fatalf("unexpected allowlisted result: %+v (CDN requests: %d)", result, cdnRequests.Load())
	}
	assertDownloadedBytes(t, result.Assets[1], []byte(js))
}

func TestFetchMCPPreservesBinaryAndContentEncoding(t *testing.T) {
	binary := []byte{0x1f, 0x8b, 0x00, 0xff, 0x01}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(binary)
	}))
	defer site.Close()

	r := NewRunner()
	r.storeRoot = t.TempDir()
	r.stdin = strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"web_fetch","arguments":{"url":%q,"max_bytes":%d}}}`, site.URL, len(binary)))
	var stdout, stderr bytes.Buffer
	if code := r.Run([]string{"mcp"}, &stdout, &stderr); code != 0 {
		t.Fatalf("MCP failed: %d, %s", code, &stderr)
	}
	var response struct {
		Result struct {
			StructuredContent struct {
				Download pipeline.DownloadResult
			}
		}
	}
	// MCP uses snake_case for the download itself and camelCase for the envelope.
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	result := response.Result.StructuredContent.Download
	digest := sha256.Sum256(binary)
	if result.Schema != pipeline.DownloadReceiptSchemaV1 || result.Primary.ContentEncoding != "gzip" ||
		result.Primary.ContentType != "application/octet-stream" || result.Primary.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("unexpected MCP result: %s", &stdout)
	}
	assertDownloadedBytes(t, result.Primary, binary)
}

func TestFetchReceiptErrorCodes(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/missing" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "hello")
	}))
	defer site.Close()
	existingFile := filepath.Join(t.TempDir(), "exists")
	if err := os.WriteFile(existingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		args     func(out string) []string
		wantCode string
	}{
		{"invalid URL", func(out string) []string {
			return []string{"fetch", "http:///file", "--out-dir", out}
		}, "invalid_url"},
		{"invalid asset origin", func(out string) []string {
			return []string{"fetch", site.URL, "--asset-origin", "https://cdn.example.com", "--out-dir", out}
		}, "invalid_arguments"},
		{"http status", func(out string) []string {
			return []string{"fetch", site.URL + "/missing", "--out-dir", out}
		}, "http_status"},
		{"assets need HTML", func(out string) []string {
			return []string{"fetch", site.URL, "--assets", "--out-dir", out}
		}, "assets_unsupported"},
		{"existing output", func(string) []string {
			return []string{"fetch", site.URL, "--out-dir", existingFile}
		}, "output_directory"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := NewRunner()
			runner.storeRoot = t.TempDir()
			var stdout, stderr bytes.Buffer
			out := filepath.Join(t.TempDir(), "out")
			if code := runner.Run(tc.args(out), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "code="+tc.wantCode) {
				t.Fatalf("code=%d stdout=%s stderr=%s, want code=%s", code, &stdout, &stderr, tc.wantCode)
			}
		})
	}
}

func TestFetchDeniesIPv4MappedLoopback(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("mapped loopback fetch reached the server")
	}))
	defer site.Close()

	runner := NewRunner()
	runner.storeRoot = t.TempDir()
	var stdout, stderr bytes.Buffer
	code := runner.Run([]string{"fetch", "--config", publicPolicyConfig(t), site.URL + "/file"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "code=network_denied") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}

	port := site.Listener.Addr().(*net.TCPAddr).Port
	code = runner.Run([]string{"fetch", "--config", publicPolicyConfig(t), fmt.Sprintf("http://[::ffff:127.0.0.1]:%d/file", port)}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "code=network_denied") {
		t.Fatalf("mapped code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
}

func TestFetchRollsBackIncompleteBundle(t *testing.T) {
	disallowed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("asset redirect requested a disallowed origin")
	}))
	defer disallowed.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<script src='/redirect'></script>")
		case "/redirect":
			http.Redirect(w, req, disallowed.URL, http.StatusFound)
		case "/chunked":
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, "1234")
		}
	}))
	defer site.Close()

	r := NewRunner()
	r.storeRoot = t.TempDir()
	for _, tc := range []struct {
		args     []string
		want     string
		wantCode string
	}{
		{[]string{site.URL, "--assets"}, "asset redirect leaves the allowed origins", "invalid_url"},
		{[]string{site.URL + "/chunked", "--max-bytes", "3"}, "download exceeds total byte limit", "max_bytes_exceeded"},
	} {
		out := filepath.Join(t.TempDir(), "incomplete")
		var stdout, stderr bytes.Buffer
		args := append([]string{"fetch", "--out-dir", out}, tc.args...)
		if code := r.Run(args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), tc.want) || !strings.Contains(stderr.String(), "code="+tc.wantCode) {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("failed download left a destination: %v", err)
		}
	}

	existing := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := r.Run([]string{"fetch", site.URL, "--out-dir", existing}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "code=output_directory") {
		t.Fatalf("accepted existing directory: code=%d stderr=%s", code, &stderr)
	}
	if _, err := os.Stat(existing); err != nil {
		t.Fatalf("existing directory was affected: %v", err)
	}
}

func TestFetchCancellationDiscardsDownload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(w, "incomplete")
		w.(http.Flusher).Flush()
		cancel()
		<-req.Context().Done()
	}))
	defer site.Close()

	r := NewRunner()
	r.operationContext = ctx
	out := filepath.Join(t.TempDir(), "cancelled")
	_, err := r.download(pipeline.DownloadRequest{URL: site.URL}, out)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("download error = %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("cancelled download left a destination: %v", err)
	}
}

func runFetchTestCLI(t *testing.T, r Runner, args ...string) pipeline.DownloadResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := r.Run(append([]string{"fetch"}, args...), &stdout, &stderr); code != 0 {
		t.Fatalf("fetch failed: %d, %s", code, &stderr)
	}
	var result pipeline.DownloadResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != pipeline.DownloadReceiptSchemaV1 {
		t.Fatalf("receipt schema = %q, want %q", result.Schema, pipeline.DownloadReceiptSchemaV1)
	}
	return result
}

func assertDownloadedBytes(t *testing.T, file pipeline.DownloadFile, want []byte) {
	t.Helper()
	got, err := os.ReadFile(file.Path)
	digest := sha256.Sum256(want)
	if err != nil || !bytes.Equal(got, want) || file.Bytes != int64(len(want)) || file.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("downloaded %q: bytes=%q size=%d sha256=%q want=%q digest=%x err=%v", file.Path, got, file.Bytes, file.SHA256, want, digest, err)
	}
}

func assertDownloadModes(t *testing.T, directory, file string) {
	t.Helper()
	dirInfo, err := os.Stat(directory)
	if err != nil || dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("download directory mode = %v err=%v, want 0700", dirInfo, err)
	}
	fileInfo, err := os.Stat(file)
	if err != nil || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("download file mode = %v err=%v, want 0600", fileInfo, err)
	}
}

func TestFetchCLISessionHeadersCookiesAndURLUserinfo(t *testing.T) {
	const secret = "Bearer session-secret"
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		basicUser, basicPass, basicOK := req.BasicAuth()
		authorized := req.Header.Get("Authorization") == secret && strings.Contains(req.Header.Get("Cookie"), "sid=cookie-secret")
		if basicOK && basicUser == "user" && basicPass == "pass" {
			authorized = true
		}
		if !authorized {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "protected bytes")
	}))
	defer site.Close()

	r := NewRunner()
	r.storeRoot = t.TempDir()
	out := filepath.Join(t.TempDir(), "protected")
	result := runFetchTestCLI(t, r, site.URL+"/private", "--out-dir", out,
		"--header", "Authorization: "+secret, "--cookie", "sid=cookie-secret")
	if result.Primary.Status != http.StatusOK || result.Primary.URL != site.URL+"/private" {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertDownloadedBytes(t, result.Primary, []byte("protected bytes"))

	var stdout, stderr bytes.Buffer
	r = NewRunner()
	r.storeRoot = t.TempDir()
	userinfoURL := strings.Replace(site.URL, "http://", "http://user:pass@", 1) + "/private"
	if code := r.Run([]string{"fetch", userinfoURL, "--out-dir", filepath.Join(t.TempDir(), "userinfo")}, &stdout, &stderr); code != 0 {
		t.Fatalf("userinfo fetch failed: %d %s", code, &stderr)
	}
	if strings.Contains(stdout.String(), "pass@") || strings.Contains(stdout.String(), "user:pass") {
		t.Fatalf("credentials leaked in receipt: %s", &stdout)
	}
}

func TestFetchMCPInlineSessionCredentials(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-API-Key") != "mcp-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("mcp protected bytes"))
	}))
	defer site.Close()

	r := NewRunner()
	r.storeRoot = t.TempDir()
	r.stdin = strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"web_fetch","arguments":{"url":%q,"headers":{"X-API-Key":"mcp-secret"}}}}`, site.URL))
	var stdout, stderr bytes.Buffer
	if code := r.Run([]string{"mcp"}, &stdout, &stderr); code != 0 {
		t.Fatalf("MCP failed: %d %s", code, &stderr)
	}
	if strings.Contains(stdout.String(), "mcp-secret") {
		t.Fatalf("credential leaked in MCP response: %s", &stdout)
	}
	var response struct {
		Result struct {
			StructuredContent struct {
				Download pipeline.DownloadResult
			}
		}
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	assertDownloadedBytes(t, response.Result.StructuredContent.Download.Primary, []byte("mcp protected bytes"))
}
