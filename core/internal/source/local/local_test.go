package local

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.senan.xyz/taglib"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/source"
)

// writeWAV writes one second of silence, tagged when tags is not nil.
func writeWAV(t *testing.T, path string, tags map[string][]string) {
	t.Helper()
	const rate, n = 8000, 8000
	data := make([]byte, 44+2*n)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(36+2*n))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*2)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(2*n))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
	if tags != nil {
		require.NoError(t, taglib.WriteTags(path, tags, 0))
	}
}

func tags(title, artist, album, track string) map[string][]string {
	return map[string][]string{
		taglib.Title: {title}, taglib.Artist: {artist}, taglib.Album: {album},
		taglib.TrackNumber: {track}, taglib.Date: {"2021-03-04"},
	}
}

func TestScan(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	other := t.TempDir()
	writeWAV(t, filepath.Join(root, "A/X/02.wav"), tags("Two", "Ann", "Ex", "2/9"))
	writeWAV(t, filepath.Join(root, "A/X/01.wav"), tags("One\x07", "Ann", "Ex", "1/9"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "A/X/cover.jpg"), []byte("jpg"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "A/X/notes.txt"), []byte("x"), 0o644))
	writeWAV(t, filepath.Join(root, ".hidden/skip.wav"), tags("Hidden", "Ann", "Ex", "3"))
	writeWAV(t, filepath.Join(other, "Y/only.wav"), tags("Solo", "Bob", "Why", "1"))
	writeWAV(t, filepath.Join(other, "untagged.wav"), nil)

	store, err := library.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	lib := source.Library{ID: "lib1", Kind: Kind, Name: "Music", Enabled: true,
		Config: map[string]any{"folders": []any{root, other, "relative/ignored"}}}
	require.NoError(t, store.AddLibrary(ctx, lib))
	src, err := Factory(store)(lib, source.Env{CacheDir: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { src.Close() })
	s := src.(*Source)

	stats, err := s.Scan(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, source.ScanStats{Seen: 4, Updated: 4}, stats)

	artists, err := s.Artists(ctx)
	require.NoError(t, err)
	var names []string
	for _, a := range artists {
		names = append(names, a.Name)
	}
	assert.Equal(t, []string{"Ann", "Bob", "Unknown Artist"}, names)

	albums, err := s.Albums(ctx, artists[0].ID)
	require.NoError(t, err)
	require.Len(t, albums, 1)
	album, err := s.Album(ctx, albums[0].ID)
	require.NoError(t, err)
	require.Len(t, album.Tracks, 2)
	assert.Equal(t, "One", album.Tracks[0].Title, "ordered by track number, control characters dropped")
	assert.Equal(t, 2, album.Tracks[1].TrackNo)
	assert.Equal(t, 2021, album.Year)
	assert.InDelta(t, 1.0, album.Tracks[0].Duration, 0.01)
	cover, err := s.Cover(ctx, album.CoverID, 0)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "A/X/cover.jpg"), cover)

	path, err := s.StreamURL(ctx, album.Tracks[0].ID)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "A/X/01.wav"), path)

	stats, err = s.Scan(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, source.ScanStats{Seen: 4}, stats, "unchanged files are not reread")

	require.NoError(t, os.Remove(filepath.Join(other, "Y/only.wav")))
	stats, err = s.Scan(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Removed)
	artists, err = s.Artists(ctx)
	require.NoError(t, err)
	assert.Len(t, artists, 2, "an artist without tracks is dropped")

	res, err := s.Search(ctx, "tw", 10)
	require.NoError(t, err)
	require.Len(t, res.Tracks, 1)
	assert.Equal(t, "Two", res.Tracks[0].Title)
}

func TestFilesRejectsNonAudioPaths(t *testing.T) {
	dir := t.TempDir()
	txt := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(txt, []byte("x"), 0o644))
	for _, id := range []string{txt, "relative.wav", filepath.Join(dir, "missing.flac")} {
		_, err := Files{}.StreamURL(context.Background(), id)
		assert.Error(t, err, id)
	}
}

func newLibrary(t *testing.T, root string, meter *loudness.Meter) (*Source, *library.Store) {
	t.Helper()
	ctx := context.Background()
	store, err := library.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	lib := source.Library{ID: "lib1", Kind: Kind, Name: "Music", Enabled: true, Config: map[string]any{"folders": []any{root}}}
	require.NoError(t, store.AddLibrary(ctx, lib))
	src, err := Factory(store)(lib, source.Env{CacheDir: t.TempDir(), Meter: meter})
	require.NoError(t, err)
	t.Cleanup(func() { src.Close() })
	return src.(*Source), store
}

func TestFavoritesAndRatings(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeWAV(t, filepath.Join(root, "a.wav"), tags("One", "Ann", "Ex", "1"))
	writeWAV(t, filepath.Join(root, "b.wav"), tags("Two", "Ann", "Ex", "2"))
	s, _ := newLibrary(t, root, nil)
	_, err := s.Scan(ctx, nil)
	require.NoError(t, err)
	albums, err := s.Albums(ctx, "")
	require.NoError(t, err)
	album, err := s.Album(ctx, albums[0].ID)
	require.NoError(t, err)
	one := album.Tracks[0]

	require.NoError(t, s.SetRating(ctx, source.ItemTrack, one.ID, 4))
	require.NoError(t, s.SetFavorite(ctx, source.ItemTrack, one.ID, true))
	require.NoError(t, s.SetFavorite(ctx, source.ItemAlbum, album.ID, true))
	assert.Error(t, s.SetRating(ctx, source.ItemTrack, one.ID, 6))
	assert.Error(t, s.SetFavorite(ctx, "genre", "x", true))

	_, err = s.Scan(ctx, nil)
	require.NoError(t, err)
	got, err := s.Track(ctx, one.ID)
	require.NoError(t, err)
	assert.True(t, got.Favorite, "marks survive a rescan")
	assert.Equal(t, 4, got.Rating, "setting the favorite keeps the rating")

	fav, err := s.Favorites(ctx)
	require.NoError(t, err)
	require.Len(t, fav.Tracks, 1)
	assert.Equal(t, "One", fav.Tracks[0].Title)
	require.Len(t, fav.Albums, 1)
	assert.True(t, fav.Albums[0].Favorite)
	assert.Empty(t, fav.Artists)

	require.NoError(t, s.SetFavorite(ctx, source.ItemTrack, one.ID, false))
	fav, err = s.Favorites(ctx)
	require.NoError(t, err)
	assert.Empty(t, fav.Tracks)
}

func TestLoudnessIsMeasuredOnlyWithoutReplayGain(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tagged := tags("Tagged", "Ann", "Ex", "1")
	tagged["REPLAYGAIN_TRACK_GAIN"] = []string{"-6.50 dB"}
	writeWAV(t, filepath.Join(root, "tagged.wav"), tagged)
	writeWAV(t, filepath.Join(root, "plain.wav"), tags("Plain", "Ann", "Ex", "2"))

	s, store := newLibrary(t, root, &loudness.Meter{})
	assert.Contains(t, s.Notices(), source.Notice{Code: source.NoticeNoFFmpeg,
		Message: "ffmpeg is not installed, so tracks without ReplayGain tags play without loudness correction"}, "a missing ffmpeg is reported")
	_, err := s.Scan(ctx, nil)
	require.NoError(t, err)
	pending, err := store.PendingMeasure(ctx, "lib1", false, 10)
	require.NoError(t, err)
	assert.Len(t, pending, 1, "only the file without ReplayGain tags waits for measuring")

	meter := loudness.Find()
	if !meter.Available() {
		t.Skip("ffmpeg not installed")
	}
	meter.SetWanted(false, false)
	m, store := newLibrary(t, root, meter)
	assert.Empty(t, m.Notices())
	_, err = m.Scan(ctx, nil)
	require.NoError(t, err)
	time.Sleep(300 * time.Millisecond)
	pending, err = store.PendingMeasure(ctx, "lib1", false, 10)
	require.NoError(t, err)
	assert.Len(t, pending, 1, "nothing is measured while ReplayGain is off")
	meter.SetWanted(true, false)
	require.Eventually(t, func() bool {
		pending, _ := store.PendingMeasure(ctx, "lib1", false, 10)
		return len(pending) == 0
	}, 10*time.Second, 20*time.Millisecond)
	pending, err = store.PendingMeasure(ctx, "lib1", true, 10)
	require.NoError(t, err)
	assert.Len(t, pending, 1, "the tagged file still lacks its silence")
	// Turning skipping silence on wakes the finished loop for the backfill.
	meter.SetWanted(true, true)
	require.Eventually(t, func() bool {
		pending, _ := store.PendingMeasure(ctx, "lib1", true, 10)
		return len(pending) == 0
	}, 10*time.Second, 20*time.Millisecond)
	tracks, err := m.RandomTracks(ctx, 10)
	require.NoError(t, err)
	for _, tr := range tracks {
		_, measured := m.LoudnessGain(ctx, tr.ID)
		assert.Equal(t, tr.Title == "Plain", measured, tr.Title)
	}
}

// browse.tracks on a local library pages straight from the index.
func TestTrackPage(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	put := func(path, title, artist, album, year string, age time.Duration) {
		tg := tags(title, artist, album, "1")
		tg[taglib.Date] = []string{year}
		writeWAV(t, filepath.Join(root, path), tg)
		mt := time.Now().Add(-age)
		require.NoError(t, os.Chtimes(filepath.Join(root, path), mt, mt))
	}
	put("a.wav", "Charlie", "Bob", "Old", "1990", 48*time.Hour)
	put("b.wav", "alpha", "Ann", "Zed", "2020", 24*time.Hour)
	put("c.wav", "Bravo", "Cat", "Mid", "2000", time.Hour)
	s, _ := newLibrary(t, root, nil)
	_, err := s.Scan(ctx, nil)
	require.NoError(t, err)

	titles := func(sort string, offset, limit int) []string {
		page, err := s.TrackPage(ctx, source.TrackQuery{Sort: sort, Offset: offset, Limit: limit})
		require.NoError(t, err)
		assert.Equal(t, 3, page.Total)
		assert.Equal(t, sort, page.Sort)
		var out []string
		for _, tr := range page.Tracks {
			out = append(out, tr.Title)
		}
		return out
	}
	assert.Equal(t, []string{"alpha", "Charlie", "Bravo"}, titles(source.SortDefault, 0, 0), "by album artist")
	assert.Equal(t, []string{"alpha", "Bravo", "Charlie"}, titles(source.SortTitle, 0, 0))
	assert.Equal(t, []string{"Bravo"}, titles(source.SortTitle, 1, 1))
	assert.Equal(t, []string{"Bravo", "Charlie", "alpha"}, titles(source.SortAlbum, 0, 0))
	assert.Equal(t, []string{"Charlie", "Bravo", "alpha"}, titles(source.SortYear, 0, 0))
	assert.Equal(t, []string{"Bravo", "alpha", "Charlie"}, titles(source.SortAdded, 0, 0), "newest first")
	assert.Empty(t, titles(source.SortTitle, 5, 10))
}

func TestFileDetails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.wav")
	writeWAV(t, path, map[string][]string{taglib.Title: {"A"}, taglib.DiscNumber: {"1/2"}, taglib.Genre: {"Jazz", "Funk"},
		"REPLAYGAIN_TRACK_GAIN": {"-6.5 dB"}, "REPLAYGAIN_TRACK_PEAK": {"0.98"}, "MUSICBRAINZ_TRACKID": {"rec-1"}})
	d, err := fileDetails(path)
	require.NoError(t, err)
	assert.Equal(t, "WAV", d.Format)
	assert.Equal(t, 8000, d.SampleRate)
	assert.Equal(t, 1, d.Channels)
	assert.Equal(t, 16, d.BitDepth)
	assert.Greater(t, d.Size, int64(16044), "the file with its tags")
	assert.Equal(t, 2, d.DiscCount)
	assert.Equal(t, []string{"Jazz", "Funk"}, d.Genres)
	require.NotNil(t, d.ReplayGain)
	assert.InDelta(t, -6.5, *d.ReplayGain.TrackGain, 0.001)
	assert.InDelta(t, 0.98, *d.ReplayGain.TrackPeak, 0.001)
	assert.Equal(t, map[string]string{"recordingId": "rec-1"}, d.MusicBrainz)
	assert.Equal(t, []string{"A"}, d.Tags[taglib.Title])
}

// Hidden files, like the copies a tag write stages, are not tracks.
func TestCollectSkipsHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.wav", ".a.wav.123.wav", ".hidden/b.wav"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		writeWAV(t, filepath.Join(dir, name), nil)
	}
	got, err := Collect(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(dir, "a.wav")}, got)
}

// One gone folder of several is a notice naming it.
func TestMissingFolderNotice(t *testing.T) {
	ctx := context.Background()
	store, err := library.Open(ctx, filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	gone := filepath.Join(t.TempDir(), "gone")
	lib := source.Library{ID: "lib1", Kind: Kind, Name: "Music", Enabled: true, Config: map[string]any{"folders": []any{t.TempDir(), gone}}}
	require.NoError(t, store.AddLibrary(ctx, lib))
	src, err := Factory(store)(lib, source.Env{CacheDir: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { src.Close() })
	s := src.(*Source)
	s.gone.Store(&map[string]bool{gone: true})
	assert.Contains(t, s.Notices(), source.Notice{Code: source.NoticeFolderMissing, Message: folderMissing(gone).Message, Detail: gone})
}
