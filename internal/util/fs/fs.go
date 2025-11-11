package fsutil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// AtomicWrite writes the provided data to path using a temp file + rename.
func AtomicWrite(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*.json")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // best-effort cleanup

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	if dirF, err := os.Open(dir); err == nil {
		defer dirF.Close()
		_ = dirF.Sync()
	}
	return nil
}
