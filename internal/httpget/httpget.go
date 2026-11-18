// Package httpget holds the two steps every bounded download shares: a GET
// request that honors the context, and a body read that refuses to grow past
// a limit. What a status means (200 only, 304 too) stays with the caller.
package httpget

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// Do sends GET url with header through client (http.DefaultClient when nil).
// The caller closes the body and checks the status.
func Do(ctx context.Context, client *http.Client, url string, header http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for name, values := range header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

// Bytes reads the whole body, failing when it is over limit bytes.
func Bytes(resp *http.Response, limit int) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, fmt.Errorf("response over %d bytes", limit)
	}
	return body, nil
}
