package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Conava/dux-player/core/internal/playlists"
	"github.com/Conava/dux-player/core/internal/source/local"
)

// open is what the desktop entry runs, also for "Open with": files,
// folders and playlist files, as paths or file:// URIs. With none it only
// shows the window.
var openCmd = &cobra.Command{
	Use:   "open [path or file:// URI...]",
	Short: "Play audio files, folders and playlist files, and show the window",
	RunE: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return showWindow()
		}
		paths, skipped := openTargets(args)
		if skipped > 0 {
			fmt.Fprintf(os.Stderr, "skipped %d entries that aren't local audio files\n", skipped)
		}
		if len(paths) == 0 {
			return errors.New("nothing to play")
		}
		list := make([]any, len(paths))
		for i, p := range paths {
			list[i] = p
		}
		if err := call(true, "player.play", map[string]any{"paths": list}, nil); err != nil {
			return err
		}
		return showWindow()
	},
}

func init() { rootCmd.AddCommand(openCmd) }

// maxPlaylistFile matches what playlist import reads.
const maxPlaylistFile = 8 << 20

// openTargets turns arguments into absolute paths to play: a playlist file
// becomes its local entries; streams, missing files and other files are
// skipped and counted.
func openTargets(args []string) (paths []string, skipped int) {
	keep := func(p string) {
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() && !local.IsAudio(p) {
			skipped++
			return
		}
		paths = append(paths, p)
	}
	for _, a := range args {
		p, ok := localPath(a)
		if !ok {
			skipped++
			continue
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".m3u", ".m3u8", ".pls":
			entries, err := readPlaylist(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", p, err)
				skipped++
				continue
			}
			for _, e := range entries {
				if e.Path == "" {
					skipped++
					continue
				}
				keep(e.Path)
			}
		default:
			keep(p)
		}
	}
	return paths, skipped
}

func readPlaylist(path string) ([]playlists.Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPlaylistFile+1))
	if err != nil {
		return nil, err
	}
	return playlists.Parse(data, filepath.Dir(path))
}

// localPath reads a path or a file:// URI as an absolute path.
func localPath(arg string) (string, bool) {
	if strings.Contains(arg, "://") {
		u, err := url.Parse(arg)
		if err != nil || u.Scheme != "file" || u.Path == "" || (u.Host != "" && u.Host != "localhost") {
			return "", false
		}
		return filepath.Clean(u.Path), true
	}
	abs, err := filepath.Abs(arg)
	return abs, err == nil
}
