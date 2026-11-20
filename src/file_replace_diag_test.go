//go:build windows

package src

import (
	"bytes"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestWindowsAtomicReplacementDiagnostics(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test.m3u")

	versions := [][]byte{
		append([]byte("#EXTM3U\n"), bytes.Repeat([]byte("a"), 1<<20)...),
		append([]byte("#EXTM3U\n"), bytes.Repeat([]byte("b"), 2<<20)...),
	}

	write := func(data []byte) {
		f, err := createAtomic(file)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Discard()

		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}

		if err := f.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	write(versions[0])

	var stop atomic.Bool
	var reads atomic.Int64
	var readErrors atomic.Int64
	var mismatches atomic.Int64
	var once sync.Once
	var firstError string
	var firstMismatch int

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		for !stop.Load() {
			f, err := openSharedFile(file)
			var data []byte

			if err == nil {
				data, err = io.ReadAll(f)

				closeErr := f.Close()
				if err == nil {
					err = closeErr
				}
			}

			reads.Add(1)

			if err != nil {
				readErrors.Add(1)
				once.Do(func() {
					firstError = err.Error()
				})
				continue
			}

			if !bytes.Equal(data, versions[0]) &&
				!bytes.Equal(data, versions[1]) {
				mismatches.Add(1)
				once.Do(func() {
					firstMismatch = len(data)
				})
			}
		}
	}()

	for i := 0; i < 20; i++ {
		write(versions[i%2])
	}

	stop.Store(true)
	wg.Wait()

	// Verify concurrent reads actually happened.
	if reads.Load() == 0 {
		t.Fatal("no concurrent reads were performed")
	}

	// Verify there were no read errors.
	if readErrors.Load() != 0 {
		t.Fatalf("concurrent reads failed %d times: %s",
			readErrors.Load(), firstError)
	}

	// Verify readers never received partial or unexpected contents.
	if mismatches.Load() != 0 {
		t.Fatalf("detected %d partial or unexpected file contents; first size: %d",
			mismatches.Load(), firstMismatch)
	}

	t.Logf("Total reads: %d", reads.Load())
	t.Logf("Read errors: %d", readErrors.Load())
	t.Logf("Content mismatches: %d", mismatches.Load())
	t.Logf("First read error: %s", firstError)
	t.Logf("First mismatched length: %d", firstMismatch)

}
