package cast

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/logtext"
)

// FindHelper returns dux-cast's cast-helper: on PATH, else where the shell's
// Cast plugin installs it. Empty when it isn't installed, or when
// DPLAYER_CAST=off keeps a sandbox run off the real network's devices.
func FindHelper() string {
	if os.Getenv("DPLAYER_CAST") == "off" {
		return ""
	}
	if p, err := exec.LookPath("cast-helper"); err == nil {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".local", "share", "dms", "cast", "bin", "cast-helper")
	if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0 {
		return p
	}
	return ""
}

// event is one line from the helper. Only the fields the player uses.
type event struct {
	Event    string   `json:"event"`
	Devices  []Device `json:"devices"`
	Session  *session `json:"session"`
	Protocol int      `json:"protocol"`
	Version  string   `json:"version"`
	Code     string   `json:"code"`
	Detail   string   `json:"detail"`
	Missing  []string `json:"missing"`
}

type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Model    string `json:"model,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

type owner struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type session struct {
	Kind        string  `json:"kind"`
	Device      string  `json:"device"`
	Name        string  `json:"name"`
	Phase       string  `json:"phase"`
	Volume      float64 `json:"volume"`
	CanVolume   bool    `json:"canVolume"`
	Owner       *owner  `json:"owner"`
	Title       string  `json:"title"`
	PlayerState string  `json:"playerState"`
	Position    float64 `json:"position"`
	PositionAt  int64   `json:"positionAt"`
	Duration    float64 `json:"duration"`
	Rate        float64 `json:"rate"`
	Index       int     `json:"index"`
	Paused      bool    `json:"paused"`
	// Queue holds the helper's per-entry status and source. The source
	// carries stream tokens: it stays out of SessionInfo and the logs.
	Queue []queueEntry `json:"queue"`
}

type queueEntry struct {
	Status string `json:"status"`
	Source string `json:"source"`
}

// proc is one running `cast-helper attach`. Commands go through a buffered
// channel so callers on the player loop never wait for the pipe.
type proc struct {
	out    chan []byte
	done   chan struct{}
	mu     sync.Mutex
	closed bool
}

const maxLine = 4 << 20

func startHelper(ctx context.Context, bin string, onEvent func(*proc, event)) (*proc, error) {
	cmd := exec.CommandContext(ctx, bin, "attach")
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	errDone := make(chan struct{})
	go func() {
		defer close(errDone)
		logStderr(stderr)
	}()
	p := &proc{out: make(chan []byte, 64), done: make(chan struct{})}
	go func() {
		defer stdin.Close()
		for {
			select {
			case line, ok := <-p.out:
				if !ok {
					return
				}
				if _, err := stdin.Write(line); err != nil {
					return
				}
			case <-p.done:
				return
			}
		}
	}()
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), maxLine)
		for sc.Scan() {
			var ev event
			if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.Event != "" {
				onEvent(p, ev)
			}
		}
		_, _ = io.Copy(io.Discard, stdout)
		<-errDone
		err := cmd.Wait()
		close(p.done)
		p.close()
		if err != nil && ctx.Err() == nil {
			log.Infof("cast-helper attach ended: %v", err)
		}
		onEvent(p, event{Event: "exit"})
	}()
	return p, nil
}

// maxStderrLines caps what one helper run may add to the log.
const maxStderrLines = 20

// logStderr logs the helper's own diagnostics, URLs stripped, and drains the pipe.
func logStderr(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4<<10), 64<<10)
	n := 0
	for sc.Scan() {
		if n++; n <= maxStderrLines {
			log.Infof("cast-helper: %s", logtext.Clean(sc.Text()))
		}
	}
	if err := sc.Err(); err != nil {
		log.Warnf("cast-helper: stderr reading stopped: %v", err)
	}
	_, _ = io.Copy(io.Discard, r)
	if n > maxStderrLines {
		log.Infof("cast-helper: %d more lines not logged", n-maxStderrLines)
	}
}

var errHelperBusy = errors.New("cast-helper isn't reading its commands")

// send queues one command. Stream URLs ride in commands: never log them.
func (p *proc) send(cmd map[string]any) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("cast-helper is not running")
	}
	select {
	case p.out <- append(data, '\n'):
		return nil
	default:
		return errHelperBusy
	}
}

// close ends the attach: it exits when its stdin closes.
func (p *proc) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.out)
	}
}
