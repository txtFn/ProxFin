package src

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDirectHLSRetryAfterInterruptedSegment(t *testing.T) {
	var attempts atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			attempt := attempts.Add(1)

			w.Header().Set("Content-Type", "video/mp2t")
			w.Header().Set("Content-Length", "10")

			if attempt == 1 {
				_, _ = w.Write([]byte("12345"))
				return
			}

			_, _ = w.Write([]byte("1234567890"))
		},
	))
	defer upstream.Close()

	data, contentType, err := fetchDirectHLSMediaWithRetry(
		context.Background(),
		upstream.Client(),
		upstream.URL+"/segment.ts",
	)

	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	if string(data) != "1234567890" {
		t.Fatalf("unexpected segment: %q", data)
	}

	if contentType != "video/mp2t" {
		t.Fatalf("unexpected content type: %q", contentType)
	}

	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts.Load())
	}
}

func TestDirectHLSRetryExhausted(t *testing.T) {
	var attempts atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)

			w.Header().Set("Content-Type", "video/mp2t")
			w.Header().Set("Content-Length", "10")

			_, _ = w.Write([]byte("12345"))
		},
	))
	defer upstream.Close()

	data, _, err := fetchDirectHLSMediaWithRetry(
		context.Background(),
		upstream.Client(),
		upstream.URL+"/segment.ts",
	)

	if err == nil {
		t.Fatal("expected recovery failure")
	}

	if len(data) != 0 {
		t.Fatalf("expected no partial data, got %d bytes", len(data))
	}

	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts.Load())
	}

	t.Logf("Correctly stopped after %d failed attempts", attempts.Load())
	t.Logf("Final error: %v", err)
}

func TestDirectHLSRetryNon200(t *testing.T) {
	var attempts atomic.Int32

	upstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			http.Error(w, "Not Found", http.StatusNotFound)
		},
	))
	defer upstream.Close()

	data, _, err := fetchDirectHLSMediaWithRetry(
		context.Background(),
		upstream.Client(),
		upstream.URL+"/missing.ts",
	)

	if err == nil {
		t.Fatal("expected an error for HTTP 404")
	}

	if len(data) != 0 {
		t.Fatalf("expected no media data, got %d bytes", len(data))
	}

	if attempts.Load() != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts.Load())
	}

	t.Logf("HTTP 404 correctly rejected after %d attempt", attempts.Load())
}
