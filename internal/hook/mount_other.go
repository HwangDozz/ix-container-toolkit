//go:build !linux

package hook

import (
	"fmt"
	"os"
	"path/filepath"
)

func bindMount(src, dst string) error {
	return fmt.Errorf("bind-mount not supported on this platform (src=%s, dst=%s)", src, dst)
}

func ensureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
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

func runLdconfig(_ string) error {
	return nil // no-op on non-Linux
}

func copyFile(src, dst string, _ os.FileMode) error {
	return fmt.Errorf("copying files not supported on this platform (src=%s, dst=%s)", src, dst)
}
