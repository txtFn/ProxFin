package src

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestDirectHLSLargeMediaFallback(t *testing.T) {
	const mediaSize = 17 * 1024 * 1024

	upstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "video/mp2t")
			w.Header().Set(
				"Content-Length",
				strconv.Itoa(mediaSize),
			)

			chunk := bytes.Repeat([]byte("A"), 64*1024)

			remaining := mediaSize

			for remaining > 0 {
				n := len(chunk)
				if n > remaining {
					n = remaining
				}

				if _, err := w.Write(chunk[:n]); err != nil {
					return
				}

				remaining -= n
			}
		},
	))
	defer upstream.Close()

	downstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			serveDirectHLSUpstreamWithClient(
				w,
				r,
				strings.Repeat("a", 32),
				upstream.URL+"/large.ts",
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

	received, err := io.Copy(io.Discard, response.Body)
	if err != nil {
		t.Fatalf("large media transfer failed: %v", err)
	}

	if received != mediaSize {
		t.Fatalf(
			"expected %d bytes, got %d",
			mediaSize,
			received,
		)
	}

	t.Logf("Large-media fallback succeeded")
	t.Logf("Transferred bytes: %d", received)
	t.Logf("HTTP status: %d", response.StatusCode)
}
