// Package loudness measures integrated loudness with ffmpeg's ebur128 filter,
// for tracks that carry no ReplayGain tags. ffmpeg is optional.
package loudness

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// ReplayGain 2.0 reference level.
	referenceLUFS  = -18.0
	measureTimeout = 3 * time.Minute
	stderrLimit    = 64 << 10

	// Fixed clamp because a fallback gain has no peak value for
	// mpv's clipping guard; measure true peak too if loud tracks clip.
	minGain = -24.0
	maxGain = 10.0
	// Silence is below silenceNoise dB; only leading silence of minLead
	// seconds and trailing silence of minTrail seconds is skipped.
	silenceNoise = -50.0
	minLead      = 0.3
	minTrail     = 0.5
	// How close to 0 or to the end of the file silence has to reach to be
	// leading or trailing.
	edgeSlack = 0.05
)

var (
	integrated   = regexp.MustCompile(`I:\s+(-?[0-9.]+) LUFS`)
	silenceEvent = regexp.MustCompile(`silence_(start|end): (-?[0-9.]+)`)
	// ffmpeg's final report says how far it decoded, even with -nostats.
	decodedTime = regexp.MustCompile(`time=(\d+):(\d\d):(\d\d(?:\.\d+)?)`)
)

type Meter struct {
	ffmpeg string
	// prefix runs ffmpeg through nice and ionice, so all its threads get
	// the lowest CPU and disk priority from the start.
	prefix []string
	// One measurement at a time across all libraries.
	slot chan struct{}

	mu      sync.Mutex
	loud    bool
	silence bool
	// gen counts the times more measuring became wanted.
	gen     uint64
	waiting []func()
}

// Silence is a file's leading and trailing silence: its music starts at
// Start and ends at End, 0 being the end of the file.
type Silence struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// Result is one measurement: the gain in dB that brings the file to the
// reference loudness, and its silence.
type Result struct {
	Gain    float64
	Silence Silence
}

// Find looks for ffmpeg on PATH. Without it the meter is unavailable and
// Measure fails.
func Find() *Meter {
	path, _ := exec.LookPath("ffmpeg")
	m := &Meter{ffmpeg: path, slot: make(chan struct{}, 1)}
	if nice, err := exec.LookPath("nice"); err == nil {
		m.prefix = append(m.prefix, nice, "-n", "19")
	}
	if ionice, err := exec.LookPath("ionice"); err == nil {
		m.prefix = append(m.prefix, ionice, "-c", "3")
	}
	return m
}

func (m *Meter) Available() bool { return m != nil && m.ffmpeg != "" }

// SetWanted says what background measuring is for: loudness while
// ReplayGain is in use, silence while skipping silence is on. Wanting more
// wakes the loops waiting for it.
func (m *Meter) SetWanted(loudness, silence bool) {
	m.mu.Lock()
	var run []func()
	if loudness && !m.loud || silence && !m.silence {
		m.gen++
		run, m.waiting = m.waiting, nil
	}
	m.loud, m.silence = loudness, silence
	m.mu.Unlock()
	for _, f := range run {
		go f()
	}
}

// Enabled says whether background measuring is wanted at all.
func (m *Meter) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loud || m.silence
}

// Silence says whether files that only lack their silence are measured too.
func (m *Meter) Silence() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.silence
}

func (m *Meter) generation() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gen
}

// whenWanted runs f once more measuring is wanted than at generation gen;
// at once if that already happened.
func (m *Meter) whenWanted(gen uint64, f func()) {
	m.mu.Lock()
	if m.gen == gen {
		m.waiting = append(m.waiting, f)
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	go f()
}

// Measure measures path's loudness and silence in one decode.
func (m *Meter) Measure(ctx context.Context, path string) (Result, error) {
	if !m.Available() {
		return Result{}, errors.New("ffmpeg is not installed")
	}
	select {
	case m.slot <- struct{}{}:
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
	defer func() { <-m.slot }()
	ctx, cancel := context.WithTimeout(ctx, measureTimeout)
	defer cancel()
	// Only local files: a crafted playlist or HLS file must not make ffmpeg
	// open URLs.
	argv := append(slices.Clone(m.prefix), m.ffmpeg, "-nostdin", "-hide_banner", "-nostats", "-threads", "1",
		"-protocol_whitelist", "file", "-i", path, "-map", "0:a:0", "-filter_threads", "1",
		"-af", fmt.Sprintf("silencedetect=noise=%gdB:d=%g,ebur128=framelog=quiet", silenceNoise, minLead), "-f", "null", "-")
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	out := &tail{}
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Result{}, fmt.Errorf("ffmpeg: %w", ctx.Err())
		}
		return Result{}, fmt.Errorf("ffmpeg: %w", err)
	}
	gain, err := parse(out.String())
	if err != nil {
		return Result{}, err
	}
	return Result{Gain: gain, Silence: parseSilence(out.String())}, nil
}

// Job is one file for a Loop; ID is the caller's key for it.
type Job struct{ ID, Path string }

// Loop measures files in the background, one at a time, while the Meter
// wants measuring. Pending lists the next files; with silence set, files
// that only lack their silence count too. Save stores each outcome, err
// being the measurement's. With nothing left to do it sleeps until the
// Meter wants more.
type Loop struct {
	Meter   *Meter
	Pending func(ctx context.Context, silence bool) ([]Job, error)
	Save    func(ctx context.Context, j Job, r Result, err error) error

	running, waiting atomic.Bool
}

// Start runs the loop unless it runs already; it ends with ctx.
func (l *Loop) Start(ctx context.Context) {
	if !l.Meter.Available() || ctx.Err() != nil || !l.running.CompareAndSwap(false, true) {
		return
	}
	go func() {
		gen := l.Meter.generation()
		defer func() {
			l.running.Store(false)
			if ctx.Err() == nil && l.waiting.CompareAndSwap(false, true) {
				l.Meter.whenWanted(gen, func() {
					l.waiting.Store(false)
					l.Start(ctx)
				})
			}
		}()
		for l.Meter.Enabled() {
			gen = l.Meter.generation()
			jobs, err := l.Pending(ctx, l.Meter.Silence())
			if err != nil || len(jobs) == 0 {
				return
			}
			for _, j := range jobs {
				if !l.Meter.Enabled() {
					return
				}
				r, err := l.Meter.Measure(ctx, j.Path)
				if ctx.Err() != nil {
					return
				}
				if err := l.Save(ctx, j, r, err); err != nil {
					return
				}
			}
		}
	}()
}

// parse reads the summary's integrated loudness; the last match is the
// summary, earlier ones would be per-frame logs.
func parse(stderr string) (float64, error) {
	matches := integrated.FindAllStringSubmatch(stderr, -1)
	if len(matches) == 0 {
		return 0, errors.New("ffmpeg printed no loudness summary")
	}
	lufs, err := strconv.ParseFloat(matches[len(matches)-1][1], 64)
	if err != nil {
		return 0, err
	}
	return min(max(referenceLUFS-lufs, minGain), maxGain), nil
}

// parseSilence finds leading and trailing silence in silencedetect's log.
// Silence inside the track is never skipped, and a file that is silent
// throughout is played whole.
func parseSilence(stderr string) Silence {
	type span struct{ start, end float64 }
	var spans []span
	for _, m := range silenceEvent.FindAllStringSubmatch(stderr, -1) {
		v, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		switch {
		case m[1] == "start":
			spans = append(spans, span{start: v, end: -1})
		case len(spans) > 0 && spans[len(spans)-1].end < 0:
			spans[len(spans)-1].end = v
		}
	}
	if len(spans) == 0 {
		return Silence{}
	}
	eof := -1.0
	if ms := decodedTime.FindAllStringSubmatch(stderr, -1); len(ms) > 0 {
		m := ms[len(ms)-1]
		h, _ := strconv.ParseFloat(m[1], 64)
		mi, _ := strconv.ParseFloat(m[2], 64)
		sec, _ := strconv.ParseFloat(m[3], 64)
		eof = h*3600 + mi*60 + sec
	}
	// An open span runs to the end of the file; older ffmpeg doesn't close it.
	if last := &spans[len(spans)-1]; last.end < 0 {
		last.end = eof
	}
	first, last := spans[0], spans[len(spans)-1]
	trailing := eof > 0 && last.end >= eof-edgeSlack
	if first.start <= edgeSlack && trailing && len(spans) == 1 {
		return Silence{}
	}
	var s Silence
	if first.start <= edgeSlack && first.end-first.start >= minLead {
		s.Start = first.end
	}
	if trailing && last.end-last.start >= minTrail && last.start > s.Start {
		s.End = last.start
	}
	return s
}

// tail keeps the last stderrLimit bytes written to it.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - stderrLimit; over > 0 {
		t.buf = t.buf[over:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
