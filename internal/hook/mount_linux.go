//go:build linux

package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// bindMount performs a bind mount of src onto dst.
// dst must already exist (as a file or directory).
func bindMount(src, dst string) error {
	if err := syscall.Mount(src, dst, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("mount --bind %s %s: %w", src, dst, err)
	}
	return nil
}

// ensureFile creates an empty file at path if it doesn't already exist,
// including any missing parent directories.
func ensureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0600)
	if err != nil {
		return err
	}
	return f.Close()
}
