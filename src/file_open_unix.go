//go:build !windows

package src

import "os"

func openSharedFile(name string) (*os.File, error) {
	return os.Open(name)
}
