package src

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type directHLSFailingReader struct{}

func (directHLSFailingReader) Read(p []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestDirectHLSMediaCopy(t *testing.T) {
	t.Run("normal transfer", func(t *testing.T) {
		input := []byte("sample HLS media segment")
		var output bytes.Buffer

		err := copyDirectHLSMedia(&output, bytes.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(output.Bytes(), input) {
			t.Fatal("copied media does not match input")
		}
	})

	t.Run("oversized transfer", func(t *testing.T) {
		source := io.LimitReader(
			bytes.NewReader(make([]byte, directHLSMaxMediaSize+1)),
			directHLSMaxMediaSize+1,
		)

		err := copyDirectHLSMedia(io.Discard, source)
		if !errors.Is(err, errDirectHLSMediaTooLarge) {
			t.Fatalf("expected media size error, got %v", err)
		}
	})

	t.Run("interrupted transfer", func(t *testing.T) {
		err := copyDirectHLSMedia(
			io.Discard,
			directHLSFailingReader{},
		)

		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("expected interrupted transfer error, got %v", err)
		}
	})
}

type directHLSCountingWriter struct {
	written int64
}

func (w *directHLSCountingWriter) Write(p []byte) (int, error) {
	w.written += int64(len(p))
	return len(p), nil
}

func TestDirectHLSMediaOutputLimit(t *testing.T) {
	source := io.LimitReader(
		bytes.NewReader(make([]byte, directHLSMaxMediaSize+1)),
		directHLSMaxMediaSize+1,
	)

	var destination directHLSCountingWriter

	err := copyDirectHLSMedia(&destination, source)

	if !errors.Is(err, errDirectHLSMediaTooLarge) {
		t.Fatalf("expected size limit error, got %v", err)
	}

	if destination.written > directHLSMaxMediaSize {
		t.Fatalf(
			"wrote %d bytes, maximum allowed is %d",
			destination.written,
			directHLSMaxMediaSize,
		)
	}
}

func TestDirectHLSMediaWithLength(t *testing.T) {
	tests := []struct {
		name           string
		data           string
		expectedLength int64
		expectedError  error
	}{
		{
			name:           "complete segment",
			data:           "1234567890",
			expectedLength: 10,
		},
		{
			name:           "incomplete segment",
			data:           "12345",
			expectedLength: 10,
			expectedError:  io.ErrUnexpectedEOF,
		},
		{
			name:           "unknown length",
			data:           "1234567890",
			expectedLength: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer

			err := copyDirectHLSMediaWithLength(
				&output,
				bytes.NewBufferString(tt.data),
				tt.expectedLength,
			)

			if !errors.Is(err, tt.expectedError) {
				t.Fatalf(
					"expected error %v, got %v",
					tt.expectedError,
					err,
				)
			}

			if output.String() != tt.data {
				t.Fatalf(
					"expected %q, got %q",
					tt.data,
					output.String(),
				)
			}
		})
	}
}
