//go:build windows

package src

import (
	"errors"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func openSharedFile(name string) (*os.File, error) {
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(2 * time.Second)

	for {
		handle, err := windows.CreateFile(
			path,
			windows.GENERIC_READ,
			windows.FILE_SHARE_READ|
				windows.FILE_SHARE_WRITE|
				windows.FILE_SHARE_DELETE,
			nil,
			windows.OPEN_EXISTING,
			windows.FILE_ATTRIBUTE_NORMAL,
			0,
		)

		if err == nil {
			file := os.NewFile(uintptr(handle), name)
			if file == nil {
				windows.CloseHandle(handle)
				return nil, os.ErrInvalid
			}
			return file, nil
		}

		retryable :=
			errors.Is(err, syscall.Errno(32)) ||
				errors.Is(err, syscall.Errno(33)) ||
				errors.Is(err, syscall.Errno(2)) ||
				errors.Is(err, syscall.Errno(3))

		if !retryable || time.Now().After(deadline) {
			return nil, err
		}

		time.Sleep(5 * time.Millisecond)
	}
}
