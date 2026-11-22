package src

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDirectHLSMediaHTTPResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "video/mp2t")
			w.Header().Set("Content-Length", "10")
			_, _ = io.WriteString(w, "1234567890")
		},
	))
	defer upstream.Close()

	request := httptest.NewRequest(
		http.MethodGet,
		"/hls-direct/test",
		nil,
	)

	recorder := httptest.NewRecorder()

	serveDirectHLSUpstreamWithClient(
		recorder,
		request,
		strings.Repeat("a", 32),
		upstream.URL+"/segment.ts",
		upstream.Client(),
	)

	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", response.StatusCode)
	}

	if response.Header.Get("Content-Length") != "10" {
		t.Fatalf(
			"expected Content-Length 10, got %q",
			response.Header.Get("Content-Length"),
		)
	}

	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != "1234567890" {
		t.Fatalf("unexpected media content: %q", content)
	}
}

func TestDirectHLSInterruptedHTTPResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "video/mp2t")
			w.Header().Set("Content-Length", "10")

			_, _ = io.WriteString(w, "12345")
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
		t.Fatalf("expected HTTP 200, got %d", response.StatusCode)
	}

	if response.ContentLength != 10 {
		t.Fatalf(
			"expected Content-Length 10, got %d",
			response.ContentLength,
		)
	}

	content, err := io.ReadAll(response.Body)

	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf(
			"expected incomplete transfer error, got %v (received %d bytes)",
			err,
			len(content),
		)
	}

	t.Logf("HTTP status: %d", response.StatusCode)
	t.Logf("Content-Length: %d", response.ContentLength)
	t.Logf("Received bytes: %d", len(content))
	t.Logf("Read error: %v", err)
	t.Logf("Partial content: %q", content)

	if len(content) >= 10 {
		t.Fatalf(
			"expected incomplete media, received %d bytes",
			len(content),
		)
	}
}
