package platform

import (
	"context"
	"fmt"
	"net/http"
)

// OpenHTTPGet starts a GET. The caller owns the response body on success.
func OpenHTTPGet(ctx context.Context, client *http.Client, address string, headers http.Header, observe func()) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	request.Header = headers
	if observe != nil {
		observe()
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}
	return response, nil
}
