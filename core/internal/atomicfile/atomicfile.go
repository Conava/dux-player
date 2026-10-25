// Package atomicfile writes files so a reader or a crash never sees half of
// one: a synced temp file renamed over the old one.
package atomicfile

import (
	"os"
	"path/filepath"
)

// WriteFile replaces path with data. A missing parent is created 0700.
func WriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := fill(tmp, data, mode); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func fill(tmp *os.File, data []byte, mode os.FileMode) error {
	_, err := tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(mode)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	return err
}
