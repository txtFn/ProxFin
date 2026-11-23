package src

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

const directHLSMediaMaxAttempts = 2

// Limit the number of simultaneous buffered upstream downloads.
var directHLSRecoverySlots = make(chan struct{}, 4)

// fetchDirectHLSMediaWithRetry retrieves a complete media resource.
// It retries transient failures before any bytes are sent downstream.
func fetchDirectHLSMediaWithRetry(
	ctx context.Context,
	client *http.Client,
	upstreamURL string,
) ([]byte, string, error) {
	select {
	case directHLSRecoverySlots <- struct{}{}:
		defer func() { <-directHLSRecoverySlots }()
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}

	var lastErr error

	for attempt := 0; attempt < directHLSMediaMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}

		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			upstreamURL,
			nil,
		)
		if err != nil {
			return nil, "", err
		}
		req.Header.Set("Accept", "*/*")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, "", errors.New("upstream returned non-200 status")
			}

			contentType := resp.Header.Get("Content-Type")

			data, readErr := readDirectHLSMediaFully(
				resp.Body,
				resp.ContentLength,
			)
			resp.Body.Close()

			if readErr == nil {
				return data, contentType, nil
			}

			if errors.Is(readErr, errDirectHLSMediaTooLarge) {
				return nil, "", readErr
			}

			lastErr = readErr
		}

		if attempt+1 < directHLSMediaMaxAttempts {
			select {
			case <-ctx.Done():
				return nil, "", ctx.Err()
			case <-time.After(250 * time.Millisecond):
			}
		}
	}

	if lastErr == nil {
		lastErr = io.ErrUnexpectedEOF
	}

	return nil, "", lastErr
}

// fetchDirectHLSMediaFromResponse uses the already-open upstream
// response and makes at most one additional attempt on failure.
func fetchDirectHLSMediaFromResponse(
	ctx context.Context,
	client *http.Client,
	upstreamURL string,
	firstResponse *http.Response,
) ([]byte, string, error) {
	if firstResponse == nil {
		return nil, "", errors.New("missing upstream response")
	}

	select {
	case directHLSRecoverySlots <- struct{}{}:
		defer func() { <-directHLSRecoverySlots }()
	case <-ctx.Done():
		firstResponse.Body.Close()
		return nil, "", ctx.Err()
	}

	contentType := firstResponse.Header.Get("Content-Type")

	data, err := readDirectHLSMediaFully(
		firstResponse.Body,
		firstResponse.ContentLength,
	)
	firstResponse.Body.Close()

	if err == nil {
		return data, contentType, nil
	}

	if errors.Is(err, errDirectHLSMediaTooLarge) {
		return nil, "", err
	}

	if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}

	// Retry once without releasing the concurrency slot.
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case <-time.After(250 * time.Millisecond):
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		upstreamURL,
		nil,
	)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.New("retry returned non-200 status")
	}

	data, err = readDirectHLSMediaFully(
		resp.Body,
		resp.ContentLength,
	)
	if err != nil {
		return nil, "", err
	}

	return data, resp.Header.Get("Content-Type"), nil
}
