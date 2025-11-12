package fsutil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// AtomicWrite writes the provided data to path using a temp file + rename.
func AtomicWrite(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure config dir: %w", err)
	}

	uid, gid := currentOwner(path)

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
	if uid != nil && gid != nil {
		if err := os.Chown(tmpPath, *uid, *gid); err != nil {
			return fmt.Errorf("chown temp file: %w", err)
		}
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

func currentOwner(path string) (*int, *int) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, nil
	}
	uid := int(stat.Uid)
	gid := int(stat.Gid)
	return &uid, &gid
}
