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

// A database from before the track fields keeps its rows and rereads every file.
func TestTrackFieldsMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "library.db")
	old, err := OpenDB(ctx, path, migrations[:6])
	require.NoError(t, err)
	_, err = old.ExecContext(ctx, `INSERT INTO libraries (id, kind, name, enabled, config) VALUES ('l', 'local', 'L', 1, '{}');
		INSERT INTO albums (library_id, id, name, artist, artist_id) VALUES ('l', 'al', 'A', 'B', 'ar');
		INSERT INTO tracks (library_id, id, path, title, artist, artist_id, album, album_id, album_artist, size, mtime, scan_gen)
		VALUES ('l', 'a', '/m/a.flac', 'T', 'B', 'ar', 'A', 'al', 'B', 1, 5000000000, 1)`)
	require.NoError(t, err)
	require.NoError(t, old.Close())

	s, err := Open(ctx, path)
	require.NoError(t, err)
	defer s.Close()
	got, err := s.Track(ctx, "l", "a")
	require.NoError(t, err)
	require.Equal(t, "T", got.Title)
	require.Empty(t, got.Format)
	require.Zero(t, got.Added, "mtime 0 sends the file to a reread")

	b, err := s.Begin(ctx, "l", 2)
	require.NoError(t, err)
	got.Composer, got.Format, got.Bitrate, got.MTime = "C", "FLAC", 900, 7000000000
	require.NoError(t, b.PutTrack(ctx, got, ""))
	_, err = b.Finish(ctx)
	require.NoError(t, err)
	got, err = s.Track(ctx, "l", "a")
	require.NoError(t, err)
	require.Equal(t, "C", got.Composer)
	require.Equal(t, "FLAC", got.Format)
	require.Equal(t, 900, got.Bitrate)
	require.EqualValues(t, 7, got.Added)
}

func TestSimilarQueries(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	defer s.Close()
	for _, id := range []string{"l", "m"} {
		require.NoError(t, s.AddLibrary(ctx, source.Library{ID: id, Kind: "local", Name: id, Enabled: true}))
	}
	ref := func(ids ...string) []source.ArtistRef {
		var out []source.ArtistRef
		for _, id := range ids {
			out = append(out, source.ArtistRef{ID: id, Name: id})
		}
		return out
	}
	put := func(b *Batch, lib, id, genre string, artists, albumArtists []string) {
		tr := IndexedTrack{Path: "/" + lib + "/" + id + ".flac"}
		tr.Library, tr.ID, tr.AlbumID, tr.ArtistID, tr.Genre = lib, id, "al-"+id, artists[0], genre
		tr.Artists = ref(artists...)
		tr.AlbumArtists = ref(albumArtists...)
		require.NoError(t, b.PutTrack(ctx, tr, ""))
	}
	b, err := s.Begin(ctx, "l", 1)
	require.NoError(t, err)
	put(b, "l", "1", "Rock", []string{"a", "b"}, []string{"a", "b"}) // b shares a track and an album
	put(b, "l", "2", "rock", []string{"a", "c"}, []string{"a"})
	put(b, "l", "3", "Jazz", []string{"b"}, []string{"b"})
	put(b, "l", "4", "Jazz", []string{"d"}, []string{"d"})
	put(b, "l", "5", "Rock", []string{"a", "b", "e"}, []string{"a"}) // e shares one item with both a and b
	_, err = b.Finish(ctx)
	require.NoError(t, err)
	// Same ids in another library must never leak in.
	b, err = s.Begin(ctx, "m", 1)
	require.NoError(t, err)
	put(b, "m", "1", "Rock", []string{"a", "z"}, []string{"a"})
	_, err = b.Finish(ctx)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, "UPDATE tracks SET missing = 1 WHERE library_id = 'l' AND id = '5'")
	require.NoError(t, err)

	ids := func(ts []source.Track) []string {
		out := []string{}
		for _, t := range ts {
			out = append(out, t.ID)
		}
		return out
	}
	byA, err := s.TracksByArtists(ctx, "l", []string{"b"}, 10)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"1", "3"}, ids(byA), "a featured artist counts; missing and other libraries don't")
	byA, err = s.TracksByArtists(ctx, "l", []string{"c", "d"}, 10)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"2", "4"}, ids(byA))
	byA, err = s.TracksByArtists(ctx, "l", []string{"a", "b"}, 2)
	require.NoError(t, err)
	require.Len(t, byA, 2)

	col, err := s.Collaborators(ctx, "l", []string{"a"}, 10)
	require.NoError(t, err)
	// b: track 1, album 1, track 5; e: track 5; c: track 2.
	require.Equal(t, []Collaborator{{"b", 3}, {"c", 1}, {"e", 1}}, col)
	col, err = s.Collaborators(ctx, "l", []string{"a", "b"}, 10)
	require.NoError(t, err)
	for _, c := range col {
		require.NotContains(t, []string{"a", "b"}, c.ID)
	}
	require.Equal(t, 1, col[0].Count, "an item with both seeds counts once")
	col, err = s.Collaborators(ctx, "l", []string{"a"}, 1)
	require.NoError(t, err)
	require.Equal(t, []Collaborator{{"b", 3}}, col)

	g, err := s.TracksByGenre(ctx, "l", "ROCK", 10)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"1", "2"}, ids(g), "missing and other libraries don't count")
	g, err = s.TracksByGenre(ctx, "l", "rock", 1)
	require.NoError(t, err)
	require.Len(t, g, 1)

	// More matches than the limit: calls must not keep returning one slice.
	b, err = s.Begin(ctx, "m", 2)
	require.NoError(t, err)
	for i := range 50 {
		put(b, "m", fmt.Sprintf("x%d", i), "Pop", []string{"q"}, []string{"q"})
	}
	_, err = b.Finish(ctx)
	require.NoError(t, err)
	seenA, seenG := map[string]bool{}, map[string]bool{}
	for range 5 {
		byA, err = s.TracksByArtists(ctx, "m", []string{"q"}, 10)
		require.NoError(t, err)
		for _, id := range ids(byA) {
			seenA[id] = true
		}
		g, err = s.TracksByGenre(ctx, "m", "pop", 10)
		require.NoError(t, err)
		for _, id := range ids(g) {
			seenG[id] = true
		}
	}
	require.Greater(t, len(seenA), 10, "artist pool is a random slice")
	require.Greater(t, len(seenG), 10, "genre pool is a random slice")
}
