package player

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/logtext"
	"github.com/Conava/dux-player/core/internal/player/engine"
)

// Variables so tests can shorten them.
var (
	commandTimeout = 5 * time.Second
	startTimeout   = 5 * time.Second
	// readyTimeout guards a new mpv whose core never comes up: it is
	// killed and started once more.
	readyTimeout = 15 * time.Second
)

const (
	// mpv lines are small; a metadata blob with an embedded lyric tag is the
	// largest thing it sends.
	maxLine = 4 << 20
)

// message is one line from mpv: a reply when Event is empty, an event
// otherwise.
type message struct {
	RequestID int64           `json:"request_id"`
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`

	Event     string `json:"event"`
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Reason    string `json:"reason"`
	FileError string `json:"file_error"`
	EntryID   int64  `json:"playlist_entry_id"`

	// log-message events
	Prefix string `json:"prefix"`
	Level  string `json:"level"`
	Text   string `json:"text"`
}

func parseMessage(line []byte) (message, error) {
	var m message
	if err := json.Unmarshal(line, &m); err != nil {
		return message{}, fmt.Errorf("decode mpv message: %w", err)
	}
	if m.Event == "" && m.RequestID == 0 {
		return message{}, errors.New("mpv message is neither a reply nor an event")
	}
	return m, nil
}

func (m message) isReply() bool { return m.Event == "" }

// replyError maps mpv's "error" field to a Go error; "success" is nil.
func (m message) replyError() error {
	if m.Error == "" || m.Error == "success" {
		return nil
	}
	return fmt.Errorf("mpv: %s", m.Error)
}

// conn is a JSON IPC connection to one mpv process.
type conn struct {
	nc     net.Conn
	writeM sync.Mutex
	nextID atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan message
	closed  bool

	// Events queue without bound: a blocked reader would starve replies,
	// and a dropped start-file would desync the queue.
	events []message
	notify chan struct{}
	done   chan struct{}
	err    error
}

func newConn(nc net.Conn) *conn {
	c := &conn{
		nc:      nc,
		pending: make(map[int64]chan message),
		notify:  make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *conn) readLoop() {
	sc := bufio.NewScanner(c.nc)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		m, err := parseMessage(sc.Bytes())
		if err != nil {
			continue
		}
		if m.isReply() {
			c.deliver(m)
			continue
		}
		c.mu.Lock()
		c.events = append(c.events, m)
		c.mu.Unlock()
		select {
		case c.notify <- struct{}{}:
		default:
		}
	}
	err := sc.Err()
	if err == nil {
		err = errors.New("mpv closed the connection")
	}
	c.shutdown(err)
}

func (c *conn) deliver(m message) {
	c.mu.Lock()
	ch, ok := c.pending[m.RequestID]
	delete(c.pending, m.RequestID)
	c.mu.Unlock()
	if ok {
		ch <- m
	}
}

// takeEvents returns and clears the queued events.
func (c *conn) takeEvents() []message {
	c.mu.Lock()
	defer c.mu.Unlock()
	evs := c.events
	c.events = nil
	return evs
}

func (c *conn) shutdown(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	c.err = err
	c.nc.Close()
	close(c.done)
}

func (c *conn) close() { c.shutdown(errors.New("connection closed")) }

// command sends args as an mpv command and waits for its reply.
func (c *conn) command(ctx context.Context, args ...any) (json.RawMessage, error) {
	return c.commandWithin(ctx, commandTimeout, args...)
}

func (c *conn) commandWithin(ctx context.Context, timeout time.Duration, args ...any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	id := c.nextID.Add(1)
	line, err := json.Marshal(map[string]any{"command": args, "request_id": id})
	if err != nil {
		return nil, err
	}
	ch := make(chan message, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("mpv %v: %w", args[0], c.err)
	}
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	c.writeM.Lock()
	_ = c.nc.SetWriteDeadline(time.Now().Add(commandTimeout))
	_, err = c.nc.Write(append(line, '\n'))
	c.writeM.Unlock()
	if err != nil {
		return nil, fmt.Errorf("mpv %v: write: %w", args[0], err)
	}

	select {
	case m := <-ch:
		return m.Data, m.replyError()
	case <-c.done:
		return nil, fmt.Errorf("mpv %v: %w", args[0], c.err)
	case <-ctx.Done():
		return nil, fmt.Errorf("mpv %v: %w", args[0], ctx.Err())
	}
}

// process is a running mpv with its IPC connection, or the built-in
// engine (cmd nil) answering the same IPC in-process.
type process struct {
	cmd    *exec.Cmd
	conn   *conn
	socket string
	exited chan struct{}
}

func (p *process) builtin() bool { return p.cmd == nil }

// builtinOutput is where the built-in engine plays; tests replace it.
var builtinOutput = func(ctx context.Context) (engine.Output, error) { return engine.Pulse(ctx) }

// startBuiltin starts the built-in engine; env is what mpv would get
// (PIPEWIRE_PROPS).
func startBuiltin(ctx context.Context, env []string) (*process, error) {
	out, err := builtinOutput(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoSound, err)
	}
	a, b := net.Pipe()
	p := &process{exited: make(chan struct{})}
	go func() {
		defer close(p.exited)
		if err := engine.Serve(a, out, env); err != nil {
			log.Debugf("built-in player stopped: %v", err)
		}
	}()
	p.conn = newConn(b)
	if _, err := p.conn.command(ctx, "get_property", "mpv-version"); err != nil {
		p.kill()
		return nil, fmt.Errorf("built-in player: %w", err)
	}
	return p, nil
}

func baseArgs(socket string) []string {
	return []string{
		"--idle=yes",
		"--no-video",
		"--audio-display=no",
		"--no-terminal",
		"--no-config",
		"--load-scripts=no",
		"--ytdl=no",
		"--input-default-bindings=no",
		"--prefetch-playlist=yes",
		"--network-timeout=20",
		// FFmpeg's default for https is no certificate check; stream URLs
		// carry credentials.
		"--tls-verify=yes",
		// A playlist or .ref file must not make mpv open other URLs or files.
		"--access-references=no",
		"--audio-client-name=dplayer",
		"--input-ipc-server=" + socket,
	}
}

func startProcess(ctx context.Context, binary, socket string, extra, env []string) (*process, error) {
	_ = os.Remove(socket)
	args := append(baseArgs(socket), extra...)
	cmd := exec.Command(binary, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	if err := StartLocked(cmd); err != nil {
		return nil, fmt.Errorf("start mpv: %w", err)
	}
	p := &process{cmd: cmd, socket: socket, exited: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.exited)
	}()

	nc, err := dialSocket(ctx, socket, p.exited)
	if err != nil {
		p.kill()
		return nil, err
	}
	p.conn = newConn(nc)
	if _, err := p.conn.commandWithin(ctx, readyTimeout, "get_property", "mpv-version"); err != nil {
		p.kill()
		return nil, fmt.Errorf("mpv started but did not answer within %s: %w", readyTimeout, err)
	}
	return p, nil
}

// pipewireReachable says whether a PipeWire server answers where libpipewire
// looks for it. mpv 0.41 lists audio devices through every backend, and its
// PipeWire backend sometimes deadlocks mpv's core when it can't connect, so
// without a server the device list is never asked for.
func pipewireReachable() bool {
	name := cmp.Or(os.Getenv("PIPEWIRE_REMOTE"), "pipewire-0")
	if !filepath.IsAbs(name) {
		name = filepath.Join(cmp.Or(os.Getenv("PIPEWIRE_RUNTIME_DIR"), os.Getenv("XDG_RUNTIME_DIR")), name)
	}
	c, err := net.DialTimeout("unix", name, time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func dialSocket(ctx context.Context, socket string, exited <-chan struct{}) (net.Conn, error) {
	deadline := time.Now().Add(startTimeout)
	for {
		nc, err := net.DialTimeout("unix", socket, time.Second)
		if err == nil {
			return nc, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("mpv ipc socket did not come up: %w", err)
		}
		select {
		case <-exited:
			return nil, errors.New("mpv exited during startup")
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Pdeathsig fires when the thread that forked the child exits, not the
// process, and the Go runtime retires idle threads. Children started
// through StartLocked are forked from one locked thread that never exits.
var spawner = sync.OnceValue(func() chan func() {
	ch := make(chan func())
	go func() {
		runtime.LockOSThread()
		for f := range ch {
			f()
		}
	}()
	return ch
})

// StartLocked is cmd.Start for a child that sets Pdeathsig.
func StartLocked(cmd *exec.Cmd) error {
	errc := make(chan error, 1)
	spawner() <- func() { errc <- cmd.Start() }
	return <-errc
}

func (p *process) kill() {
	if p.conn != nil {
		p.conn.close()
	}
	if p.builtin() {
		select {
		case <-p.exited:
		case <-time.After(2 * time.Second):
			log.Warnf("built-in player did not stop")
		}
		return
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.exited:
	case <-time.After(2 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.exited
	}
	_ = os.Remove(p.socket)
}

// MinMPV is the oldest mpv the player is tested with.
var MinMPV = [2]int{0, 38}

var mpvVersionPattern = regexp.MustCompile(`^mpv v?(\d+)\.(\d+)`)

// CheckMPV runs `mpv --version` and returns its version, or why it can't be used.
func CheckMPV(ctx context.Context, binary string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	var out limitedBuffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s --version: %w", binary, err)
	}
	line, _, _ := strings.Cut(out.String(), "\n")
	m := mpvVersionPattern.FindStringSubmatch(line)
	if m == nil {
		return "", fmt.Errorf("%s --version printed %q, not an mpv version", binary, logtext.Clean(line))
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	version := m[1] + "." + m[2]
	if major < MinMPV[0] || major == MinMPV[0] && minor < MinMPV[1] {
		return version, fmt.Errorf("mpv %s is too old; dplayer needs %d.%d or newer", version, MinMPV[0], MinMPV[1])
	}
	return version, nil
}

// limitedBuffer keeps the first 4 KiB written to it.
type limitedBuffer struct{ strings.Builder }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := 4<<10 - b.Len(); room > 0 {
		b.Builder.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
