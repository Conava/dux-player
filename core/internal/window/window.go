// Package window runs the QML UI as a child process of the daemon. The UI
// attaches to the daemon's socket and exits when its window closes; playback
// stays in the daemon.
package window

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/shellembed"
)

const (
	AppID = "com.marlonkranz.player"
	// Bounds Wait after qs exits; processes it forks can keep its output
	// pipes open.
	waitDelay = 2 * time.Second
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

	mu   sync.Mutex
	proc *os.Process
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
	defer l.mu.Unlock()
	if l.proc != nil {
		return false, nil
	}
	if env := os.Getenv("DPLAYER_SHELL_DIR"); dir != "" && shellembed.Available() && filepath.Clean(dir) != filepath.Clean(env) {
		return false, errors.New("the running daemon opens only its own UI; quit it (dplayer quit) and start it with -c to use another")
	}
	dir, embedded, err := l.ResolveDir(dir)
	if err != nil {
		return false, err
	}
	logFile, err := OpenLog(l.cfg.LogPath)
	if err != nil {
		return false, err
	}
	cmd := exec.Command("qs", "-p", dir)
	cmd.Env = uiEnv(os.Environ(), l.cfg.Socket, embedded)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.WaitDelay = waitDelay
	// The UI is useless without the daemon; it goes when the daemon goes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err := player.StartLocked(cmd); err != nil {
		logFile.Close()
		return false, fmt.Errorf("start the UI (qs): %w", err)
	}
	l.proc = cmd.Process
	log.Infof("UI started (pid=%d, dir=%s)", cmd.Process.Pid, dir)
	go func() {
		err := cmd.Wait()
		logFile.Close()
		l.mu.Lock()
		l.proc = nil
		l.mu.Unlock()
		log.Infof("UI exited (%v)", err)
		if l.cfg.OnExit != nil {
			l.cfg.OnExit()
		}
	}()
	return true, nil
}

// maxLog is where a log is rotated: at open, a bigger one becomes .1 (the
// previous .1 is dropped).
const maxLog = 2 << 20

// OpenLog opens a log for appending, rotated once past maxLog.
func OpenLog(path string) (*os.File, error) {
	if info, err := os.Stat(path); err == nil && info.Size() > maxLog {
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

// Stop ends the UI, if one runs.
func (l *Launcher) Stop() {
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
