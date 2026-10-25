package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	dpaths "github.com/duxlabs-dev/dux-go/paths"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/window"
)

const daemonStartTimeout = 10 * time.Second

var appPaths = dpaths.New(ipc.AppName)

// socketPath finds the running daemon. With start set it launches one in the
// background first when none answers, so keybinds work with no window open.
func socketPath(start bool) (string, error) {
	if s := os.Getenv("DPLAYER_SOCKET"); s != "" {
		return s, nil
	}
	if err := requireRuntimeDir(); err != nil {
		return "", err
	}
	if s, err := ipc.FindRunningSocket(); err == nil {
		return s, nil
	}
	if !start {
		return "", errors.New("dplayer daemon is not running")
	}
	return startDaemon()
}

// requireRuntimeDir refuses to run without XDG_RUNTIME_DIR: dux-go would
// fall back to /tmp, where another user can plant the socket or the UI.
func requireRuntimeDir() error {
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		return errors.New("XDG_RUNTIME_DIR is not set; dplayer keeps its socket there and won't use /tmp")
	}
	return nil
}

// lockFile takes an flock on path (created 0600, never through a symlink);
// closing the file releases it.
func lockFile(path string, how int) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	if err := syscall.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	return f, nil
}

// startDaemon holds a lock while it spawns, so two keybind presses at once
// start one daemon.
func startDaemon() (string, error) {
	lock, err := lockFile(filepath.Join(appPaths.SocketDir(), ipc.AppName+".lock"), syscall.LOCK_EX)
	if err != nil {
		return "", err
	}
	defer lock.Close()

	if s, err := ipc.FindRunningSocket(); err == nil {
		return s, nil
	}

	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	logFile, err := daemonLog()
	if err != nil {
		return "", err
	}
	defer logFile.Close()
	cmd := exec.Command(self, "daemon")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start daemon: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	deadline := time.Now().Add(daemonStartTimeout)
	for time.Now().Before(deadline) {
		if s, err := ipc.FindRunningSocket(); err == nil {
			return s, nil
		}
		select {
		case err := <-exited:
			return "", fmt.Errorf("daemon exited during startup (%v); see %s", err, logFile.Name())
		case <-time.After(100 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("daemon did not come up within %s; see %s", daemonStartTimeout, logFile.Name())
}

func daemonLog() (*os.File, error) {
	dir, err := appPaths.StateDir()
	if err != nil {
		return nil, err
	}
	return window.OpenLog(filepath.Join(dir, "daemon.log"))
}

// call sends one request and decodes the result into out (if not nil).
func call(start bool, method string, params map[string]any, out any) error {
	sock, err := socketPath(start)
	if err != nil {
		return err
	}
	client, err := ipc.Dial(sock)
	if err != nil {
		return err
	}
	defer client.Close()
	resp, err := client.Call(ipc.Request{ID: 1, Method: method, Params: params})
	if err != nil {
		return err
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	if out == nil || resp.Result == nil {
		return nil
	}
	data, err := json.Marshal(*resp.Result)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
