// Package window runs the QML UI as a child process of the daemon. The UI
// attaches to the daemon's socket and exits when its window closes; playback
// stays in the daemon.
package window

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	"github.com/duxlabs-dev/dux-go/qtbase"

	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/shellembed"
)

const (
	AppID = "io.github.conava.DuxPlayer"
	// Bounds Wait after the UI exits; processes it forks can keep its output
	// pipes open.
	waitDelay = 2 * time.Second
	// How long Stop gives the UI to save what is pending after the daemon's socket closed, before it is terminated.
	saveGrace = 500 * time.Millisecond
)

type Config struct {
	// Socket is the daemon socket the UI connects to (DPLAYER_SOCKET).
	Socket     string
	RuntimeDir string
	LogPath    string
	// OnExit runs after the UI process has ended, whatever the reason.
	OnExit func()
}

type Launcher struct {
	cfg Config

	mu       sync.Mutex
	proc     *os.Process
	starting bool // a Start is between its check and having a process
}

func New(cfg Config) *Launcher { return &Launcher{cfg: cfg} }

func (l *Launcher) Running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.proc != nil
}

// ResolveDir picks the UI tree: an explicit dir, then DPLAYER_SHELL_DIR, then
// the tree embedded in release builds.
func (l *Launcher) ResolveDir(dir string) (string, bool, error) {
	if dir == "" {
		dir = os.Getenv("DPLAYER_SHELL_DIR")
	}
	if dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", false, err
		}
		if info, err := os.Stat(filepath.Join(abs, "shell.qml")); err != nil || info.IsDir() {
			return "", false, fmt.Errorf("%s has no shell.qml", abs)
		}
		// Without it the UI only exits with status 255.
		if _, err := os.Stat(filepath.Join(abs, "DCommon", "Widgets", "DIcon.qml")); err != nil {
			return "", false, fmt.Errorf("%s has no dux-qml-common: run git submodule update --init", abs)
		}
		return abs, false, nil
	}
	if !shellembed.Available() {
		return "", false, errors.New("this dplayer build has no embedded UI; pass -c <quickshell dir> or set DPLAYER_SHELL_DIR")
	}
	base := filepath.Join(l.cfg.RuntimeDir, "dplayer-shell")
	extracted, err := shellembed.Extract(base)
	if err != nil {
		return "", false, fmt.Errorf("extract embedded UI: %w", err)
	}
	shellembed.Prune(base, extracted)
	return extracted, true, nil
}

// Start launches the UI unless one is running; it reports whether it started
// one. dir comes from a client and runs as code, so a release build only
// takes the tree it was started with (DPLAYER_SHELL_DIR); a dev build, which
// has no UI of its own, takes any.
func (l *Launcher) Start(dir string) (bool, error) {
	l.mu.Lock()
	if l.proc != nil || l.starting {
		l.mu.Unlock()
		return false, nil
	}
	l.starting = true
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		l.starting = false
		l.mu.Unlock()
	}()
	if env := os.Getenv("DPLAYER_SHELL_DIR"); dir != "" && shellembed.Available() && filepath.Clean(dir) != filepath.Clean(env) {
		return false, errors.New("the running daemon opens only its own UI; quit it (dplayer quit) and start it with -c to use another")
	}
	dir, embedded, err := l.ResolveDir(dir)
	if err != nil {
		return false, err
	}
	logFile, err := newRotatingLog(l.cfg.LogPath, maxLog)
	if err != nil {
		return false, err
	}
	cmd := qtbase.Command(context.Background(), "--log-times", "--no-color", "--log-rules", "qt.qpa.wayland.textinput=false", "-p", dir)
	cmd.Env = uiEnv(os.Environ(), l.cfg.Socket, embedded)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.WaitDelay = waitDelay
	// The UI is useless without the daemon; it goes when the daemon goes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err := player.StartLocked(cmd); err != nil {
		logFile.Close()
		return false, fmt.Errorf("start the UI: %w", err)
	}
	l.mu.Lock()
	l.proc = cmd.Process
	l.mu.Unlock()
	log.Infof("UI started (pid=%d, dir=%s)", cmd.Process.Pid, dir)
	go func() {
		err := cmd.Wait()
		logFile.Close()
		l.mu.Lock()
		l.proc = nil
		l.mu.Unlock()
		var failed *exec.ExitError
		if errors.As(err, &failed) && failed.ExitCode() > 0 {
			log.Errorf("the UI failed (%v); the reason is in %s", err, l.cfg.LogPath)
		} else {
			log.Infof("UI exited (%v)", err)
		}
		if l.cfg.OnExit != nil {
			l.cfg.OnExit()
		}
	}()
	return true, nil
}

// maxLog is where a log is rotated: a bigger one becomes .1 (the previous
// .1 is dropped), at open and, for the UI's log, while it runs.
const maxLog = 2 << 20

// rotatingLog is the UI's log: writes go through this process (not straight
// to the file) so the file can be swapped when it passes limit.
type rotatingLog struct {
	mu    sync.Mutex
	path  string
	limit int64
	f     *os.File
	size  int64
}

func newRotatingLog(path string, limit int64) (*rotatingLog, error) {
	r := &rotatingLog{path: path, limit: limit}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rotatingLog) open() error {
	f, err := OpenLog(r.path)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.f, r.size = f, info.Size()
	return nil
}

func (r *rotatingLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size > 0 && r.size+int64(len(p)) > r.limit {
		// On failure the old file stays open and keeps taking output.
		if err := r.rotate(); err != nil {
			log.Warnf("ui log: can't rotate %s: %v", r.path, err)
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotatingLog) rotate() error {
	if err := os.Rename(r.path, r.path+".1"); err != nil {
		return err
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	r.f.Close()
	r.f, r.size = f, 0
	return nil
}

func (r *rotatingLog) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f.Close()
}

// OpenLog opens a log for appending, rotated once past maxLog.
func OpenLog(path string) (*os.File, error) {
	if info, err := os.Stat(path); err == nil && info.Size() > maxLog {
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

// Stop ends the UI, if one runs. The UI saves its pending settings when the
// daemon's socket closes, which happens just before this, so it gets saveGrace
// before the signal.
func (l *Launcher) Stop() {
	for deadline := time.Now().Add(saveGrace); l.Running() && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.proc != nil {
		_ = l.proc.Signal(syscall.SIGTERM)
	}
}

// uiEnv mirrors what dux-go shellapp gives its UI. exec keeps the last
// duplicate key, so these override the inherited environment.
func uiEnv(env []string, socket string, embedded bool) []string {
	env = append(env, "DPLAYER_SOCKET="+socket, "QS_APP_ID="+AppID)
	if os.Getenv("QT_QPA_PLATFORMTHEME") == "" {
		env = append(env, "QT_QPA_PLATFORMTHEME=gtk3")
	}
	if os.Getenv("QT_QPA_PLATFORMTHEME_QT6") == "" {
		env = append(env, "QT_QPA_PLATFORMTHEME_QT6=gtk3")
	}
	if os.Getenv("QT_QPA_PLATFORM") == "" {
		env = append(env, "QT_QPA_PLATFORM=wayland;xcb")
	}
	// The extracted tree is read-only and never changes; a dev tree reloads
	// on save.
	if embedded && os.Getenv("DPLAYER_DISABLE_HOT_RELOAD") == "" {
		env = append(env, "DPLAYER_DISABLE_HOT_RELOAD=1")
	}
	return env
}
