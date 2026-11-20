//go:build !windows

package src

import "os"

func replaceFile(src, dst string) error {
	return os.Rename(src, dst)
}
