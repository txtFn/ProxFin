package src

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDirectHLSFirstResponseRecovery(t *testing.T) {
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

	client := upstream.Client()
	url := upstream.URL + "/segment.ts"

	firstResponse, err := client.Get(url)
	if err != nil {
		t.Fatalf("initial upstream request failed: %v", err)
	}

	data, contentType, err := fetchDirectHLSMediaFromResponse(
		context.Background(),
		client,
		url,
		firstResponse,
	)

	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	if string(data) != "1234567890" {
		t.Fatalf("unexpected recovered data: %q", data)
	}

	if contentType != "video/mp2t" {
		t.Fatalf("unexpected content type: %q", contentType)
	}

	if attempts.Load() != 2 {
		t.Fatalf(
			"expected exactly 2 upstream requests, got %d",
			attempts.Load(),
		)
	}
}

// Verify that two incomplete responses produce no media.
func TestDirectHLSFirstResponseRecoveryExhausted(t *testing.T) {
	var attempts atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)

			w.Header().Set("Content-Type", "video/mp2t")
			w.Header().Set("Content-Length", "10")

			_, _ = io.WriteString(w, "12345")
		},
	))
	defer upstream.Close()

	client := upstream.Client()
	url := upstream.URL + "/segment.ts"

	firstResponse, err := client.Get(url)
	if err != nil {
		t.Fatalf("initial upstream request failed: %v", err)
	}

	data, _, err := fetchDirectHLSMediaFromResponse(
		context.Background(),
		client,
		url,
		firstResponse,
	)

	if err == nil {
		t.Fatal("expected recovery to fail")
	}

	if len(data) != 0 {
		t.Fatalf("expected no partial media, got %d bytes", len(data))
	}

	if attempts.Load() != 2 {
		t.Fatalf(
			"expected exactly 2 upstream requests, got %d",
			attempts.Load(),
		)
	}

	t.Logf("Recovery correctly stopped after %d attempts", attempts.Load())
}
