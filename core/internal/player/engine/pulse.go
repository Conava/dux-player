package engine

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jfreymuth/pulse/proto"
)

// requestTimeout bounds every request to the sound server, and each write
// to its socket.
var requestTimeout = 5 * time.Second

// latency is how much audio the server buffers ahead; a volume change
// takes this long to be heard.
const latency = 0.2

var errLost = errors.New("the sound server went away")

// pulseOut is a connection to the PulseAudio server, which PipeWire
// desktops serve through pipewire-pulse.
type pulseOut struct {
	c    *proto.Client
	conn net.Conn

	mu      sync.Mutex
	streams map[uint32]*pulseStream
	// early holds data requests that came before their stream's reply was
	// handled.
	early   map[uint32]int64
	changed chan struct{}
	lost    chan struct{}
	once    sync.Once
}

// Pulse connects to the sound server.
func Pulse(ctx context.Context) (Output, error) {
	path, err := pulseSocket()
	if err != nil {
		return nil, err
	}
	nc, err := (&net.Dialer{Timeout: requestTimeout}).DialContext(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("no sound server: %w", err)
	}
	o := &pulseOut{conn: nc, streams: map[uint32]*pulseStream{}, early: map[uint32]int64{},
		changed: make(chan struct{}, 1), lost: make(chan struct{})}
	o.c = &proto.Client{}
	o.c.Open(&pulseConn{Conn: nc, lose: o.lose})
	o.c.SetTimeout(requestTimeout)
	o.c.Callback = o.callback
	if err := o.handshake(); err != nil {
		o.Close()
		return nil, fmt.Errorf("sound server: %w", err)
	}
	return o, nil
}

func pulseSocket() (string, error) {
	if s := strings.Fields(os.Getenv("PULSE_SERVER")); len(s) > 0 {
		path := strings.TrimPrefix(s[0], "unix:")
		if !filepath.IsAbs(path) {
			return "", errors.New("PULSE_SERVER: only a local socket is supported")
		}
		return path, nil
	}
	if dir := os.Getenv("PULSE_RUNTIME_PATH"); dir != "" {
		return filepath.Join(dir, "native"), nil
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "pulse", "native"), nil
	}
	return "", errors.New("no sound server: XDG_RUNTIME_DIR is not set")
}

func (o *pulseOut) handshake() error {
	cookie := make([]byte, 256)
	path := os.Getenv("PULSE_COOKIE")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "pulse", "cookie")
	}
	if b, err := os.ReadFile(path); err == nil && len(b) == 256 {
		cookie = b
	}
	var auth proto.AuthReply
	if err := o.c.Request(&proto.Auth{Version: o.c.Version(), Cookie: cookie}, &auth); err != nil {
		return err
	}
	o.c.SetVersion(auth.Version)
	props := proto.PropList{
		"application.name":      proto.PropListString("dplayer"),
		"application.id":        proto.PropListString("io.github.conava.DuxPlayer"),
		"application.icon_name": proto.PropListString("io.github.conava.DuxPlayer"),
	}
	if err := o.c.Request(&proto.SetClientName{Props: props}, &proto.SetClientNameReply{}); err != nil {
		return err
	}
	return o.c.Request(&proto.Subscribe{Mask: proto.SubscriptionMaskSink | proto.SubscriptionMaskServer}, nil)
}

// callback runs on the client's read loop and must not block.
func (o *pulseOut) callback(msg any) {
	switch m := msg.(type) {
	case *proto.Request:
		o.mu.Lock()
		s := o.streams[m.StreamIndex]
		if s == nil {
			o.early[m.StreamIndex] += int64(m.Length)
		}
		o.mu.Unlock()
		if s != nil {
			s.grant(int64(m.Length), nil)
		}
	case *proto.PlaybackStreamKilled:
		o.mu.Lock()
		s := o.streams[m.StreamIndex]
		o.mu.Unlock()
		if s != nil {
			s.grant(0, errors.New("the output was removed"))
		}
	case *proto.SubscribeEvent:
		if f := m.Event.GetFacility(); f == proto.EventSink || f == proto.EventServer {
			select {
			case o.changed <- struct{}{}:
			default:
			}
		}
	case *proto.ConnectionClosed:
		o.lose()
	}
}

func (o *pulseOut) lose() {
	o.once.Do(func() {
		close(o.lost)
		o.conn.Close()
	})
}

func (o *pulseOut) Changed() <-chan struct{} { return o.changed }
func (o *pulseOut) Lost() <-chan struct{}    { return o.lost }
func (o *pulseOut) Close()                   { o.lose() }

func (o *pulseOut) Sinks() ([]Sink, error) {
	var list proto.GetSinkInfoListReply
	if err := o.c.Request(&proto.GetSinkInfoList{}, &list); err != nil {
		return nil, err
	}
	out := make([]Sink, 0, len(list))
	for _, s := range list {
		out = append(out, Sink{Name: s.SinkName, Description: s.Device})
	}
	return out, nil
}

// channelMaps follow WAV and FLAC channel order.
var channelMaps = [][]byte{
	1: {proto.ChannelMono},
	2: {proto.ChannelLeft, proto.ChannelRight},
	3: {proto.ChannelLeft, proto.ChannelRight, proto.ChannelCenter},
	4: {proto.ChannelLeft, proto.ChannelRight, proto.ChannelRearLeft, proto.ChannelRearRight},
	5: {proto.ChannelLeft, proto.ChannelRight, proto.ChannelCenter, proto.ChannelRearLeft, proto.ChannelRearRight},
	6: {proto.ChannelLeft, proto.ChannelRight, proto.ChannelCenter, proto.ChannelLFE, proto.ChannelRearLeft, proto.ChannelRearRight},
	7: {proto.ChannelLeft, proto.ChannelRight, proto.ChannelCenter, proto.ChannelLFE, proto.ChannelRearCenter, proto.ChannelLeftSide, proto.ChannelRightSide},
	8: {proto.ChannelLeft, proto.ChannelRight, proto.ChannelCenter, proto.ChannelLFE, proto.ChannelRearLeft, proto.ChannelRearRight, proto.ChannelLeftSide, proto.ChannelRightSide},
}

func (o *pulseOut) Open(f format, sink string, props map[string]string, wait bool, paused bool) (Stream, error) {
	if !f.valid() {
		return nil, errUnsupportedFm
	}
	fb := 4 * f.channels
	pl := proto.PropList{
		"media.name": proto.PropListString("Music"),
		"media.role": proto.PropListString("music"),
	}
	for k, v := range props {
		pl[k] = proto.PropListString(v)
	}
	vol := make(proto.ChannelVolumes, f.channels)
	for i := range vol {
		vol[i] = 0x10000
	}
	req := proto.CreatePlaybackStream{
		SampleSpec:            proto.SampleSpec{Format: proto.FormatFloat32LE, Channels: byte(f.channels), Rate: uint32(f.rate)},
		ChannelMap:            channelMaps[f.channels],
		SinkIndex:             proto.Undefined,
		SinkName:              sink,
		BufferMaxLength:       proto.Undefined,
		Corked:                paused,
		BufferTargetLength:    uint32(latency*float64(f.rate)) * uint32(fb),
		BufferPrebufferLength: proto.Undefined,
		BufferMinimumRequest:  proto.Undefined,
		ChannelVolumes:        vol,
		NoMove:                wait,
		AdjustLatency:         true,
		Properties:            pl,
	}
	var reply proto.CreatePlaybackStreamReply
	if err := o.c.Request(&req, &reply); err != nil {
		return nil, err
	}
	s := &pulseStream{o: o, index: reply.StreamIndex, f: f, fb: fb, more: make(chan struct{}, 1)}
	o.mu.Lock()
	o.streams[s.index] = s
	early := o.early[s.index]
	delete(o.early, s.index)
	o.mu.Unlock()
	s.grant(int64(reply.Missing)+early, nil)
	return s, nil
}

type pulseStream struct {
	o     *pulseOut
	index uint32
	f     format
	fb    int

	mu   sync.Mutex
	want int64
	err  error
	more chan struct{}
	buf  []byte
	// drained is set when a drain finished; a flush starts a new drainGen.
	drained  bool
	drainGen int
}

// grant adds bytes the server asked for, or sets the error that ends the
// stream.
func (s *pulseStream) grant(n int64, err error) {
	s.mu.Lock()
	s.want += n
	if err != nil {
		s.err = err
	}
	s.mu.Unlock()
	select {
	case s.more <- struct{}{}:
	default:
	}
}

func (s *pulseStream) Write(ctx context.Context, frames []float32) (int, error) {
	need := len(frames) * 4
	if cap(s.buf) < need {
		s.buf = make([]byte, need)
	}
	data := s.buf[:need]
	for i, v := range frames {
		binary.LittleEndian.PutUint32(data[4*i:], math.Float32bits(v))
	}
	sent := 0
	for sent < need {
		s.mu.Lock()
		err := s.err
		n := min(s.want, int64(need-sent))
		n -= n % int64(s.fb)
		s.want -= n
		s.mu.Unlock()
		if err != nil {
			return sent / 4, err
		}
		if n == 0 {
			select {
			case <-s.more:
			case <-ctx.Done():
				return sent / 4, ctx.Err()
			case <-s.o.lost:
				return sent / 4, errLost
			}
			continue
		}
		if err := s.o.c.Send(s.index, data[sent:sent+int(n)]); err != nil {
			return sent / 4, err
		}
		sent += int(n)
	}
	return sent / 4, nil
}

func (s *pulseStream) Position() (int64, int64, error) {
	now := time.Now()
	var r proto.GetPlaybackLatencyReply
	err := s.o.c.Request(&proto.GetPlaybackLatency{StreamIndex: s.index,
		Time: proto.Time{Seconds: uint32(now.Unix()), Microseconds: uint32(now.Nanosecond() / 1000)}}, &r)
	if err != nil {
		return 0, 0, err
	}
	fb := int64(s.fb)
	written, read := r.WriteIndex/fb, r.ReadIndex/fb
	s.mu.Lock()
	drained := s.drained
	s.mu.Unlock()
	if drained || read >= written {
		return written, written, nil
	}
	behind := int64(float64(r.Latency) * float64(s.f.rate) / 1e6)
	return written, max(read-behind, 0), nil
}

func (s *pulseStream) Flush() error {
	s.mu.Lock()
	s.drained = false
	s.drainGen++
	s.mu.Unlock()
	return s.o.c.Request(&proto.FlushPlaybackStream{StreamIndex: s.index}, nil)
}

func (s *pulseStream) SetPaused(paused bool) error {
	return s.o.c.Request(&proto.CorkPlaybackStream{StreamIndex: s.index, Corked: paused}, nil)
}

// Drain doesn't wait: the server answers only once the stream played out,
// which a pause holds up. pipewire-pulse keeps the last partial buffer
// until asked to drain.
func (s *pulseStream) Drain() {
	s.mu.Lock()
	gen := s.drainGen
	s.mu.Unlock()
	go func() {
		if s.o.c.Request(&proto.DrainPlaybackStream{StreamIndex: s.index}, nil) != nil {
			return
		}
		s.mu.Lock()
		s.drained = s.drained || gen == s.drainGen
		s.mu.Unlock()
	}()
}

func (s *pulseStream) Close() {
	_ = s.o.c.Request(&proto.DeletePlaybackStream{StreamIndex: s.index}, nil)
	s.o.mu.Lock()
	delete(s.o.streams, s.index)
	s.o.mu.Unlock()
}

// pulseConn bounds every write to the server and reports a dead connection.
type pulseConn struct {
	net.Conn
	lose func()
}

func (c *pulseConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if err != nil {
		c.lose()
	}
	return n, err
}

func (c *pulseConn) Write(p []byte) (int, error) {
	_ = c.SetWriteDeadline(time.Now().Add(requestTimeout))
	n, err := c.Conn.Write(p)
	if err != nil {
		c.lose()
	}
	return n, err
}
