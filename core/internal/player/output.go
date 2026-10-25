package player

// Where playback goes. Features outside the player steer it only through
// this file (audio device and env, a Remote, restart), and only this file
// knows a crossfade runs a second mpv.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/source"
)

// restart replaces mpv (after a crash, or for a new environment) and resumes
// the current track where it was, paused or playing. A remote keeps playing.
func (p *Player) restart(ctx context.Context) error {
	p.dropProc()
	if err := p.ensureProc(ctx); err != nil {
		p.active = false
		return err
	}
	if !p.active || p.remote != nil {
		return nil
	}
	// pause is global in mpv, so setting it first keeps a paused track silent.
	p.setProp(ctx, "pause", p.paused)
	if err := p.loadCurrent(ctx, p.pos, false); err != nil && !errors.Is(err, errLoading) {
		p.active = false
		return err
	}
	return nil
}

func (p *Player) dropProc() {
	p.stopFade()
	if p.proc != nil {
		p.proc.kill()
	}
	p.proc = nil
	p.curEntry, p.nextEntry, p.nextUID = 0, 0, 0
}

// AudioDevices returns mpv's audio-device-list; changes arrive as
// TopicAudioDevices events. While mpv is stopped for being idle, it is the
// list mpv last reported.
func (p *Player) AudioDevices(ctx context.Context) (json.RawMessage, error) {
	var list json.RawMessage
	err := p.do(ctx, func(ctx context.Context) error {
		if p.proc == nil && p.deviceList != nil {
			list = p.deviceList
			return nil
		}
		if err := p.ensureProc(ctx); err != nil {
			return err
		}
		if !p.devices {
			list = json.RawMessage("[]")
			return nil
		}
		var err error
		list, err = p.cmd(ctx, "get_property", "audio-device-list")
		return err
	})
	return list, err
}

// SetAudioOutput sends playback to an mpv audio device. A different env
// only reaches a new mpv, so mpv restarts and resumes where it was.
func (p *Player) SetAudioOutput(ctx context.Context, device string, env []string) error {
	return p.doPlay(ctx, func(ctx context.Context) error {
		p.audioDevice = device
		// A fading mpv would stay on the old device.
		p.stopFade()
		if p.proc == nil || slices.Equal(env, p.audioEnv) {
			p.audioEnv = env
			if p.proc != nil {
				p.setProp(ctx, "audio-device", device)
			}
			return nil
		}
		p.audioEnv = env
		return p.restart(ctx)
	})
}

// Remote plays the queue on another device (a cast receiver). While one is
// attached, mpv stays idle and every transport command goes to the remote.
// Its methods are called on the player loop and must not block.
type Remote interface {
	// Load plays items[index] from start seconds; items is the whole queue.
	// start < 0 is a queue edit while items[index] plays on.
	Load(items []Item, index int, start float64)
	SetPaused(paused bool)
	Seek(pos float64)
	SetVolume(level float64)
	// SetLoop mirrors repeat all: the remote starts over after the last item.
	SetLoop(on bool)
	Stop()
}

// SetRemote hands playback to r at the current track and position, playing,
// or with r nil takes it back: local playback continues at the last
// reported position, playing when resume is set.
func (p *Player) SetRemote(ctx context.Context, r Remote, resume bool) error {
	return p.doPlay(ctx, func(ctx context.Context) error {
		if r == nil {
			if p.remote == nil {
				return nil
			}
			p.remote = nil
			p.active = false
			p.volume = p.localVolume
			if !resume {
				p.publishStatus()
				return nil
			}
			return p.resume(ctx)
		}
		pos := p.pos
		if p.remote == nil {
			p.localVolume = p.volume
			p.stop(ctx)
		}
		p.remote = r
		if _, ok := p.queue.Current(); !ok {
			p.publishStatus()
			return nil
		}
		// The remote starts playing on load.
		p.paused = false
		return p.loadCurrent(ctx, pos, false)
	})
}

// RemoteUpdate is what the remote reports: the queue entry it plays (0 when
// unknown), its position, whether it is paused and its volume (negative
// when unknown).
func (p *Player) RemoteUpdate(ctx context.Context, uid uint64, pos float64, paused bool, volume float64) error {
	return p.do(ctx, func(context.Context) error {
		if p.remote == nil {
			return nil
		}
		if uid != 0 && uid != p.queue.currentUID() {
			p.queue.selectUID(uid)
			if it, ok := p.queue.Current(); ok {
				p.dur = it.Track.Duration
			}
			p.lastPos = -1
			p.beginPlay(uid, 0)
			p.publishQueue()
		}
		p.paused = paused
		if volume >= 0 {
			p.volume = volume
		}
		p.pos = max(pos, 0)
		p.countPlay(p.pos)
		if math.Abs(p.pos-p.lastPos) >= 1 {
			p.publishPosition()
		}
		p.publishStatus()
		return nil
	})
}

func (p *Player) loadRemote(start float64) {
	p.active = true
	p.loaded, p.pendingSeek = true, -1
	p.pos, p.lastPos = start, -1
	p.beginPlay(p.queue.currentUID(), start)
	if it, ok := p.queue.Current(); ok {
		p.dur = it.Track.Duration
	}
	p.remote.SetLoop(p.queue.Repeat() == RepeatAll)
	p.remote.Load(p.queue.Items(), p.queue.Index(), start)
	p.publishQueue()
	p.publishStatus()
}

// A crossfade runs a second mpv: fadeLead seconds early it comes up paused
// with the next track and a per-file fade-in, and the main mpv gets the
// fade-out. At the fade it becomes the main mpv while the old one plays out.
// afade uses stream timestamps, so both ramps are sample-accurate.
const fadeLead = 2.0

// fadeHead starts the second mpv this much early: it takes about that long
// from unpausing to sound (measured 55 ms behind the fade-out without it).
const fadeHead = 0.05

type fade struct {
	uid uint64
	// at is where in the old track the fade starts; length is its duration.
	// start is where the next track starts.
	at, length, start float64
	// in is the mpv lined up with the next track (entry), until it takes
	// over; out is the old one playing out the fade after that.
	in, out *process
	entry   int64
	// skip is set when the second mpv couldn't start: this change of track
	// stays gapless.
	skip bool
}

// fadeLinedUp is a fade that hasn't started yet and still needs the next
// track to stay the same.
func (p *Player) fadeLinedUp() bool {
	return p.xf != nil && p.xf.out == nil && !p.xf.skip
}

// The old mpv of a running fade is done at its end-file (or exit); every
// other change of track or position stops the fade before that.
func (p *Player) fadeChans() (notify, exited <-chan struct{}) {
	if p.xf == nil || p.xf.out == nil {
		return nil, nil
	}
	return p.xf.out.conn.notify, p.xf.out.exited
}

// crossfadeNext says whether the change to the next track should fade.
func (p *Player) crossfadeNext() bool {
	if p.opts.Crossfade <= 0 || p.remote != nil || !p.active || !p.loaded || p.dur <= 0 || p.proc == nil ||
		p.queue.Repeat() == RepeatOne || p.sleep.Mode == SleepEndOfTrack {
		return false
	}
	cur, ok := p.queue.Current()
	next, more := p.queue.Peek()
	if !ok || !more || next.UID == cur.UID {
		return false
	}
	return !p.opts.AlbumGapless || !sameAlbum(cur.Track, next.Track)
}

func sameAlbum(a, b source.Track) bool {
	if a.Library != b.Library || a.Album == source.UnknownAlbum || b.Album == source.UnknownAlbum {
		return false
	}
	if a.AlbumID != "" || b.AlbumID != "" {
		return a.AlbumID == b.AlbumID
	}
	return a.Album != "" && a.Album == b.Album && a.AlbumArtist == b.AlbumArtist
}

// crossfade follows the main mpv's position: it lines up a fade near the end
// of the track and starts it on time.
func (p *Player) crossfade() {
	if p.xf != nil {
		if p.xf.in != nil && p.pos >= p.xf.at-fadeHead {
			p.startFade()
		}
		return
	}
	n := p.opts.Crossfade
	// A fade longer than half the next track would run into its own end.
	if next, ok := p.queue.Peek(); ok && next.Track.Duration > 0 {
		n = min(n, next.Track.Duration/2)
	}
	end := p.playEnd()
	if p.pos < end-n-fadeLead || !p.crossfadeNext() {
		return
	}
	// The fade-out has to reach mpv before it decodes that far.
	at := max(end-n, p.pos+1)
	if end-at < 1 {
		return
	}
	next, _ := p.queue.Peek()
	xf := &fade{uid: next.UID, at: at, length: end - at}
	p.xf = xf
	socket := p.cfg.Socket + ".2"
	if p.proc.socket == socket {
		socket = p.cfg.Socket
	}
	ctx, env := p.runCtx, p.audioEnv
	go func() {
		proc, err := startProcess(ctx, p.cfg.Binary, socket, p.cfg.ExtraArgs, env)
		if p.do(ctx, func(ctx context.Context) error { p.fadeReady(ctx, xf, proc, err); return nil }) != nil && proc != nil {
			proc.kill()
		}
	}()
}

func (p *Player) fadeReady(ctx context.Context, xf *fade, proc *process, err error) {
	if p.xf != xf {
		if proc != nil {
			proc.kill()
		}
		return
	}
	next, ok := p.queue.Peek()
	if err == nil && (!ok || next.UID != xf.uid || !p.crossfadeNext()) {
		err = fmt.Errorf("the next track changed")
	}
	if err == nil {
		err = p.lineUp(ctx, xf, proc, next)
	}
	if err != nil {
		log.Warnf("crossfade: %v; playing gapless", err)
		if proc != nil {
			proc.kill()
		}
		xf.skip = true
		p.syncNext(ctx)
		return
	}
	xf.in = proc
}

// lineUp loads next on proc, paused, and gives the main mpv its fade-out.
func (p *Player) lineUp(ctx context.Context, xf *fade, proc *process, next Item) error {
	p.observe(ctx, proc)
	props := append(p.optionProps(), prop{"mute", p.mpvMuted()}, prop{"pause", true}, prop{"audio-device", p.audioDevice})
	for _, pr := range props {
		if pr.name == "audio-device" && p.audioDevice == "" {
			continue
		}
		if _, err := proc.conn.command(ctx, "set_property", pr.name, pr.value); err != nil {
			return fmt.Errorf("set %s: %w", pr.name, err)
		}
	}
	st, ok := p.streams[next.UID]
	if !ok {
		return errors.New("the next track isn't resolved yet")
	}
	cur, _ := p.queue.Current()
	xf.start, _ = trimmed(p.opts, st.Silence, p.joins(cur, next), false)
	in := fmt.Sprintf("lavfi=[afade=t=in:curve=qsin:st=%.3f:d=%.3f]", xf.start, xf.length)
	entry, err := p.loadfile(ctx, proc, next, st, "replace", xf.start, fmt.Sprintf("af=%%%d%%%s", len(in), in))
	if err != nil {
		return err
	}
	// The main mpv must not move on to the next track by itself any more.
	if p.nextEntry != 0 {
		if _, err := p.cmd(ctx, "playlist-clear"); err != nil {
			return err
		}
		p.nextEntry, p.nextUID = 0, 0
	}
	xf.entry = entry
	p.setProp(ctx, "af", fmt.Sprintf("lavfi=[afade=t=out:curve=qsin:st=%.3f:d=%.3f]", xf.at, xf.length))
	return nil
}

// startFade makes the lined-up mpv the main one and lets the old one fade
// out.
func (p *Player) startFade() {
	xf := p.xf
	xf.out, p.proc, xf.in = p.proc, xf.in, nil
	p.queue.selectUID(xf.uid)
	p.curEntry = xf.entry
	p.nextEntry, p.nextUID = 0, 0
	p.pos, p.lastPos = xf.start, -1
	p.startAt, p.endAt = xf.start, 0
	p.beginPlay(xf.uid, 0)
	p.heardPos = xf.start
	if it, ok := p.queue.Current(); ok {
		p.dur = it.Track.Duration
	}
	p.loaded, p.pendingSeek = false, -1
	// What the new main mpv said while it waited (file loaded, duration);
	// its pause was only for waiting.
	paused := p.paused
	for _, m := range p.proc.conn.takeEvents() {
		if m.Event != "property-change" || m.Name != "pause" {
			p.handle(m)
		}
	}
	ctx := context.Background()
	p.setProp(ctx, "pause", paused)
	p.syncNext(ctx)
	p.publishQueue()
	p.publishStatus()
}

// onFadeEvents ends the fade once the old mpv has played its track out.
func (p *Player) onFadeEvents() {
	for _, m := range p.xf.out.conn.takeEvents() {
		if m.Event == "end-file" || m.Event == "idle" {
			p.endFade()
			return
		}
	}
}

func (p *Player) endFade() {
	if p.xf != nil && p.xf.out != nil {
		p.xf.out.kill()
	}
	p.xf = nil
}

// stopFade ends any crossfade at once: a lined-up one is dropped and the
// main mpv loses its fade-out, a running one cuts the old track off.
func (p *Player) stopFade() {
	xf := p.xf
	if xf == nil {
		return
	}
	p.xf = nil
	if xf.in != nil {
		xf.in.kill()
		if p.proc != nil {
			p.setProp(context.Background(), "af", "")
		}
	}
	if xf.out != nil {
		xf.out.kill()
	}
}

// mirror sets a property on the other mpv of a crossfade too, so volume,
// mute, speed and pause act on both. The lined-up one stays paused.
func (p *Player) mirror(ctx context.Context, name string, value any) {
	if p.xf == nil {
		return
	}
	switch name {
	case "volume", "volume-max", "mute", "speed", "replaygain", "gapless-audio":
	case "pause":
		if p.xf.out == nil {
			return
		}
	default:
		return
	}
	for _, proc := range []*process{p.xf.in, p.xf.out} {
		if proc == nil {
			continue
		}
		if _, err := proc.conn.command(ctx, "set_property", name, value); err != nil {
			log.Warnf("mpv set %s on the fading mpv: %v", name, err)
		}
	}
}
