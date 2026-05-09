package benchmarks

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// BenchmarkHTTPClientPerRequest creates a new http.Client for every request —
// each client has its own Transport with a fresh connection pool, so TCP
// connections are never reused across iterations.
func BenchmarkHTTPClientPerRequest(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b.ResetTimer()
	for b.Loop() {
		c := &http.Client{Timeout: 5 * time.Second}
		resp, err := c.Get(srv.URL)
		if err == nil {
			resp.Body.Close() //nolint:errcheck // benchmark: body close error not meaningful
		}
	}
}

// BenchmarkHTTPClientReused uses a single shared client so the underlying
// Transport can reuse the idle TCP connection across iterations.
func BenchmarkHTTPClientReused(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	b.ResetTimer()
	for b.Loop() {
		resp, err := client.Get(srv.URL)
		if err == nil {
			resp.Body.Close() //nolint:errcheck // benchmark: body close error not meaningful
		}
	}
}
