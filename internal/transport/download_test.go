package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	if result.Primary.ContentEncoding != "gzip" || result.Primary.ContentType != "application/octet-stream" {
		t.Fatalf("unexpected MCP result: %s", &stdout)
	}
	assertDownloadedBytes(t, result.Primary, binary)
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
		args []string
		want string
	}{
		{[]string{site.URL, "--assets"}, "asset redirect leaves the allowed origins"},
		{[]string{site.URL + "/chunked", "--max-bytes", "3"}, "download exceeds total byte limit"},
	} {
		out := filepath.Join(t.TempDir(), "incomplete")
		var stdout, stderr bytes.Buffer
		args := append([]string{"fetch", "--out-dir", out}, tc.args...)
		if code := r.Run(args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), tc.want) {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("failed download left a destination: %v", err)
		}
	}

	existing := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := r.Run([]string{"fetch", site.URL, "--out-dir", existing}, &stdout, &stderr); code != 1 {
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
	return result
}

func assertDownloadedBytes(t *testing.T, file pipeline.DownloadFile, want []byte) {
	t.Helper()
	got, err := os.ReadFile(file.Path)
	if err != nil || !bytes.Equal(got, want) || file.Bytes != int64(len(want)) {
		t.Fatalf("downloaded %q: bytes=%q size=%d want=%q err=%v", file.Path, got, file.Bytes, want, err)
	}
}
