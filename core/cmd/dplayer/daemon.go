package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	"github.com/spf13/cobra"

	"github.com/Conava/dux-player/core/internal/devices"
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/keyring"
	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/pendingmarks"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/player/engine"
	"github.com/Conava/dux-player/core/internal/settings"
	"github.com/Conava/dux-player/core/internal/shuffle"
	"github.com/Conava/dux-player/core/internal/similar"
	"github.com/Conava/dux-player/core/internal/source"
	"github.com/Conava/dux-player/core/internal/source/jellyfin"
	"github.com/Conava/dux-player/core/internal/source/local"
	"github.com/Conava/dux-player/core/internal/source/subsonic"
	"github.com/Conava/dux-player/core/internal/sysvol"
	"github.com/Conava/dux-player/core/internal/window"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the playback daemon in the foreground (no UI)",
	Long: `Run the playback daemon in the foreground (no UI).

Every playback command starts it in the background when it isn't running.
For development only: DPLAYER_MPV_ARGS adds mpv options, e.g.
DPLAYER_MPV_ARGS=--ao=null. They come last and override the daemon's own,
TLS verification included. DPLAYER_BACKEND=builtin plays with the built-in
player even when mpv is installed.`,
	RunE: func(_ *cobra.Command, _ []string) error {
		err := runDaemon()
		if err != nil && !errors.Is(err, errDaemonRunning) {
			notifyStartFailure(err)
		}
		return err
	},
}

var errDaemonRunning = errors.New("a dplayer daemon is already running")

// runDaemon opens the socket before anything that can be slow (the keyring
// may prompt), so a CLI waiting for it never gives up and starts a second
// daemon; requests wait until startup is done.
func runDaemon() error {
	log.GetLogger().SetReportTimestamp(true)
	if err := requireRuntimeDir(); err != nil {
		return err
	}
	lock, err := lockFile(filepath.Join(appPaths.SocketDir(), ipc.AppName+".daemon.lock"), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return errDaemonRunning
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	configDir, err := appPaths.ConfigDir()
	if err != nil {
		return err
	}
	dataDir, err := appPaths.DataDir()
	if err != nil {
		return err
	}
	cacheDir, err := appPaths.CacheDir()
	if err != nil {
		return err
	}
	stateDir, err := appPaths.StateDir()
	if err != nil {
		return err
	}
	// Earlier versions made these 0755; history and server addresses live there.
	for _, dir := range []string{configDir, dataDir, cacheDir, stateDir} {
		if err := os.Chmod(dir, 0o700); err != nil {
			log.Warnf("make %s private: %v", dir, err)
		}
	}
	go watchDaemonLog(ctx, stateDir)
	prefs, err := settings.Open(configDir)
	if err != nil {
		return err
	}
	store, err := library.Open(ctx, filepath.Join(dataDir, "library.db"))
	if err != nil {
		return err
	}
	defer store.Close()

	registry := source.Registry{
		local.Kind:    local.Factory(store),
		subsonic.Kind: subsonic.Factory,
		jellyfin.Kind: jellyfin.Factory,
	}
	meter := loudness.Find()
	cur := prefs.Get()
	meter.SetWanted(cur.ReplayGainMode != "off", cur.SkipSilenceStart || cur.SkipSilenceEnd)
	if !meter.Available() {
		log.Infof("ffmpeg not found; tracks without ReplayGain tags won't be measured")
	}
	secrets := keyring.Open()
	env := source.Env{Secrets: secrets, CacheDir: cacheDir, Meter: meter, Version: Version}
	libs := library.NewManager(store, registry, env, local.Files{})
	defer libs.Close()

	// mpv's socket lives in a private subdirectory so FindRunningSocket,
	// which dials every dplayer-*.sock, never mistakes it for ours.
	mpvDir := filepath.Join(appPaths.SocketDir(), ipc.AppName)
	if err := os.MkdirAll(mpvDir, 0o700); err != nil {
		return err
	}
	bus := ipc.NewBus()
	mixer := sysvol.Find()
	var system player.SystemVolume
	if mixer != nil {
		system = mixer
	}
	pl := player.New(player.Config{
		Socket:    filepath.Join(mpvDir, fmt.Sprintf("mpv-%d.sock", os.Getpid())),
		ExtraArgs: strings.Fields(os.Getenv("DPLAYER_MPV_ARGS")),
		Backend:   os.Getenv("DPLAYER_BACKEND"),
		OnBackend: func(backend string) {
			if backend == player.BackendBuiltin {
				libs.SetPlayable(engine.Containers)
			} else {
				libs.SetPlayable(nil)
			}
		},
		Resolver: libs,
		Publish:  bus.Publish,
		Continue: continueQueue(libs, prefs),
		Refeed: func(ctx context.Context, o player.Origin) (player.Feed, error) {
			sh, err := libs.PickSource(ctx, o.Kind, o.ID, prefs.Get().Picks())
			if err == nil && sh.Feed == nil {
				err = fmt.Errorf("%s is no longer a shuffle over whole libraries", o.Kind)
			}
			return sh.Feed, err
		},
		StatePath: filepath.Join(stateDir, "player-state.json"),
		Options:   prefs.Get().PlayerOptions(),
		System:    system,
	})
	playerDone := make(chan struct{})
	go func() {
		pl.Run(ctx)
		close(playerDone)
	}()

	ui := window.New(window.Config{
		Socket:     appPaths.SocketPath(),
		RuntimeDir: appPaths.SocketDir(),
		LogPath:    filepath.Join(stateDir, "ui.log"),
		OnExit: func() {
			if !prefs.Get().KeepPlayingOnClose {
				log.Infof("window closed and keep playing is off; stopping")
				cancel()
			}
		},
	})
	defer ui.Stop()

	d := &daemon{
		Deps: ipc.Deps{Bus: bus, Player: pl, Libraries: libs, Settings: prefs, Window: ui, FFmpeg: meter.Available(),
			Version: Version, Commit: Commit, Ctx: ctx, Quit: cancel, CacheDir: cacheDir,
			Marks: pendingmarks.Open(filepath.Join(stateDir, pendingmarks.FileName))},
		ConfigDir: configDir, DataDir: dataDir, StateDir: stateDir, Secrets: secrets, Mixer: mixer, Meter: meter,
		PipeWire: devices.PipeWireAvailable(),
	}
	ready := make(chan struct{})
	srv := ipc.NewServer(newRegistry(d), bus)
	srv.Ready = ready
	if err := srv.Listen(); err != nil {
		return err
	}
	defer srv.Close()
	srvErr := make(chan error, 1)
	go func() { srvErr <- srv.Serve(ctx) }()

	if err := libs.Load(ctx); err != nil {
		return err
	}
	for _, f := range features {
		if f.start == nil {
			continue
		}
		if err := f.start(d); err != nil {
			return err
		}
	}
	go library.WatchNetwork(ctx, func() {
		for _, f := range d.onNetwork {
			f()
		}
	})
	close(ready)
	log.Infof("dplayer daemon ready (ipc=%s)", srv.SocketPath())
	go func() {
		version, err := player.CheckMPV(ctx, "mpv")
		if err != nil {
			log.Warnf("mpv check: %v", err)
			return
		}
		log.Infof("using mpv %s", version)
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigs:
		log.Infof("received %s, shutting down", sig)
	case <-ctx.Done():
		log.Infof("quit requested, shutting down")
	case err := <-srvErr:
		if err != nil {
			log.Errorf("ipc server: %v", err)
		}
	}
	cancel()
	srv.Close()
	<-playerDone
	for _, f := range features {
		if f.stop != nil {
			f.stop()
		}
	}
	return nil
}

// continueQueue picks what continues the queue when it runs out:
// continueTracks tracks like the session (seeds), else a shuffle over all
// libraries, none already queued.
func continueQueue(libs *library.Manager, prefs *settings.Store) func(context.Context, string, []source.Track, map[source.Ref]bool) ([]source.Track, error) {
	lists := &playlistIndex{libs: libs}
	return func(ctx context.Context, mode string, seeds []source.Track, queued map[source.Ref]bool) ([]source.Track, error) {
		cur := prefs.Get()
		n := cur.ContinueTracks
		if mode == player.ContinueSimilar {
			if ts := pickSimilar(ctx, libs, lists, cur, seeds, queued); len(ts) > 0 {
				return ts, nil
			}
		}
		sh, err := libs.PickSource(ctx, "tracks", "", shuffle.Picks{Skip: cur.SkipRules})
		if err != nil {
			return nil, err
		}
		next := func(ctx context.Context, k int) ([]source.Track, error) {
			if sh.Feed != nil {
				return sh.Feed.Next(ctx, k)
			}
			ts := sh.Tracks
			sh.Tracks = nil
			return ts, nil
		}
		return similar.TopUp(ctx, n, next, queued, cur.SkipRules.Skips)
	}
}

const (
	// similarServerTimeout bounds the server's own similar call per seed.
	similarServerTimeout = 3 * time.Second
	// similarTimeout leaves the random fallback time within a continue.
	similarTimeout = 20 * time.Second
)

func pickSimilar(ctx context.Context, libs *library.Manager, lists *playlistIndex, cur settings.Settings, seeds []source.Track, queued map[source.Ref]bool) []source.Track {
	ctx, cancel := context.WithTimeout(ctx, similarTimeout)
	defer cancel()
	all, err := libs.Store().Libraries(ctx)
	if err != nil {
		log.Warnf("similar tracks: %v", err)
		return nil
	}
	enabled := map[string]bool{}
	for _, l := range all {
		enabled[l.ID] = l.Enabled
	}
	seeds = slices.DeleteFunc(slices.Clone(seeds), func(t source.Track) bool { return !enabled[t.Library] })
	in := similar.Input{
		Seeds:  seeds,
		Queued: queued,
		Skip: func(t source.Track) bool {
			return !enabled[t.Library] || cur.SkipRules.Skips(t)
		},
		Catalog:       catalog{libs},
		Playlists:     lists,
		Server:        serverSimilar(libs),
		ServerTimeout: similarServerTimeout,
		Recent:        recentPlays(ctx),
		Profile:       similar.Profile(cur.SimilarTracks),
	}
	return similar.Pick(ctx, in, cur.ContinueTracks)
}

func serverSimilar(libs *library.Manager) similar.Server {
	return func(ctx context.Context, seed source.Track) ([]source.Track, error) {
		src, err := libs.Get(seed.Library)
		if err != nil {
			return nil, err
		}
		s, ok := src.(source.SimilarTracks)
		if !ok {
			return nil, nil
		}
		ts, err := s.SimilarTracks(ctx, seed, source.MaxSimilarTracks)
		if errors.Is(err, source.ErrUnsupported) {
			return nil, nil
		}
		return ts, err
	}
}

// recentPlays is when the latest plays were; none while the history is
// unavailable.
func recentPlays(ctx context.Context) map[source.Ref]time.Time {
	h := historyStore.Load()
	if h == nil {
		return nil
	}
	plays, err := h.List(ctx, 500, 0)
	if err != nil {
		log.Warnf("similar tracks: recent plays: %v", err)
		return nil
	}
	out := make(map[source.Ref]time.Time, len(plays))
	for _, p := range plays {
		if _, ok := out[p.Track.Ref()]; !ok {
			out[p.Track.Ref()] = time.Unix(p.PlayedAt, 0)
		}
	}
	return out
}

// catalog reads candidates from the index for local libraries and synced
// servers, and from the server for the others.
type catalog struct{ libs *library.Manager }

func (c catalog) indexed(ctx context.Context, lib string) bool {
	return !c.libs.Server(lib) || c.libs.Store().Synced(ctx, lib)
}

// Live lookups are bounded: a few artists, a few albums each.
const (
	liveArtists = 4
	liveAlbums  = 3
	liveRandom  = 200
)

func (c catalog) TracksByArtists(ctx context.Context, lib string, artistIDs []string, limit int) ([]source.Track, error) {
	if c.indexed(ctx, lib) {
		return c.libs.Store().TracksByArtists(ctx, lib, artistIDs, limit)
	}
	b, err := c.libs.Browser(ctx, lib)
	if err != nil {
		return nil, err
	}
	var out []source.Track
	for _, id := range source.Window(artistIDs, 0, liveArtists) {
		albums, err := b.Albums(ctx, id)
		if err != nil {
			return source.Window(out, 0, limit), err
		}
		rand.Shuffle(len(albums), func(i, j int) { albums[i], albums[j] = albums[j], albums[i] })
		for _, a := range source.Window(albums, 0, liveAlbums) {
			full, err := b.Album(ctx, a.ID)
			if err != nil {
				return source.Window(out, 0, limit), err
			}
			out = append(out, full.Tracks...)
		}
	}
	return source.Window(out, 0, limit), nil
}

// Collaborators is index-only: asking a server would take a call per album.
func (c catalog) Collaborators(ctx context.Context, lib string, artistIDs []string, limit int) ([]similar.Collaborator, error) {
	if !c.indexed(ctx, lib) {
		return nil, nil
	}
	cs, err := c.libs.Store().Collaborators(ctx, lib, artistIDs, limit)
	out := make([]similar.Collaborator, len(cs))
	for i, co := range cs {
		out[i] = similar.Collaborator{ID: co.ID, Count: co.Count}
	}
	return out, err
}

func (c catalog) TracksByGenre(ctx context.Context, lib, genre string, limit int) ([]source.Track, error) {
	if c.indexed(ctx, lib) {
		return c.libs.Store().TracksByGenre(ctx, lib, genre, limit)
	}
	src, err := c.libs.Get(lib)
	if err != nil {
		return nil, err
	}
	r, ok := src.(source.RandomTracks)
	if !ok {
		return nil, nil
	}
	ts, err := r.RandomTracks(ctx, liveRandom)
	ts = slices.DeleteFunc(ts, func(t source.Track) bool { return !strings.EqualFold(t.Genre, genre) })
	return source.Window(ts, 0, limit), err
}

// playlistIndex is what scoring needs of every local playlist and of the
// server playlists of the libraries asked about; server playlists are read at
// most once per playlistTTL, or playlistRetry after a failed read.
type playlistIndex struct {
	libs *library.Manager
	mu   sync.Mutex
	// sets maps a server library to its playlists.
	sets map[string]*playlistSet
}

const (
	playlistTTL   = 30 * time.Minute
	playlistRetry = 2 * time.Minute
	// maxPlaylists bounds the playlists read per library.
	maxPlaylists     = 50
	playlistsTimeout = 10 * time.Second
	// playlistSample bounds the tracks kept per playlist as candidates.
	playlistSample = 300
	// maxPlaylistRefs bounds the tracks per library whose playlists are kept.
	maxPlaylistRefs = 20000
)

// playlistSet is a library's playlists: each one's length and first tracks,
// and which of them hold a track.
type playlistSet struct {
	until   time.Time
	lens    map[string]int
	samples map[string][]source.Track
	holding map[source.Ref][]string
}

func newPlaylistSet() *playlistSet {
	return &playlistSet{lens: map[string]int{}, samples: map[string][]source.Track{}, holding: map[source.Ref][]string{}}
}

func (s *playlistSet) add(id string, ts []source.Track) {
	s.lens[id] = len(ts)
	s.samples[id] = slices.Clone(source.Window(ts, 0, playlistSample))
	for _, t := range ts {
		r := t.Ref()
		ids, ok := s.holding[r]
		switch {
		case !ok && len(s.holding) >= maxPlaylistRefs:
			continue
		case len(ids) > 0 && ids[len(ids)-1] == id:
			continue
		}
		s.holding[r] = append(ids, id)
	}
}

// match adds the playlists holding any of refs to out, keyed prefix+id.
func (s *playlistSet) match(refs []source.Ref, limit int, prefix string, out map[string]similar.Playlist) {
	for _, r := range refs {
		for _, id := range s.holding[r] {
			pl := out[prefix+id]
			if pl.Len == 0 {
				pl = similar.Playlist{Len: s.lens[id], Tracks: source.Window(s.samples[id], 0, limit)}
			}
			pl.Seeds = append(pl.Seeds, r)
			out[prefix+id] = pl
		}
	}
}

func (x *playlistIndex) PlaylistsWith(ctx context.Context, refs []source.Ref, limit int) (map[string]similar.Playlist, error) {
	refs = slices.Compact(slices.SortedFunc(slices.Values(refs), func(a, b source.Ref) int {
		return cmp.Or(strings.Compare(a.Library, b.Library), strings.Compare(a.ID, b.ID))
	}))
	libs := map[string]bool{"": true}
	for _, r := range refs {
		libs[r.Library] = true
	}
	x.evict()
	out := map[string]similar.Playlist{}
	var errs []error
	for lib := range libs {
		set, err := x.get(ctx, lib)
		errs = append(errs, err)
		set.match(refs, limit, lib+"\x00", out)
	}
	return out, errors.Join(errs...)
}

// evict drops expired sets, so a removed library's go too.
func (x *playlistIndex) evict() {
	x.mu.Lock()
	defer x.mu.Unlock()
	for lib, set := range x.sets {
		if time.Now().After(set.until) {
			delete(x.sets, lib)
		}
	}
}

func (x *playlistIndex) get(ctx context.Context, lib string) (*playlistSet, error) {
	ctx, cancel := context.WithTimeout(ctx, playlistsTimeout)
	defer cancel()
	if lib == "" {
		return localPlaylistSet(ctx)
	}
	x.mu.Lock()
	set, ok := x.sets[lib]
	x.mu.Unlock()
	if ok {
		return set, nil
	}
	set, err := x.serverPlaylistSet(ctx, lib)
	set.until = time.Now().Add(playlistTTL)
	if err != nil {
		set.until = time.Now().Add(playlistRetry)
	}
	x.mu.Lock()
	if x.sets == nil {
		x.sets = map[string]*playlistSet{}
	}
	x.sets[lib] = set
	x.mu.Unlock()
	return set, err
}

// localPlaylistSet is empty until the playlists feature has started.
func localPlaylistSet(ctx context.Context) (*playlistSet, error) {
	set := newPlaylistSet()
	svc := localPlaylists.Load()
	if svc == nil {
		return set, nil
	}
	all, err := svc.Store.List(ctx)
	if err != nil {
		return set, err
	}
	for _, p := range source.Window(all, 0, maxPlaylists) {
		full, err := svc.Store.Get(ctx, p.ID)
		if err != nil {
			return set, err
		}
		set.add(p.ID, full.Tracks)
	}
	return set, nil
}

// serverPlaylistSet returns what it read before an error too.
func (x *playlistIndex) serverPlaylistSet(ctx context.Context, lib string) (*playlistSet, error) {
	set := newPlaylistSet()
	src, err := x.libs.Get(lib)
	if err != nil {
		return set, err
	}
	pl, ok := src.(source.Playlists)
	if !ok {
		return set, nil
	}
	all, err := pl.Playlists(ctx)
	if err != nil {
		return set, err
	}
	for _, p := range source.Window(all, 0, maxPlaylists) {
		full, err := pl.Playlist(ctx, p.ID)
		if err != nil {
			return set, err
		}
		for i := range full.Tracks {
			if full.Tracks[i].Library == "" {
				full.Tracks[i].Library = lib
			}
		}
		set.add(p.ID, full.Tracks)
	}
	return set, nil
}
