package library

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/source"
)

// With the built-in player, a file in a format only mpv plays can't play,
// in lists and for the queue; with mpv everything plays.
func TestFormatsOnlyMPVPlays(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.AddLibrary(ctx, source.Library{ID: "l", Kind: "local", Name: "L", Enabled: true}))
	b, err := s.Begin(ctx, "l", 1)
	require.NoError(t, err)
	for id, path := range map[string]string{"flac": "/m/a.FLAC", "m4a": "/m/b.m4a"} {
		tr := IndexedTrack{Path: path}
		tr.Library, tr.ID, tr.AlbumID, tr.ArtistID = "l", id, "al", "ar"
		require.NoError(t, b.PutTrack(ctx, tr, ""))
	}
	_, err = b.Finish(ctx)
	require.NoError(t, err)

	for _, tt := range []struct {
		playable []string
		want     map[string]string
	}{
		{nil, map[string]string{"flac": "", "m4a": ""}},
		{[]string{"flac", "mp3"}, map[string]string{"flac": "", "m4a": CodeNeedsMPV}},
	} {
		s.SetPlayable(tt.playable)
		for id, code := range tt.want {
			require.Equal(t, code, s.TrackReason(ctx, "l", id).Code, "%s with %v", id, tt.playable)
			got, err := s.Track(ctx, "l", id)
			require.NoError(t, err)
			require.Equal(t, code, got.UnavailableCode, "%s listed with %v", id, tt.playable)
		}
	}
}

// A scan writing its batch must not stall lookups: playback asks the index
// on its loop while a big library is scanned.
func TestReadsDontWaitForAScan(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.AddLibrary(ctx, source.Library{ID: "l", Kind: "local", Name: "L", Enabled: true}))

	b, err := s.Begin(ctx, "l", 1)
	require.NoError(t, err)
	tr := IndexedTrack{Path: "/m/a.flac"}
	tr.Library, tr.ID, tr.AlbumID, tr.ArtistID = "l", "a", "al", "ar"
	require.NoError(t, b.PutTrack(ctx, tr, ""))

	done := make(chan error, 1)
	go func() {
		_, err := s.Libraries(ctx)
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		b.Abort()
		t.Fatal("a read waited for the scan's batch")
	}
	_, err = b.Finish(ctx)
	require.NoError(t, err)
	got, err := s.Track(ctx, "l", "a")
	require.NoError(t, err)
	require.Equal(t, "/m/a.flac", got.Path)
}

// A file reread after a tag write keeps its measurement; changed audio is
// measured again.
func TestRereadKeepsTheMeasurement(t *testing.T) {
	tests := []struct {
		name     string
		duration float64
		size     int64
		kept     bool
	}{
		{"tags rewritten", 200, 10_000_100, true},
		{"other length", 201, 10_000_100, false},
		{"other size", 200, 12_000_000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s, err := Open(ctx, filepath.Join(t.TempDir(), "library.db"))
			require.NoError(t, err)
			defer s.Close()
			require.NoError(t, s.AddLibrary(ctx, source.Library{ID: "l", Kind: "local", Name: "L", Enabled: true}))
			put := func(gen int64, tr IndexedTrack) {
				b, err := s.Begin(ctx, "l", gen)
				require.NoError(t, err)
				require.NoError(t, b.PutTrack(ctx, tr, ""))
				_, err = b.Finish(ctx)
				require.NoError(t, err)
			}
			tr := IndexedTrack{Path: "/m/a.flac", Size: 10_000_000, MTime: 1}
			tr.Library, tr.ID, tr.AlbumID, tr.ArtistID, tr.Duration = "l", "a", "al", "ar", 200
			put(1, tr)
			require.NoError(t, s.SetMeasured(ctx, "l", "a", &loudness.Result{Gain: -3, Silence: loudness.Silence{Start: 1, End: 2}}))
			tr.Size, tr.Duration, tr.MTime = tt.size, tt.duration, 2
			put(2, tr)
			_, ok := s.LoudnessGain(ctx, "l", "a")
			require.Equal(t, tt.kept, ok)
			pending, err := s.PendingMeasure(ctx, "l", true, 10)
			require.NoError(t, err)
			require.Equal(t, !tt.kept, len(pending) == 1)
		})
	}
}

// An album shows the genre most of its tracks carry; the Genres page lists
// albums by it.
func TestAlbumGenreIsTheMostCommon(t *testing.T) {
	tests := []struct {
		name   string
		genres []string
		want   string
	}{
		{"most common wins", []string{"A", "B", "A"}, "A"},
		{"empty tags ignored", []string{"", "", "B"}, "B"},
		{"no genre", []string{"", ""}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s, err := Open(ctx, filepath.Join(t.TempDir(), "library.db"))
			require.NoError(t, err)
			defer s.Close()
			require.NoError(t, s.AddLibrary(ctx, source.Library{ID: "l", Kind: "local", Name: "L", Enabled: true}))
			b, err := s.Begin(ctx, "l", 1)
			require.NoError(t, err)
			for i, g := range tt.genres {
				tr := IndexedTrack{Path: fmt.Sprintf("/m/%d.flac", i)}
				tr.Library, tr.ID, tr.AlbumID, tr.ArtistID, tr.Genre = "l", fmt.Sprint(i), "al", "ar", g
				require.NoError(t, b.PutTrack(ctx, tr, ""))
			}
			_, err = b.Finish(ctx)
			require.NoError(t, err)
			al, err := s.Album(ctx, "l", "al")
			require.NoError(t, err)
			require.Equal(t, tt.want, al.Genre)
		})
	}
}
