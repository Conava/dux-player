package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
)

// pump decodes the current entry into the output stream. It alone touches
// the stream and the decoders of the current entry.
type pump struct {
	e  *engine
	st Stream
	// stFmt and stSink are what the stream was opened with; corked mirrors
	// its pause.
	stFmt  format
	stSink string
	corked bool

	cur *entry
	// frame is the decoder's position in cur; pending holds gained frames
	// not written yet; eof is set once the decoder ran out.
	frame   int64
	pending []float32
	eof     bool
	// segAt is the stream's written count where cur's segFrom frame went
	// in: the base for the position heard.
	segAt, segFrom int64
	// lastHeard is the frame last reported as heard.
	lastHeard int64
	lastTick  time.Time
	buf       []float32
}

func (e *engine) pump() {
	p := &pump{e: e}
	defer p.closeStream()
	for e.ctx.Err() == nil {
		p.step()
	}
}

// step does one unit of work and returns; anything that waits returns early
// when a command changes the state (gen).
func (p *pump) step() {
	e := p.e
	e.mu.Lock()
	cur, paused, seek, reopen, wctx := e.cur, e.paused, e.seek, e.reopen, e.wctx
	e.seek, e.reopen = -1, false
	e.mu.Unlock()

	if cur != p.cur {
		p.begin(cur)
		return
	}
	if cur == nil {
		p.closeStream()
		p.wait(wctx, nil)
		return
	}
	if p.frame < 0 {
		// Not loaded yet.
		if !p.wait(wctx, cur.ready) {
			return
		}
		p.loaded()
		return
	}
	if reopen {
		p.moveDevice()
	}
	if seek >= 0 {
		p.seek(seek)
	}
	if paused {
		p.setCorked(true)
		p.report(true)
		p.wait(wctx, nil)
		return
	}
	if p.eof {
		p.finish(wctx)
		return
	}
	if p.st == nil && !p.open() {
		return
	}
	p.setCorked(false)
	if len(p.pending) == 0 && !p.decode() {
		return
	}
	// A stream whose sink is gone stays open and just stops taking audio.
	sctx, cancel := context.WithTimeout(wctx, stallTimeout)
	n, err := p.st.Write(sctx, p.pending)
	cancel()
	p.pending = p.pending[n:]
	if err != nil && wctx.Err() == nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = errStalled
		}
		sink := p.stSink
		p.closeStream()
		// Without a stream the position falls back to segFrom.
		p.segFrom = p.lastHeard
		p.outputFailed(sink, err)
	}
	p.report(false)
}

// wait blocks until a command changes the state or ch closes; it says
// whether ch closed.
func (p *pump) wait(wctx context.Context, ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-wctx.Done():
	case <-p.e.wake:
	case <-p.e.ctx.Done():
	}
	return false
}

// begin makes en current: a new file replaced the old one, or the old one
// ended.
func (p *pump) begin(en *entry) {
	if p.cur != nil && p.cur.err == nil && p.st != nil && !p.eof {
		// Replaced while playing: cut it off.
		_ = p.st.Flush()
	}
	p.cur, p.frame, p.pending, p.eof = en, -1, nil, false
	if en != nil {
		p.e.event("start-file", map[string]any{"playlist_entry_id": en.id})
	}
}

// loaded follows the current entry's open: it fails, or it is ready to play.
func (p *pump) loaded() {
	cur := p.cur
	if cur.err != nil {
		reason := cur.err
		if errors.Is(reason, errNeedsMPV) {
			reason = errNeedsMPV
		}
		p.e.endFile(cur, "error", reason)
		p.advance(cur)
		return
	}
	f := cur.dec.format()
	p.frame = int64(cur.start * float64(f.rate))
	p.segFrom = p.frame
	if p.st != nil && p.stFmt == f && p.stSink == p.e.sink() {
		// Gapless: the new file goes on after what is in the stream.
		if written, _, err := p.st.Position(); err == nil {
			p.segAt = written
		}
	} else {
		p.closeStream()
	}
	p.e.event("file-loaded", nil)
	if d := cur.duration(); d > 0 {
		p.e.property("duration", d)
	}
	p.e.property("seekable", cur.in.seekable)
	p.lastTick = time.Time{}
	p.report(true)
}

// advance moves on from en to the next entry, if any.
func (p *pump) advance(en *entry) {
	e := p.e
	e.mu.Lock()
	if e.cur == en {
		e.cur, e.ranOut = nil, true
		if len(e.next) > 0 {
			e.cur, e.next, e.ranOut = e.next[0], e.next[1:], false
		}
	}
	e.mu.Unlock()
	en.close()
}

func (e *engine) sink() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return sinkName(e.device)
}

// stallTimeout is how long a playing output may refuse audio.
const stallTimeout = 5 * time.Second

var errStalled = errors.New("the output stopped taking audio")

// outputFailed pauses and says why.
func (p *pump) outputFailed(sink string, err error) {
	if sink == "" {
		sink = "default output"
	}
	p.e.event(OutputError, map[string]any{"file_error": fmt.Sprintf("%s: %v", sink, err)})
	p.e.pauseFor()
}

func (p *pump) open() bool {
	f := p.cur.dec.format()
	sink := p.e.sink()
	st, err := p.e.out.Open(f, sink, p.e.props, p.e.wait, false)
	if err != nil {
		p.outputFailed(sink, err)
		return false
	}
	p.st, p.stFmt, p.stSink, p.corked = st, f, sink, false
	p.segAt, p.segFrom = 0, p.frame
	return true
}

func (p *pump) closeStream() {
	if p.st != nil {
		p.st.Close()
		p.st = nil
	}
}

// pauseFor pauses after the output failed, as if the user had.
func (e *engine) pauseFor() {
	e.mu.Lock()
	e.paused = true
	e.mu.Unlock()
	e.property("pause", true)
}

func (p *pump) setCorked(c bool) {
	if p.st == nil || p.corked == c {
		return
	}
	if err := p.st.SetPaused(c); err != nil {
		log.Warnf("built-in player: pause: %v", err)
	}
	p.corked = c
}

// decode reads the next chunk into pending, with gain applied. False when
// the file ended (eof set) or failed.
func (p *pump) decode() bool {
	cur := p.cur
	f := cur.dec.format()
	frames := int64(f.rate / 20)
	p.e.mu.Lock()
	end := cur.end
	p.e.mu.Unlock()
	if end > 0 {
		frames = min(frames, int64(end*float64(f.rate))-p.frame)
	}
	if frames <= 0 {
		p.eof = true
		return false
	}
	if need := int(frames) * f.channels; cap(p.buf) < need {
		p.buf = make([]float32, need)
	}
	var n int
	err := safely(func() error {
		var err error
		n, err = cur.dec.read(p.buf[:int(frames)*f.channels])
		return err
	})
	if n > 0 {
		p.frame += int64(n / f.channels)
		p.e.mu.Lock()
		g := replayGain(p.e.rgMode, cur.tags, cur.fallback) * volumeFactor(p.e.volume, p.e.muted)
		p.e.mu.Unlock()
		out := p.buf[:n]
		for i, v := range out {
			out[i] = clamp(float64(v) * g)
		}
		p.pending = out
		return true
	}
	if errors.Is(err, io.EOF) {
		p.eof = true
		return false
	}
	log.Warnf("built-in player: decode: %v", err)
	p.e.endFile(cur, "error", err)
	p.advance(cur)
	return false
}

// finish ends the current file once the decoder ran out: it loops, goes on
// gapless to the next file on the same stream, or lets the stream play out
// first.
func (p *pump) finish(wctx context.Context) {
	cur, e := p.cur, p.e
	e.mu.Lock()
	loop, gapless := e.loop, e.gapless
	var next *entry
	if len(e.next) > 0 {
		next = e.next[0]
	}
	e.mu.Unlock()

	if loop {
		if err := safely(func() error { return cur.dec.seek(0) }); err == nil {
			p.frame, p.eof = 0, false
			if p.st != nil {
				if written, _, err := p.st.Position(); err == nil {
					p.segAt, p.segFrom = written, 0
				}
			}
			return
		}
	}
	if next != nil && gapless != "no" {
		if !p.wait(wctx, next.ready) {
			return
		}
		if next.err == nil && next.dec.format() == p.stFmt {
			e.endFile(cur, "eof", nil)
			p.advance(cur)
			return
		}
	}
	if !p.drained(wctx) {
		return
	}
	e.endFile(cur, "eof", nil)
	p.closeStream()
	p.advance(cur)
}

// drained lets the stream play out what it holds; false when a command
// came first.
func (p *pump) drained(wctx context.Context) bool {
	if p.st == nil {
		return true
	}
	p.st.Drain()
	for {
		written, played, err := p.st.Position()
		if err != nil || played >= written {
			return true
		}
		p.report(false)
		select {
		case <-time.After(20 * time.Millisecond):
		case <-wctx.Done():
			return false
		case <-p.e.ctx.Done():
			return false
		}
	}
}

func (p *pump) seek(pos float64) {
	cur := p.cur
	target := int64(pos * float64(cur.dec.format().rate))
	if l := cur.dec.length(); l > 0 {
		target = min(target, l)
	}
	if err := safely(func() error { return seekTo(cur.dec, target, p.frame) }); err != nil {
		log.Warnf("built-in player: seek: %v", err)
		return
	}
	p.frame, p.pending, p.eof = target, nil, false
	p.flushed()
	p.e.event("seek", nil)
	p.report(true)
}

// flushed drops what the stream holds, so the position heard starts again
// at frame.
func (p *pump) flushed() {
	p.segFrom = p.frame
	if p.st == nil {
		return
	}
	if err := p.st.Flush(); err != nil {
		p.closeStream()
		return
	}
	if written, _, err := p.st.Position(); err == nil {
		p.segAt = written
	}
}

// moveDevice reopens the stream on the new device, from what was heard.
func (p *pump) moveDevice() {
	if p.st == nil {
		return
	}
	heard := p.heard()
	if err := safely(func() error { return p.cur.dec.seek(heard) }); err == nil {
		p.frame, p.pending, p.eof = heard, nil, false
	}
	p.closeStream()
}

// heard is the frame of cur that is playing now.
func (p *pump) heard() int64 {
	if p.st == nil {
		return p.segFrom
	}
	_, played, err := p.st.Position()
	if err != nil {
		return p.segFrom
	}
	return p.segFrom + max(played-p.segAt, 0)
}

// report sends time-pos every tick while playing, and at once with now.
func (p *pump) report(now bool) {
	if !now && time.Since(p.lastTick) < tick {
		return
	}
	p.lastTick = time.Now()
	p.lastHeard = p.heard()
	pos := float64(p.lastHeard) / float64(p.cur.dec.format().rate)
	p.e.mu.Lock()
	p.e.pos = pos
	p.e.mu.Unlock()
	p.e.property("time-pos", pos)
}
