package lyrics

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

// No provider is on in these tests, so nothing reaches the network; offline
// they'd skip the fake servers too.
func init() { online = func() bool { return true } }

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
	s := newService(t, Config{ConfigDir: dir, Libraries: libs{"loc": localLib{dir}, "srv": serverLib{}},
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
	s := newService(t, Config{ConfigDir: dir, Libraries: libs{"srv": serverLib{}},
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
	s := newService(t, Config{ConfigDir: dir, Libraries: libs{"srv": serverLib{}},
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
			got := align(order, tt.results).res
			assert.Equal(t, tt.results[0].Source, got.Source, "word timing wins")
			assert.InDelta(t, tt.want, float64(got.Synced[0].Start), 0.001)
			assert.InDelta(t, tt.want, float64(got.Synced[0].Words[0].Start), 0.001)
			assert.InDelta(t, before, float64(tt.results[0].Synced[0].Start), 0.001, "the input stays as it was")
			assert.InDelta(t, wordBefore, float64(tt.results[0].Synced[0].Words[0].Start), 0.001, "the input's words too")
		})
	}
}

func newService(t *testing.T, cfg Config) *Service {
	t.Helper()
	cfg.CacheDir = t.TempDir()
	s := New(context.Background(), cfg)
	finishWork(t, s)
	return s
}

// finishWork lets the service's lookups end before the test's temp folders
// go, or one would write into them during their removal. They finish rather
// than being cancelled, and without Close's time limit: dux-go goes on with a
// cancelled lookup and writes its cache entry later.
func finishWork(t *testing.T, s *Service) {
	t.Cleanup(func() {
		s.work.Wait()
		s.Close()
	})
}

// published keeps the states a service sent, from whichever goroutine.
type published struct {
	mu     sync.Mutex
	states []string
}

func (p *published) add(_ string, data any) {
	if ev, ok := data.(Event); ok {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.states = append(p.states, ev.State)
	}
}

func (p *published) all() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.states)
}

func quietService(t *testing.T, dir string, publish func(string, any), on *bool) *Service {
	t.Helper()
	ctx, stop := context.WithCancel(context.Background())
	s := &Service{baseCtx: ctx, stop: stop, providers: openProviders(filepath.Join(dir, providersFile)), cfg: Config{
		CacheDir: t.TempDir(), DataDir: dir, Libraries: libs{"srv": serverLib{}}, Publish: publish,
		HasSubscriber: func(string) bool { return true },
		Enabled:       func() bool { return on == nil || *on }}}
	finishWork(t, s)
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
	data, err := json.Marshal(choices[0])
	require.NoError(t, err)
	assert.Contains(t, string(data), `"status":{"kind":"not-measured"}`, "nothing was decided for the track yet")
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

// flakyLib's server fails once fail is set.
type flakyLib struct {
	countingLib
	fail *atomic.Bool
}

func (f flakyLib) Lyrics(ctx context.Context, id string) (*duxlyrics.Lyrics, error) {
	if f.fail.Load() {
		f.asked.Add(1)
		return nil, errors.New("rate limited")
	}
	return f.countingLib.Lyrics(ctx, id)
}

// The menu lists what the last automatic pick weighed, without asking
// anyone; a track without one, as a hand-picked one, asks the sources.
func TestChoicesListTheLastDecision(t *testing.T) {
	track := source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"}
	ref := track.Ref()
	tests := []struct {
		name   string
		decide bool
		chosen bool
		asks   bool
		status string
	}{
		{"after a decision, a failing source changes nothing", true, false, false, "shown"},
		{"no decision yet", false, false, true, "not-measured"},
		{"hand-picked, never decided", false, true, true, "shown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var asked atomic.Int32
			var fail atomic.Bool
			s := quietService(t, t.TempDir(), func(string, any) {}, nil)
			s.cfg.Libraries = libs{"srv": flakyLib{countingLib{asked: &asked}, &fail}}
			if tt.decide {
				require.NotNil(t, s.Best(context.Background(), track))
				fail.Store(true)
			}
			if tt.chosen {
				require.NoError(t, s.Choose(context.Background(), ref, ServerSource))
				s.decided = nil
			}
			// an empty cache, so a lookup would reach the server
			s.client = s.newClient(t.TempDir())
			asked.Store(0)
			choices, err := s.Choices(context.Background(), ref)
			require.NoError(t, err)
			require.Len(t, choices, 1)
			assert.Equal(t, Choice{Source: ServerSource, Name: "Navidrome", Timing: "line", Status: Status{Kind: tt.status}}, choices[0])
			assert.Equal(t, tt.asks, asked.Load() > 0, "asked the server")
		})
	}
}

func TestLyricsOffAsksNobody(t *testing.T) {
	on := false
	var got published
	s := quietService(t, t.TempDir(), got.add, &on)
	s.TrackChanged(&source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"})
	assert.Equal(t, []string{"idle"}, got.all())
	_, err := s.Get(context.Background(), source.Ref{Library: "srv", ID: "song"})
	require.ErrorIs(t, err, errOff)
	on = true
	s.SettingsChanged()
	assert.Equal(t, "loading", got.all()[1], "turning lyrics on shows the current track's")
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
	shift, confidence := fit(words, found, fitReach)
	assert.InDelta(t, 0.9, shift, 0.025)
	assert.GreaterOrEqual(t, confidence, float64(fitConfidence))

	quiet := make([]float64, 2000)
	for i := range quiet {
		quiet[i] = random.Float64()
	}
	_, confidence = fit(words, quiet, fitReach)
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
	fitted := func(res *duxlyrics.Result) (float64, bool) {
		known, ok := s.recording(context.Background(), track).measure(res, true)
		return known.Shift, ok && known.Confident
	}
	res := &duxlyrics.Result{Found: true, Source: duxlyrics.KuGou, Lyrics: duxlyrics.Lyrics{Synced: lines}}
	shift, ok := fitted(res)
	require.True(t, ok)
	assert.InDelta(t, 0.9, shift, 0.025)
	_, ok = fitted(res)
	require.True(t, ok)
	assert.Equal(t, 1, decodes, "a track is decoded once")

	res.Source = duxlyrics.Sidecar
	_, ok = fitted(res)
	assert.False(t, ok)

	res.Source = duxlyrics.BetterLyrics
	var other []float64
	for at := 0.5; at < 40; at += 1.5 + random.Float64() {
		other = append(other, at)
	}
	audio = stereo(other, nil, 44)
	_, ok = fitted(res)
	assert.False(t, ok, "audio the words don't fit moves nothing")
	s.cfg.AudioTiming = func() bool { return false }
	assert.Nil(t, s.recording(context.Background(), track), "off in settings")
}

// Lines every few seconds, as sung.
func lineTimes(random *rand.Rand, count int) []float64 {
	var out []float64
	at := 6.0
	for range count {
		out = append(out, at)
		at += 3 + random.Float64()*2
	}
	return out
}

func TestLineFit(t *testing.T) {
	random := rand.New(rand.NewPCG(5, 6))
	lines := lineTimes(random, 48)
	moved := func(by func(i int) float64) []float64 {
		out := make([]float64, len(lines))
		for i, at := range lines {
			out[i] = at + by(i)
		}
		return out
	}
	noise := make([]float64, 12000)
	for i := range noise {
		noise[i] = random.Float64()
	}
	onset := func(sung []float64) []float64 {
		found, err := attacks(bytes.NewReader(stereo(sung, nil, 240)))
		require.NoError(t, err)
		return found
	}
	shift, ok := fitLines(lines, onset(moved(func(int) float64 { return 3 })))
	assert.True(t, ok, "past the reach of the word fit")
	assert.InDelta(t, 3, shift, 0.05)

	_, ok = fitLines(lines, onset(moved(func(i int) float64 {
		if i < len(lines)/2 {
			return 3
		}
		return -2
	})))
	assert.False(t, ok, "a song that doesn't hold one shift moves nothing")

	_, ok = fitLines(lines, noise)
	assert.False(t, ok, "noise fits nothing")
}

func TestLineFitCarriesItsShiftToTheShownSource(t *testing.T) {
	s := quietService(t, t.TempDir(), func(string, any) {}, nil)
	random := rand.New(rand.NewPCG(7, 8))
	starts := lineTimes(random, 48)
	sung := make([]float64, len(starts))
	reference := make([]duxlyrics.Line, len(starts))
	var shown []duxlyrics.Line
	for i, at := range starts {
		sung[i] = at + 3
		text := fmt.Sprintf("line %d", i)
		reference[i] = duxlyrics.Line{Start: duxlyrics.Seconds(at), Text: text}
		if i < 10 {
			shown = append(shown, duxlyrics.Line{Start: duxlyrics.Seconds(at - 0.7), Text: text})
		}
	}
	audio := stereo(sung, nil, 240)
	s.cfg.AudioTiming = func() bool { return true }
	s.cfg.AudioFile = func(context.Context, source.Ref) (string, bool) { return "/x", true }
	s.cfg.Decode = func(_ context.Context, _ string, read func(io.Reader) error) error {
		return read(bytes.NewReader(audio))
	}
	track := source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"}
	top := &duxlyrics.Result{Found: true, Source: duxlyrics.KuGou, Lyrics: duxlyrics.Lyrics{Synced: shown}}
	line := &duxlyrics.Result{Found: true, Source: duxlyrics.LRCLIB, Lyrics: duxlyrics.Lyrics{Synced: reference}}

	order := []duxlyrics.Provider{duxlyrics.KuGou, duxlyrics.LRCLIB}
	got := fallback(order, []*duxlyrics.Result{top, line}, s.recording(context.Background(), track)).res
	assert.Equal(t, duxlyrics.KuGou, got.Source)
	assert.InDelta(t, 3.7, float64(got.Synced[0].Start-top.Synced[0].Start), 0.05, "the source that can be measured speaks for the one shown")
}

func allProviders() []duxlyrics.Provider {
	var out []duxlyrics.Provider
	for _, p := range defaultProviders() {
		out = append(out, duxlyrics.Provider(p.ID))
	}
	return out
}

// fakeRecording measures each source at its fit, for its own timing only,
// and every laid copy at laid.
type fakeRecording struct {
	fits map[duxlyrics.Provider]fitResult
	laid *fitResult
}

func (f fakeRecording) measure(r *duxlyrics.Result, words bool) (fitResult, bool) {
	known, ok := f.fits[r.Source]
	return known, ok && words == (tier(answer{res: r}) == 2)
}

func (f fakeRecording) measureLaid(_, _ *duxlyrics.Result) (fitResult, bool) {
	if f.laid == nil {
		return fitResult{}, false
	}
	return *f.laid, true
}

// With the audio measured, the source that lands on the recording is shown;
// when none does, the timing tier and the order decide as before.
func TestDecidePicksTheSourceOnTheRecording(t *testing.T) {
	result := func(src duxlyrics.Provider, words bool) *duxlyrics.Result {
		lines := make([]duxlyrics.Line, 40)
		for i := range lines {
			at := duxlyrics.Seconds(10 + 4*i)
			lines[i] = duxlyrics.Line{Start: at, Text: fmt.Sprintf("line %d", i)}
			if words {
				lines[i].Words = []duxlyrics.Word{{Start: at, Text: "line"}, {Start: at + 1, Text: fmt.Sprint(i)}}
			}
		}
		return &duxlyrics.Result{Found: true, Source: src, Lyrics: duxlyrics.Lyrics{Synced: lines}}
	}
	kugou, lrclib, better := duxlyrics.KuGou, duxlyrics.LRCLIB, duxlyrics.BetterLyrics
	tests := []struct {
		name  string
		words []duxlyrics.Provider // word-timed; the others are line-timed
		fits  map[duxlyrics.Provider]fitResult
		want  duxlyrics.Provider
		shift float64
	}{
		{"an on-time word source beats an on-time line source", []duxlyrics.Provider{kugou},
			map[duxlyrics.Provider]fitResult{kugou: {0.1, false}, better: {0, false}}, kugou, 0},
		{"an on-time line source beats a drifting word source", []duxlyrics.Provider{kugou},
			map[duxlyrics.Provider]fitResult{kugou: {-1.8, false}, better: {-0.14, false}, lrclib: {-3.08, false}}, better, 0},
		{"between on-time line sources the order decides", nil,
			map[duxlyrics.Provider]fitResult{lrclib: {0.06, false}, better: {-0.3, false}}, better, 0},
		{"a sure on-time fit is applied", nil,
			map[duxlyrics.Provider]fitResult{better: {-0.9, false}, lrclib: {0.3, true}}, lrclib, 0.3},
		{"none on time: the word source as before", []duxlyrics.Provider{kugou},
			map[duxlyrics.Provider]fitResult{kugou: {-1.8, false}, lrclib: {-3.08, false}}, kugou, 0},
		{"none on time: a sure shift is applied as before", []duxlyrics.Provider{kugou},
			map[duxlyrics.Provider]fitResult{kugou: {2, true}, lrclib: {-3.08, false}}, kugou, 2},
		{"an unsure shift is never applied", []duxlyrics.Provider{kugou},
			map[duxlyrics.Provider]fitResult{kugou: {0.3, false}}, kugou, 0},
		{"a lyrics file beside the track is never overridden", nil,
			map[duxlyrics.Provider]fitResult{duxlyrics.Sidecar: {-3, true}, ServerSource: {0, true}}, duxlyrics.Sidecar, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var results []*duxlyrics.Result
			for src := range tt.fits {
				results = append(results, result(src, slices.Contains(tt.words, src)))
			}
			got, _ := decide(allProviders(), results, nil, fakeRecording{fits: tt.fits})
			assert.Equal(t, tt.want, got.Source)
			assert.InDelta(t, 10+tt.shift, float64(got.Synced[0].Start), 0.001)
		})
	}
}

// Lookups leave the album out, and a song only found without its length or
// in the cache entries stored with the album is still found.
func TestFindRetriesWithoutLengthAndAlbum(t *testing.T) {
	synced := func(text string) *duxlyrics.Lyrics {
		return &duxlyrics.Lyrics{Synced: []duxlyrics.Line{{Start: 1, Text: text}}}
	}
	none := &duxlyrics.Lyrics{}
	byLength := func(with, without *duxlyrics.Lyrics, err error) func(duxlyrics.Request) (*duxlyrics.Lyrics, error) {
		return func(r duxlyrics.Request) (*duxlyrics.Lyrics, error) {
			if r.Duration > 0 {
				return with, err
			}
			return without, nil
		}
	}
	tests := []struct {
		name   string
		answer func(duxlyrics.Request) (*duxlyrics.Lyrics, error)
		cached bool   // the provider answered a lookup with the album once
		want   string // the synced line shown, "" for none, then "+" and a loose answer's
		first  bool   // the first show, which takes the first synced answer
	}{
		{"found with the length", byLength(synced("with"), none, nil), false, "with", false},
		{"found only without the length: loose", byLength(none, synced("without"), nil), false, "+without", false},
		{"the first show takes what is found without the length", byLength(none, synced("without"), nil), false, "without", true},
		{"synced without the length beats plain with it: loose", byLength(&duxlyrics.Lyrics{Plain: "plain"}, synced("without"), nil), false, "+without", false},
		{"synced with the length beats other lyrics without it", byLength(synced("with"), synced("without"), nil), false, "with", false},
		{"word timing without the length comes beside line timing with it", byLength(synced("with"), &duxlyrics.Lyrics{Synced: []duxlyrics.Line{{Start: 1, Text: "words", Words: []duxlyrics.Word{{Start: 1, Text: "words"}}}}}, nil), false, "with+words", false},
		{"the first show takes line timing with the length", byLength(synced("with"), &duxlyrics.Lyrics{Synced: []duxlyrics.Line{{Start: 1, Text: "words", Words: []duxlyrics.Word{{Start: 1, Text: "words"}}}}}, nil), false, "with", true},
		{"a failed lookup isn't asked again without the length", byLength(nil, synced("without"), errors.New("offline")), false, "", false},
		{"cached with the album", func(r duxlyrics.Request) (*duxlyrics.Lyrics, error) {
			if r.Album != "" {
				return synced("album"), nil
			}
			return none, nil
		}, true, "album", false},
		{"nowhere", byLength(none, none, nil), false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var albums atomic.Int32
			client := duxlyrics.New(duxlyrics.Options{CacheDir: t.TempDir(), Resolve: func(duxlyrics.Provider) (duxlyrics.Source, bool) {
				return duxlyrics.Source{Fetch: func(_ context.Context, r duxlyrics.Request) (*duxlyrics.Lyrics, error) {
					if r.Album != "" {
						albums.Add(1)
					}
					return tt.answer(r)
				}}, true
			}})
			s := quietService(t, t.TempDir(), func(string, any) {}, nil)
			track := source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a", Album: "Single", Duration: 236}
			req, _ := s.request(context.Background(), track, nil)
			req.Providers = []duxlyrics.Provider{"fake"}
			if tt.cached {
				old := req
				old.Album = track.Album
				_, err := client.Lookup(context.Background(), old)
				require.NoError(t, err)
				albums.Store(0)
			}
			got := ""
			res, loose, err := find(context.Background(), client, req, track.Album, !tt.first)
			if err == nil && res.Found && len(res.Synced) > 0 {
				got = res.Synced[0].Text
			}
			if loose != nil {
				got += "+" + loose.Synced[0].Text
			}
			assert.Equal(t, tt.want, got)
			assert.Zero(t, albums.Load(), "no provider is asked with the album")
		})
	}
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
	var got published
	s := quietService(t, t.TempDir(), got.add, &on)
	s.cfg.HasSubscriber = func(string) bool { return watching }
	s.TrackChanged(&source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"})
	require.Equal(t, []string{"idle"}, got.all())

	watching = false
	on = true
	s.SettingsChanged()
	watching = true
	s.Subscribed()
	assert.Equal(t, "loading", got.all()[1])
}

// A timing offset set or moved per track stays across restarts and moves whatever
// is shown: a picked source as well as a fitted result.
func TestTimingOffset(t *testing.T) {
	type step struct {
		v        float64
		relative bool
	}
	by := func(v float64) step { return step{v, true} }
	to := func(v float64) step { return step{v, false} }
	tests := []struct {
		name   string
		fitted bool   // the shown result is a fitted one at 1.9 s, else the server's pick at 1 s
		file   string // what the offset file holds before
		steps  []step
		want   float64
	}{
		{name: "later twice adds up", steps: []step{by(0.5), by(0.5)}, want: 1},
		{name: "earlier", steps: []step{by(-0.5)}, want: -0.5},
		{name: "earlier than the song starts stops at 0", steps: []step{by(-1.5)}, want: -1.5},
		{name: "back from below 0 keeps the timing", steps: []step{by(-1.5), by(1)}, want: -0.5},
		{name: "after a fit", fitted: true, steps: []step{by(0.5)}, want: 0.5},
		{name: "rounds to the step", steps: []step{by(0.3)}, want: 0.5},
		{name: "clamped", steps: []step{by(100), by(1)}, want: 10},
		{name: "set", steps: []step{by(1), to(-2)}, want: -2},
		{name: "set rounds to the step", steps: []step{to(3.3)}, want: 3.5},
		{name: "set clamped", steps: []step{to(-25)}, want: -10},
		{name: "set to zero resets", steps: []step{by(1.5), to(0)}, want: 0},
		{name: "moving by zero keeps it", steps: []step{by(1.5), by(0)}, want: 1.5},
		{name: "a file out of range is clamped", file: "1000", want: 10},
		{name: "a corrupt file is replaced", file: "{nope", steps: []step{by(-1)}, want: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			track := &source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"}
			var last Event
			start := func(s *Service) {
				t.Helper()
				if tt.fitted {
					s.remember(warmed{key: key(track), res: &duxlyrics.Result{Found: true, Source: duxlyrics.KuGou,
						Lyrics: duxlyrics.Lyrics{Synced: []duxlyrics.Line{{Start: 1.9, Text: "x"}}}}})
				}
				s.TrackChanged(track)
				require.Equal(t, "ready", last.State)
			}
			s := quietService(t, dir, func(_ string, d any) { last = d.(Event) }, nil)
			if !tt.fitted {
				require.NoError(t, s.Choose(context.Background(), track.Ref(), ServerSource))
			}
			if tt.file != "" {
				require.NoError(t, os.MkdirAll(filepath.Dir(s.offsetPath(key(track))), 0o700))
				require.NoError(t, os.WriteFile(s.offsetPath(key(track)), []byte(tt.file), 0o600))
			}
			start(s)
			base := 1.0
			if tt.fitted {
				base = 1.9
			}
			for _, st := range tt.steps {
				_, err := s.SetOffset(st.v, st.relative)
				require.NoError(t, err)
			}
			assert.InDelta(t, tt.want, last.Offset, 1e-9, "the playing song follows at once")
			assert.InDelta(t, max(0, base+tt.want), float64(last.Result.Synced[0].Start), 1e-9)

			last = Event{}
			s.Subscribed()
			require.Equal(t, "ready", last.State, "a window opening mid-song")
			assert.InDelta(t, tt.want, last.Offset, 1e-9, "a window opening mid-song")
			assert.InDelta(t, max(0, base+tt.want), float64(last.Result.Synced[0].Start), 1e-9, "a window opening mid-song")

			again := quietService(t, dir, func(_ string, d any) { last = d.(Event) }, nil)
			start(again)
			assert.InDelta(t, tt.want, last.Offset, 1e-9, "kept across a restart")
			assert.InDelta(t, max(0, base+tt.want), float64(last.Result.Synced[0].Start), 1e-9)
			if tt.want == 0 && len(tt.steps) > 0 {
				assert.NoFileExists(t, s.offsetPath(key(track)))
			}
		})
	}
}

// An offset belongs to the lyrics it was tuned against: it shows 0 once the
// shown source or timing changes, and an older bare number belongs to what
// shows first.
func TestTimingOffsetFollowsSource(t *testing.T) {
	const server, kugou, words = "server", "kugou", "server words"
	tests := []struct {
		name  string
		file  string  // what the offset file holds before
		first string  // what shows when the offset is set
		set   float64 // moved by this much, when not 0
		live  bool    // the source is picked by hand while playing, else after a restart
		then  string  // what shows afterwards
		want  float64
	}{
		{name: "kept for its own source", first: kugou, set: 1, then: kugou, want: 1},
		{name: "dropped when the pick changes", first: kugou, set: 1, then: server, want: 0},
		{name: "dropped when a source is picked by hand", first: kugou, set: 1, live: true, then: server, want: 0},
		{name: "dropped when the timing changes", first: server, set: 1, then: words, want: 0},
		{name: "a bare number belongs to what shows", file: "1.5", first: server, then: server, want: 1.5},
		{name: "a bare number stays with that source", file: "1.5", first: server, then: kugou, want: 0},
		{name: "a bare number moves on the next set", file: "1.5", first: server, set: 0.5, then: server, want: 2},
		{name: "a corrupt file is ignored", file: "{nope", first: server, then: server, want: 0},
		{name: "a half-read file is ignored", file: `{"offset":1.5,"source":"server","tier":1,"tier":"x"}`, first: server, then: server, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			track := &source.Track{Library: "srv", ID: "song", Title: "t", Artist: "a"}
			var last Event
			show := func(s *Service, what string) {
				t.Helper()
				switch what {
				case server:
					require.NoError(t, s.Choose(context.Background(), track.Ref(), ServerSource))
				case kugou, words:
					require.NoError(t, os.RemoveAll(s.chosenPath(key(track))))
					line := duxlyrics.Line{Start: 1.9, Text: "x"}
					src := duxlyrics.KuGou
					if what == words {
						line.Words, src = []duxlyrics.Word{{Start: 1.9, Text: "x"}}, ServerSource
					}
					s.remember(warmed{key: key(track), res: &duxlyrics.Result{Found: true, Source: src,
						Lyrics: duxlyrics.Lyrics{Synced: []duxlyrics.Line{line}}}})
				}
			}
			s := quietService(t, dir, func(_ string, d any) { last = d.(Event) }, nil)
			if tt.file != "" {
				require.NoError(t, os.MkdirAll(filepath.Dir(s.offsetPath(key(track))), 0o700))
				require.NoError(t, os.WriteFile(s.offsetPath(key(track)), []byte(tt.file), 0o600))
			}
			show(s, tt.first)
			s.TrackChanged(track)
			require.Equal(t, "ready", last.State)
			if tt.set != 0 {
				_, err := s.SetOffset(tt.set, true)
				require.NoError(t, err)
			}
			if tt.live {
				show(s, tt.then)
			} else {
				s = quietService(t, dir, func(_ string, d any) { last = d.(Event) }, nil)
				show(s, tt.then)
				s.TrackChanged(track)
			}
			require.Equal(t, "ready", last.State)
			assert.InDelta(t, tt.want, last.Offset, 1e-9)
		})
	}
}

// Word timing that drifts against a line source is laid onto that source's
// lines one by one, keeping the rhythm within each line.
func TestAnchoredLaysWordsOnTheLineTiming(t *testing.T) {
	gap := 0.0 // an instrumental before line 20, in seconds
	lineAt := func(i int) float64 {
		if i >= 20 {
			return 10 + 4*float64(i) + gap
		}
		return 10 + 4*float64(i)
	}
	plain := func(i int) string { return fmt.Sprintf("Line %d!", i) }
	lines := func(text func(int) string) []duxlyrics.Line {
		out := make([]duxlyrics.Line, 40)
		for i := range out {
			out[i] = duxlyrics.Line{Start: duxlyrics.Seconds(lineAt(i)), Text: text(i)}
		}
		return out
	}
	// drift: 6 s late at the start, 8 s at the end
	words := func(text func(int) string, last float64) []duxlyrics.Line {
		out := make([]duxlyrics.Line, 40)
		for i := range out {
			at := duxlyrics.Seconds(lineAt(i) + 6 + 2*float64(i)/float64(len(out)))
			if i >= 20 && gap > 0 {
				at-- // its instrumental is a second shorter
			}
			out[i] = duxlyrics.Line{Start: at, Text: text(i), Words: []duxlyrics.Word{
				{Start: at, Text: "line"}, {Start: at + duxlyrics.Seconds(last), Text: fmt.Sprint(i)},
			}}
		}
		return out
	}
	same := func(i int) string { return fmt.Sprintf("line %d", i) }
	tests := []struct {
		name  string
		text  func(int) string
		last  float64 // when the second word starts within its line
		ok    bool
		check func(t *testing.T, got []duxlyrics.Line)
		lines func(int) string // the line source's text, plain when nil
		gap   float64
	}{
		{"each line lands on the line source's start, its rhythm kept", same, 1, true, func(t *testing.T, got []duxlyrics.Line) {
			for i, l := range got {
				assert.InDelta(t, lineAt(i), float64(l.Start), 0.001, "line %d", i)
				assert.InDelta(t, lineAt(i)+1, float64(l.Words[1].Start), 0.001, "line %d", i)
			}
		}, nil, 0},
		{"unmatched lines move as their matched neighbours do", func(i int) string {
			if i == 0 || i == 20 || i == 39 {
				return "something else"
			}
			return same(i)
		}, 1, true, func(t *testing.T, got []duxlyrics.Line) {
			assert.InDelta(t, lineAt(20), float64(got[20].Start), 0.001, "between two matched lines")
			assert.InDelta(t, lineAt(0), float64(got[0].Start), 0.06, "before the first matched line")
			assert.InDelta(t, lineAt(39), float64(got[39].Start), 0.06, "after the last matched line")
		}, nil, 0},
		{"a word never runs past the next line", same, 5, true, func(t *testing.T, got []duxlyrics.Line) {
			for i, l := range got[:len(got)-1] {
				assert.LessOrEqual(t, float64(l.Words[1].Start), float64(got[i+1].Start), "line %d", i)
			}
		}, nil, 0},
		{"too few lines in common: nothing", func(i int) string {
			if i%5 == 0 {
				return same(i)
			}
			return fmt.Sprintf("other %d", i)
		}, 1, false, nil, nil, 0},
		{"a repeated chorus keeps each repeat on its own line", func(i int) string {
			// the word source sings the chorus once more, one line early
			if i >= 9 && i <= 13 {
				return "chorus"
			}
			return same(i)
		}, 1, true, func(t *testing.T, got []duxlyrics.Line) {
			for i := 10; i <= 13; i++ {
				assert.InDelta(t, lineAt(i), float64(got[i].Start), 0.001, "line %d", i)
			}
		}, func(i int) string {
			if i >= 10 && i <= 13 {
				return "Chorus!"
			}
			return plain(i)
		}, 0},
		{"no word lands in an instrumental", func(i int) string {
			if i == 20 {
				return "something else"
			}
			return same(i)
		}, 1, true, func(t *testing.T, got []duxlyrics.Line) {
			for i, l := range got {
				for _, w := range l.Words {
					if i < 20 {
						assert.LessOrEqual(t, float64(w.Start), lineAt(19)+1.001, "line %d", i)
					} else {
						assert.GreaterOrEqual(t, float64(w.Start), lineAt(20)-0.06, "line %d", i)
					}
				}
			}
		}, nil, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gap = tt.gap
			text := tt.lines
			if text == nil {
				text = plain
			}
			w := &duxlyrics.Result{Found: true, Source: duxlyrics.KuGou, Lyrics: duxlyrics.Lyrics{Synced: words(tt.text, tt.last)}}
			l := &duxlyrics.Result{Found: true, Source: duxlyrics.LRCLIB, Lyrics: duxlyrics.Lyrics{Synced: lines(text)}}
			got, ok := anchored(w, l)
			require.Equal(t, tt.ok, ok)
			if ok {
				tt.check(t, got.Synced)
			}
		})
	}
}

// A line source on the recording takes a word source's words when, laid on
// its lines, they land on the recording too.
func TestDecideKeepsWordsLaidOnAnOnTimeLineSource(t *testing.T) {
	result := func(src duxlyrics.Provider, late float64, words bool) *duxlyrics.Result {
		lines := make([]duxlyrics.Line, 40)
		for i := range lines {
			at := duxlyrics.Seconds(10 + 4*float64(i) + late)
			lines[i] = duxlyrics.Line{Start: at, Text: fmt.Sprintf("line %d", i)}
			if words {
				lines[i].Words = []duxlyrics.Word{{Start: at, Text: "line"}, {Start: at + 1, Text: fmt.Sprint(i)}}
			}
		}
		return &duxlyrics.Result{Found: true, Source: src, Lyrics: duxlyrics.Lyrics{Synced: lines}}
	}
	tests := []struct {
		name     string
		hasWords bool
		line     float64 // the line source's measured shift
		own      float64 // the word source's
		laid     float64 // the laid copy's measured shift
		sure     bool    // and whether that fit is sure
		loose    bool    // the word source was found only without the length
		words    bool
		source   duxlyrics.Provider
		start    float64
	}{
		{"laid on time: words on the line timing", true, 0.1, -1.8, -0.2, false, false, true, duxlyrics.LRCLIB, 10},
		{"a sure laid fit is applied", true, 0.1, -1.8, 0.25, true, false, true, duxlyrics.LRCLIB, 10.25},
		{"laid but off: the line source as before", true, 0.1, -1.8, -1, false, false, false, duxlyrics.LRCLIB, 10},
		{"no word source: the line source as before", false, 0.1, -1.8, 0, false, false, false, duxlyrics.LRCLIB, 10},
		{"no line source on time: as before", true, -3, -1.8, 0, false, false, true, duxlyrics.KuGou, 16},
		{"loose words are laid on an on-time line source", true, 0.1, -1.8, -0.2, false, true, true, duxlyrics.LRCLIB, 10},
		{"loose words off the recording are never shown alone", true, -3, -1.8, 0, false, true, false, duxlyrics.LRCLIB, 10},
		{"loose words on the recording are shown", true, -3, 0.2, 0, false, true, true, duxlyrics.KuGou, 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := result(duxlyrics.LRCLIB, 0, false)
			rec := fakeRecording{fits: map[duxlyrics.Provider]fitResult{duxlyrics.LRCLIB: {Shift: tt.line}}}
			results := []*duxlyrics.Result{line}
			var loose []*duxlyrics.Result
			if tt.hasWords {
				word := result(duxlyrics.KuGou, 6, true)
				rec.fits[duxlyrics.KuGou] = fitResult{Shift: tt.own}
				rec.laid = &fitResult{Shift: tt.laid, Confident: tt.sure}
				if tt.loose {
					loose = append(loose, word)
				} else {
					results = append(results, word)
				}
			}
			got, _ := decide(allProviders(), results, loose, rec)
			assert.Equal(t, tt.source, got.Source)
			assert.Equal(t, tt.words, tier(answer{res: got}) == 2)
			assert.InDelta(t, tt.start, float64(got.Synced[0].Start), 0.001)
		})
	}
}

// The source menu tells per source what the last pick measured and did.
func TestStatusSaysWhatThePickDidWithEachSource(t *testing.T) {
	result := func(src duxlyrics.Provider, late float64, timing string) *duxlyrics.Result {
		if timing == "plain" {
			return &duxlyrics.Result{Found: true, Source: src, Lyrics: duxlyrics.Lyrics{Plain: "line"}}
		}
		lines := make([]duxlyrics.Line, 40)
		for i := range lines {
			at := duxlyrics.Seconds(10 + 4*float64(i) + late)
			lines[i] = duxlyrics.Line{Start: at, Text: fmt.Sprintf("line %d", i)}
			if timing == "word" {
				lines[i].Words = []duxlyrics.Word{{Start: at, Text: "line"}, {Start: at + 1, Text: fmt.Sprint(i)}}
			}
		}
		return &duxlyrics.Result{Found: true, Source: src, Lyrics: duxlyrics.Lyrics{Synced: lines}}
	}
	kugou, lrclib, better := duxlyrics.KuGou, duxlyrics.LRCLIB, duxlyrics.BetterLyrics
	type fits = map[duxlyrics.Provider]fitResult
	words := func() []*duxlyrics.Result {
		return []*duxlyrics.Result{result(kugou, 0, "word"), result(lrclib, 0, "line")}
	}
	tests := []struct {
		name    string
		results []*duxlyrics.Result
		fits    fits // nil: no recording
		laid    *fitResult
		src     duxlyrics.Provider
		chosen  duxlyrics.Provider
		want    Status
	}{
		{"shown on time", words(), fits{kugou: {0.2, false}, lrclib: {-6.6, false}}, nil, kugou, "", Status{Kind: "shown-on-time", Seconds: 0.2}},
		{"shown, moved by a sure fit", words(), fits{lrclib: {0.3, true}, kugou: {-1.8, false}}, nil, lrclib, "", Status{Kind: "moved", Seconds: 0.3}},
		{"shown words on another source's line timing", []*duxlyrics.Result{result(kugou, 6, "word"), result(lrclib, 0, "line")},
			fits{lrclib: {0.1, false}, kugou: {-1.8, false}}, &fitResult{Shift: -0.2}, kugou, "", Status{Kind: "aligned", Source: "lrclib"}},
		{"the line source that lent its timing", []*duxlyrics.Result{result(kugou, 6, "word"), result(lrclib, 0, "line")},
			fits{lrclib: {0.1, false}, kugou: {-1.8, false}}, &fitResult{Shift: -0.2}, lrclib, "", Status{Kind: "on-time", Seconds: 0.1}},
		{"shown off the recording, unchanged", words(), fits{kugou: {-1.8, false}}, nil, kugou, "", Status{Kind: "shown"}},
		{"on time, not chosen", []*duxlyrics.Result{result(kugou, 0, "word"), result(better, 0, "line")}, fits{kugou: {0.1, false}, better: {-0.1, false}}, nil, better, "", Status{Kind: "on-time", Seconds: -0.1}},
		{"late, not used", words(), fits{kugou: {0.1, false}, lrclib: {-6.6, false}}, nil, lrclib, "", Status{Kind: "late", Seconds: 6.6}},
		{"early, not used", words(), fits{kugou: {0.1, false}, lrclib: {1.2, true}}, nil, lrclib, "", Status{Kind: "early", Seconds: 1.2}},
		{"no recording", words(), nil, nil, lrclib, "", Status{Kind: "not-measured"}},
		{"too few lines", []*duxlyrics.Result{result(kugou, 0, "word"), result(lrclib, 0, "plain")}, fits{kugou: {0.1, false}}, nil, lrclib, "", Status{Kind: "not-measured"}},
		{"a lyrics file", []*duxlyrics.Result{result(duxlyrics.Sidecar, 0, "line"), result(lrclib, 0, "line")}, fits{lrclib: {0, false}}, nil, duxlyrics.Sidecar, "", Status{Kind: "file"}},
		{"chosen by hand", words(), fits{kugou: {0.1, false}, lrclib: {-6.6, false}}, nil, lrclib, lrclib, Status{Kind: "shown"}},
		{"another source chosen by hand", words(), fits{kugou: {0.1, false}, lrclib: {-6.6, false}}, nil, kugou, lrclib, Status{Kind: "on-time", Seconds: 0.1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rec recording
			if tt.fits != nil {
				rec = fakeRecording{fits: tt.fits, laid: tt.laid}
			}
			_, notes := decide(allProviders(), tt.results, nil, rec)
			assert.Equal(t, tt.want, statusOf(notes, tt.src, tt.chosen))
		})
	}
}

// A better pick replaces what is shown, also when only its timing tier differs.
func TestRetimedSeesABetterPick(t *testing.T) {
	result := func(src duxlyrics.Provider, start duxlyrics.Seconds, words bool) *duxlyrics.Result {
		line := duxlyrics.Line{Start: start, Text: "line"}
		if words {
			line.Words = []duxlyrics.Word{{Start: start, Text: "line"}}
		}
		return &duxlyrics.Result{Found: true, Source: src, Lyrics: duxlyrics.Lyrics{Synced: []duxlyrics.Line{line}}}
	}
	tests := []struct {
		name string
		a, b *duxlyrics.Result
		want bool
	}{
		{"the same", result(duxlyrics.LRCLIB, 1, false), result(duxlyrics.LRCLIB, 1, false), false},
		{"another source", result(duxlyrics.LRCLIB, 1, false), result(duxlyrics.KuGou, 1, false), true},
		{"moved", result(duxlyrics.LRCLIB, 1, false), result(duxlyrics.LRCLIB, 2, false), true},
		{"word timing on the same lines", result(duxlyrics.LRCLIB, 1, false), result(duxlyrics.LRCLIB, 1, true), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, retimed(tt.a, tt.b))
		})
	}
}
