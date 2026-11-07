package lyrics

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

type localLib struct{ dir string }

func (l localLib) Library() source.Library                             { return source.Library{ID: "loc", Name: "Music"} }
func (l localLib) Track(context.Context, string) (source.Track, error) { return source.Track{}, nil }
func (l localLib) StreamURL(_ context.Context, id string) (string, error) {
	return filepath.Join(l.dir, id), nil
}
func (l localLib) Close() error { return nil }

type serverLib struct{ localLib }

func (serverLib) Library() source.Library                         { return source.Library{ID: "srv", Name: "Navidrome"} }
func (serverLib) Ping(context.Context) (source.ServerInfo, error) { return source.ServerInfo{}, nil }
func (serverLib) StreamURL(context.Context, string) (string, error) {
	return "https://server/stream?t=secret", nil
}
func (serverLib) Lyrics(_ context.Context, id string) (*duxlyrics.Lyrics, error) {
	if id != "song" {
		return &duxlyrics.Lyrics{}, nil
	}
	return &duxlyrics.Lyrics{Synced: []duxlyrics.Line{{Start: 1, Text: "from the server"}}}, nil
}

type libs map[string]source.Source

func (l libs) Get(id string) (source.Source, error) { return l[id], nil }
func (l libs) Tracks(_ context.Context, refs []source.Ref) ([]source.Track, error) {
	return []source.Track{{Library: refs[0].Library, ID: refs[0].ID, Title: "t", Artist: "a"}}, nil
}

// Local tracks find their sidecar file, server tracks their server's
// lyrics; no provider is on, so nothing touches the network.
func TestLookupUsesSidecarsAndTheServer(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "song.flac"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "song.lrc"), []byte("[00:01.00]from the sidecar\n"), 0o644))
	s := New(context.Background(), Config{CacheDir: looseDir(t), ConfigDir: dir, Libraries: libs{"loc": localLib{dir}, "srv": serverLib{}},
		Publish: func(string, any) {}, HasSubscriber: func(string) bool { return false }})
	var off []Provider
	for _, p := range defaultProviders() {
		off = append(off, Provider{ID: p.ID, Enabled: p.ID == ServerSource})
	}
	_, err := s.providers.set(off)
	require.NoError(t, err)

	tests := []struct {
		ref    source.Ref
		source duxlyrics.Provider
		text   string
	}{
		{source.Ref{Library: "loc", ID: "song.flac"}, duxlyrics.Sidecar, "from the sidecar"},
		{source.Ref{Library: "srv", ID: "song"}, ServerSource, "from the server"},
		{source.Ref{Library: "srv", ID: "other"}, "", ""},
	}
	for _, tt := range tests {
		res, err := s.Get(context.Background(), tt.ref)
		require.NoError(t, err)
		assert.Equal(t, tt.source, res.Source, tt.ref.ID)
		if tt.text == "" {
			assert.False(t, res.Found)
			continue
		}
		require.NotEmpty(t, res.Synced)
		assert.Equal(t, tt.text, res.Synced[0].Text)
	}
}

func TestProviderSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), providersFile)
	s := openProviders(path)
	assert.Equal(t, defaultProviders(), s.get())

	_, err := s.set([]Provider{{ID: "nope"}})
	assert.Error(t, err)

	cli, err := providerParam([]any{"lrclib", "kugou"})
	require.NoError(t, err)
	_, err = s.set(cli)
	require.NoError(t, err)
	assert.Equal(t, []duxlyrics.Provider{duxlyrics.LRCLIB, duxlyrics.KuGou}, openProviders(path).enabled())

	// A saved file with an unknown id and a provider missing keeps working.
	require.NoError(t, os.WriteFile(path, []byte(`{"providers":[{"id":"gone","enabled":true},{"id":"kugou","enabled":true}]}`), 0o644))
	got := openProviders(path).get()
	assert.Equal(t, ServerSource, got[0].ID, "a file from before the server row gets it first")
	assert.True(t, got[0].Enabled)
	assert.Equal(t, "kugou", got[1].ID)
	assert.Len(t, got, len(defaultProviders()))
}

// Lyrics are looked up only while someone watches, once per track.
func TestCurrentTrackLyricsAreLazy(t *testing.T) {
	dir := t.TempDir()
	events := make(chan Event, 10)
	watching := false
	s := New(context.Background(), Config{CacheDir: looseDir(t), ConfigDir: dir, Libraries: libs{"srv": serverLib{}},
		Publish: func(_ string, d any) { events <- d.(Event) }, HasSubscriber: func(string) bool { return watching }})
	_, err := s.providers.set([]Provider{{ID: "lrclib"}, {ID: "betterlyrics"}, {ID: "unison"}, {ID: "lyricsplus"}, {ID: "kugou"}, {ID: "youtubemusic"}})
	require.NoError(t, err)

	s.TrackChanged(&source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"})
	assert.Empty(t, events, "nobody watches")

	watching = true
	s.Subscribed()
	assert.Equal(t, "loading", (<-events).State)
	ready := <-events
	assert.Equal(t, "ready", ready.State)
	assert.Equal(t, "song", ready.ID)

	s.Subscribed()
	assert.Equal(t, "ready", (<-events).State, "a second subscriber gets the result without a new lookup")
	assert.Empty(t, events)
}

// Synced beats plain whoever sends it; between equals the order decides.
func TestPickPrefersSyncOverOrder(t *testing.T) {
	plain := func(src duxlyrics.Provider) answer {
		return answer{res: &duxlyrics.Result{Found: true, Source: src, Lyrics: duxlyrics.Lyrics{Plain: "p"}}}
	}
	synced := func(src duxlyrics.Provider) answer {
		a := plain(src)
		a.res.Synced = []duxlyrics.Line{{Start: 1, Text: "s"}}
		return a
	}
	none := answer{res: &duxlyrics.Result{}}
	failed := answer{err: errors.New("down")}
	serverFirst := []duxlyrics.Provider{ServerSource, duxlyrics.LRCLIB}
	serverLast := []duxlyrics.Provider{duxlyrics.LRCLIB, ServerSource}
	tests := []struct {
		name      string
		order     []duxlyrics.Provider
		srv, prov answer
		want      duxlyrics.Provider
		synced    bool
	}{
		{"server plain loses to provider synced", serverFirst, plain(ServerSource), synced(duxlyrics.LRCLIB), duxlyrics.LRCLIB, true},
		{"server synced beats provider plain", serverLast, synced(ServerSource), plain(duxlyrics.LRCLIB), ServerSource, true},
		{"equal: server first", serverFirst, plain(ServerSource), plain(duxlyrics.LRCLIB), ServerSource, false},
		{"equal: server last", serverLast, synced(ServerSource), synced(duxlyrics.LRCLIB), duxlyrics.LRCLIB, true},
		{"sidecar beats an equal server", serverFirst, synced(ServerSource), synced(duxlyrics.Sidecar), duxlyrics.Sidecar, true},
		{"server plain when providers have nothing", serverFirst, plain(ServerSource), none, ServerSource, false},
		{"server plain when providers fail", serverFirst, plain(ServerSource), failed, ServerSource, false},
		{"provider when the server fails", serverFirst, failed, plain(duxlyrics.LRCLIB), duxlyrics.LRCLIB, false},
	}
	for _, tt := range tests {
		res, err := pick(tt.order, tt.srv, tt.prov)
		require.NoError(t, err, tt.name)
		assert.Equal(t, tt.want, res.Source, tt.name)
		assert.Equal(t, tt.synced, len(res.Synced) > 0, tt.name)
	}
}

// The server row turns off like any provider.
func TestServerLyricsCanBeTurnedOff(t *testing.T) {
	dir := t.TempDir()
	s := New(context.Background(), Config{CacheDir: looseDir(t), ConfigDir: dir, Libraries: libs{"srv": serverLib{}},
		Publish: func(string, any) {}, HasSubscriber: func(string) bool { return false }})
	var off []Provider
	for _, p := range defaultProviders() {
		off = append(off, Provider{ID: p.ID})
	}
	_, err := s.providers.set(off)
	require.NoError(t, err)
	res, err := s.Get(context.Background(), source.Ref{Library: "srv", ID: "song"})
	require.NoError(t, err)
	assert.False(t, res.Found)
}

func timed(start float64, lines ...string) []duxlyrics.Line {
	out := make([]duxlyrics.Line, len(lines))
	for i, text := range lines {
		at := duxlyrics.Seconds(start + float64(i)*4)
		out[i] = duxlyrics.Line{Start: at, End: at + 3, Text: text, Words: []duxlyrics.Word{{Start: at, End: at + 3, Text: text}}}
	}
	return out
}

func TestAlignMovesAnOutlierOntoWhatTheOthersAgreeOn(t *testing.T) {
	song := []string{"one", "two", "three", "four", "five", "six"}
	result := func(src duxlyrics.Provider, start float64) *duxlyrics.Result {
		return &duxlyrics.Result{Found: true, Source: src, Lyrics: duxlyrics.Lyrics{Synced: timed(start, song...)}}
	}
	lines := func(src duxlyrics.Provider, start float64) *duxlyrics.Result {
		r := result(src, start)
		for i := range r.Synced {
			r.Synced[i].Words = nil
		}
		return r
	}
	order := []duxlyrics.Provider{duxlyrics.KuGou, duxlyrics.LRCLIB, duxlyrics.LyricsPlus, duxlyrics.YouTubeMusic, duxlyrics.BetterLyrics}
	tests := []struct {
		name    string
		results []*duxlyrics.Result
		want    float64
	}{
		{"three agree", []*duxlyrics.Result{result(duxlyrics.KuGou, 33), lines(duxlyrics.LRCLIB, 32), lines(duxlyrics.LyricsPlus, 32.1), lines(duxlyrics.YouTubeMusic, 31.9)}, 32},
		{"good sources spread half a second", []*duxlyrics.Result{result(duxlyrics.KuGou, 33), lines(duxlyrics.LRCLIB, 32.2), lines(duxlyrics.LyricsPlus, 32.3), lines(duxlyrics.YouTubeMusic, 31.8)}, 32.2},
		{"two voters are enough", []*duxlyrics.Result{result(duxlyrics.KuGou, 33), lines(duxlyrics.LRCLIB, 32), lines(duxlyrics.LyricsPlus, 32)}, 32},
		{"one voter is too few", []*duxlyrics.Result{result(duxlyrics.KuGou, 33), lines(duxlyrics.LRCLIB, 32)}, 33},
		{"a majority is enough", []*duxlyrics.Result{result(duxlyrics.KuGou, 33), lines(duxlyrics.LRCLIB, 32), lines(duxlyrics.LyricsPlus, 32), lines(duxlyrics.YouTubeMusic, 36)}, 32},
		{"exactly half is no majority", []*duxlyrics.Result{result(duxlyrics.KuGou, 33), lines(duxlyrics.LRCLIB, 32), lines(duxlyrics.LyricsPlus, 32), lines(duxlyrics.YouTubeMusic, 36), lines(duxlyrics.BetterLyrics, 30)}, 33},
		{"close enough stays", []*duxlyrics.Result{result(duxlyrics.KuGou, 32.2), lines(duxlyrics.LRCLIB, 32), lines(duxlyrics.LyricsPlus, 32), lines(duxlyrics.YouTubeMusic, 32)}, 32.2},
		{"the voters disagree", []*duxlyrics.Result{result(duxlyrics.KuGou, 33), lines(duxlyrics.LRCLIB, 30), lines(duxlyrics.LyricsPlus, 31.5), lines(duxlyrics.YouTubeMusic, 36)}, 33},
		{"a sidecar is never moved", []*duxlyrics.Result{result(duxlyrics.Sidecar, 33), lines(duxlyrics.LRCLIB, 32), lines(duxlyrics.LyricsPlus, 32), lines(duxlyrics.YouTubeMusic, 32)}, 33},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, wordBefore := float64(tt.results[0].Synced[0].Start), float64(tt.results[0].Synced[0].Words[0].Start)
			got := align(order, tt.results)
			assert.Equal(t, tt.results[0].Source, got.Source, "word timing wins")
			assert.InDelta(t, tt.want, float64(got.Synced[0].Start), 0.001)
			assert.InDelta(t, tt.want, float64(got.Synced[0].Words[0].Start), 0.001)
			assert.InDelta(t, before, float64(tt.results[0].Synced[0].Start), 0.001, "the input stays as it was")
			assert.InDelta(t, wordBefore, float64(tt.results[0].Synced[0].Words[0].Start), 0.001, "the input's words too")
		})
	}
}

// looseDir is for a cache: pruning and lookups outlive a test, so it can't be a
// t.TempDir, which fails when anything writes during its removal.
func looseDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "lyrics-test")
	require.NoError(t, err)
	t.Cleanup(func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.RemoveAll(dir)
	})
	return dir
}

func quietService(t *testing.T, dir string, publish func(string, any), on *bool) *Service {
	t.Helper()
	s := &Service{baseCtx: context.Background(), providers: openProviders(filepath.Join(dir, providersFile)), cfg: Config{
		CacheDir: looseDir(t), DataDir: dir, Libraries: libs{"srv": serverLib{}}, Publish: publish,
		HasSubscriber: func(string) bool { return true },
		Enabled:       func() bool { return on == nil || *on }}}
	s.client = s.newClient(s.cacheDir())
	s.wasOn = s.enabled()
	var off []Provider
	for _, p := range defaultProviders() {
		off = append(off, Provider{ID: p.ID, Enabled: p.ID == ServerSource})
	}
	_, err := s.providers.set(off)
	require.NoError(t, err)
	return s
}

// A prefetched result shows at once, and its entries reach the cache only
// when the track plays; one that never plays leaves nothing.
func TestPrefetchedLyrics(t *testing.T) {
	dir := t.TempDir()
	var states []string
	s := quietService(t, dir, func(_ string, data any) {
		if ev, ok := data.(Event); ok {
			states = append(states, ev.State)
		}
	}, nil)
	stage := func(name string) string {
		d := filepath.Join(s.aheadDir(), name)
		require.NoError(t, os.MkdirAll(filepath.Join(d, "ab"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(d, "ab", name+".json"), []byte("{}"), 0o644))
		return d
	}
	next := &source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"}
	played := stage("played")
	s.remember(warmed{key: key(next), res: &duxlyrics.Result{Found: true, Source: ServerSource}, dir: played})
	s.TrackChanged(next)
	assert.Equal(t, []string{"ready"}, states)
	assert.FileExists(t, filepath.Join(s.cacheDir(), "ab", "played.json"))
	assert.NoDirExists(t, played)

	skipped := stage("skipped")
	s.remember(warmed{key: "skipped", res: &duxlyrics.Result{}, dir: skipped})
	for i := range maxWarmed {
		s.remember(warmed{key: string(rune('a' + i)), res: &duxlyrics.Result{}})
	}
	assert.NoDirExists(t, skipped)
	assert.NoFileExists(t, filepath.Join(s.cacheDir(), "ab", "skipped.json"))
}

// A picked source stays with its track across restarts, without asking
// anyone, until the user goes back to automatic.
func TestChosenLyricsStayWithTheTrack(t *testing.T) {
	dir := t.TempDir()
	var last Event
	s := quietService(t, dir, func(_ string, data any) {
		if ev, ok := data.(Event); ok {
			last = ev
		}
	}, nil)
	ref := source.Ref{Library: "srv", ID: "song"}
	choices, err := s.Choices(context.Background(), ref)
	require.NoError(t, err)
	require.Len(t, choices, 1)
	assert.Equal(t, "line", choices[0].Timing)
	require.Error(t, s.Choose(context.Background(), ref, "kugou"), "a source without lyrics can't be picked")
	require.NoError(t, s.Choose(context.Background(), ref, choices[0].Source))

	again := quietService(t, dir, func(_ string, data any) {
		if ev, ok := data.(Event); ok {
			last = ev
		}
	}, nil)
	again.TrackChanged(&source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"})
	require.True(t, last.Chosen)
	assert.Equal(t, "from the server", last.Result.Synced[0].Text)

	require.NoError(t, again.Choose(context.Background(), ref, ""))
	assert.Nil(t, again.chosen(key(&source.Track{Library: "srv", ID: "song"})))
}

func TestLyricsOffAsksNobody(t *testing.T) {
	on := false
	var last Event
	s := quietService(t, t.TempDir(), func(_ string, data any) {
		if ev, ok := data.(Event); ok {
			last = ev
		}
	}, &on)
	s.TrackChanged(&source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"})
	assert.Equal(t, "idle", last.State)
	_, err := s.Get(context.Background(), source.Ref{Library: "srv", ID: "song"})
	require.ErrorIs(t, err, errOff)
	on = true
	s.SettingsChanged()
	assert.Equal(t, "loading", last.State, "turning lyrics on shows the current track's")
}

// stereo is a song in raw floats: centre bursts where the vocals sing, and a
// hard-panned guitar playing a pattern of its own.
func stereo(sung, guitar []float64, seconds float64) []byte {
	out := make([]byte, 0, int(seconds*FitRate)*8)
	burst := func(at []float64, t float64) float64 {
		for _, start := range at {
			if t >= start && t < start+0.15 {
				return 0.4 * math.Sin(2*math.Pi*800*t)
			}
		}
		return 0
	}
	for i := range int(seconds * FitRate) {
		t := float64(i) / FitRate
		centre := burst(sung, t)
		out = binary.LittleEndian.AppendUint32(out, math.Float32bits(float32(centre+8*burst(guitar, t))))
		out = binary.LittleEndian.AppendUint32(out, math.Float32bits(float32(centre)))
	}
	return out
}

// Word starts that run early are found against vocals in the centre, past a
// louder guitar on one side; words that match nothing stay below confidence.
func TestFitFindsTheVocals(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	var words []float64
	for at := 1.0; at < 40; at += 0.25 + random.Float64()*0.5 {
		words = append(words, at)
	}
	sung, guitar := make([]float64, len(words)), make([]float64, len(words))
	for i, at := range words {
		sung[i] = at + 0.9
		// the same rhythm at another offset: only the centre tells them apart
		guitar[i] = at - 0.5
	}
	found, err := attacks(bytes.NewReader(stereo(sung, guitar, 44)))
	require.NoError(t, err)
	shift, confidence := fit(words, found)
	assert.InDelta(t, 0.9, shift, 0.025)
	assert.GreaterOrEqual(t, confidence, float64(fitConfidence))

	quiet := make([]float64, 2000)
	for i := range quiet {
		quiet[i] = random.Float64()
	}
	_, confidence = fit(words, quiet)
	assert.Less(t, confidence, float64(fitConfidence), "noise fits nothing")
}

// A confident fit beats the other sources, is remembered, and leaves a lyrics
// file of the user's own alone.
func TestAudioFitMovesWordTiming(t *testing.T) {
	dir := t.TempDir()
	s := quietService(t, dir, func(string, any) {}, nil)
	var words []float64
	var lines []duxlyrics.Line
	random := rand.New(rand.NewPCG(3, 4))
	for at := 1.0; at < 40; at += 0.25 + random.Float64()*0.5 {
		words = append(words, at+0.9)
		lines = append(lines, duxlyrics.Line{Start: duxlyrics.Seconds(at), Text: "w", Words: []duxlyrics.Word{{Start: duxlyrics.Seconds(at), Text: "w"}}})
	}
	audio := stereo(words, nil, 44)
	decodes := 0
	s.cfg.AudioTiming = func() bool { return true }
	s.cfg.AudioFile = func(context.Context, source.Ref) (string, bool) { return "/x", true }
	s.cfg.Decode = func(_ context.Context, _ string, read func(io.Reader) error) error {
		decodes++
		return read(bytes.NewReader(audio))
	}
	track := source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"}
	res := &duxlyrics.Result{Found: true, Source: duxlyrics.KuGou, Lyrics: duxlyrics.Lyrics{Synced: lines}}
	shift, ok := s.fitted(context.Background(), track, res)
	require.True(t, ok)
	assert.InDelta(t, 0.9, shift, 0.025)
	_, ok = s.fitted(context.Background(), track, res)
	require.True(t, ok)
	assert.Equal(t, 1, decodes, "a track is decoded once")

	res.Source = duxlyrics.Sidecar
	_, ok = s.fitted(context.Background(), track, res)
	assert.False(t, ok)

	res.Source = duxlyrics.BetterLyrics
	var other []float64
	for at := 0.5; at < 40; at += 1.5 + random.Float64() {
		other = append(other, at)
	}
	audio = stereo(other, nil, 44)
	_, ok = s.fitted(context.Background(), track, res)
	assert.False(t, ok, "audio the words don't fit moves nothing")
	s.cfg.AudioTiming = func() bool { return false }
	res.Source = duxlyrics.LRCLIB
	_, ok = s.fitted(context.Background(), track, res)
	assert.False(t, ok, "off in settings")
}

type countingLib struct {
	serverLib
	asked *atomic.Int32
}

func (c countingLib) Lyrics(ctx context.Context, id string) (*duxlyrics.Lyrics, error) {
	c.asked.Add(1)
	return c.serverLib.Lyrics(ctx, id)
}

// The current track's lyrics are looked up after a download only when they
// are kept or someone watches.
func TestStoredLooksUpTheCurrentTrackOnlyWhenWanted(t *testing.T) {
	var asked atomic.Int32
	watching := false
	s := quietService(t, t.TempDir(), func(string, any) {}, nil)
	s.cfg.Libraries = libs{"srv": countingLib{asked: &asked}}
	s.cfg.HasSubscriber = func(string) bool { return watching }
	track := source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"}
	s.track = &track

	s.Stored(context.Background(), track, false)
	assert.Zero(t, asked.Load(), "kept off and nobody watching")

	s.Stored(context.Background(), track, true)
	assert.NotZero(t, asked.Load(), "kept lyrics are looked up")
}

func TestOldMeasuredShiftsArePruned(t *testing.T) {
	dir := t.TempDir()
	old, fresh := filepath.Join(dir, "old"), filepath.Join(dir, "fresh")
	require.NoError(t, os.WriteFile(old, []byte("{}"), 0o600))
	require.NoError(t, os.WriteFile(fresh, []byte("{}"), 0o600))
	longAgo := time.Now().Add(-fitMaxAge - time.Hour)
	require.NoError(t, os.Chtimes(old, longAgo, longAgo))

	pruneFits(dir)

	assert.NoFileExists(t, old)
	assert.FileExists(t, fresh)
}

// A kept track's cached lyrics are touched at startup, so the 90-day prune of
// the lyrics cache spares them even when the entry file was replaced.
func TestKeptLyricsSurviveTheAgePrune(t *testing.T) {
	s := quietService(t, t.TempDir(), func(string, any) {}, nil)
	entry := filepath.Join(s.cacheDir(), "ab", "abc.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
	start := time.Now().Add(-time.Minute)
	require.NoError(t, os.WriteFile(entry, []byte("{}"), 0o600))
	ref := source.Ref{Library: "srv", ID: "1"}
	s.keep(ref, start)

	// The prune left the old link behind and a refetch wrote a new, old-looking file.
	longAgo := time.Now().Add(-100 * 24 * time.Hour)
	require.NoError(t, os.Remove(entry))
	require.NoError(t, os.WriteFile(entry, []byte("{}"), 0o600))
	require.NoError(t, os.Chtimes(entry, longAgo, longAgo))
	s.refreshKept()

	info, err := os.Stat(entry)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), info.ModTime(), time.Minute)
}

// A track that stops being kept, or lyrics going off, leaves no kept folder.
func TestKeptFoldersAreCleaned(t *testing.T) {
	on, kept := true, true
	s := quietService(t, t.TempDir(), func(string, any) {}, &on)
	s.cfg.Keep = func() bool { return kept }
	ref := source.Ref{Library: "srv", ID: "1"}
	link := func() {
		entry := filepath.Join(s.cacheDir(), "ab", "abc.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(entry), 0o755))
		require.NoError(t, os.WriteFile(entry, []byte("{}"), 0o600))
		s.keep(ref, time.Now().Add(-time.Minute))
	}
	folders := func() int { e, _ := os.ReadDir(s.keptDir()); return len(e) }

	link()
	s.refreshKept()
	assert.Equal(t, 1, folders())
	s.unkeep(ref)
	assert.Equal(t, 0, folders())

	link()
	kept = false
	s.refreshKept()
	assert.Equal(t, 0, folders())

	kept = true
	link()
	on = false
	s.refreshKept()
	assert.Equal(t, 0, folders())

	on = true
	link()
	known := false
	s.cfg.HasCopy = func(context.Context, source.Ref) (bool, bool) { return false, known }
	s.refreshKept()
	assert.Equal(t, 1, folders(), "a copy that can't be looked up keeps its folder")

	known = true
	s.refreshKept()
	assert.Equal(t, 0, folders())
}

// Lyrics turned off and on while nothing watches still show when the view
// opens again.
func TestLyricsBackOnAfterNobodyWatched(t *testing.T) {
	on := false
	watching := true
	var last Event
	s := quietService(t, t.TempDir(), func(_ string, data any) {
		if ev, ok := data.(Event); ok {
			last = ev
		}
	}, &on)
	s.cfg.HasSubscriber = func(string) bool { return watching }
	s.TrackChanged(&source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"})
	require.Equal(t, "idle", last.State)

	watching = false
	on = true
	s.SettingsChanged()
	watching = true
	s.Subscribed()
	assert.Equal(t, "loading", last.State)
}
