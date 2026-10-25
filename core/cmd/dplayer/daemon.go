package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/duxlabs-dev/dux-go/log"
	"github.com/spf13/cobra"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/keyring"
	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/settings"
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
TLS verification included.`,
	RunE: func(_ *cobra.Command, _ []string) error { return runDaemon() },
}

// runDaemon opens the socket before anything that can be slow (the keyring
// may prompt), so a CLI waiting for it never gives up and starts a second
// daemon; requests wait until startup is done.
func runDaemon() error {
	if err := requireRuntimeDir(); err != nil {
		return err
	}
	lock, err := lockFile(filepath.Join(appPaths.SocketDir(), ipc.AppName+".daemon.lock"), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return errors.New("a dplayer daemon is already running")
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
		Resolver:  libs,
		Publish:   bus.Publish,
		Continue:  continueQueue(libs),
		Refeed: func(ctx context.Context, o player.Origin) (player.Feed, error) {
			sh, err := libs.ShuffleSource(ctx, o.Kind, o.ID)
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
			Version: Version, Commit: Commit, Ctx: ctx, Quit: cancel, CacheDir: cacheDir},
		ConfigDir: configDir, DataDir: dataDir, StateDir: stateDir, Secrets: secrets, Mixer: mixer,
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
	return nil
}

const continueBatch = 25

// continueQueue picks what continues the queue when it runs out: tracks
// like the last one, or a shuffle over all libraries, none already queued.
func continueQueue(libs *library.Manager) func(context.Context, string, source.Track, map[source.Ref]bool) ([]source.Track, error) {
	return func(ctx context.Context, mode string, last source.Track, queued map[source.Ref]bool) ([]source.Track, error) {
		if mode == player.ContinueSimilar {
			if src, err := libs.Get(last.Library); err == nil {
				if ts := source.Similar(ctx, src, last, continueBatch, queued); len(ts) > 0 {
					return ts, nil
				}
			}
		}
		sh, err := libs.ShuffleSource(ctx, "tracks", "")
		if err != nil {
			return nil, err
		}
		var ts []source.Track
		if sh.Feed != nil {
			ts, err = sh.Feed.Next(ctx, continueBatch)
		} else {
			ts = sh.Tracks
		}
		ts = slices.DeleteFunc(ts, func(t source.Track) bool { return queued[t.Ref()] })
		return source.Window(ts, 0, continueBatch), err
	}
}
