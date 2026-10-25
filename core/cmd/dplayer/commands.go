package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Conava/dux-player/core/internal/ipc"
	"github.com/Conava/dux-player/core/internal/player"
)

var rootCmd = &cobra.Command{
	Use:           "dplayer",
	Short:         "Dux Player",
	Long:          "Dux Player plays local libraries and streams from (Open)Subsonic and Jellyfin servers.",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		cmd.SilenceUsage = true
	},
	RunE: func(*cobra.Command, []string) error { return showWindow() },
}

var shellDir string

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Open the window (starting the daemon if needed), or raise it",
	Args:  cobra.NoArgs,
	RunE:  func(*cobra.Command, []string) error { return showWindow() },
}

func showWindow() error {
	dir := shellDir
	if dir == "" {
		dir = os.Getenv("DPLAYER_SHELL_DIR")
	}
	params := map[string]any{}
	if dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		params["shellDir"] = abs
		// A daemon this starts takes the tree from its environment: it
		// runs a tree from IPC only in a dev build.
		if err := os.Setenv("DPLAYER_SHELL_DIR", abs); err != nil {
			return err
		}
	}
	return call(true, "ui.show", params, nil)
}

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Play the last queue from where it stopped, also after a restart",
	Args:  cobra.NoArgs,
	RunE:  func(*cobra.Command, []string) error { return statusCall("player.resume", nil) },
}

var shuffleCmd = &cobra.Command{
	Use:   "shuffle <tracks|favorites|library NAME|playlist NAME>",
	Short: "Shuffle all tracks, the favorites, a library or a playlist",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(_ *cobra.Command, args []string) error {
		params := map[string]any{"from": args[0]}
		switch {
		case (args[0] == "library" || args[0] == "playlist") && len(args) != 2:
			return fmt.Errorf("shuffle %s needs a name", args[0])
		case len(args) == 2:
			params["name"] = args[1]
		}
		return statusCall("player.shuffleFrom", params)
	},
}

var repeatCmd = &cobra.Command{
	Use:       "repeat <off|all|one>",
	Short:     "Set the repeat mode",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"off", "all", "one"},
	RunE: func(_ *cobra.Command, args []string) error {
		return statusCall("player.repeat", map[string]any{"mode": args[0]})
	},
}

var muteCmd = &cobra.Command{
	Use:       "mute [on|off]",
	Short:     "Mute or unmute; without an argument, toggle",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"on", "off"},
	RunE: func(_ *cobra.Command, args []string) error {
		params := map[string]any{}
		if len(args) == 1 {
			params["muted"] = args[0] == "on"
		}
		return statusCall("player.mute", params)
	},
}

var sleepCmd = &cobra.Command{
	Use:       "sleep <minutes|end|off>",
	Short:     "Pause after some minutes, at the end of the track, or cancel the sleep timer",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"end", "off"},
	RunE: func(_ *cobra.Command, args []string) error {
		switch args[0] {
		case "end":
			return statusCall("player.sleep", map[string]any{"endOfTrack": true})
		case "off":
			return statusCall("player.sleep", map[string]any{"off": true})
		}
		return statusCall("player.sleep", map[string]any{"minutes": args[0]})
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	RunE: func(_ *cobra.Command, _ []string) error {
		if jsonOutput {
			return printJSON(map[string]string{"version": Version, "commit": Commit, "buildTime": BuildTime})
		}
		fmt.Printf("dplayer %s (commit %s, built %s)\n", Version, Commit, BuildTime)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output JSON for programmatic usage (where supported)")
	rootCmd.AddCommand(versionCmd, daemonCmd, ipcCmd, playCmd, statusCmd, queueCmd, seekCmd, volumeCmd, quitCmd,
		showCmd, resumeCmd, shuffleCmd, repeatCmd, muteCmd, sleepCmd)
	showCmd.Flags().StringVarP(&shellDir, "config", "c", "", "quickshell dir to run instead of the embedded UI (env: DPLAYER_SHELL_DIR)")
	rootCmd.Flags().StringVarP(&shellDir, "config", "c", "", "quickshell dir to run instead of the embedded UI (env: DPLAYER_SHELL_DIR)")
	for _, c := range []struct{ use, short, method string }{
		{"toggle", "Play or pause", "player.toggle"},
		{"pause", "Pause", "player.pause"},
		{"stop", "Stop playback", "player.stop"},
		{"next", "Skip to the next track", "player.next"},
		{"previous", "Restart the track, or go back if it just started", "player.previous"},
	} {
		rootCmd.AddCommand(&cobra.Command{
			Use:   c.use,
			Short: c.short,
			Args:  cobra.NoArgs,
			RunE:  func(*cobra.Command, []string) error { return statusCall(c.method, nil) },
		})
	}
}

// statusCall runs a player method, starting the daemon if needed, and prints
// the resulting status.
func statusCall(method string, params map[string]any) error {
	var st player.Status
	if err := call(true, method, params, &st); err != nil {
		return err
	}
	return printStatus(st)
}

func clock(sec float64) string {
	s := int(math.Max(sec, 0))
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func printStatus(st player.Status) error {
	if jsonOutput {
		return printJSON(st)
	}
	if st.Track == nil {
		fmt.Printf("%s, queue empty, volume %.0f\n", st.State, st.Volume)
		return nil
	}
	t := st.Track
	modes := ""
	if st.Shuffle {
		modes += "  shuffle"
	}
	if st.Repeat != "" && st.Repeat != player.RepeatOff {
		modes += "  repeat " + st.Repeat
	}
	if st.Muted {
		modes += "  muted"
	}
	switch st.Sleep.Mode {
	case player.SleepTimer:
		modes += "  sleep at " + time.Unix(st.Sleep.At, 0).Format("15:04")
	case player.SleepEndOfTrack:
		modes += "  sleep at end of track"
	}
	fmt.Printf("%s: %s - %s (%s)  %s / %s  volume %.0f  [%d/%d]%s\n",
		st.State, t.Artist, t.Title, t.Album, clock(st.Position), clock(st.Duration), st.Volume, st.Index+1, st.Length, modes)
	if st.Error != "" {
		fmt.Printf("error: %s\n", st.Error)
	}
	return nil
}

var playCmd = &cobra.Command{
	Use:   "play [path...]",
	Short: "Play files and folders, or resume with no arguments",
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return statusCall("player.play", nil)
		}
		paths := make([]any, 0, len(args))
		for _, a := range args {
			abs, err := filepath.Abs(a)
			if err != nil {
				return err
			}
			paths = append(paths, abs)
		}
		return statusCall("player.play", map[string]any{"paths": paths})
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show what is playing",
	Args:  cobra.NoArgs,
	RunE:  func(*cobra.Command, []string) error { return statusCall("player.status", nil) },
}

// Flag parsing is off for seek and volume so "-10" is a value, not a flag.
func signedArgs(cmd *cobra.Command, args []string) ([]string, bool, error) {
	var out []string
	for _, a := range args {
		switch a {
		case "-h", "--help":
			return nil, true, cmd.Help()
		case "--json":
			jsonOutput = true
		default:
			out = append(out, a)
		}
	}
	return out, false, nil
}

var seekCmd = &cobra.Command{
	Use:                "seek <[+|-]seconds|m:ss>",
	Short:              "Seek to a position, or by an offset with + or -",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		args, done, err := signedArgs(cmd, args)
		if done || err != nil {
			return err
		}
		if len(args) != 1 {
			return fmt.Errorf("seek takes one position")
		}
		pos, relative, err := parseOffset(args[0])
		if err != nil {
			return err
		}
		return statusCall("player.seek", map[string]any{"position": pos, "relative": relative})
	},
}

var volumeCmd = &cobra.Command{
	Use:                "volume [[+|-]level]",
	Short:              "Show or set the volume (0-100)",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		args, done, err := signedArgs(cmd, args)
		if done || err != nil {
			return err
		}
		if len(args) > 1 {
			return fmt.Errorf("volume takes one level")
		}
		if len(args) == 0 {
			return statusCall("player.status", nil)
		}
		level, relative, err := parseOffset(args[0])
		if err != nil {
			return err
		}
		return statusCall("player.volume", map[string]any{"level": level, "relative": relative})
	},
}

// parseOffset reads "30", "1:30", "+10" and "-10"; a sign makes it relative.
func parseOffset(s string) (float64, bool, error) {
	relative := strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-")
	sign := 1.0
	if strings.HasPrefix(s, "-") {
		sign = -1
	}
	body := strings.TrimLeft(s, "+-")
	var total float64
	for part := range strings.SplitSeq(body, ":") {
		v, err := strconv.ParseFloat(part, 64)
		if err != nil || v < 0 {
			return 0, false, fmt.Errorf("%q is not a number or m:ss", s)
		}
		total = total*60 + v
	}
	return sign * total, relative, nil
}

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Show the queue",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		var q player.QueueSnapshot
		if err := call(true, "queue.list", nil, &q); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(q)
		}
		if len(q.Items) == 0 {
			fmt.Println("queue empty")
			return nil
		}
		from := q.Origin.Kind
		if q.Origin.Name != "" {
			from += " " + strconv.Quote(q.Origin.Name)
		}
		if q.Origin.Modified {
			from += ", modified"
		}
		fmt.Printf("from %s, shuffle %s, repeat %s\n", from, map[bool]string{true: "on", false: "off"}[q.Shuffle], q.Repeat)
		for i, it := range q.Items {
			mark := " "
			if i == q.Index {
				mark = ">"
			}
			t := it.Track
			fmt.Printf("%s %3d  %s - %s  %s  [%s]\n", mark, i+1, t.Artist, t.Title, clock(t.Duration), t.Library)
		}
		return nil
	},
}

var quitCmd = &cobra.Command{
	Use:   "quit",
	Short: "Stop the daemon and playback",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		if err := call(false, "daemon.quit", nil, nil); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return nil
		}
		// The daemon replies before it shuts down; wait for its socket to go,
		// so a window started right after can't attach to the old one.
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
			if _, err := ipc.FindRunningSocket(); err != nil {
				return nil
			}
		}
		fmt.Fprintln(os.Stderr, "the daemon is still shutting down")
		return nil
	},
}
