package src

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDirectHLSRecoveryBuffer(t *testing.T) {
	t.Run("complete segment", func(t *testing.T) {
		data, err := readDirectHLSMediaFully(
			strings.NewReader("1234567890"),
			10,
		)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "1234567890" {
			t.Fatalf("unexpected content: %q", data)
		}
	})

	t.Run("interrupted segment", func(t *testing.T) {
		_, err := readDirectHLSMediaFully(
			strings.NewReader("12345"),
			10,
		)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("expected unexpected EOF, got %v", err)
		}
	})

	t.Run("empty segment", func(t *testing.T) {
		_, err := readDirectHLSMediaFully(
			strings.NewReader(""),
			0,
		)
		if !errors.Is(err, errDirectHLSEmptyMedia) {
			t.Fatalf("expected empty media error, got %v", err)
		}
	})

	t.Run("unknown content length", func(t *testing.T) {
		data, err := readDirectHLSMediaFully(
			strings.NewReader("segment data"),
			-1,
		)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "segment data" {
			t.Fatalf("unexpected content: %q", data)
		}
	})
}
