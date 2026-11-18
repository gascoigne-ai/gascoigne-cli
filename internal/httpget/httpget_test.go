package httpget

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoSendsGetWithHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.Header.Get("Accept") != "application/json" {
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		writer.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()
	resp, err := Do(context.Background(), nil, srv.URL, http.Header{"Accept": {"application/json"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot { // the status is the caller's to judge
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestDoBadURL(t *testing.T) {
	if _, err := Do(context.Background(), nil, "://x", nil); err == nil {
		t.Fatal("want error")
	}
}

func TestBytesBounds(t *testing.T) {
	read := func(body string, limit int) ([]byte, error) {
		return Bytes(&http.Response{Body: io.NopCloser(strings.NewReader(body))}, limit)
	}
	if got, err := read("12345", 5); err != nil || string(got) != "12345" {
		t.Fatalf("at the limit: %q %v", got, err)
	}
	if _, err := read("123456", 5); err == nil || !strings.Contains(err.Error(), "over 5 bytes") {
		t.Fatalf("over the limit: %v", err)
	}
}
