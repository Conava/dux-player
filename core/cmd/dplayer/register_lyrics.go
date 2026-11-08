package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Conava/dux-player/core/internal/cache"
	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/lyrics"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

var lyricsCmd = &cobra.Command{
	Use:   "lyrics",
	Short: "Lyrics of the playing track",
	Args:  cobra.NoArgs,
}

var lyricsOffsetCmd = &cobra.Command{
	Use:   "offset <[+|-]seconds>",
	Short: "Set the playing track's lyrics timing in seconds (0 resets), or move it with +0.5 or -0.5",
	Long: "Set the playing track's lyrics timing in seconds, later when positive, or move it with a + or - sign.\n" +
		"A signed number always moves it, so a timing below zero is reached by steps: dplayer lyrics offset 0, then -2.\n" +
		"The timing is rounded to 0.5 s and stays within ±10 s.",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		args, done, err := signedArgs(cmd, args)
		if done || err != nil {
			return err
		}
		if len(args) != 1 {
			return fmt.Errorf("lyrics offset takes one number of seconds")
		}
		v, err := strconv.ParseFloat(args[0], 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%q is not a number of seconds", args[0])
		}
		param := "to"
		if strings.HasPrefix(args[0], "+") || strings.HasPrefix(args[0], "-") {
			param = "by"
		}
		var out struct {
			Offset float64 `json:"offset"`
		}
		if err := call(true, "lyrics.setOffset", map[string]any{param: v}, &out); err != nil {
			return err
		}
		switch {
		case jsonOutput:
			return printJSON(out)
		case out.Offset == 0:
			fmt.Println("Lyrics timing reset")
		default:
			fmt.Printf("Lyrics timing %+.1f s\n", out.Offset)
		}
		return nil
	},
}

func init() {
	lyricsCmd.AddCommand(lyricsOffsetCmd)
	rootCmd.AddCommand(lyricsCmd)
	var svc *lyrics.Service
	addFeature(feature{
		register: func(r *ipc.Registry, d *daemon) { lyrics.Register(r, func() *lyrics.Service { return svc }) },
		start: func(d *daemon) error {
			svc = lyrics.New(d.Ctx, lyrics.Config{CacheDir: d.CacheDir, ConfigDir: d.ConfigDir, DataDir: d.DataDir, Version: d.Version,
				Libraries: d.Libraries, Publish: d.Bus.Publish, HasSubscriber: d.Bus.HasSubscriber,
				Enabled:     func() bool { return d.Settings.Get().Lyrics },
				AudioTiming: func() bool { return d.Settings.Get().LyricsAudioTiming },
				Keep:        func() bool { return d.Settings.Get().LyricsForDownloads },
				HasCopy:     d.Libraries.HasCopy,
				AudioFile:   d.Libraries.AudioFile,
				Decode: func(ctx context.Context, path string, read func(io.Reader) error) error {
					return d.Meter.Decode(ctx, path, lyrics.FitRate, read)
				}})
			var queue *player.QueueSnapshot
			d.Bus.On(d.Ctx, func(topic string, data any) {
				index := 0
				switch topic {
				case player.TopicStatus:
					st := data.(player.Status)
					svc.TrackChanged(st.Track)
					index = st.Index
					// The queue restored at start was published before this listener.
					if queue == nil && st.State != player.StateStopped {
						if q, err := d.Player.Queue(d.Ctx); err == nil {
							queue = &q
						}
					}
				case player.TopicQueue:
					q := data.(player.QueueSnapshot)
					queue, index = &q, q.Index
				}
				if queue == nil {
					return
				}
				var next *source.Track
				if up := queue.Upcoming(index, 1); len(up) > 0 {
					next = &up[0]
				}
				svc.NextChanged(next)
			}, player.TopicStatus, player.TopicQueue)
			// On its own listener, so a download never waits for lyrics.
			d.Bus.On(d.Ctx, func(_ string, data any) {
				svc.Stored(d.Ctx, data.(source.Track), d.Settings.Get().LyricsForDownloads)
			}, cache.TopicStored)
			d.Bus.On(d.Ctx, func(string, any) { svc.SettingsChanged() }, ipc.TopicSettings)
			return nil
		},
		stop: func() { svc.Close() },
	})
}
