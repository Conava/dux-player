// Package engine plays audio without mpv: it answers the part of mpv's
// JSON IPC the player uses, decodes FLAC, MP3, Ogg Vorbis, Ogg Opus, WAV and
// AIFF in Go, and plays through the PulseAudio protocol (pipewire-pulse on
// PipeWire desktops).
package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version is what get_property mpv-version answers.
const Version = "builtin"

var (
	// writeTimeout bounds one reply or event to the player.
	writeTimeout = 5 * time.Second
	// tick is how often time-pos is reported while playing.
	tick = 250 * time.Millisecond
)

const maxLine = 1 << 20

type engine struct {
	conn  net.Conn
	out   Output
	props map[string]string
	wait  bool
	ctx   context.Context
	wmu   sync.Mutex

	mu        sync.Mutex
	observed  map[string]bool
	paused    bool
	volume    float64
	volumeMax float64
	muted     bool
	rgMode    string
	gapless   string
	loop      bool
	device    string
	cur       *entry
	next      []*entry
	nextID    int64
	// seek is a pending seek in seconds (-1: none), reopen a pending move
	// to another device; pos is the last reported position.
	seek   float64
	reopen bool
	pos    float64
	// ranOut is set when the last entry ended by itself.
	ranOut bool
	// gen changes with anything the pump must look at again at once; wctx
	// ends with it, cutting a write that waits for the server short.
	gen     uint64
	wctx    context.Context
	wcancel context.CancelFunc
	wake    chan struct{}
}

type entry struct {
	id       int64
	url      string
	start    float64
	fallback *float64
	end      float64

	ctx    context.Context
	cancel context.CancelFunc
	ready  chan struct{}
	// Set once ready is closed.
	in   input
	dec  decoder
	tags tagGain
	err  error
}

// Serve answers the player on conn and plays to out until conn closes or
// out is lost; env is the backend's extra environment (PIPEWIRE_PROPS).
// It closes both.
func Serve(conn net.Conn, out Output, env []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := &engine{conn: conn, out: out, ctx: ctx, observed: map[string]bool{}, volume: 100, volumeMax: 130,
		rgMode: "no", gapless: "weak", device: "auto", seek: -1, wake: make(chan struct{}, 1)}
	e.props, e.wait = streamProps(env)
	e.wctx, e.wcancel = context.WithCancel(ctx)
	defer func() {
		conn.Close()
		out.Close()
		e.mu.Lock()
		e.dropAll()
		e.mu.Unlock()
	}()

	pumped := make(chan struct{})
	go func() {
		defer close(pumped)
		e.pump()
	}()
	defer func() { cancel(); <-pumped }()
	go e.watchSinks()

	lines := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(conn)
		sc.Buffer(make([]byte, 64<<10), maxLine)
		for sc.Scan() {
			select {
			case lines <- append([]byte(nil), sc.Bytes()...):
			case <-ctx.Done():
				return
			}
		}
		readErr <- cmpErr(sc.Err(), errors.New("the player closed the connection"))
	}()
	for {
		select {
		case line := <-lines:
			e.serve(line)
		case err := <-readErr:
			return err
		case <-out.Lost():
			return errLost
		}
	}
}

func cmpErr(err, def error) error {
	if err != nil {
		return err
	}
	return def
}

// streamProps turns PIPEWIRE_PROPS={ k = v ... } into stream properties;
// wait is node.dont-fallback.
func streamProps(env []string) (map[string]string, bool) {
	props := map[string]string{}
	for _, kv := range env {
		v, ok := strings.CutPrefix(kv, "PIPEWIRE_PROPS=")
		if !ok {
			continue
		}
		f := strings.Fields(strings.Trim(strings.TrimSpace(v), "{}"))
		for i := 0; i+2 < len(f); i += 3 {
			if f[i+1] == "=" {
				props[f[i]] = f[i+2]
			}
		}
	}
	return props, props["node.dont-fallback"] == "true"
}

// send writes one line to the player; a player that doesn't read ends the
// connection.
func (e *engine) send(v any) {
	line, err := json.Marshal(v)
	if err != nil {
		return
	}
	e.wmu.Lock()
	defer e.wmu.Unlock()
	_ = e.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if _, err := e.conn.Write(append(line, '\n')); err != nil {
		e.conn.Close()
	}
}

func (e *engine) event(name string, fields map[string]any) {
	m := map[string]any{"event": name}
	for k, v := range fields {
		m[k] = v
	}
	e.send(m)
}

// property reports a change of an observed property; value nil leaves the
// data out, as mpv does for an unavailable one.
func (e *engine) property(name string, value any) {
	e.mu.Lock()
	ok := e.observed[name]
	e.mu.Unlock()
	if !ok {
		return
	}
	m := map[string]any{"event": "property-change", "name": name}
	if value != nil {
		m["data"] = value
	}
	e.send(m)
}

func (e *engine) endFile(en *entry, reason string, fileErr error) {
	f := map[string]any{"playlist_entry_id": en.id, "reason": reason}
	if fileErr != nil {
		f["file_error"] = fileErr.Error()
	}
	e.event("end-file", f)
}

// bump makes the pump look again at once; mu held.
func (e *engine) bump() {
	e.gen++
	e.wcancel()
	e.wctx, e.wcancel = context.WithCancel(e.ctx)
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *engine) serve(line []byte) {
	var req struct {
		Command   []json.RawMessage `json:"command"`
		RequestID int64             `json:"request_id"`
	}
	if json.Unmarshal(line, &req) != nil || len(req.Command) == 0 {
		return
	}
	var name string
	_ = json.Unmarshal(req.Command[0], &name)
	data, err := e.command(name, req.Command[1:])
	reply := map[string]any{"request_id": req.RequestID, "error": "success"}
	if err != nil {
		reply["error"] = err.Error()
	}
	if data != nil {
		reply["data"] = data
	}
	e.send(reply)
}

var (
	errUnavailable = errors.New("property unavailable")
	errBadArgs     = errors.New("invalid parameter")
)

func str(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return strings.Trim(string(raw), `"`)
	}
	return s
}

func num(raw json.RawMessage) (float64, bool) {
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f, true
	}
	f, err := strconv.ParseFloat(str(raw), 64)
	return f, err == nil
}

func (e *engine) command(name string, args []json.RawMessage) (any, error) {
	switch name {
	case "get_property":
		if len(args) < 1 {
			return nil, errBadArgs
		}
		return e.get(str(args[0]))
	case "set_property":
		if len(args) < 2 {
			return nil, errBadArgs
		}
		return nil, e.set(str(args[0]), args[1])
	case "observe_property":
		if len(args) < 2 {
			return nil, errBadArgs
		}
		prop := str(args[1])
		e.mu.Lock()
		e.observed[prop] = true
		e.mu.Unlock()
		if v, err := e.get(prop); err == nil {
			e.property(prop, v)
		} else {
			e.property(prop, nil)
		}
		return nil, nil
	case "request_log_messages":
		return nil, nil
	case "loadfile":
		return e.loadfile(args)
	case "seek":
		return nil, e.seekCmd(args)
	case "stop":
		e.stop()
		return nil, nil
	case "playlist-clear":
		e.mu.Lock()
		dropped := e.next
		e.next = nil
		e.mu.Unlock()
		for _, en := range dropped {
			en.close()
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unsupported command %q", name)
}

func (e *engine) get(name string) (any, error) {
	if name == "audio-device-list" {
		return e.deviceList()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	switch name {
	case "mpv-version":
		return Version, nil
	case "pause":
		return e.paused, nil
	case "volume":
		return e.volume, nil
	case "mute":
		return e.muted, nil
	case "audio-device":
		return e.device, nil
	case "time-pos":
		if e.cur == nil || !e.cur.loaded() {
			return nil, errUnavailable
		}
		return e.pos, nil
	case "duration":
		if d := e.cur.duration(); d > 0 {
			return d, nil
		}
		return nil, errUnavailable
	case "seekable":
		if e.cur == nil || !e.cur.loaded() {
			return nil, errUnavailable
		}
		return e.cur.in.seekable, nil
	}
	return nil, errUnavailable
}

func (e *engine) set(name string, raw json.RawMessage) error {
	switch name {
	case "pause":
		var v bool
		if json.Unmarshal(raw, &v) != nil {
			return errBadArgs
		}
		e.mu.Lock()
		changed := v != e.paused
		e.paused = v
		e.bump()
		e.mu.Unlock()
		if changed {
			e.property("pause", v)
		}
	case "volume", "volume-max":
		v, ok := num(raw)
		if !ok || v < 0 || v > 1000 {
			return errBadArgs
		}
		e.mu.Lock()
		if name == "volume-max" {
			e.volumeMax = max(v, 100)
		}
		e.volume = min(e.volume, e.volumeMax)
		if name == "volume" {
			e.volume = min(v, e.volumeMax)
		}
		vol := e.volume
		e.mu.Unlock()
		e.property("volume", vol)
	case "mute":
		var v bool
		if json.Unmarshal(raw, &v) != nil {
			return errBadArgs
		}
		e.mu.Lock()
		e.muted = v
		e.mu.Unlock()
	case "replaygain":
		e.mu.Lock()
		e.rgMode = str(raw)
		e.mu.Unlock()
	case "gapless-audio":
		e.mu.Lock()
		e.gapless = str(raw)
		e.mu.Unlock()
	case "loop-file":
		e.mu.Lock()
		e.loop = str(raw) == "inf" || str(raw) == "yes"
		e.mu.Unlock()
	case "audio-device":
		e.mu.Lock()
		if dev := str(raw); dev != e.device {
			e.device = dev
			e.reopen = true
			e.bump()
		}
		e.mu.Unlock()
	case "speed":
		if v, ok := num(raw); !ok || v != 1 {
			return errUnavailable
		}
	case "af":
		if str(raw) != "" {
			return errUnavailable
		}
	case "file-local-options/end":
		end := 0.0
		if s := str(raw); s != "none" {
			v, ok := num(raw)
			if !ok || v < 0 {
				return errBadArgs
			}
			end = v
		}
		e.mu.Lock()
		if e.cur != nil {
			e.cur.end = end
		}
		e.mu.Unlock()
	case "file-local-options/replaygain-fallback":
		v, ok := num(raw)
		if !ok {
			return errBadArgs
		}
		e.mu.Lock()
		if e.cur != nil {
			e.cur.fallback = &v
		}
		e.mu.Unlock()
	default:
		return errUnavailable
	}
	return nil
}

// loadfile url replace|append [index] [options]
func (e *engine) loadfile(args []json.RawMessage) (any, error) {
	if len(args) < 1 {
		return nil, errBadArgs
	}
	mode := "replace"
	if len(args) > 1 {
		mode = str(args[1])
	}
	en := &entry{url: str(args[0]), ready: make(chan struct{})}
	if len(args) > 3 {
		for _, opt := range strings.Split(str(args[3]), ",") {
			k, v, _ := strings.Cut(opt, "=")
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			switch k {
			case "start":
				en.start = max(f, 0)
			case "replaygain-fallback":
				en.fallback = &f
			}
		}
	}
	e.mu.Lock()
	e.nextID++
	en.id = e.nextID
	en.ctx, en.cancel = context.WithCancel(e.ctx)
	var dropped []*entry
	switch mode {
	case "replace":
		if e.cur != nil {
			dropped = append(dropped, e.cur)
		}
		dropped = append(dropped, e.next...)
		e.cur, e.next, e.ranOut = en, nil, false
		e.seek = -1
		e.bump()
	case "append", "append-play":
		// An entry the player lines up after one that just ended plays, as
		// mpv would have had it in its playlist by then.
		if e.cur == nil && (mode == "append-play" || e.ranOut) {
			e.cur, e.ranOut = en, false
		} else {
			e.next = append(e.next, en)
		}
		e.bump()
	default:
		e.mu.Unlock()
		return nil, errBadArgs
	}
	e.mu.Unlock()
	go en.prepare()
	for i, old := range dropped {
		if i == 0 && mode == "replace" {
			e.endFile(old, "stop", nil)
		}
		old.close()
	}
	return map[string]int64{"playlist_entry_id": en.id}, nil
}

func (e *engine) seekCmd(args []json.RawMessage) error {
	if len(args) < 1 {
		return errBadArgs
	}
	pos, ok := num(args[0])
	if !ok {
		return errBadArgs
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cur == nil || !e.cur.loaded() || e.cur.err != nil {
		return errors.New("error running command")
	}
	if !e.cur.in.seekable {
		return errNotSeekable
	}
	if len(args) > 1 && str(args[1]) == "relative" {
		pos += e.pos
	}
	e.seek = max(pos, 0)
	e.bump()
	return nil
}

func (e *engine) stop() {
	e.mu.Lock()
	old := e.cur
	dropped := e.next
	e.cur, e.next, e.ranOut = nil, nil, false
	e.seek = -1
	e.bump()
	e.mu.Unlock()
	if old != nil {
		e.endFile(old, "stop", nil)
		old.close()
	}
	for _, en := range dropped {
		en.close()
	}
}

// dropAll closes every entry; mu held.
func (e *engine) dropAll() {
	if e.cur != nil {
		e.cur.close()
	}
	for _, en := range e.next {
		en.close()
	}
	e.cur, e.next = nil, nil
}

func (e *engine) deviceList() (any, error) {
	sinks, err := e.out.Sinks()
	if err != nil {
		return nil, err
	}
	type dev struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	list := []dev{{Name: "auto", Description: "Autoselect device"}}
	for _, s := range sinks {
		list = append(list, dev{Name: "pipewire/" + s.Name, Description: s.Description})
	}
	return list, nil
}

func (e *engine) watchSinks() {
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.out.Changed():
			if list, err := e.deviceList(); err == nil {
				e.property("audio-device-list", list)
			}
		}
	}
}

// sinkName is the server's sink for an mpv audio-device; "" is the default.
func sinkName(device string) string {
	if device == "auto" || device == "" {
		return ""
	}
	if _, name, ok := strings.Cut(device, "/"); ok {
		return name
	}
	return device
}

// prepare opens the entry's file and decoder off the command loop, and
// moves it to its start.
func (en *entry) prepare() {
	defer close(en.ready)
	in, err := openInput(en.ctx, en.url)
	if err != nil {
		en.err = err
		return
	}
	en.in = in
	err = safely(func() error {
		dec, err := newDecoder(in)
		if err != nil {
			return err
		}
		en.dec = dec
		return seekTo(dec, int64(en.start*float64(dec.format().rate)), 0)
	})
	if err != nil {
		en.err = err
		return
	}
	if in.path != "" {
		en.tags = readTags(in.path)
	}
}

func (en *entry) loaded() bool {
	select {
	case <-en.ready:
		return en.err == nil
	default:
		return false
	}
}

func (en *entry) duration() float64 {
	if en == nil || !en.loaded() {
		return 0
	}
	return float64(en.dec.length()) / float64(en.dec.format().rate)
}

func (en *entry) close() {
	en.cancel()
	go func() {
		<-en.ready
		if en.in.close != nil {
			_ = en.in.close()
		}
	}()
}

// seekTo moves d to frame, reading on where it can't seek; from is where
// it is now.
func seekTo(d decoder, frame, from int64) error {
	if frame == from {
		return nil
	}
	err := d.seek(frame)
	if errors.Is(err, errNotSeekable) && frame > from {
		err = skipFrames(d, frame-from)
	}
	return err
}

// safely runs decoder code, which parses hostile input, so that a panic
// ends the track instead of the daemon.
func safely(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("decoder failed: %v", r)
		}
	}()
	return f()
}

// clamp keeps a sample in range after gain.
func clamp(v float64) float32 {
	return float32(math.Max(-1, math.Min(1, v)))
}
