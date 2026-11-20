//go:build windows

package src

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	replaceFileW = kernel32.NewProc("ReplaceFileW")
)

func replaceFile(src, dst string) error {
	from, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return err
	}

	to, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}

	const (
		sharingViolation = syscall.Errno(32)
		lockViolation    = syscall.Errno(33)
	)

	deadline := time.Now().Add(2 * time.Second)

	for {
		_, statErr := os.Stat(dst)

		if errors.Is(statErr, os.ErrNotExist) {
			return windows.MoveFileEx(
				from,
				to,
				windows.MOVEFILE_WRITE_THROUGH,
			)
		}
		if statErr != nil {
			return statErr
		}

		result, _, callErr := replaceFileW.Call(
			uintptr(unsafe.Pointer(to)),
			uintptr(unsafe.Pointer(from)),
			0,
			0,
			0,
			0,
		)

		if result != 0 {
			return nil
		}

		if !errors.Is(callErr, sharingViolation) &&
			!errors.Is(callErr, lockViolation) {
			return fmt.Errorf("ReplaceFileW failed: %w", callErr)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf(
				"ReplaceFileW timed out waiting for file access: %w",
				callErr,
			)
		}

		time.Sleep(10 * time.Millisecond)
	}
}
