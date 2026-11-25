package src

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type cancellationRecoveryBody struct {
	reader io.Reader
	cancel context.CancelFunc
	reads  int
	closes int
}

func (b *cancellationRecoveryBody) Read(p []byte) (int, error) {
	b.reads++

	if b.cancel != nil {
		b.cancel()
	}

	return b.reader.Read(p)
}

func (b *cancellationRecoveryBody) Close() error {
	b.closes++
	return nil
}

type cancellationRecoveryTransport func(
	*http.Request,
) (*http.Response, error)

func (f cancellationRecoveryTransport) RoundTrip(
	r *http.Request,
) (*http.Response, error) {
	return f(r)
}

func cancellationRecoveryResponse(
	body *cancellationRecoveryBody,
	length int64,
) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"video/mp2t"},
		},
		Body:          body,
		ContentLength: length,
	}
}

func assertCancellationRecoveryResult(
	t *testing.T,
	data []byte,
	contentType string,
	err error,
	expected error,
	slots int,
) {
	t.Helper()

	if !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}

	if len(data) != 0 {
		t.Fatalf("cancelled request returned %d bytes", len(data))
	}

	if contentType != "" {
		t.Fatalf("unexpected content type: %q", contentType)
	}

	if got := len(directHLSRecoverySlots); got != slots {
		t.Fatalf("slot count changed: expected %d, got %d", slots, got)
	}
}

func occupyCancellationRecoverySlots(t *testing.T) {
	t.Helper()

	if len(directHLSRecoverySlots) != 0 {
		t.Fatal("recovery slots were not initially empty")
	}

	capacity := cap(directHLSRecoverySlots)

	for i := 0; i < capacity; i++ {
		directHLSRecoverySlots <- struct{}{}
	}

	t.Cleanup(func() {
		for i := 0; i < capacity; i++ {
			select {
			case <-directHLSRecoverySlots:
			default:
				t.Error("occupied recovery slot unexpectedly missing")
			}
		}
	})
}

func TestDirectHLSCancellationRecoveryBeforeDownload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	before := len(directHLSRecoverySlots)
	attempts := 0

	client := &http.Client{
		Transport: cancellationRecoveryTransport(
			func(r *http.Request) (*http.Response, error) {
				attempts++
				return nil, errors.New("unexpected request")
			},
		),
	}

	data, contentType, err := fetchDirectHLSMediaWithRetry(
		ctx, client, "https://example.invalid/segment.ts",
	)

	assertCancellationRecoveryResult(
		t, data, contentType, err, context.Canceled, before,
	)

	if attempts != 0 {
		t.Fatalf("expected zero requests, got %d", attempts)
	}
}

func TestDirectHLSCancellationRecoveryOccupiedSlots(t *testing.T) {
	occupyCancellationRecoverySlots(t)

	ctx, cancel := context.WithTimeout(
		context.Background(), 25*time.Millisecond,
	)
	defer cancel()

	before := len(directHLSRecoverySlots)
	attempts := 0

	client := &http.Client{
		Transport: cancellationRecoveryTransport(
			func(r *http.Request) (*http.Response, error) {
				attempts++
				return nil, errors.New("unexpected request")
			},
		),
	}

	data, contentType, err := fetchDirectHLSMediaWithRetry(
		ctx, client, "https://example.invalid/segment.ts",
	)

	assertCancellationRecoveryResult(
		t, data, contentType, err,
		context.DeadlineExceeded, before,
	)

	if attempts != 0 {
		t.Fatalf("expected zero requests, got %d", attempts)
	}
}

func TestDirectHLSCancellationRecoveryOccupiedResponse(t *testing.T) {
	occupyCancellationRecoverySlots(t)

	ctx, cancel := context.WithTimeout(
		context.Background(), 25*time.Millisecond,
	)
	defer cancel()

	before := len(directHLSRecoverySlots)

	body := &cancellationRecoveryBody{
		reader: strings.NewReader("1234567890"),
	}

	attempts := 0

	client := &http.Client{
		Transport: cancellationRecoveryTransport(
			func(r *http.Request) (*http.Response, error) {
				attempts++
				return nil, errors.New("unexpected request")
			},
		),
	}

	data, contentType, err := fetchDirectHLSMediaFromResponse(
		ctx,
		client,
		"https://example.invalid/segment.ts",
		cancellationRecoveryResponse(body, 10),
	)

	assertCancellationRecoveryResult(
		t, data, contentType, err,
		context.DeadlineExceeded, before,
	)

	if body.reads != 0 || body.closes != 1 || attempts != 0 {
		t.Fatalf(
			"reads=%d closes=%d requests=%d",
			body.reads, body.closes, attempts,
		)
	}
}

func TestDirectHLSCancellationRecoveryInitialRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := len(directHLSRecoverySlots)

	body := &cancellationRecoveryBody{
		reader: strings.NewReader("1234567890"),
		cancel: cancel,
	}

	attempts := 0

	client := &http.Client{
		Transport: cancellationRecoveryTransport(
			func(r *http.Request) (*http.Response, error) {
				attempts++
				return nil, errors.New("unexpected retry")
			},
		),
	}

	data, contentType, err := fetchDirectHLSMediaFromResponse(
		ctx,
		client,
		"https://example.invalid/segment.ts",
		cancellationRecoveryResponse(body, 10),
	)

	assertCancellationRecoveryResult(
		t, data, contentType, err, context.Canceled, before,
	)

	if body.reads == 0 || body.closes != 1 || attempts != 0 {
		t.Fatalf(
			"reads=%d closes=%d requests=%d",
			body.reads, body.closes, attempts,
		)
	}
}

func TestDirectHLSCancellationRecoveryDownloadRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := len(directHLSRecoverySlots)

	body := &cancellationRecoveryBody{
		reader: strings.NewReader("1234567890"),
		cancel: cancel,
	}

	attempts := 0

	client := &http.Client{
		Transport: cancellationRecoveryTransport(
			func(r *http.Request) (*http.Response, error) {
				attempts++
				return cancellationRecoveryResponse(body, 10), nil
			},
		),
	}

	data, contentType, err := fetchDirectHLSMediaWithRetry(
		ctx, client, "https://example.invalid/segment.ts",
	)

	assertCancellationRecoveryResult(
		t, data, contentType, err, context.Canceled, before,
	)

	if body.reads == 0 || body.closes != 1 || attempts != 1 {
		t.Fatalf(
			"reads=%d closes=%d requests=%d",
			body.reads, body.closes, attempts,
		)
	}
}

func TestDirectHLSCancellationRecoveryRetryRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := len(directHLSRecoverySlots)

	first := &cancellationRecoveryBody{
		reader: strings.NewReader("12345"),
	}

	retry := &cancellationRecoveryBody{
		reader: strings.NewReader("1234567890"),
		cancel: cancel,
	}

	attempts := 0

	client := &http.Client{
		Transport: cancellationRecoveryTransport(
			func(r *http.Request) (*http.Response, error) {
				attempts++
				return cancellationRecoveryResponse(retry, 10), nil
			},
		),
	}

	data, contentType, err := fetchDirectHLSMediaFromResponse(
		ctx,
		client,
		"https://example.invalid/segment.ts",
		cancellationRecoveryResponse(first, 10),
	)

	assertCancellationRecoveryResult(
		t, data, contentType, err, context.Canceled, before,
	)

	if first.closes != 1 || retry.closes != 1 {
		t.Fatalf(
			"first closes=%d retry closes=%d",
			first.closes, retry.closes,
		)
	}

	if retry.reads == 0 || attempts != 1 {
		t.Fatalf(
			"retry reads=%d requests=%d",
			retry.reads, attempts,
		)
	}
}

func TestDirectHLSCancellationRecoverySecondAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := len(directHLSRecoverySlots)

	first := &cancellationRecoveryBody{
		reader: strings.NewReader("12345"),
	}

	retry := &cancellationRecoveryBody{
		reader: strings.NewReader("1234567890"),
		cancel: cancel,
	}

	attempts := 0

	client := &http.Client{
		Transport: cancellationRecoveryTransport(
			func(r *http.Request) (*http.Response, error) {
				attempts++

				switch attempts {
				case 1:
					return cancellationRecoveryResponse(first, 10), nil
				case 2:
					return cancellationRecoveryResponse(retry, 10), nil
				default:
					return nil, errors.New("unexpected extra attempt")
				}
			},
		),
	}

	data, contentType, err := fetchDirectHLSMediaWithRetry(
		ctx, client, "https://example.invalid/segment.ts",
	)

	assertCancellationRecoveryResult(
		t, data, contentType, err, context.Canceled, before,
	)

	if first.closes != 1 || retry.closes != 1 {
		t.Fatalf(
			"first closes=%d retry closes=%d",
			first.closes, retry.closes,
		)
	}

	if retry.reads == 0 || attempts != 2 {
		t.Fatalf(
			"retry reads=%d requests=%d",
			retry.reads, attempts,
		)
	}
}
