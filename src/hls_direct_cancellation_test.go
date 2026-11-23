package src

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDirectHLSRecoveryCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	response := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader("1234567890")),
		ContentLength: 10,
	}

	before := len(directHLSRecoverySlots)

	data, _, err := fetchDirectHLSMediaFromResponse(
		ctx,
		http.DefaultClient,
		"https://example.com/segment.ts",
		response,
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if len(data) != 0 {
		t.Fatalf("expected no media, got %d bytes", len(data))
	}

	after := len(directHLSRecoverySlots)

	if after != before {
		t.Fatalf(
			"concurrency slot leak: before=%d after=%d",
			before,
			after,
		)
	}

	t.Log("Cancellation correctly handled without leaking a slot")
}
