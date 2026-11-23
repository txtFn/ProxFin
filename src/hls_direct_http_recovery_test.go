package src

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDirectHLSHTTPRecoverySuccess(t *testing.T) {
	var attempts atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			attempt := attempts.Add(1)

			w.Header().Set("Content-Type", "video/mp2t")
			w.Header().Set("Content-Length", "10")

			if attempt == 1 {
				_, _ = io.WriteString(w, "12345")
				return
			}

			_, _ = io.WriteString(w, "1234567890")
		},
	))
	defer upstream.Close()

	downstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			serveDirectHLSUpstreamWithClient(
				w,
				r,
				strings.Repeat("a", 32),
				upstream.URL+"/segment.ts",
				upstream.Client(),
			)
		},
	))
	defer downstream.Close()

	response, err := downstream.Client().Get(downstream.URL)
	if err != nil {
		t.Fatalf("downstream request failed: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf(
			"expected HTTP 200, got %d",
			response.StatusCode,
		)
	}

	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("unable to read recovered media: %v", err)
	}

	if string(data) != "1234567890" {
		t.Fatalf("unexpected recovered media: %q", data)
	}

	if response.ContentLength != 10 {
		t.Fatalf(
			"expected Content-Length 10, got %d",
			response.ContentLength,
		)
	}

	if attempts.Load() != 2 {
		t.Fatalf(
			"expected exactly 2 upstream requests, got %d",
			attempts.Load(),
		)
	}

	t.Logf("Successfully recovered segment after %d requests", attempts.Load())
	t.Logf("Downstream HTTP status: %d", response.StatusCode)
	t.Logf("Recovered media bytes: %d", len(data))
}
