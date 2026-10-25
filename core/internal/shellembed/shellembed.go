// Package shellembed carries the quickshell UI inside the binary and extracts
// it at runtime, since quickshell needs a real path. Customize through -c or
// DPLAYER_SHELL_DIR, not by editing the extraction.
package shellembed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"syscall"

	"github.com/duxlabs-dev/dux-go/shellapp/shellfs"
)

const (
	distRoot   = "dist"
	shellEntry = "shell.qml"
)

// Available reports whether this binary was built with the embedded UI
// (the withshell build tag).
func Available() bool {
	info, err := fs.Stat(distFS, path.Join(distRoot, shellEntry))
	return err == nil && !info.IsDir()
}

// Extract writes the UI below baseDir, which must be this user's alone: qs
// runs what is there, and shellfs trusts an existing extraction.
func Extract(baseDir string) (string, error) {
	if err := privateDir(baseDir); err != nil {
		return "", err
	}
	sub, err := fs.Sub(distFS, distRoot)
	if err != nil {
		return "", err
	}
	return shellfs.Extract(sub, baseDir)
}

func privateDir(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || int(st.Uid) != os.Getuid() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s must be a directory only this user can open (mode 0700); remove it", dir)
	}
	return nil
}

func Prune(baseDir, keep string) {
	shellfs.Prune(baseDir, keep)
}
