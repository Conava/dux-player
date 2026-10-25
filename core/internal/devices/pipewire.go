package devices

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/atomicfile"
	"github.com/Conava/dux-player/core/internal/player"
)

// Several outputs: a combine-stream sink (dplayer_multi) fans out to one
// filter-chain per output, whose builtin delay trims it. Each is its own
// `pipewire -c` child, so its nodes vanish with it and no PipeWire or
// WirePlumber config is written.
const (
	combineNode = "dplayer_multi"
	outPrefix   = "dplayer_out."
	// MultiDevice is what mpv plays to while several outputs are on.
	MultiDevice = "pipewire/" + combineNode
	// maxDelay sizes each output's delay buffer, in seconds.
	maxDelay    = 2.0
	execTimeout = 5 * time.Second
	maxOutput   = 64 << 10
)

// WaitEnv keeps mpv's stream waiting, never moved to the default sink, when
// its target is gone: the combine sink during a child restart, or a single
// output set to pause on disconnect.
var WaitEnv = []string{"PIPEWIRE_PROPS={ node.dont-fallback = true node.linger = true }"}

// Sink names go into a config file, so only plain node-name characters pass.
var validSink = regexp.MustCompile(`^[A-Za-z0-9_.:@+-]{1,200}$`)

const confHead = `context.properties = { log.level = 2 }
context.spa-libs = {
    audio.convert.* = audioconvert/libspa-audioconvert
    support.*       = support/libspa-support
}
context.modules = [
  { name = libpipewire-module-protocol-native }
  { name = libpipewire-module-client-node }
  { name = libpipewire-module-adapter }
`

// The stream rule needs the media class and an anchored name: matches are a
// regex search, and a looser rule also matches the combine's own streams,
// which then multiply until the child runs out of file descriptors.
const combineConf = confHead + `  { name = libpipewire-module-combine-stream
    args = {
      combine.mode = sink
      node.name = "` + combineNode + `"
      node.description = "Dux Player (several outputs)"
      combine.latency-compensate = true
      combine.props = { audio.position = [ FL FR ] media.class = "Audio/Sink/Internal" state.restore-props = false }
      stream.props = {
        media.class = "Stream/Output/Audio/Internal"
        stream.dont-remix = true node.dont-fallback = true node.linger = true node.dont-move = true
        state.restore-props = false
      }
      stream.rules = [
        { matches = [ { media.class = "Audio/Sink/Internal" node.name = "~^dplayer_out\\.[^.]*$" } ]
          actions = { create-stream = { } } }
      ]
    }
  }
]
`

// token names an output's nodes; no dots, so the combine's rule matches it.
func token(sink string) string {
	h := fnv.New32a()
	h.Write([]byte(sink))
	return fmt.Sprintf("%08x", h.Sum32())
}

func outputConf(sink string, delay float64) string {
	return confHead + `  { name = libpipewire-module-filter-chain
    args = {
      node.description = "Dux Player output"
      audio.channels = 2
      audio.position = [ FL FR ]
      filter.graph = {
        nodes = [
          { type = builtin label = delay name = dl config = { "max-delay" = ` + secs(maxDelay) + ` } control = { "Delay (s)" = ` + secs(delay) + ` } }
        ]
      }
      capture.props = { node.name = "` + outPrefix + token(sink) + `" media.class = "Audio/Sink/Internal" state.restore-props = false }
      playback.props = {
        node.name = "` + outPrefix + token(sink) + `.play" media.class = "Stream/Output/Audio/Internal" target.object = "` + sink + `"
        node.dont-fallback = true node.linger = true node.dont-move = true stream.dont-remix = true
        state.restore-props = false
      }
    }
  }
]
`
}

func secs(s float64) string { return strconv.FormatFloat(s, 'f', 4, 64) }

// normalize turns the user's per-output delays (ms) into the delays to set
// (s): the earliest output gets none, so the whole set adds as little
// latency as possible.
func normalize(sinks []string, delays map[string]int) map[string]float64 {
	low := -1
	for _, s := range sinks {
		if d := delays[s]; low < 0 || d < low {
			low = d
		}
	}
	out := make(map[string]float64, len(sinks))
	for _, s := range sinks {
		out[s] = float64(delays[s]-low) / 1000
	}
	return out
}

// multi is the running set of children. Only the service loop touches it.
type multi struct {
	dir     string
	combine *child
	outs    map[string]*child
	applied map[string]float64
}

func newMulti(dir string) *multi {
	return &multi{dir: dir, outs: map[string]*child{}, applied: map[string]float64{}}
}

func (m *multi) running() bool { return m.combine != nil }

// set makes the children match sinks with delays (seconds), starting the
// combine first and waiting until its node exists.
func (m *multi) set(ctx context.Context, sinks []string, delays map[string]float64) error {
	if m.combine == nil {
		c, err := startChild(m.dir, "multi", combineConf)
		if err != nil {
			return err
		}
		m.combine = c
		if err := waitNode(ctx, combineNode); err != nil {
			m.stop()
			return err
		}
	}
	want := map[string]bool{}
	for _, s := range sinks {
		want[s] = true
	}
	for s, c := range m.outs {
		if !want[s] {
			c.close()
			delete(m.outs, s)
			delete(m.applied, s)
		}
	}
	for _, s := range sinks {
		d := delays[s]
		c, ok := m.outs[s]
		if !ok {
			c, err := startChild(m.dir, "out-"+token(s), outputConf(s, d))
			if err != nil {
				return err
			}
			m.outs[s], m.applied[s] = c, d
			continue
		}
		if m.applied[s] == d {
			continue
		}
		// The file is what a restarted child reads; pw-cli changes the live one.
		if err := c.rewrite(outputConf(s, d)); err != nil {
			return err
		}
		m.applied[s] = d
		if err := setDelay(ctx, outPrefix+token(s), d); err != nil {
			log.Warnf("output delay: %v", err)
		}
	}
	return nil
}

func (m *multi) stop() {
	for s, c := range m.outs {
		c.close()
		delete(m.outs, s)
		delete(m.applied, s)
	}
	if m.combine != nil {
		m.combine.close()
		m.combine = nil
	}
}

// pwRun runs a PipeWire tool with a timeout and bounded output.
func pwRun(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	var out limited
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

var errNoServer = errors.New("PipeWire isn't running")

// nodeID looks a node up by name; pw-cli exits 0 even when it's missing.
func nodeID(ctx context.Context, name string) (string, error) {
	out, err := pwRun(ctx, "pw-cli", "info", name)
	if strings.Contains(out, "failed to connect") {
		return "", errNoServer
	}
	if err != nil {
		return "", fmt.Errorf("pw-cli: %w", err)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	id, ok := strings.CutPrefix(first, "id: ")
	if !ok {
		return "", fmt.Errorf("no PipeWire node %s", name)
	}
	return strings.TrimSpace(id), nil
}

func waitNode(ctx context.Context, name string) error {
	deadline := time.Now().Add(execTimeout)
	for {
		_, err := nodeID(ctx, name)
		if err == nil || errors.Is(err, errNoServer) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("PipeWire didn't create %s: %w", name, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// setDelay changes a running output's delay. The node id changes with every
// child restart, so it is looked up each time.
func setDelay(ctx context.Context, node string, delay float64) error {
	id, err := nodeID(ctx, node)
	if err != nil {
		return err
	}
	return setParams(ctx, id, `{ params = [ "dl:Delay (s)" `+secs(delay)+` ] }`)
}

// setParams sets a node's Props; pw-cli exits 0 on errors too.
func setParams(ctx context.Context, id, params string) error {
	out, err := pwRun(ctx, "pw-cli", "set-param", id, "Props", params)
	if err == nil && strings.Contains(out, "Error:") {
		err = errors.New("failed")
	}
	if err != nil {
		return fmt.Errorf("pw-cli set-param: %v: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// child is one `pipewire -c <conf>` process, restarted when it dies. gen
// counts the processes started, so a restart shows.
type child struct {
	conf string
	stop chan struct{}
	done chan struct{}
	gen  atomic.Uint64
}

func startChild(dir, name, conf string) (*child, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	c := &child{conf: filepath.Join(dir, name+".conf"), stop: make(chan struct{}), done: make(chan struct{})}
	if err := c.rewrite(conf); err != nil {
		return nil, err
	}
	cmd, exited, err := c.spawn()
	if err != nil {
		return nil, err
	}
	go c.supervise(cmd, exited)
	return c, nil
}

func (c *child) rewrite(conf string) error {
	return atomicfile.WriteFile(c.conf, []byte(conf), 0o600)
}

func (c *child) spawn() (*exec.Cmd, chan error, error) {
	cmd := exec.Command("pipewire", "-c", c.conf)
	var tail limited
	cmd.Stdout, cmd.Stderr = &tail, &tail
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	if err := player.StartLocked(cmd); err != nil {
		return nil, nil, fmt.Errorf("start pipewire: %w", err)
	}
	c.gen.Add(1)
	exited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if t := strings.TrimSpace(tail.String()); t != "" {
			err = fmt.Errorf("%v: %s", err, t)
		}
		exited <- err
	}()
	return cmd, exited, nil
}

// supervise restarts the child until close, giving up after 3 crashes in a
// minute (a broken PipeWire would otherwise respawn forever).
func (c *child) supervise(cmd *exec.Cmd, exited chan error) {
	defer close(c.done)
	var crashes []time.Time
	for {
		select {
		case <-c.stop:
			_ = cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-exited:
			case <-time.After(2 * time.Second):
				_ = cmd.Process.Kill()
				<-exited
			}
			return
		case err := <-exited:
			log.Warnf("%s exited: %v", filepath.Base(c.conf), err)
		}
		now := time.Now()
		recent := crashes[:0]
		for _, t := range crashes {
			if now.Sub(t) < time.Minute {
				recent = append(recent, t)
			}
		}
		if crashes = append(recent, now); len(crashes) > 3 {
			log.Errorf("%s keeps crashing; giving up", filepath.Base(c.conf))
			<-c.stop
			return
		}
		select {
		case <-c.stop:
			return
		case <-time.After(time.Second):
		}
		var err error
		if cmd, exited, err = c.spawn(); err != nil {
			log.Errorf("%v", err)
			<-c.stop
			return
		}
	}
}

func (c *child) close() {
	close(c.stop)
	<-c.done
	_ = os.Remove(c.conf)
}

// limited keeps the first maxOutput bytes written to it.
type limited struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *limited) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := maxOutput - l.buf.Len(); room > 0 {
		l.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (l *limited) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

var errNoPipeWire = errors.New("several outputs at once need PipeWire (pipewire and pw-cli)")

func pipewireAvailable() bool {
	for _, bin := range []string{"pipewire", "pw-cli", "pw-play"} {
		if _, err := exec.LookPath(bin); err != nil {
			return false
		}
	}
	return true
}
