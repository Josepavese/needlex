package pipeline

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

const (
	DefaultDownloadMaxBytes = 64 << 20
	DefaultDownloadTimeout  = time.Minute
	MaxDownloadAssets       = 64
	maxDownloadRedirects    = 10
)

type DownloadRequest struct {
	URL          string
	Assets       bool
	AssetOrigins []string
	MaxBytes     int64
	// Egress enforces the configured network policy on the primary representation
	// and on every linked asset. Nil keeps the historical unrestricted transport.
	Egress *platform.Egress
}

type DownloadFile struct {
	URL             string `json:"url"`
	FinalURL        string `json:"final_url"`
	Path            string `json:"path"`
	ContentType     string `json:"content_type,omitempty"`
	ContentEncoding string `json:"content_encoding,omitempty"`
	Bytes           int64  `json:"bytes"`
	Status          int    `json:"status"`
}

type DownloadResult struct {
	Directory           string         `json:"directory"`
	Primary             DownloadFile   `json:"primary"`
	Assets              []DownloadFile `json:"assets,omitempty"`
	SkippedAssetOrigins []string       `json:"skipped_asset_origins,omitempty"`
}

const downloadHTMLName = "body.html"

// Download saves one HTTP representation and optionally its direct, allowed
// stylesheet/script references. A failed bundle discards its own files.
func Download(ctx context.Context, request DownloadRequest, createDirectory func() (platform.DownloadDirectory, error)) (result DownloadResult, err error) {
	target, err := downloadURL(request.URL)
	if err != nil {
		return result, err
	}
	if err := request.Egress.ValidateURL(target); err != nil {
		return result, err
	}
	if request.MaxBytes < 0 {
		return result, errors.New("max_bytes must be positive")
	}
	remaining := request.MaxBytes
	if remaining == 0 {
		remaining = DefaultDownloadMaxBytes
	}
	origins, err := downloadOrigins(request)
	if err != nil {
		return result, err
	}

	directory, err := createDirectory()
	if err != nil {
		return result, err
	}
	retain := false
	defer func() { err = errors.Join(err, directory.Close(retain)) }()
	result, err = downloadFiles(ctx, request.Assets, target, origins, directory, remaining, request.Egress)
	if err == nil {
		err = ctx.Err()
	}
	retain = err == nil
	return result, err
}

func downloadFiles(ctx context.Context, withAssets bool, target *url.URL, origins map[string]bool, directory platform.DownloadDirectory, remaining int64, egress *platform.Egress) (DownloadResult, error) {
	var result DownloadResult
	var err error

	result.Directory = directory.Path()
	result.Primary, err = saveDownload(ctx, newDownloadClient(nil, egress), directory, target, "", remaining)
	if err != nil || !withAssets {
		return result, err
	}
	if !downloadHTML(result.Primary.ContentType) {
		return result, errors.New("assets require an HTML response")
	}
	if encoding := result.Primary.ContentEncoding; encoding != "" && !strings.EqualFold(encoding, "identity") {
		return result, fmt.Errorf("cannot find linked assets in HTML served with Content-Encoding %q", encoding)
	}

	// Resolve asset references against the final HTTP URL after redirects.
	finalURL, err := downloadURL(result.Primary.FinalURL)
	if err != nil {
		return result, err
	}
	origins[downloadOrigin(finalURL)] = true
	assets, skipped, err := discoverDownloadAssets(ctx, directory, finalURL, origins)
	if err != nil {
		return result, err
	}
	result.SkippedAssetOrigins = skipped
	remaining -= result.Primary.Bytes
	client := newDownloadClient(origins, egress)
	for i, asset := range assets {
		name := fmt.Sprintf("asset-%02d.%s", i+1, asset.kind)
		file, err := saveDownload(ctx, client, directory, asset.url, name, remaining)
		if err != nil {
			return result, fmt.Errorf("download asset %d: %w", i+1, err)
		}
		result.Assets = append(result.Assets, file)
		remaining -= file.Bytes
	}
	return result, nil
}

func newDownloadClient(origins map[string]bool, egress *platform.Egress) *http.Client {
	client := &http.Client{
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxDownloadRedirects {
				return errors.New("too many HTTP redirects")
			}
			target, err := downloadURL(request.URL.String())
			if err != nil {
				return err
			}
			if origins != nil && !origins[downloadOrigin(target)] {
				return errors.New("asset redirect leaves the allowed origins")
			}
			request.Header.Del("Referer")
			return nil
		},
	}
	if egress != nil {
		client = egress.HTTPClient(client)
	}
	return client
}

func saveDownload(ctx context.Context, client *http.Client, directory platform.DownloadDirectory, target *url.URL, name string, limit int64) (DownloadFile, error) {
	headers := make(http.Header)
	headers.Set("User-Agent", defaultUserAgent)
	// Setting this explicitly disables net/http's automatic gzip decoding.
	headers.Set("Accept-Encoding", "identity")
	response, err := platform.OpenHTTPGet(ctx, client, target.String(), headers, nil)
	if err != nil {
		return DownloadFile{}, err
	}
	defer platform.Close(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 || response.StatusCode == http.StatusPartialContent {
		return DownloadFile{}, fmt.Errorf("download received HTTP %d", response.StatusCode)
	}
	if response.ContentLength > limit {
		return DownloadFile{}, errors.New("download exceeds total byte limit")
	}
	contentType := response.Header.Get("Content-Type")
	if name == "" {
		name = "body"
		if downloadHTML(contentType) {
			name = downloadHTMLName
		}
	}
	path, size, err := directory.Save(name, response.Body, limit)
	if err != nil {
		return DownloadFile{}, fmt.Errorf("save response body: %w", err)
	}
	return DownloadFile{
		URL: target.String(), FinalURL: response.Request.URL.String(), Path: path,
		ContentType: contentType, ContentEncoding: strings.Join(response.Header.Values("Content-Encoding"), ", "),
		Bytes: size, Status: response.StatusCode,
	}, nil
}

func downloadURL(address string) (*url.URL, error) {
	target, err := url.Parse(strings.TrimSpace(address))
	if err != nil || target.Hostname() == "" || target.User != nil || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, errors.New("download requires an absolute HTTP(S) URL without credentials")
	}
	target.Fragment, target.RawFragment = "", ""
	return target, nil
}

func downloadOrigin(target *url.URL) string {
	host := strings.ToLower(target.Host)
	if port := target.Port(); (port == "80" && target.Scheme == "http") || (port == "443" && target.Scheme == "https") {
		host = strings.TrimSuffix(host, ":"+port)
	}
	return target.Scheme + "://" + host
}

func downloadOrigins(request DownloadRequest) (map[string]bool, error) {
	if !request.Assets && len(request.AssetOrigins) != 0 {
		return nil, errors.New("asset_origins requires assets")
	}
	origins := make(map[string]bool)
	for _, address := range request.AssetOrigins {
		origin, err := downloadURL(address)
		if err != nil || (origin.Path != "" && origin.Path != "/") || origin.RawQuery != "" || origin.ForceQuery || strings.Contains(address, "#") {
			return nil, errors.New("asset_origins entries must contain only scheme, host and optional port")
		}
		origins[downloadOrigin(origin)] = true
	}
	return origins, nil
}

func downloadHTML(contentType string) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}
