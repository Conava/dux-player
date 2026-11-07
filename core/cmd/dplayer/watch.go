package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
	"github.com/Conava/dux-player/core/internal/template"
)

const maxEventLine = 16 << 20

var (
	statusFormat string
	watchFormat  string
	watchTopics  []string
)

func init() {
	statusCmd.Flags().StringVar(&statusFormat, "format", "", "print one line from a template, like '%artist% - %title%'")
	watchCmd.Flags().StringVar(&watchFormat, "format", "", "print each change from a template instead of JSON")
	watchCmd.Flags().StringSliceVar(&watchTopics, "topics", []string{player.TopicStatus}, "event topics to follow, as JSON lines")
	rootCmd.AddCommand(watchCmd)
}

// libraryNames maps library ids to names; an unknown id prints as itself.
func libraryNames() map[string]string {
	var libs []source.Library
	out := map[string]string{}
	if call(false, "libraries.list", nil, &libs) == nil {
		for _, l := range libs {
			out[l.ID] = l.Name
		}
	}
	return out
}

// formatStatus fills format in with the current track, plus %state%.
func formatStatus(format string, st player.Status, libraries map[string]string) string {
	var t source.Track
	if st.Track != nil {
		t = *st.Track
	}
	fields := template.TrackFields(t)
	fields["state"] = string(st.State)
	if name, ok := libraries[t.Library]; ok {
		fields["library"] = name
	}
	if strings.Contains(format, "playingfrom") {
		var q player.QueueSnapshot
		if call(false, "queue.list", nil, &q) == nil {
			fields["playingfrom"] = q.Origin.Name
		}
	}
	return template.Format(format, fields)
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Print a line whenever the track or the play state changes",
	Long: "Print a line whenever the track or the play state changes, until Ctrl+C or the daemon stops.\n" +
		"Without --format each line is one JSON event. --topics replaces the default status topic with the topics given; --format needs the status topic.",
	Args: cobra.NoArgs,
	RunE: func(*cobra.Command, []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return watch(ctx)
	},
}

func watch(ctx context.Context) error {
	if watchFormat != "" && !slices.Contains(watchTopics, player.TopicStatus) {
		return fmt.Errorf("--format prints status events: add %s to --topics", player.TopicStatus)
	}
	// A bar's restart loop would otherwise bring the daemon back after a quit.
	sock, err := socketPath(false)
	if err != nil {
		return err
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return fmt.Errorf("dial %s: %w", sock, err)
	}
	defer conn.Close()
	context.AfterFunc(ctx, func() { conn.Close() })

	req, err := json.Marshal(map[string]any{"id": 1, "method": "subscribe", "params": map[string]any{"topics": watchTopics}})
	if err != nil {
		return err
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	libraries := map[string]string{}
	if watchFormat != "" {
		libraries = libraryNames()
	}
	lines := bufio.NewScanner(conn)
	lines.Buffer(make([]byte, 0, 64<<10), maxEventLine)
	last := ""
	for lines.Scan() {
		var ev struct {
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		// the greeting and the subscribe reply carry no event
		if json.Unmarshal(lines.Bytes(), &ev) != nil || ev.Event == "" {
			continue
		}
		if ev.Event != player.TopicStatus {
			if watchFormat == "" {
				fmt.Println(lines.Text())
			}
			continue
		}
		var st player.Status
		if err := json.Unmarshal(ev.Data, &st); err != nil {
			return fmt.Errorf("read status: %w", err)
		}
		key := string(st.State)
		if st.Track != nil {
			key += "\x00" + st.Track.Library + "\x00" + st.Track.ID
		}
		if key == last {
			continue
		}
		last = key
		if watchFormat == "" {
			fmt.Println(lines.Text())
		} else {
			fmt.Println(formatStatus(watchFormat, st, libraries))
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	if errors.Is(lines.Err(), bufio.ErrTooLong) {
		return fmt.Errorf("an event is over %d MiB", maxEventLine>>20)
	}
	return errors.New("daemon stopped")
}
