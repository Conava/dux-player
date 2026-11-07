package main

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/window"
)

const (
	logCheckEvery = 10 * time.Minute
	// maxDaemonLog is where a running daemon rotates its log; window.OpenLog
	// only rotates when it opens one.
	maxDaemonLog = 8 << 20
)

// watchDaemonLog keeps daemon.log from growing for as long as the daemon
// runs. It does nothing when stderr is not that file (a terminal, journald).
func watchDaemonLog(ctx context.Context, stateDir string) {
	path := filepath.Join(stateDir, "daemon.log")
	t := time.NewTicker(logCheckEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if err := rotateDaemonLog(path, maxDaemonLog); err != nil {
			log.Warnf("log: can't rotate %s: %v", path, err)
		}
	}
}

// rotateDaemonLog moves a log past limit to path.1 and points stdout and
// stderr at a fresh file.
func rotateDaemonLog(path string, limit int64) error {
	cur, err := os.Stderr.Stat()
	if err != nil || !cur.Mode().IsRegular() || cur.Size() <= limit {
		return nil
	}
	if onDisk, err := os.Stat(path); err != nil || !os.SameFile(cur, onDisk) {
		return nil
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return err
	}
	f, err := window.OpenLog(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, fd := range []int{1, 2} {
		if err := syscall.Dup3(int(f.Fd()), fd, 0); err != nil {
			return err
		}
	}
	return nil
}
