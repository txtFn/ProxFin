package src

import (
	"errors"
	"io"
)

const directHLSRecoveryMaxMediaSize int64 = 16 * 1024 * 1024

var errDirectHLSEmptyMedia = errors.New("empty upstream media response")

// readDirectHLSMediaFully buffers a complete media resource.
// It rejects oversized, empty, and incomplete responses.
func readDirectHLSMediaFully(
	src io.Reader,
	expectedLength int64,
) ([]byte, error) {
	if expectedLength > directHLSRecoveryMaxMediaSize {
		return nil, errDirectHLSMediaTooLarge
	}

	limited := io.LimitReader(src, directHLSRecoveryMaxMediaSize+1)

	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > directHLSRecoveryMaxMediaSize {
		return nil, errDirectHLSMediaTooLarge
	}

	if expectedLength >= 0 && int64(len(data)) != expectedLength {
		return nil, io.ErrUnexpectedEOF
	}

	if len(data) == 0 {
		return nil, errDirectHLSEmptyMedia
	}

	return data, nil
}
