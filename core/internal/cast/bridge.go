// Package cast plays the queue on a Chromecast through dux-cast's shared
// cast-helper (protocol 2). The helper runs as `cast-helper attach` only
// while something needs it: the device list on screen or a cast of ours.
package cast

import (
	"context"
	"errors"
	"net/netip"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	// Topic carries Status; it is in the default subscription.
	Topic = "cast"
	// TopicDevices carries the device list. Discovery runs while a client
	// subscribes to it, so the UI subscribes while its device list shows.
	TopicDevices = "castDevices"

	clientID    = "dplayer"
	clientName  = "Dux Player"
	minProtocol = 2
	// editProtocol added edit: queue changes reach a playing cast at once.
	editProtocol = 3

	// The cast gets this many queue entries from the current one on, and
	// the next window once it plays the last of them. Art is looked up for
	// the first few only (server covers are downloads).
	maxItems = 200
	artItems = 5

	readyTimeout = 5 * time.Second
	// While discovering, this often the daemon checks that someone still
	// looks at the device list, and restarts a helper that went away.
	watchEvery = 3 * time.Second
)

var (
	ErrNotInstalled = errors.New("cast-helper is not installed")
	ErrUpdate       = errors.New("cast-helper is too old for Dux Player; update cast-helper")
)

// errInsecure is ErrorInfo code insecureServer: a server track's stream URL
// carries the library's credentials and the receiver fetches it itself, so
// plain http is cast only from a server on the home network.
const errInsecure = "insecureServer"

type Player interface {
	SetRemote(ctx context.Context, r player.Remote, resume bool) error
	RemoteUpdate(ctx context.Context, uid uint64, pos float64, paused bool, volume float64) error
}

type Libraries interface {
	Get(id string) (source.Source, error)
	Stream(ctx context.Context, ref source.Ref) (player.Stream, error)
}

type Config struct {
	Ctx           context.Context
	Player        Player
	Libraries     Libraries
	Publish       func(topic string, data any)
	HasSubscriber func(topic string) bool
	// Find locates cast-helper; FindHelper when nil.
	Find func() string
}

// Status is what the UI shows: whether casting is possible, and the cast
// session, ours or another client's (the shell's Cast plugin).
type Status struct {
	Available    bool         `json:"available"`
	UpdateNeeded bool         `json:"updateNeeded"`
	Version      string       `json:"version,omitempty"`
	Casting      bool         `json:"casting"`
	Session      *SessionInfo `json:"session"`
	Error        *ErrorInfo   `json:"error,omitempty"`
}

type SessionInfo struct {
	Device    string  `json:"device"`
	Name      string  `json:"name"`
	Phase     string  `json:"phase"`
	Title     string  `json:"title,omitempty"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
	Paused    bool    `json:"paused"`
	Volume    float64 `json:"volume"`
	CanVolume bool    `json:"canVolume"`
	Owner     *owner  `json:"owner,omitempty"`
}

// ErrorInfo is the helper's error code; clients word it themselves.
type ErrorInfo struct {
	Code    string   `json:"code"`
	Detail  string   `json:"detail,omitempty"`
	Missing []string `json:"missing,omitempty"`
}

type Bridge struct {
	cfg Config

	mu          sync.Mutex
	proc        *proc
	ready       chan struct{}
	protocol    int
	version     string
	devices     []Device
	sess        *session
	discovering bool
	lastErr     *ErrorInfo
	// Our cast. started is set once the helper shows a session on our device;
	// sent is the queue uids in the cast's order, at the receiver's index in
	// it; gen discards stale queue builds; stale means items changed and a
	// receiver without edit gets them at its next track change.
	casting     bool
	started     bool
	device      string
	sent        []uint64
	gen         int
	pendingSeek float64
	items       []player.Item
	stale       bool
	at          int
	tickStop    chan struct{}
	// loop is the player's repeat all: the receiver starts over after its
	// last item, so the whole queue is sent when it fits.
	loop bool
}

func New(cfg Config) *Bridge {
	if cfg.Find == nil {
		cfg.Find = FindHelper
	}
	return &Bridge{cfg: cfg, pendingSeek: -1}
}

// ensure starts the attach if none runs. Called with mu held.
func (b *Bridge) ensure() error {
	if b.proc != nil {
		return nil
	}
	bin := b.cfg.Find()
	if bin == "" {
		return ErrNotInstalled
	}
	p, err := startHelper(b.cfg.Ctx, bin, b.onEvent)
	if err != nil {
		return err
	}
	b.proc, b.ready, b.protocol = p, make(chan struct{}), 0
	_ = p.send(map[string]any{"cmd": "hello", "client": clientID, "name": clientName})
	_ = p.send(map[string]any{"cmd": "check"})
	return nil
}

// releaseIfIdle ends the attach once nothing needs it. Called with mu held.
func (b *Bridge) releaseIfIdle() {
	if b.proc != nil && !b.discovering && !b.casting {
		b.proc.close()
		b.proc = nil
	}
}

func (b *Bridge) send(cmd map[string]any) {
	if b.proc == nil {
		return
	}
	if err := b.proc.send(cmd); err != nil {
		log.Warnf("cast %v: %v", cmd["cmd"], err)
	}
}

func (s *session) position(now time.Time) float64 {
	pos := s.Position
	if s.PlayerState == "PLAYING" && !s.Paused && s.PositionAt > 0 {
		pos += now.Sub(time.UnixMilli(s.PositionAt)).Seconds() * max(s.Rate, 1)
	}
	if s.Duration > 0 {
		pos = min(pos, s.Duration)
	}
	return max(pos, 0)
}

func (s *session) mine() bool { return s.Owner != nil && s.Owner.ID == clientID }

func (b *Bridge) onEvent(p *proc, ev event) {
	b.mu.Lock()
	if p != b.proc {
		b.mu.Unlock()
		return
	}
	detach := false
	var update func()
	var devices []Device
	publishDevices := false
	switch ev.Event {
	case "check":
		b.protocol, b.version = max(ev.Protocol, 1), ev.Version
		close(b.ready)
		if b.protocol >= minProtocol && b.discovering {
			b.send(map[string]any{"cmd": "discover", "on": true})
		}
	case "state":
		b.devices, b.sess = ev.Devices, ev.Session
		if b.discovering {
			devices, publishDevices = append([]Device{}, b.devices...), true
		}
		detach, update = b.followLocked()
	case "error":
		b.lastErr = &ErrorInfo{Code: ev.Code, Detail: ev.Detail, Missing: ev.Missing}
		if b.casting && !b.started {
			b.casting, detach = false, true
		}
	case "exit":
		// discovering stays set: watchDiscovery restarts the helper while
		// the device list is still on screen.
		b.proc = nil
		select {
		case <-b.ready:
		default:
			close(b.ready)
		}
		if b.casting {
			b.casting, b.started, detach = false, false, true
		}
	default:
		b.mu.Unlock()
		return
	}
	if detach {
		b.releaseIfIdle()
	}
	b.retick()
	b.mu.Unlock()
	if detach {
		if err := b.cfg.Player.SetRemote(b.cfg.Ctx, nil, false); err != nil {
			log.Warnf("cast ended: %v", err)
		}
	}
	if update != nil {
		update()
	}
	if publishDevices {
		b.cfg.Publish(TopicDevices, devices)
	}
	b.cfg.Publish(Topic, b.Status())
}

// followLocked matches our cast to the helper's session: it ends when the
// session is gone or another client took it over, else the player follows
// its track, position, pause and volume.
func (b *Bridge) followLocked() (detach bool, update func()) {
	if !b.casting {
		return false, nil
	}
	s := b.sess
	if !b.started {
		if s == nil || s.Device != b.device {
			return false, nil
		}
		b.started = true
	}
	if s == nil || (s.Owner != nil && !s.mine()) {
		b.casting, b.started = false, false
		return true, nil
	}
	b.device = s.Device
	if b.pendingSeek > 1 && s.Phase == "casting" && s.PlayerState == "PLAYING" {
		b.send(map[string]any{"cmd": "seek", "position": b.pendingSeek})
		b.pendingSeek = -1
	}
	update = b.updateLocked()
	if s.Index != b.at {
		b.at = s.Index
		b.refillLocked()
	}
	return false, update
}

// refillLocked runs when the receiver starts another item: a queue that
// changed meanwhile, or the last item of the window, sends the queue again
// from that item. Without edit that reloads the item the receiver just
// started, which is the least disruptive point.
func (b *Bridge) refillLocked() {
	if b.at < 0 || b.at >= len(b.sent) {
		return
	}
	i := slices.IndexFunc(b.items, func(it player.Item) bool { return it.UID == b.sent[b.at] })
	if i < 0 {
		return
	}
	last := b.at == len(b.sent)-1 && i+1 < len(b.items)
	if !b.stale && !last {
		return
	}
	b.queueLocked(i, -1)
}

// queueLocked sends the window from items[i]: an edit of the running
// queue where the helper has it, else a new queue starting at start.
func (b *Bridge) queueLocked(i int, start float64) {
	b.stale = false
	b.gen++
	cmd := "queue"
	if b.protocol >= editProtocol && len(b.sent) > 0 {
		cmd = "edit"
	} else {
		b.pendingSeek = start
	}
	go b.sendQueue(b.gen, cmd, b.device, slices.Clone(b.windowLocked(i)), b.items[i].UID, b.loop)
}

// windowLocked is what the receiver gets from items[i]: the next maxItems,
// or with repeat all the whole queue when it fits, so looping it starts over
// at the queue's first track.
func (b *Bridge) windowLocked(i int) []player.Item {
	if b.loop && len(b.items) <= maxItems {
		return b.items
	}
	return b.items[i:min(len(b.items), i+maxItems)]
}

// SetLoop mirrors repeat all to the receiver.
func (b *Bridge) SetLoop(on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.loop == on {
		return
	}
	b.loop = on
	if !b.casting || len(b.sent) == 0 {
		return
	}
	if b.protocol < editProtocol {
		// A new queue would restart the current track; wait for the next.
		b.stale = true
		return
	}
	b.send(map[string]any{"cmd": "repeat", "repeat": map[bool]string{true: "show", false: ""}[on]})
	if i := slices.IndexFunc(b.items, func(it player.Item) bool { return b.at >= 0 && b.at < len(b.sent) && it.UID == b.sent[b.at] }); i >= 0 {
		b.queueLocked(i, -1)
	}
}

// updateLocked captures what the player should follow; the returned func
// calls the player, which must happen without mu.
func (b *Bridge) updateLocked() func() {
	s := b.sess
	if s == nil {
		return nil
	}
	var uid uint64
	if s.Index >= 0 && s.Index < len(b.sent) {
		uid = b.sent[s.Index]
	}
	pos, paused := s.position(time.Now()), s.Paused || s.PlayerState == "PAUSED"
	vol := -1.0
	if s.CanVolume {
		vol = s.Volume * 100
	}
	return func() {
		if err := b.cfg.Player.RemoteUpdate(b.cfg.Ctx, uid, pos, paused, vol); err != nil {
			log.Debugf("cast follow: %v", err)
		}
	}
}

// Play casts the queue to a device from the current track and position,
// or moves a running cast there.
func (b *Bridge) Play(ctx context.Context, deviceID string) error {
	if deviceID == "" {
		return errors.New("device is required")
	}
	b.mu.Lock()
	if err := b.ensure(); err != nil {
		b.mu.Unlock()
		return err
	}
	ready := b.ready
	b.mu.Unlock()
	select {
	case <-ready:
	case <-time.After(readyTimeout):
		return errors.New("cast-helper didn't answer")
	case <-ctx.Done():
		return ctx.Err()
	}
	b.mu.Lock()
	if b.proc == nil {
		b.mu.Unlock()
		return errors.New("cast-helper attach exited; is it protocol 2 or newer?")
	}
	if b.protocol < minProtocol {
		b.releaseIfIdle()
		b.mu.Unlock()
		return ErrUpdate
	}
	if b.casting {
		b.device = deviceID
		b.send(map[string]any{"cmd": "move", "device": deviceID})
		b.mu.Unlock()
		return nil
	}
	b.casting, b.started, b.device, b.sent, b.lastErr, b.at = true, false, deviceID, nil, nil, -1
	b.mu.Unlock()
	if err := b.cfg.Player.SetRemote(ctx, b, false); err != nil {
		b.mu.Lock()
		b.casting = false
		b.releaseIfIdle()
		b.mu.Unlock()
		return err
	}
	b.cfg.Publish(Topic, b.Status())
	return nil
}

// retick runs the position ticker exactly while our cast plays. Called
// with mu held after anything that changes the session or the cast.
func (b *Bridge) retick() {
	s := b.sess
	on := b.casting && b.started && s != nil && s.PlayerState == "PLAYING" && !s.Paused
	switch {
	case on && b.tickStop == nil:
		b.tickStop = make(chan struct{})
		go b.tick(b.tickStop)
	case !on && b.tickStop != nil:
		close(b.tickStop)
		b.tickStop = nil
	}
}

// tick moves the player's position on while the cast plays: the receiver
// reports it only on changes.
func (b *Bridge) tick(stop <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-b.cfg.Ctx.Done():
			return
		case <-stop:
			return
		case <-t.C:
		}
		b.mu.Lock()
		update := b.updateLocked()
		b.mu.Unlock()
		if update != nil {
			update()
		}
	}
}

// StopCast ends our cast and plays on locally from its position; resume
// keeps it playing when it was.
func (b *Bridge) StopCast(ctx context.Context, resume bool) error {
	b.mu.Lock()
	if !b.casting {
		b.mu.Unlock()
		return nil
	}
	resume = resume && b.sess != nil && !b.sess.Paused && b.sess.PlayerState != "PAUSED"
	b.casting, b.started = false, false
	b.send(map[string]any{"cmd": "stop"})
	b.releaseIfIdle()
	b.retick()
	b.mu.Unlock()
	err := b.cfg.Player.SetRemote(ctx, nil, resume)
	b.cfg.Publish(Topic, b.Status())
	return err
}

// Devices is the last device list the helper reported.
func (b *Bridge) Devices() []Device {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Device{}, b.devices...)
}

func (b *Bridge) Status() Status {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := Status{Casting: b.casting, Version: b.version, Error: b.lastErr}
	st.UpdateNeeded = b.protocol > 0 && b.protocol < minProtocol
	st.Available = !st.UpdateNeeded && b.cfg.Find() != ""
	if s := b.sess; s != nil {
		st.Session = &SessionInfo{Device: s.Device, Name: s.Name, Phase: s.Phase, Title: s.Title,
			Position: s.position(time.Now()), Duration: s.Duration, Paused: s.Paused || s.PlayerState == "PAUSED",
			Volume: s.Volume, CanVolume: s.CanVolume, Owner: s.Owner}
	}
	return st
}

// Subscribed runs after every subscribe: discovery starts when someone
// subscribes to TopicDevices, and new subscribers get the status.
func (b *Bridge) Subscribed() {
	b.cfg.Publish(Topic, b.Status())
	if !b.cfg.HasSubscriber(TopicDevices) {
		return
	}
	b.cfg.Publish(TopicDevices, b.Devices())
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.discovering {
		return
	}
	if err := b.ensure(); err != nil {
		return
	}
	b.discovering = true
	if b.protocol >= minProtocol {
		b.send(map[string]any{"cmd": "discover", "on": true})
	}
	go b.watchDiscovery()
}

func (b *Bridge) watchDiscovery() {
	t := time.NewTicker(watchEvery)
	defer t.Stop()
	for {
		select {
		case <-b.cfg.Ctx.Done():
			return
		case <-t.C:
		}
		b.mu.Lock()
		if b.cfg.HasSubscriber(TopicDevices) {
			if b.proc == nil {
				// The helper went away; "check" turns discovery back on.
				_ = b.ensure()
			}
			b.mu.Unlock()
			continue
		}
		b.discovering = false
		b.send(map[string]any{"cmd": "discover", "on": false})
		b.releaseIfIdle()
		b.mu.Unlock()
		return
	}
}

// Load, SetPaused, Seek, SetVolume and Stop make the bridge the player's
// Remote while casting. They run on the player loop and never wait.

// Load with start < 0 is a queue edit while items[index] plays on.
func (b *Bridge) Load(items []player.Item, index int, start float64) {
	if index < 0 || index >= len(items) {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.items = items
	window := b.windowLocked(index)
	if start < 0 {
		have := b.sent
		if len(window) == 0 || window[0].UID == items[index].UID {
			have = b.sent[min(max(b.at, 0), len(b.sent)):]
		}
		if len(have) > 0 && slices.Equal(uids(window), have) {
			return
		}
		if b.protocol >= editProtocol || len(b.sent) == 0 {
			b.queueLocked(index, b.pendingSeek)
		} else {
			b.stale = true
		}
		return
	}
	if i := slices.Index(b.sent, items[index].UID); i >= 0 && start < 1 {
		b.send(map[string]any{"cmd": "jump", "index": i})
		return
	}
	b.queueLocked(index, start)
}

func uids(items []player.Item) []uint64 {
	out := make([]uint64, len(items))
	for i, it := range items {
		out[i] = it.UID
	}
	return out
}

// sendQueue resolves what the receiver opens: files as paths, server tracks
// as stream URLs. Those carry credentials (never log them) and the receiver
// fetches them itself, so a server on plain http isn't cast at all.
func (b *Bridge) sendQueue(gen int, cmd, device string, window []player.Item, current uint64, loop bool) {
	ctx, cancel := context.WithTimeout(b.cfg.Ctx, 30*time.Second)
	defer cancel()
	var items []map[string]any
	var sent []uint64
	insecure := false
	for i, it := range window {
		t := it.Track
		st, err := b.cfg.Libraries.Stream(ctx, t.Ref())
		if err != nil {
			log.Infof("cast: skipping %q: %v", t.Title, err)
			continue
		}
		item := map[string]any{"title": t.Title, "artist": t.Artist, "album": t.Album}
		switch {
		case filepath.IsAbs(st.URL):
			item["path"] = st.URL
		case strings.HasPrefix(st.URL, "https://") || homeNetwork(st.URL):
			item["url"] = st.URL
		default:
			insecure = true
			log.Infof("cast: skipping %q: its server is public and not reached over https", t.Title)
			continue
		}
		if i < artItems {
			if art := b.cover(ctx, t); art != "" {
				item["art"] = art
			}
		}
		items = append(items, item)
		sent = append(sent, it.UID)
	}
	b.mu.Lock()
	if gen != b.gen || !b.casting {
		b.mu.Unlock()
		return
	}
	if insecure {
		b.lastErr = &ErrorInfo{Code: errInsecure, Detail: "server tracks from a public server are cast only over https"}
	}
	if len(items) == 0 {
		// Nothing the receiver may open: the cast ends before it starts.
		b.casting, b.started = false, false
		b.releaseIfIdle()
		b.retick()
		b.mu.Unlock()
		if err := b.cfg.Player.SetRemote(b.cfg.Ctx, nil, false); err != nil {
			log.Warnf("cast ended: %v", err)
		}
		b.cfg.Publish(Topic, b.Status())
		return
	}
	index := max(slices.Index(sent, current), 0)
	b.sent, b.at = sent, index
	if cmd == "edit" {
		b.send(map[string]any{"cmd": "edit", "items": items, "index": index})
	} else {
		b.send(map[string]any{"cmd": "queue", "device": device, "items": items, "index": index, "loop": loop})
	}
	b.mu.Unlock()
	if insecure {
		b.cfg.Publish(Topic, b.Status())
	}
}

// homeNetwork is a plain-http URL whose host is on the home network: a
// private (RFC 1918 or IPv6 ULA), link-local or loopback address, or a
// .local name. Names aren't resolved.
func homeNetwork(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
	}
	return strings.HasSuffix(host, ".local") || host == "localhost"
}

func (b *Bridge) cover(ctx context.Context, t source.Track) string {
	if t.CoverID == "" {
		return ""
	}
	src, err := b.cfg.Libraries.Get(t.Library)
	if err != nil {
		return ""
	}
	c, ok := src.(source.CoverArt)
	if !ok {
		return ""
	}
	path, err := c.Cover(ctx, t.CoverID, 600)
	if err != nil {
		return ""
	}
	return path
}

func (b *Bridge) SetPaused(paused bool) {
	cmd := "resume"
	if paused {
		cmd = "pause"
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.send(map[string]any{"cmd": cmd})
}

func (b *Bridge) Seek(pos float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pendingSeek = -1
	b.send(map[string]any{"cmd": "seek", "position": pos})
}

func (b *Bridge) SetVolume(level float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.send(map[string]any{"cmd": "volume", "level": min(max(level/100, 0), 1)})
}

// Stop is the player's stop (or a cleared queue) while casting: the cast
// ends and the player is local again.
func (b *Bridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.casting, b.started = false, false
	b.send(map[string]any{"cmd": "stop"})
	b.releaseIfIdle()
	b.retick()
	go func() { b.cfg.Publish(Topic, b.Status()) }()
}
