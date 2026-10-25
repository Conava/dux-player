// Package player drives mpv over its JSON IPC socket. The daemon owns the
// queue; mpv only ever holds the current track and the one after it, which
// is enough for gapless playback and prefetching.
package player

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	TopicStatus   = "player"
	TopicPosition = "position"
	TopicQueue    = "queue"
	TopicError    = "playerError"
	// TopicPlayed is a play that counts: half the track or 4 minutes heard.
	// Not in the default subscription; history and scrobbling follow it.
	TopicPlayed = "played"
	// TopicAudioDevices carries mpv's audio-device-list on every change.
	TopicAudioDevices = "mpvAudioDevices"

	maxRestarts   = 3
	restartWindow = time.Minute

	saveDelay = 2 * time.Second
	// resolveTimeout bounds one stream lookup, which may wait on a server.
	resolveTimeout = 30 * time.Second
	// While playing, the saved position may lag this far behind.
	positionSaveEvery = 15.0
)

type State string

const (
	StatePlaying State = "playing"
	StatePaused  State = "paused"
	StateStopped State = "stopped"
)

// Stream is what mpv opens for a track. GainDB is a measured loudness
// correction, used only when the file has no ReplayGain tags; Silence is
// the file's measured leading and trailing silence.
type Stream struct {
	URL     string
	GainDB  *float64
	Silence loudness.Silence
}

// Resolver turns a track into what mpv should open.
type Resolver interface {
	Stream(ctx context.Context, ref source.Ref) (Stream, error)
}

// Prober is optional on a Resolver: it measures, in the background, the
// loudness of a track that has no gain at all, and calls done with it.
type Prober interface {
	// ctx ends when the track is no longer current or next.
	Probe(ctx context.Context, ref source.Ref, done func(gain float64))
}

// Availability is optional on a Resolver: the queue passes over the tracks
// it says to skip (library offline, file gone). Neither method may block.
type Availability interface {
	Skip(ref source.Ref) bool
	// Failed is told that mpv couldn't open ref. It checks the track's
	// library and calls recheck once the answer is in.
	Failed(ref source.Ref, recheck func())
}

type Config struct {
	Binary    string
	Socket    string
	ExtraArgs []string
	Resolver  Resolver
	Publish   func(topic string, data any)
	// Continue returns tracks to play after last when the queue runs out
	// and continuing is on; mode is ContinueRandom or ContinueSimilar.
	// Tracks in queued are already in the queue and should not come back.
	Continue func(ctx context.Context, mode string, last source.Track, queued map[source.Ref]bool) ([]source.Track, error)
	// Refeed rebuilds the feed of a restored queue that was a shuffle over
	// whole libraries (PlayFeed), from its origin.
	Refeed func(ctx context.Context, origin Origin) (Feed, error)
	// StatePath is where the queue, position, volume and modes are saved;
	// empty disables saving.
	StatePath string
	Options   Options
	// System is the default output's volume, for VolumeSystem; nil when
	// the system has no mixer we can drive.
	System SystemVolume
}

// SystemVolume is the default audio output's volume in percent and its
// mute. Each call runs a command, so the player never calls it on its loop.
type SystemVolume interface {
	Get(ctx context.Context) (percent float64, muted bool, err error)
	SetVolume(ctx context.Context, percent float64) error
	SetMute(ctx context.Context, muted bool) error
}

// Options are the playback settings the player applies to mpv.
type Options struct {
	// Previous restarts the track instead of going back once it has played
	// this many seconds.
	PreviousThreshold float64
	VolumeMax         float64
	// Gapless is mpv's gapless-audio: weak, yes or no.
	Gapless string
	// ReplayGain is mpv's replaygain: track, album or no.
	ReplayGain string
	// Speed is mpv's speed; mpv keeps the pitch (scaletempo2).
	Speed float64
	// Crossfade is how many seconds each track blends into the next; 0 is
	// off. AlbumGapless keeps consecutive tracks of one album gapless.
	Crossfade    float64
	AlbumGapless bool
	// AutoContinue is what plays when the queue runs out.
	AutoContinue string
	// IdleExit ends mpv after this long with nothing playing; 0 keeps it.
	IdleExit time.Duration
	// VolumeTarget is what volume and mute act on: VolumeApp (mpv) or
	// VolumeSystem (the default output; mpv stays at 100).
	VolumeTarget string
	// SkipSilenceStart and SkipSilenceEnd play a track from the end of its
	// leading silence and up to the start of its trailing silence.
	SkipSilenceStart bool
	SkipSilenceEnd   bool
}

const (
	VolumeApp    = "app"
	VolumeSystem = "system"
)

const (
	ContinueOff     = "off"
	ContinueRandom  = "random"
	ContinueSimilar = "similar"
)

func DefaultOptions() Options {
	return Options{PreviousThreshold: 5, VolumeMax: 100, Gapless: "weak", ReplayGain: "track", Speed: 1, AutoContinue: ContinueOff, AlbumGapless: true,
		VolumeTarget: VolumeApp}
}

// Sleep is the sleep timer: Mode off, timer (pause at At, unix seconds) or
// endOfTrack (pause when the current track ends).
type Sleep struct {
	Mode string `json:"mode"`
	At   int64  `json:"at,omitempty"`
}

const (
	SleepOff        = "off"
	SleepTimer      = "timer"
	SleepEndOfTrack = "endOfTrack"
)

type Status struct {
	State    State         `json:"state"`
	Position float64       `json:"position"`
	Duration float64       `json:"duration"`
	Volume   float64       `json:"volume"`
	Muted    bool          `json:"muted"`
	Sleep    Sleep         `json:"sleep"`
	Index    int           `json:"index"`
	Length   int           `json:"length"`
	Track    *source.Track `json:"track"`
	// Loading is set while the current track's stream is still looked up;
	// State already says playing or paused.
	Loading bool   `json:"loading"`
	Shuffle bool   `json:"shuffle"`
	Repeat  string `json:"repeat"`
	// Continuing says the queue is extended endlessly when it runs out: by
	// the autoContinue setting, or turned on for this queue (Continue).
	Continuing bool `json:"continuing"`
	// VolumeTarget is what Volume and Muted are: VolumeApp or VolumeSystem.
	// SystemVolume says the system target is available.
	VolumeTarget string `json:"volumeTarget"`
	SystemVolume bool   `json:"systemVolume"`
	Error        string `json:"error,omitempty"`
}

// Played is a TopicPlayed event.
type Played struct {
	Track    source.Track `json:"track"`
	PlayedAt int64        `json:"playedAt"`
}

// Position is a TopicPosition event. At is when Position was true, in unix
// ms (the clock of JavaScript's Date.now()); while Playing, the position
// moves on at Speed seconds per second from there.
type Position struct {
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
	Playing  bool    `json:"playing"`
	Speed    float64 `json:"speed"`
	At       int64   `json:"at"`
}

type QueueSnapshot struct {
	Index   int    `json:"index"`
	Items   []Item `json:"items"`
	Origin  Origin `json:"origin"`
	Shuffle bool   `json:"shuffle"`
	Repeat  string `json:"repeat"`
}

type Player struct {
	cfg  Config
	ops  chan func()
	done chan struct{}

	// Owned by the loop goroutine.
	queue     *Queue
	proc      *process
	procErr   error
	curEntry  int64
	nextEntry int64
	nextUID   uint64
	active    bool

	// Skipping silence: the current file started at startAt and ends at
	// endAt (0: its end); the prefetched one starts at nextStart.
	startAt, endAt, nextStart float64

	// loaded is false until mpv has opened the current file; seeks before
	// that fail in mpv, so they wait in pendingSeek.
	loaded      bool
	pendingSeek float64
	paused      bool
	pos         float64
	dur         float64
	volume      float64
	lastPos     float64
	restarts    []time.Time
	published   Status
	opts        Options
	dirty       bool
	saveTimer   <-chan time.Time
	savedPos    float64
	// feed tops up a shuffle over whole libraries; feeding is set while a
	// refill runs.
	feed    Feed
	feeding bool
	// queueGen changes with every new queue, so background work for an older
	// one (refs still resolving) knows to stop.
	queueGen uint64
	// refeed is a restored feed queue whose feed Config.Refeed rebuilds.
	refeed bool
	// contMode is continuing turned on for this queue; contTried is the
	// last item a continue already found nothing after.
	contMode  string
	contTried uint64
	// ended is set when the queue played out to its end.
	ended  bool
	runCtx context.Context
	// remote, when set, plays instead of mpv; localVolume is mpv's volume
	// to return to.
	remote      Remote
	localVolume float64
	// audioDevice is mpv's audio-device; audioEnv is extra environment for
	// mpv (PIPEWIRE_PROPS), which only a restart applies.
	audioDevice string
	audioEnv    []string
	// devices is set when mpv may list audio devices (see pipewireReachable).
	devices bool
	muted   bool
	// sysVolume and sysMuted are the system output's, for VolumeSystem.
	sysVolume  float64
	sysMuted   bool
	sleep      Sleep
	sleepTimer <-chan time.Time
	// heard is how much of the current track was listened to; counted is
	// set once it made a play (TopicPlayed).
	heard     float64
	heardPos  float64
	counted   bool
	startedAt time.Time
	// playUID is the queue item heard and counted belong to.
	playUID uint64
	// xf is the crossfade being lined up or playing (output.go).
	xf *fade
	// streams are the resolved URLs of the current and next items; resolving
	// may wait on a server, so it runs off the loop (resolving, loading).
	streams   map[uint64]Stream
	probes    map[uint64]context.CancelFunc
	resolving map[uint64]bool
	loading   *load
	idleTimer <-chan time.Time
	// deviceList is mpv's last audio-device-list, kept while mpv is down.
	deviceList json.RawMessage
}

// load is a current-track load waiting for its stream URL.
type load struct {
	uid   uint64
	start float64
	// play unpauses once loaded; otherwise the pause state stays.
	play bool
	// waited is set when a caller waits on done for the result; otherwise
	// a failure is published as a player error.
	waited bool
	done   chan error
}

// finish reports the result to the waiter; only the first one counts.
func (l *load) finish(err error) {
	select {
	case l.done <- err:
	default:
	}
}

// errLoading is loadCurrent saying it finishes off the loop (see doPlay).
var errLoading = errors.New("loading")

func New(cfg Config) *Player {
	if cfg.Binary == "" {
		cfg.Binary = "mpv"
	}
	if cfg.Publish == nil {
		cfg.Publish = func(string, any) {}
	}
	if cfg.Options == (Options{}) {
		cfg.Options = DefaultOptions()
	}
	return &Player{
		cfg:         cfg,
		ops:         make(chan func()),
		done:        make(chan struct{}),
		queue:       newQueue(cfg.Resolver),
		volume:      -1,
		pendingSeek: -1,
		opts:        cfg.Options,
		sleep:       Sleep{Mode: SleepOff},
		streams:     map[uint64]Stream{},
		probes:      map[uint64]context.CancelFunc{},
		resolving:   map[uint64]bool{},
	}
}

// Run serves commands until ctx ends. mpv starts with the first command
// that needs it and ends after Options.IdleExit with nothing playing. A
// missing mpv does not stop the loop; commands report it and retry.
func (p *Player) Run(ctx context.Context) {
	defer close(p.done)
	p.runCtx = ctx
	p.restore()
	for {
		p.armIdle()
		var notify <-chan struct{}
		var exited <-chan struct{}
		if p.proc != nil {
			notify = p.proc.conn.notify
			exited = p.proc.exited
		}
		fadeNotify, fadeExited := p.fadeChans()
		select {
		case <-ctx.Done():
			p.dirty = p.cfg.StatePath != ""
			p.save()
			p.dropProc()
			return
		case <-p.idleTimer:
			p.idleTimer = nil
			if p.idle() {
				log.Infof("mpv idle for %s; stopping it until the next play", p.opts.IdleExit)
				p.dropProc()
			}
		case <-fadeNotify:
			p.onFadeEvents()
		case <-fadeExited:
			p.endFade()
		case <-p.saveTimer:
			p.save()
		case <-p.sleepTimer:
			p.sleepTimer = nil
			p.sleep = Sleep{Mode: SleepOff}
			if p.active {
				p.setProp(ctx, "pause", true)
			}
			p.publishStatus()
		case op := <-p.ops:
			p.drainEvents()
			op()
		case <-notify:
			p.drainEvents()
		case <-exited:
			p.onCrash(ctx)
		}
	}
}

// idle is mpv with nothing to do: stopped, finished or with no queue.
func (p *Player) idle() bool {
	return p.proc != nil && !p.active && p.loading == nil && p.xf == nil && p.remote == nil
}

func (p *Player) armIdle() {
	switch {
	case !p.idle() || p.opts.IdleExit <= 0:
		p.idleTimer = nil
	case p.idleTimer == nil:
		p.idleTimer = time.After(p.opts.IdleExit)
	}
}

func (p *Player) drainEvents() {
	if p.proc == nil {
		return
	}
	for _, m := range p.proc.conn.takeEvents() {
		p.handle(m)
	}
}

// doPlay is do for commands that may load a track: when fn leaves the
// current track loading (errLoading), it waits off the loop for the result.
func (p *Player) doPlay(ctx context.Context, fn func(ctx context.Context) error) error {
	var l *load
	err := p.do(ctx, func(ctx context.Context) error {
		err := fn(ctx)
		if errors.Is(err, errLoading) {
			l, err = p.loading, nil
			l.waited = true
		}
		return err
	})
	if err != nil || l == nil {
		return err
	}
	select {
	case err := <-l.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return errors.New("player stopped")
	}
}

// do runs fn on the loop goroutine and waits for it.
func (p *Player) do(ctx context.Context, fn func(ctx context.Context) error) error {
	errCh := make(chan error, 1)
	op := func() { errCh <- fn(ctx) }
	select {
	case p.ops <- op:
	case <-p.done:
		return errors.New("player stopped")
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-errCh:
		return err
	case <-p.done:
		return errors.New("player stopped")
	}
}

func (p *Player) ensureProc(ctx context.Context) error {
	if p.proc != nil {
		return nil
	}
	proc, err := startProcess(ctx, p.cfg.Binary, p.cfg.Socket, p.cfg.ExtraArgs, p.audioEnv)
	if err != nil && ctx.Err() == nil {
		log.Warnf("mpv: %v; starting it again", err)
		proc, err = startProcess(ctx, p.cfg.Binary, p.cfg.Socket, p.cfg.ExtraArgs, p.audioEnv)
	}
	if err != nil {
		p.procErr = err
		log.Warnf("mpv unavailable: %v", err)
		return fmt.Errorf("mpv unavailable: %w", err)
	}
	p.proc = proc
	p.procErr = nil
	p.devices = pipewireReachable()
	p.observe(ctx, proc)
	p.applyOptions(ctx)
	p.applyRepeat(ctx)
	p.setProp(ctx, "mute", p.mpvMuted())
	if p.audioDevice != "" {
		p.setProp(ctx, "audio-device", p.audioDevice)
	}
	return nil
}

func (p *Player) observe(ctx context.Context, proc *process) {
	props := []string{"pause", "time-pos", "duration", "volume"}
	if p.devices {
		props = append(props, "audio-device-list")
	}
	for i, name := range props {
		if _, err := proc.conn.command(ctx, "observe_property", i+1, name); err != nil {
			log.Warnf("mpv observe %s: %v", name, err)
		}
	}
}

type prop struct {
	name  string
	value any
}

func (p *Player) optionProps() []prop {
	if p.volume > p.opts.VolumeMax {
		p.volume = p.opts.VolumeMax
	}
	props := []prop{{"volume-max", p.opts.VolumeMax}, {"gapless-audio", p.opts.Gapless}, {"replaygain", p.opts.ReplayGain},
		{"speed", max(p.opts.Speed, 0.5)}}
	switch {
	case p.system():
		props = append(props, prop{"volume", 100})
	case p.volume >= 0:
		props = append(props, prop{"volume", p.volume})
	}
	return props
}

// system says volume and mute act on the system output. A remote has its
// own volume.
func (p *Player) system() bool {
	return p.opts.VolumeTarget == VolumeSystem && p.cfg.System != nil && p.remote == nil
}

// mpvMuted is the mute mpv gets: with the system target the output mutes.
func (p *Player) mpvMuted() bool { return p.muted && !p.system() }

func (p *Player) applyOptions(ctx context.Context) {
	for _, pr := range p.optionProps() {
		p.setProp(ctx, pr.name, pr.value)
	}
}

// Repeat one loops inside mpv, so the track repeats without a gap.
func (p *Player) applyRepeat(ctx context.Context) {
	loop := "no"
	if p.queue.Repeat() == RepeatOne && p.sleep.Mode != SleepEndOfTrack {
		loop = "inf"
	}
	p.setProp(ctx, "loop-file", loop)
}

func (p *Player) onCrash(ctx context.Context) {
	log.Warnf("mpv exited unexpectedly")

	now := time.Now()
	recent := p.restarts[:0]
	for _, t := range p.restarts {
		if now.Sub(t) < restartWindow {
			recent = append(recent, t)
		}
	}
	p.restarts = append(recent, now)
	if len(p.restarts) > maxRestarts {
		p.dropProc()
		p.procErr = errors.New("mpv keeps crashing; giving up until the next command")
		p.restarts = nil
		p.active = false
		p.publishError(p.procErr.Error())
		p.publishStatus()
		return
	}

	if err := p.restart(ctx); err != nil {
		p.publishError(err.Error())
		p.publishStatus()
		return
	}
	log.Infof("mpv restarted")
}

func (p *Player) handle(m message) {
	switch m.Event {
	case "property-change":
		p.onProperty(m.Name, m.Data)
	case "start-file":
		p.onStartFile(m.EntryID)
	case "end-file":
		p.onEndFile(m)
	case "file-loaded":
		p.loaded = true
		p.applyEnd(context.Background())
		if p.pendingSeek >= 0 {
			if _, err := p.cmd(context.Background(), "seek", p.pendingSeek, "absolute"); err != nil {
				log.Warnf("mpv seek: %v", err)
			}
			p.pendingSeek = -1
		}
		p.publishStatus()
	case "seek":
		p.publishStatus()
	}
}

func (p *Player) onProperty(name string, data json.RawMessage) {
	switch name {
	case "pause":
		was := p.paused
		_ = json.Unmarshal(data, &p.paused)
		p.publishStatus()
		if p.loaded && was != p.paused {
			p.publishPosition()
		}
	case "time-pos":
		var pos float64
		// Until a pending seek lands, mpv reports the file's start, which
		// would overwrite the position the user asked for. Before the new
		// file is loaded, a time-pos is still the old track's.
		if json.Unmarshal(data, &pos) != nil || p.pendingSeek >= 0 || !p.loaded {
			return
		}
		p.pos = pos
		p.countPlay(pos)
		if math.Abs(pos-p.lastPos) >= 1 {
			p.publishPosition()
		}
		if math.Abs(pos-p.savedPos) >= positionSaveEvery {
			p.markDirty()
		}
		p.crossfade()
	case "duration":
		var d float64
		if json.Unmarshal(data, &d) == nil {
			p.dur = d
			p.publishStatus()
		}
	case "volume":
		if p.system() {
			return
		}
		_ = json.Unmarshal(data, &p.volume)
		p.publishStatus()
	case "audio-device-list":
		p.deviceList = data
		p.cfg.Publish(TopicAudioDevices, data)
	}
}

// A start-file for the prefetched entry is mpv moving on by itself: the
// gapless transition. Advance the queue and line up the next track.
func (p *Player) onStartFile(entry int64) {
	if entry == 0 || entry != p.nextEntry {
		return
	}
	p.stopFade()
	p.queue.selectUID(p.nextUID)
	p.curEntry = entry
	start := p.nextStart
	p.nextEntry, p.nextUID = 0, 0
	// -1: the new track's first time-pos is reported at once.
	p.pos, p.lastPos = start, -1
	p.startAt, p.endAt = start, 0
	p.beginPlay(p.queue.currentUID(), 0)
	p.heardPos = start
	if it, ok := p.queue.Current(); ok {
		// mpv sends no duration change when both tracks are equally long.
		p.dur = it.Track.Duration
	}
	p.loaded, p.pendingSeek = false, -1
	ctx := context.Background()
	if _, err := p.cmd(ctx, "playlist-clear"); err != nil {
		log.Warnf("mpv playlist-clear: %v", err)
	}
	p.syncNext(ctx)
	p.publishQueue()
	p.publishStatus()
}

func (p *Player) onEndFile(m message) {
	if m.EntryID != p.curEntry {
		return
	}
	switch m.Reason {
	case "error":
		p.publishError(fmt.Sprintf("couldn't play %q: %s", p.currentTitle(), m.FileError))
		if a, ok := p.cfg.Resolver.(Availability); ok {
			if it, ok := p.queue.Current(); ok {
				a.Failed(it.Track.Ref(), func() { _ = p.do(p.runCtx, p.recheck) })
			}
		}
	case "eof":
		if p.xf != nil && p.xf.in != nil {
			// Ended before a position update reached the fade's start.
			p.startFade()
			return
		}
	default:
		return
	}
	if p.nextEntry != 0 {
		return
	}
	p.active = false
	p.pos, p.lastPos = 0, 0
	_, more := p.queue.Peek()
	p.ended = !more
	if p.sleep.Mode == SleepEndOfTrack {
		// Stopped before the next track, which resume then plays.
		p.queue.Advance()
		p.setSleep(Sleep{Mode: SleepOff})
		p.publishQueue()
	}
	p.publishStatus()
}

// recheck follows a change in what can play: a current track that can't
// any more is passed over, and the prefetched one is lined up again.
func (p *Player) recheck(ctx context.Context) error {
	if !p.active {
		return nil
	}
	if it, ok := p.queue.Current(); ok && p.queue.skip != nil && p.queue.skip(it) {
		if err := p.loadCurrent(ctx, 0, false); err != nil && !errors.Is(err, errLoading) {
			p.stop(ctx)
			p.publishError(err.Error())
			p.publishStatus()
		}
		return nil
	}
	p.syncNext(ctx)
	return nil
}

func (p *Player) currentTitle() string {
	it, ok := p.queue.Current()
	if !ok {
		return ""
	}
	return it.Track.Title
}

func (p *Player) cmd(ctx context.Context, args ...any) (json.RawMessage, error) {
	if err := p.ensureProc(ctx); err != nil {
		return nil, err
	}
	return p.proc.conn.command(ctx, args...)
}

func (p *Player) setProp(ctx context.Context, name string, value any) {
	if v, ok := value.(bool); ok && name == "pause" && p.remote != nil {
		p.remote.SetPaused(v)
		p.paused = v
		return
	}
	if _, err := p.cmd(ctx, "set_property", name, value); err != nil {
		log.Warnf("mpv set %s: %v", name, err)
		return
	}
	p.mirror(ctx, name, value)
	// The reply goes out before the property-change event; set it now so
	// the status in this reply is already right.
	if v, ok := value.(bool); ok && name == "pause" && v != p.paused {
		p.paused = v
		if p.loaded {
			p.publishPosition()
		}
	}
}

// loadfile loads it on proc, or on the main mpv when proc is nil; opts are
// extra per-file options.
func (p *Player) loadfile(ctx context.Context, proc *process, it Item, st Stream, mode string, start float64, opts ...string) (int64, error) {
	args := []any{"loadfile", st.URL, mode, "-1"}
	if start > 0 {
		opts = append(opts, "start="+strconv.FormatFloat(start, 'f', 3, 64))
	}
	if gain := p.fallbackGain(it.Track, st.GainDB); gain != nil {
		// Per-file option: mpv uses it only when the file has no ReplayGain
		// tags and ReplayGain is on.
		opts = append(opts, "replaygain-fallback="+strconv.FormatFloat(*gain, 'f', 2, 64))
	}
	if len(opts) > 0 {
		args = append(args, strings.Join(opts, ","))
	}
	send := p.cmd
	if proc != nil {
		send = proc.conn.command
	}
	data, err := send(ctx, args...)
	if err != nil {
		// The error text never includes the URL, which may carry a token.
		return 0, fmt.Errorf("load %q: %w", it.Track.Title, err)
	}
	var reply struct {
		EntryID int64 `json:"playlist_entry_id"`
	}
	if err := json.Unmarshal(data, &reply); err != nil || reply.EntryID == 0 {
		return 0, errors.New("mpv did not return a playlist entry id (mpv 0.38 or newer is required)")
	}
	return reply.EntryID, nil
}

// keepStream keeps a resolved stream; one without any gain is probed, and
// the gain applies once it is in.
func (p *Player) keepStream(it Item, st Stream) {
	p.streams[it.UID] = st
	pr, ok := p.cfg.Resolver.(Prober)
	if !ok || p.opts.ReplayGain == "no" || p.fallbackGain(it.Track, st.GainDB) != nil {
		return
	}
	ref := it.Track.Ref()
	if p.probes[it.UID] != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.runCtx)
	p.probes[it.UID] = cancel
	pr.Probe(ctx, ref, func(gain float64) {
		_ = p.do(p.runCtx, func(ctx context.Context) error { p.gainProbed(ctx, ref, gain); return nil })
	})
}

// gainProbed gives the current or next track its probed gain: the playing
// one at once, a lined-up one by lining it up again.
func (p *Player) gainProbed(ctx context.Context, ref source.Ref, gain float64) {
	cur, _ := p.queue.Current()
	next, _ := p.queue.Peek()
	for _, it := range []Item{cur, next} {
		st, ok := p.streams[it.UID]
		if !ok || st.GainDB != nil || it.Track.Ref() != ref {
			continue
		}
		st.GainDB = &gain
		p.streams[it.UID] = st
		switch {
		case it.UID == cur.UID:
			if p.active && p.remote == nil && p.proc != nil {
				p.setProp(ctx, "file-local-options/replaygain-fallback", gain)
			}
		case p.nextUID == it.UID:
			if _, err := p.cmd(ctx, "playlist-clear"); err != nil {
				log.Warnf("mpv playlist-clear: %v", err)
			}
			p.nextEntry, p.nextUID = 0, 0
			p.syncNext(ctx)
		}
	}
}

// fallbackGain picks the gain for a file whose own tags may be missing: the
// server's ReplayGain value for the current mode, else the measured one.
func (p *Player) fallbackGain(t source.Track, measured *float64) *float64 {
	if rg := t.ReplayGain; rg != nil {
		if p.opts.ReplayGain == "album" && rg.Album != nil {
			return rg.Album
		}
		if rg.Track != nil {
			return rg.Track
		}
		if rg.Album != nil {
			return rg.Album
		}
	}
	return measured
}

// loadCurrent replaces whatever plays with the queue's current item; play
// unpauses it. A stream not resolved yet is looked up off the loop, and
// loadCurrent returns errLoading: the old track stops and the new one
// starts once its URL is in (doPlay waits for that).
func (p *Player) loadCurrent(ctx context.Context, start float64, play bool) error {
	p.ended = false
	it, ok := p.queue.Current()
	if !ok {
		return errors.New("queue is empty")
	}
	if p.queue.skip != nil && p.queue.skip(it) {
		if !p.queue.Advance() {
			return errors.New("none of the remaining tracks can play right now")
		}
		it, _ = p.queue.Current()
		start = 0
	}
	p.stopFade()
	p.cancelLoad()
	if p.remote != nil {
		p.loadRemote(start)
		if play {
			p.setProp(ctx, "pause", false)
		}
		return nil
	}
	if st, ok := p.streams[it.UID]; ok {
		return p.open(ctx, it, st, start, play)
	}
	if p.active {
		p.stop(ctx)
	}
	l := &load{uid: it.UID, start: start, play: play, done: make(chan error, 1)}
	p.loading = l
	p.pos, p.dur = start, it.Track.Duration
	go func() {
		st, err := p.resolve(it)
		if p.do(p.runCtx, func(ctx context.Context) error { p.finishLoad(ctx, l, it, st, err); return nil }) != nil {
			l.finish(errors.New("player stopped"))
		}
	}()
	p.publishStatus()
	return errLoading
}

func (p *Player) resolve(it Item) (Stream, error) {
	ctx, cancel := context.WithTimeout(p.runCtx, resolveTimeout)
	defer cancel()
	st, err := p.cfg.Resolver.Stream(ctx, it.Track.Ref())
	if err != nil {
		return st, fmt.Errorf("resolve %q: %w", it.Track.Title, err)
	}
	return st, nil
}

func (p *Player) finishLoad(ctx context.Context, l *load, it Item, st Stream, err error) {
	if p.loading != l {
		return
	}
	p.loading = nil
	if err == nil {
		p.keepStream(it, st)
		err = p.open(ctx, it, st, l.start, l.play)
	}
	if err != nil {
		p.publishStatus()
		if !l.waited {
			p.publishError(err.Error())
		}
	}
	l.finish(err)
}

// cancelLoad drops a pending load; its waiter gets nil.
func (p *Player) cancelLoad() {
	if l := p.loading; l != nil {
		p.loading = nil
		l.finish(nil)
	}
}

// open plays it on the main mpv from start, replacing what plays there. On
// failure nothing plays, so the queue and mpv agree.
func (p *Player) open(ctx context.Context, it Item, st Stream, start float64, play bool) error {
	head, _ := trimmed(p.opts, st.Silence, false, false)
	from := start
	if from == 0 {
		from = head
	}
	id, err := p.loadfile(ctx, nil, it, st, "replace", from)
	if err != nil {
		p.stop(ctx)
		return err
	}
	p.curEntry = id
	p.nextEntry, p.nextUID = 0, 0
	p.active = true
	p.loaded, p.pendingSeek = false, -1
	p.pos, p.lastPos = from, -1
	p.startAt, p.endAt = head, 0
	p.dur = it.Track.Duration
	p.beginPlay(it.UID, start)
	p.heardPos = from
	if play {
		p.setProp(ctx, "pause", false)
	}
	p.syncNext(ctx)
	p.publishQueue()
	p.publishStatus()
	return nil
}

// syncNext makes mpv's playlist [current, next] for the queue as it is now.
func (p *Player) syncNext(ctx context.Context) {
	if p.remote != nil && p.active {
		p.remote.Load(p.queue.Items(), p.queue.Index(), -1)
		return
	}
	if !p.active {
		return
	}
	defer p.applyEnd(ctx)
	want, ok := p.queue.Peek()
	// The sleep timer's end of track stops mpv there instead of moving on.
	ok = ok && p.sleep.Mode != SleepEndOfTrack
	for uid := range p.streams {
		if uid != p.queue.currentUID() && (!ok || uid != want.UID) {
			delete(p.streams, uid)
		}
	}
	for uid, cancel := range p.probes {
		if _, kept := p.streams[uid]; !kept {
			cancel()
			delete(p.probes, uid)
		}
	}
	if p.fadeLinedUp() {
		if ok && want.UID == p.xf.uid && p.crossfadeNext() {
			return
		}
		p.stopFade()
	}
	if ok && p.nextEntry != 0 && want.UID == p.nextUID {
		return
	}
	if p.nextEntry != 0 {
		if _, err := p.cmd(ctx, "playlist-clear"); err != nil {
			log.Warnf("mpv playlist-clear: %v", err)
		}
		p.nextEntry, p.nextUID = 0, 0
	}
	if !ok {
		return
	}
	st, resolved := p.streams[want.UID]
	if !resolved {
		p.prefetch(want)
		return
	}
	cur, _ := p.queue.Current()
	start, _ := trimmed(p.opts, st.Silence, p.joins(cur, want), false)
	id, err := p.loadfile(ctx, nil, want, st, "append", start)
	if err != nil {
		log.Warnf("prefetch next: %v", err)
		return
	}
	p.nextEntry, p.nextUID, p.nextStart = id, want.UID, start
}

// joins says the change from a to b is one the album rule keeps gapless;
// skipping silence never cuts into it.
func (p *Player) joins(a, b Item) bool {
	return p.opts.AlbumGapless && sameAlbum(a.Track, b.Track)
}

// trimmed is the part of a file that plays when skipping silence: from
// start until end, 0 being the end of the file. A start or end that is one
// side of a gapless same-album change (joinsPrev, joinsNext) stays whole.
func trimmed(o Options, s loudness.Silence, joinsPrev, joinsNext bool) (start, end float64) {
	if o.SkipSilenceStart && !joinsPrev {
		start = s.Start
	}
	if o.SkipSilenceEnd && !joinsNext {
		end = s.End
	}
	return start, end
}

// applyEnd ends the current file where skipping silence wants it to, which
// depends on the track after it, so it follows queue changes.
func (p *Player) applyEnd(ctx context.Context) {
	if p.remote != nil || !p.active || !p.loaded || p.proc == nil {
		return
	}
	want := p.trimEnd()
	if want == p.endAt {
		return
	}
	end := "none"
	if want > 0 {
		end = strconv.FormatFloat(want, 'f', 3, 64)
	}
	if _, err := p.cmd(ctx, "set_property", "file-local-options/end", end); err != nil {
		log.Warnf("mpv set end: %v", err)
		return
	}
	p.endAt = want
}

// trimEnd is where the current file should end, 0 being its end.
func (p *Player) trimEnd() float64 {
	cur, ok := p.queue.Current()
	if !ok {
		return 0
	}
	next, more := p.queue.Peek()
	_, end := trimmed(p.opts, p.streams[cur.UID].Silence, false, more && p.joins(cur, next))
	return end
}

// playEnd is where the current track's music ends.
func (p *Player) playEnd() float64 {
	if p.endAt > 0 && p.endAt < p.dur {
		return p.endAt
	}
	return p.dur
}

// prefetch resolves it off the loop and lines it up once that's done.
func (p *Player) prefetch(it Item) {
	if p.resolving[it.UID] {
		return
	}
	p.resolving[it.UID] = true
	go func() {
		st, err := p.resolve(it)
		_ = p.do(p.runCtx, func(ctx context.Context) error {
			delete(p.resolving, it.UID)
			if err != nil {
				log.Warnf("prefetch next: %v", err)
				return nil
			}
			p.keepStream(it, st)
			p.syncNext(ctx)
			return nil
		})
	}()
}

// stop unloads the track but keeps the position, so resume continues from
// it; callers that end the track for good reset pos themselves.
func (p *Player) stop(ctx context.Context) {
	p.stopFade()
	p.cancelLoad()
	// A stream URL may carry a session token that expires; resolve again.
	clear(p.streams)
	for _, cancel := range p.probes {
		cancel()
	}
	clear(p.probes)
	if p.remote != nil {
		p.remote.Stop()
		p.remote = nil
		p.volume = p.localVolume
	}
	if p.proc != nil {
		if _, err := p.cmd(ctx, "stop"); err != nil {
			log.Warnf("mpv stop: %v", err)
		}
	}
	p.active = false
	p.curEntry, p.nextEntry, p.nextUID = 0, 0, 0
	p.lastPos = 0
	p.loaded, p.pendingSeek = false, -1
}

// newQueue forgets what belonged to the queue being replaced.
func (p *Player) newQueue() {
	p.queueGen++
	p.feed, p.refeed = nil, false
	p.contMode, p.ended = "", false
}

// Play replaces the queue with tracks and starts at index start.
func (p *Player) Play(ctx context.Context, tracks []source.Track, start int, origin Origin) error {
	if len(tracks) == 0 {
		return errors.New("nothing to play")
	}
	if start < 0 || start >= len(tracks) {
		return fmt.Errorf("index %d is out of range for %d tracks", start, len(tracks))
	}
	return p.doPlay(ctx, func(ctx context.Context) error {
		// Plain play is in order: only the shuffle entry points turn shuffle on.
		p.queue.SetShuffle(false)
		p.newQueue()
		p.queue.Replace(tracks, start, origin)
		return p.loadCurrent(ctx, 0, true)
	})
}

// PlayShuffled replaces the queue with tracks in shuffled order and plays.
func (p *Player) PlayShuffled(ctx context.Context, tracks []source.Track, origin Origin) error {
	if len(tracks) == 0 {
		return errors.New("nothing to play")
	}
	return p.doPlay(ctx, func(ctx context.Context) error {
		p.queue.SetShuffle(true)
		p.newQueue()
		p.queue.Replace(tracks, -1, origin)
		return p.loadCurrent(ctx, 0, true)
	})
}

// Feed hands out more tracks for a queue that grows as it plays (a shuffle
// over whole libraries). An empty result ends it.
type Feed interface {
	Next(ctx context.Context, n int) ([]source.Track, error)
}

const (
	feedBatch = 100
	// The feed refills once this few tracks are left after the current one.
	feedLow = 20
)

// PlayFeed starts a queue from feed and keeps topping it up.
func (p *Player) PlayFeed(ctx context.Context, feed Feed, origin Origin) error {
	first, err := feed.Next(ctx, feedBatch)
	if err != nil {
		return err
	}
	if len(first) == 0 {
		return errors.New("nothing to play")
	}
	return p.doPlay(ctx, func(ctx context.Context) error {
		p.queue.SetShuffle(true)
		p.newQueue()
		p.queue.Replace(first, -1, origin)
		p.feed = feed
		return p.loadCurrent(ctx, 0, true)
	})
}

// maybeFeed refills a feed queue that runs low. The fetch may hit the
// network, so it runs off the loop and hands its tracks back through do.
func (p *Player) maybeFeed() {
	if p.feed == nil && p.refeed && p.cfg.Refeed != nil && !p.feeding {
		p.refill()
		return
	}
	if p.feed == nil {
		p.maybeContinue()
	}
	if p.feed == nil || p.feeding || p.queue.Len()-p.queue.Index()-1 > feedLow {
		return
	}
	p.feeding = true
	feed := p.feed
	go func() {
		tracks, err := feed.Next(p.runCtx, feedBatch)
		_ = p.do(p.runCtx, func(ctx context.Context) error {
			p.feeding = false
			if p.feed != feed {
				return nil
			}
			if err != nil {
				// A library didn't answer; the next track change tries again.
				log.Warnf("shuffle refill: %v", err)
				return nil
			}
			if len(tracks) == 0 {
				p.feed = nil
				return nil
			}
			p.queue.Extend(tracks)
			p.syncNext(ctx)
			p.publishQueue()
			p.publishStatus()
			return nil
		})
	}()
}

// refill rebuilds the feed of a restored queue, off the loop.
func (p *Player) refill() {
	p.feeding = true
	origin := p.queue.Origin()
	go func() {
		feed, err := p.cfg.Refeed(p.runCtx, origin)
		_ = p.do(p.runCtx, func(context.Context) error {
			p.feeding = false
			if !p.refeed {
				return nil
			}
			p.refeed = false
			if err != nil {
				log.Warnf("continue the shuffle: %v", err)
				return nil
			}
			p.feed = feed
			p.maybeFeed()
			return nil
		})
	}()
}

// Resume plays from where playback is, or restarts the current item once the
// queue has finished.
func (p *Player) Resume(ctx context.Context) error {
	return p.doPlay(ctx, p.resume)
}

func (p *Player) resume(ctx context.Context) error {
	if l := p.loading; l != nil {
		l.play = true
		return nil
	}
	if !p.active {
		return p.loadCurrent(ctx, p.pos, true)
	}
	p.setProp(ctx, "pause", false)
	return nil
}

func (p *Player) Pause(ctx context.Context) error {
	return p.do(ctx, func(ctx context.Context) error {
		if l := p.loading; l != nil {
			l.play = false
		}
		if p.active || p.loading != nil {
			p.setProp(ctx, "pause", true)
		}
		return nil
	})
}

func (p *Player) Toggle(ctx context.Context) error {
	return p.doPlay(ctx, func(ctx context.Context) error {
		if !p.active && p.loading == nil || p.paused {
			return p.resume(ctx)
		}
		if l := p.loading; l != nil {
			l.play = false
		}
		p.setProp(ctx, "pause", true)
		return nil
	})
}

func (p *Player) Stop(ctx context.Context) error {
	return p.do(ctx, func(ctx context.Context) error {
		p.stop(ctx)
		p.publishStatus()
		return nil
	})
}

// Next plays the next track. At the end of the queue it turns continuing
// on for the queue and plays the first track that continues it.
func (p *Player) Next(ctx context.Context) error {
	var done chan error
	err := p.doPlay(ctx, func(ctx context.Context) error {
		if p.queue.Advance() {
			return p.loadCurrent(ctx, 0, false)
		}
		if p.cfg.Continue == nil || p.queue.Len() == 0 {
			return errors.New("end of queue")
		}
		if p.continueMode() == "" {
			p.contMode = ContinueSimilar
		}
		done = make(chan error, 1)
		p.startContinue(true, done)
		return nil
	})
	if err != nil || done == nil {
		return err
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Continue turns continuing on for the current queue with mode (random or
// similar) and extends it now, in the background; a queue that has ended
// plays on with the first new track.
func (p *Player) Continue(ctx context.Context, mode string) error {
	if mode != ContinueRandom && mode != ContinueSimilar {
		return fmt.Errorf("mode must be %s or %s", ContinueRandom, ContinueSimilar)
	}
	return p.do(ctx, func(context.Context) error {
		if p.cfg.Continue == nil {
			return errors.New("continuing is not available")
		}
		if p.queue.Len() == 0 {
			return errors.New("the queue is empty")
		}
		p.contMode = mode
		p.markDirty()
		p.startContinue(p.ended, nil)
		p.publishStatus()
		return nil
	})
}

func (p *Player) Previous(ctx context.Context) error {
	return p.doPlay(ctx, func(ctx context.Context) error {
		if p.active && p.pos > p.opts.PreviousThreshold || !p.queue.Back() {
			return p.seek(ctx, p.startAt, false)
		}
		return p.loadCurrent(ctx, 0, false)
	})
}

func (p *Player) Seek(ctx context.Context, pos float64, relative bool) error {
	return p.do(ctx, func(ctx context.Context) error { return p.seek(ctx, pos, relative) })
}

func (p *Player) seek(ctx context.Context, pos float64, relative bool) error {
	if l := p.loading; l != nil {
		if relative {
			pos += l.start
		}
		l.start = max(pos, 0)
		p.pos = l.start
		return nil
	}
	if !p.active {
		return errors.New("nothing is playing")
	}
	p.stopFade()
	if !p.loaded {
		if relative {
			pos += max(p.pos, 0)
		}
		p.pendingSeek = max(pos, 0)
		p.pos = p.pendingSeek
		return nil
	}
	if p.remote != nil {
		if relative {
			pos += p.pos
		}
		p.pos, p.lastPos = max(pos, 0), -1
		p.remote.Seek(p.pos)
		return nil
	}
	mode := "absolute"
	if relative {
		mode = "relative"
	}
	if _, err := p.cmd(ctx, "seek", pos, mode); err != nil {
		return err
	}
	if relative {
		pos += p.pos
	}
	p.pos, p.lastPos = max(pos, 0), -1
	return nil
}

func (p *Player) SetVolume(ctx context.Context, v float64, relative bool) error {
	var sys bool
	var cur, top float64
	if err := p.do(ctx, func(context.Context) error {
		sys, cur, top = p.system(), p.sysVolume, p.opts.VolumeMax
		return nil
	}); err != nil {
		return err
	}
	if sys {
		if relative {
			if got, _, err := p.cfg.System.Get(ctx); err == nil {
				cur = got
			}
			v += cur
		}
		v = min(max(v, 0), top)
		if err := p.cfg.System.SetVolume(ctx, v); err != nil {
			return err
		}
		return p.do(ctx, func(context.Context) error {
			p.sysVolume = v
			p.publishStatus()
			return nil
		})
	}
	return p.do(ctx, func(ctx context.Context) error {
		cur := max(p.volume, 0)
		if relative {
			v += cur
		}
		v = min(max(v, 0), p.opts.VolumeMax)
		if p.remote != nil {
			p.remote.SetVolume(v)
			p.volume = v
			return nil
		}
		if p.proc == nil {
			// The next mpv starts with it (optionProps).
			p.volume = v
			p.publishStatus()
			return nil
		}
		if _, err := p.cmd(ctx, "set_property", "volume", v); err != nil {
			return err
		}
		p.mirror(ctx, "volume", v)
		p.volume = v
		return nil
	})
}

// Enqueue adds tracks to the end, or after the current track with next.
func (p *Player) Enqueue(ctx context.Context, tracks []source.Track, next bool) error {
	return p.do(ctx, func(ctx context.Context) error {
		p.queue.Add(tracks, next)
		p.syncNext(ctx)
		p.publishQueue()
		p.publishStatus()
		return nil
	})
}

func (p *Player) Remove(ctx context.Context, index int) error {
	return p.doPlay(ctx, func(ctx context.Context) error {
		wasCurrent, err := p.queue.Remove(index)
		if err != nil {
			return err
		}
		if wasCurrent {
			p.pos = 0
		}
		if wasCurrent && (p.active || p.loading != nil) {
			if _, ok := p.queue.Current(); ok {
				return p.loadCurrent(ctx, 0, false)
			}
			p.stop(ctx)
		}
		p.syncNext(ctx)
		p.publishQueue()
		p.publishStatus()
		return nil
	})
}

func (p *Player) Move(ctx context.Context, from, to int) error {
	return p.do(ctx, func(ctx context.Context) error {
		if err := p.queue.Move(from, to); err != nil {
			return err
		}
		p.syncNext(ctx)
		p.publishQueue()
		p.publishStatus()
		return nil
	})
}

func (p *Player) Jump(ctx context.Context, index int) error {
	return p.doPlay(ctx, func(ctx context.Context) error {
		if err := p.queue.Jump(index); err != nil {
			return err
		}
		return p.loadCurrent(ctx, 0, true)
	})
}

// Clear empties the queue; with keepCurrent the current track stays and
// keeps playing.
func (p *Player) Clear(ctx context.Context, keepCurrent bool) error {
	return p.do(ctx, func(ctx context.Context) error {
		if cur, ok := p.queue.Current(); ok && keepCurrent {
			p.feed, p.refeed = nil, false
			p.queue.KeepOnly(cur.UID)
			p.syncNext(ctx)
			p.publishQueue()
			p.publishStatus()
			return nil
		}
		p.stop(ctx)
		p.pos = 0
		p.newQueue()
		p.queue.Clear()
		p.publishQueue()
		p.publishStatus()
		return nil
	})
}

// Configure applies new playback options, live.
func (p *Player) Configure(ctx context.Context, o Options) error {
	var vol float64
	var muted bool
	var sysErr error
	if o.VolumeTarget == VolumeSystem && p.cfg.System != nil {
		vol, muted, sysErr = p.cfg.System.Get(ctx)
	}
	return p.do(ctx, func(ctx context.Context) error {
		wasOff := p.continueMode() == ""
		p.opts = o
		p.idleTimer = nil
		if sysErr == nil {
			p.sysVolume, p.sysMuted = vol, muted
		}
		if p.proc != nil {
			p.applyOptions(ctx)
			p.setProp(ctx, "mute", p.mpvMuted())
		}
		p.syncNext(ctx)
		switch {
		case wasOff && p.continueMode() != "" && p.ended:
			p.startContinue(true, nil)
		case wasOff:
			p.maybeFeed()
		}
		p.publishStatus()
		return nil
	})
}

func (p *Player) SetShuffle(ctx context.Context, on bool) error {
	return p.do(ctx, func(ctx context.Context) error {
		p.queue.SetShuffle(on)
		p.syncNext(ctx)
		p.publishQueue()
		p.publishStatus()
		return nil
	})
}

func (p *Player) SetRepeat(ctx context.Context, mode string) error {
	return p.do(ctx, func(ctx context.Context) error {
		if err := p.queue.SetRepeat(mode); err != nil {
			return err
		}
		if p.proc != nil {
			p.applyRepeat(ctx)
		}
		if p.remote != nil {
			p.remote.SetLoop(mode == RepeatAll)
		}
		p.syncNext(ctx)
		p.publishQueue()
		p.publishStatus()
		return nil
	})
}

// Mark follows a favorite or rating set on a track, so the playing track and
// the saved queue don't keep the value they were queued with.
func (p *Player) Mark(ctx context.Context, ref source.Ref, favorite *bool, rating *int) error {
	return p.do(ctx, func(context.Context) error {
		if p.queue.Mark(ref, favorite, rating) {
			p.publishQueue()
			p.publishStatus()
		}
		return nil
	})
}

// Snapshot calls fn on the loop with the current status and queue, so no
// event the loop publishes afterwards can arrive before what fn sends.
func (p *Player) Snapshot(ctx context.Context, fn func(Status, QueueSnapshot)) error {
	return p.do(ctx, func(context.Context) error {
		fn(p.status(), p.queueSnapshot())
		return nil
	})
}

func (p *Player) Status(ctx context.Context) (Status, error) {
	var st Status
	err := p.do(ctx, func(context.Context) error {
		st = p.status()
		return nil
	})
	return st, err
}

func (p *Player) Queue(ctx context.Context) (QueueSnapshot, error) {
	var q QueueSnapshot
	err := p.do(ctx, func(context.Context) error {
		q = p.queueSnapshot()
		return nil
	})
	return q, err
}

func (p *Player) status() Status {
	st := Status{
		State:  StateStopped,
		Volume: max(p.volume, 0),
		Muted:  p.muted,

		VolumeTarget: VolumeApp,
		SystemVolume: p.cfg.System != nil,
		Sleep:        p.sleep,
		Index:        p.queue.Index(),
		Length:       p.queue.Len(),
		Shuffle:      p.queue.Shuffle(),
		Repeat:       p.queue.Repeat(),

		Continuing: p.cfg.Continue != nil && p.continueMode() != "",
	}
	if p.system() {
		st.Volume, st.Muted, st.VolumeTarget = p.sysVolume, p.sysMuted, VolumeSystem
	}
	if p.procErr != nil {
		st.Error = p.procErr.Error()
	}
	if it, ok := p.queue.Current(); ok {
		t := it.Track
		st.Track = &t
		st.Duration = t.Duration
	}
	if !p.active && p.loading == nil {
		st.Position = p.pos
		return st
	}
	st.State = StatePlaying
	st.Loading = p.loading != nil
	if p.paused {
		st.State = StatePaused
	}
	st.Position = p.pos
	if p.dur > 0 {
		st.Duration = p.dur
	}
	return st
}

func (p *Player) queueSnapshot() QueueSnapshot {
	return QueueSnapshot{Index: p.queue.Index(), Items: p.queue.Items(), Origin: p.queue.Origin(),
		Shuffle: p.queue.Shuffle(), Repeat: p.queue.Repeat()}
}

func (p *Player) publishStatus() {
	st := p.status()
	if reflect.DeepEqual(st, p.published) {
		return
	}
	p.published = st
	p.cfg.Publish(TopicStatus, st)
	p.markDirty()
}

func (p *Player) publishQueue() {
	p.cfg.Publish(TopicQueue, p.queueSnapshot())
	p.markDirty()
	p.maybeFeed()
}

func (p *Player) publishPosition() {
	p.lastPos = p.pos
	p.cfg.Publish(TopicPosition, Position{Position: p.pos, Duration: p.dur, Playing: p.active && p.loaded && !p.paused,
		Speed: max(p.opts.Speed, 0.5), At: time.Now().UnixMilli()})
}

func (p *Player) publishError(msg string) {
	log.Warnf("playback: %s", msg)
	p.cfg.Publish(TopicError, map[string]string{"message": msg})
}

// SetMute mutes mpv (or a remote, through its volume; it is saved) or,
// with the system target, the system output.
func (p *Player) SetMute(ctx context.Context, muted bool) error {
	var sys bool
	if err := p.do(ctx, func(context.Context) error { sys = p.system(); return nil }); err != nil {
		return err
	}
	if sys {
		if err := p.cfg.System.SetMute(ctx, muted); err != nil {
			return err
		}
		return p.SystemChanged(ctx, -1, muted)
	}
	return p.do(ctx, func(ctx context.Context) error {
		p.muted = muted
		if p.remote != nil {
			p.remote.SetVolume(map[bool]float64{true: 0, false: p.volume}[muted])
		}
		// mpv keeps it too, so taking playback back from a remote is muted
		// the same; a new mpv gets it from ensureProc.
		if p.proc != nil {
			if _, err := p.proc.conn.command(ctx, "set_property", "mute", muted); err != nil && p.remote == nil {
				return err
			}
		}
		p.mirror(ctx, "mute", muted)
		p.publishStatus()
		return nil
	})
}

// SystemChanged follows the system output's volume (percent; negative
// leaves it) and mute, changed here or elsewhere.
func (p *Player) SystemChanged(ctx context.Context, volume float64, muted bool) error {
	return p.do(ctx, func(context.Context) error {
		if volume >= 0 {
			p.sysVolume = volume
		}
		p.sysMuted = muted
		p.publishStatus()
		return nil
	})
}

// SystemVolumeAvailable says whether the system volume target can work.
func (p *Player) SystemVolumeAvailable() bool { return p != nil && p.cfg.System != nil }

// SetSleep sets the sleep timer: pause after d, at the end of the current
// track (endOfTrack), or off. It runs in the daemon, window or not.
func (p *Player) SetSleep(ctx context.Context, mode string, d time.Duration) error {
	switch mode {
	case SleepOff, SleepEndOfTrack:
	case SleepTimer:
		if d <= 0 || d > 24*time.Hour {
			return errors.New("sleep minutes must be between 1 and 1440")
		}
	default:
		return fmt.Errorf("sleep mode %q is not off, timer or endOfTrack", mode)
	}
	return p.do(ctx, func(ctx context.Context) error {
		sl := Sleep{Mode: mode}
		if mode == SleepTimer {
			sl.At = time.Now().Add(d).Unix()
		}
		p.setSleep(sl)
		p.syncNext(ctx)
		if p.proc != nil {
			p.applyRepeat(ctx)
		}
		p.publishStatus()
		return nil
	})
}

func (p *Player) setSleep(sl Sleep) {
	p.sleep, p.sleepTimer = sl, nil
	if sl.Mode == SleepTimer {
		p.sleepTimer = time.After(time.Until(time.Unix(sl.At, 0)))
	}
}

// Thresholds for a play that counts, as Last.fm defines a scrobble: half the
// track or 4 minutes, for tracks of 30 seconds or more. Position steps
// above maxHeardStep are seeks, not listening.
const (
	minCountedTrack = 30.0
	maxNeeded       = 240.0
	maxHeardStep    = 3.0
)

func (p *Player) resetPlay(pos float64) {
	p.heard, p.heardPos, p.counted, p.startedAt = 0, pos, false, time.Now()
}

// beginPlay starts a listen of uid at start, unless it goes on with one: a
// reload of the same item mid-track (mpv restart, new output, cast) is the
// same listen and must not count twice.
func (p *Player) beginPlay(uid uint64, start float64) {
	if uid == p.playUID && start > 0 {
		p.heardPos = start
		return
	}
	p.playUID = uid
	p.resetPlay(start)
}

func (p *Player) countPlay(pos float64) {
	step := pos - p.heardPos
	p.heardPos = pos
	// Repeat one loops inside mpv without a new start-file.
	if p.counted && pos < maxHeardStep && step < -minCountedTrack/2 {
		p.resetPlay(pos)
		return
	}
	if !p.paused && step > 0 && step <= maxHeardStep {
		p.heard += step
	}
	it, ok := p.queue.Current()
	if p.counted || !ok || (p.dur > 0 && p.dur < minCountedTrack) {
		return
	}
	need := maxNeeded
	if p.dur > 0 {
		need = min(p.dur/2, maxNeeded)
	}
	if p.heard < need {
		return
	}
	p.counted = true
	t := it.Track
	if p.dur > 0 {
		t.Duration = p.dur
	}
	p.cfg.Publish(TopicPlayed, Played{Track: t, PlayedAt: p.startedAt.Unix()})
}

// continueMode is how the queue continues when it runs out; "" is not at
// all.
func (p *Player) continueMode() string {
	if p.contMode != "" {
		return p.contMode
	}
	if p.opts.AutoContinue == ContinueOff {
		return ""
	}
	return p.opts.AutoContinue
}

// continueTimeout bounds one continue, so a hung server can't hold off
// refills for good.
const continueTimeout = 30 * time.Second

// maybeContinue extends a queue that is about to run out with what
// continuing picks, off the loop like a feed refill.
func (p *Player) maybeContinue() {
	if p.cfg.Continue == nil || p.continueMode() == "" || p.feeding || !p.active {
		return
	}
	if _, more := p.queue.Peek(); more {
		return
	}
	if items := p.queue.Items(); len(items) == 0 || items[len(items)-1].UID == p.contTried {
		return
	}
	p.startContinue(false, nil)
}

// startContinue fetches tracks to follow the queue's last item, off the
// loop, and appends those not queued yet. With play, the track after the
// current one then starts. done, when set, gets the result.
func (p *Player) startContinue(play bool, done chan<- error) {
	finish := func(err error) {
		if done != nil {
			done <- err
		}
	}
	items := p.queue.Items()
	mode := p.continueMode()
	if len(items) == 0 || mode == "" {
		finish(errors.New("end of queue"))
		return
	}
	last := items[len(items)-1]
	gen := p.queueGen
	p.feeding = true
	go func() {
		cctx, cancel := context.WithTimeout(p.runCtx, continueTimeout)
		tracks, err := p.cfg.Continue(cctx, mode, last.Track, queuedRefs(items))
		cancel()
		finish(p.doPlay(p.runCtx, func(ctx context.Context) error {
			p.feeding = false
			if p.queueGen != gen {
				return nil
			}
			if err != nil {
				log.Warnf("continue the queue: %v", err)
			}
			grew := false
			now := p.queue.Items()
			if n := len(now); n > 0 && now[n-1].UID == last.UID {
				queued := queuedRefs(now)
				fresh := slices.DeleteFunc(tracks, func(t source.Track) bool { return queued[t.Ref()] })
				switch {
				case len(fresh) > 0:
					p.queue.Extend(fresh)
					grew = true
				case err == nil:
					// Asking again after the same track would find the same.
					p.contTried = last.UID
				}
			}
			if play {
				if !p.queue.Advance() {
					p.publishStatus()
					return cmp.Or(err, errors.New("found nothing to continue the queue with"))
				}
				return p.loadCurrent(ctx, 0, true)
			}
			if err != nil && !grew {
				// Nothing changed; publishing would ask again at once. The
				// next queue change tries again.
				return err
			}
			p.syncNext(ctx)
			p.publishQueue()
			p.publishStatus()
			return err
		}))
	}()
}

func queuedRefs(items []Item) map[source.Ref]bool {
	out := make(map[source.Ref]bool, len(items))
	for _, it := range items {
		out[it.Track.Ref()] = true
	}
	return out
}
