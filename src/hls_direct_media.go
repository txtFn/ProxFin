package src

import (
	"errors"
	"io"
)

var errDirectHLSMediaTooLarge = errors.New("direct HLS media exceeds size limit")

const directHLSMaxMediaSize int64 = 128 * 1024 * 1024

func copyDirectHLSMedia(dst io.Writer, src io.Reader) error {
	// Never send more than the configured media size limit.
	n, err := io.Copy(
		dst,
		io.LimitReader(src, directHLSMaxMediaSize),
	)

	if err != nil {
		return err
	}

	// A shorter transfer is within the size limit.
	if n < directHLSMaxMediaSize {
		return nil
	}

	// Check for additional upstream data without sending it.
	var extra [1]byte

	count, err := src.Read(extra[:])

	if count > 0 {
		return errDirectHLSMediaTooLarge
	}

	if err == io.EOF {
		return nil
	}

	if err != nil {
		return err
	}

	// A reader may temporarily return zero bytes without EOF.
	// Avoid treating that as successful completion.
	return io.ErrNoProgress
}

func copyDirectHLSMediaWithLength(
	dst io.Writer,
	src io.Reader,
	expectedLength int64,
) error {
	if expectedLength < 0 {
		return copyDirectHLSMedia(dst, src)
	}

	if expectedLength > directHLSMaxMediaSize {
		return errDirectHLSMediaTooLarge
	}

	counter := &directHLSByteCounter{Writer: dst}

	if err := copyDirectHLSMedia(counter, src); err != nil {
		return err
	}

	if counter.written != expectedLength {
		return io.ErrUnexpectedEOF
	}

	return nil
}

type directHLSByteCounter struct {
	io.Writer
	written int64
}

func (c *directHLSByteCounter) Write(p []byte) (int, error) {
	n, err := c.Writer.Write(p)
	c.written += int64(n)
	return n, err
}
