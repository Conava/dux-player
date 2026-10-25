package player

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/source"
)

// Where the current track ends and the next one starts when skipping
// silence, with the album rule and crossfade.
func TestSkipSilenceTrims(t *testing.T) {
	sil := loudness.Silence{Start: 1, End: 9}
	tests := []struct {
		name         string
		start, end   bool
		albumGapless bool
		crossfade    float64
		albums       []string
		wantEnd      float64
		wantNext     float64
	}{
		{name: "off plays whole files", albumGapless: true, albums: []string{"x", "y"}},
		{name: "different albums are trimmed", start: true, end: true, albumGapless: true, albums: []string{"x", "y"}, wantEnd: 9, wantNext: 1},
		{name: "only the start", start: true, albumGapless: true, albums: []string{"x", "y"}, wantNext: 1},
		{name: "only the end", end: true, albumGapless: true, albums: []string{"x", "y"}, wantEnd: 9},
		{name: "a gapless album change stays whole", start: true, end: true, albumGapless: true, albums: []string{"x", "x"}},
		{name: "same album without the album rule is trimmed", start: true, end: true, albums: []string{"x", "x"}, wantEnd: 9, wantNext: 1},
		{name: "crossfade between albums fades the music", start: true, end: true, albumGapless: true, crossfade: 3, albums: []string{"x", "y"}, wantEnd: 9, wantNext: 1},
		{name: "crossfade keeps a gapless album whole", start: true, end: true, albumGapless: true, crossfade: 3, albums: []string{"x", "x"}},
		{name: "the last track ends at its music", start: true, end: true, albumGapless: true, albums: []string{"x"}, wantEnd: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := DefaultOptions()
			opts.SkipSilenceStart, opts.SkipSilenceEnd = tt.start, tt.end
			opts.AlbumGapless, opts.Crossfade = tt.albumGapless, tt.crossfade
			p := New(Config{Options: opts})
			var q []source.Track
			for i, album := range tt.albums {
				q = append(q, source.Track{Library: "l", ID: string(rune('a' + i)), Album: album, AlbumID: album})
			}
			p.queue.Replace(q, 0, Origin{Kind: "adhoc"})
			for _, it := range p.queue.Items() {
				p.streams[it.UID] = Stream{Silence: sil}
			}
			assert.Equal(t, tt.wantEnd, p.trimEnd(), "end of the current track")
			if next, ok := p.queue.Peek(); ok {
				cur, _ := p.queue.Current()
				start, _ := trimmed(p.opts, sil, p.joins(cur, next), false)
				assert.Equal(t, tt.wantNext, start, "start of the next track")
			}
		})
	}
}

type silenceResolver map[string]Stream

func (r silenceResolver) Stream(_ context.Context, ref source.Ref) (Stream, error) {
	return r[ref.ID], nil
}

// Runs real mpv (null output): a track plays from the end of its leading
// silence to the start of its trailing silence, and a crossfade fades
// between the music, not into the silence.
func TestSkipSilenceInMPV(t *testing.T) {
	bin, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv not installed")
	}
	dir := t.TempDir()
	res := silenceResolver{}
	// The crossfade needs fadeLead seconds of music ahead of the fade.
	for id, sil := range map[string]loudness.Silence{"a": {Start: 1, End: 6.5}, "b": {Start: 1, End: 2.5}} {
		path := filepath.Join(dir, id+".wav")
		writeWAV(t, path, sil.End+1.5)
		res[id] = Stream{URL: path, Silence: sil}
	}
	for _, crossfade := range []float64{0, 1} {
		t.Run("crossfade "+string(rune('0'+int(crossfade))), func(t *testing.T) {
			opts := DefaultOptions()
			opts.SkipSilenceStart, opts.SkipSilenceEnd, opts.Crossfade = true, true, crossfade
			rec := &recorder{}
			p := New(Config{Binary: bin, Socket: filepath.Join(dir, "mpv.sock"), ExtraArgs: []string{"--ao=null"},
				Resolver: res, Options: opts, Publish: rec.publish})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go p.Run(ctx)
			q := []source.Track{
				{Library: "l", ID: "a", Title: "a", Album: "x", Duration: 8},
				{Library: "l", ID: "b", Title: "b", Album: "y", Duration: 4},
			}
			begin := time.Now()
			require.NoError(t, p.Play(ctx, q, 0, Origin{Kind: "adhoc"}))

			var fadeAt float64
			var changed time.Duration
			maxPos := map[int]float64{}
			minPos := map[int]float64{0: 99, 1: 99}
			for time.Since(begin) < 25*time.Second {
				var st Status
				_ = p.do(ctx, func(context.Context) error {
					st = p.status()
					if p.xf != nil && fadeAt == 0 {
						fadeAt = p.xf.at
					}
					return nil
				})
				if st.State == StateStopped {
					break
				}
				if st.Index == 1 && changed == 0 {
					changed = time.Since(begin)
				}
				if st.Position > 0 {
					maxPos[st.Index] = max(maxPos[st.Index], st.Position)
					minPos[st.Index] = min(minPos[st.Index], st.Position)
				}
				time.Sleep(10 * time.Millisecond)
			}
			assert.GreaterOrEqual(t, minPos[0], 0.99, "the first track starts after its leading silence")
			assert.GreaterOrEqual(t, minPos[1], 0.99, "the next track starts after its leading silence")
			assert.LessOrEqual(t, maxPos[1], 2.6, "the last track ends at its trailing silence")
			if crossfade == 0 {
				assert.InDelta(t, 5.5, changed.Seconds(), 0.4, "the change of track comes at the first track's trailing silence")
			} else {
				assert.InDelta(t, 5.5, fadeAt, 0.01, "the fade ends where the music ends")
			}
		})
	}
}
