package playlists

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
)

func ids(tracks []source.Track) []string {
	out := []string{}
	for _, t := range tracks {
		out = append(out, t.Library+":"+t.ID)
	}
	return out
}

func tr(lib, id string) source.Track {
	return source.Track{Library: lib, ID: id, Title: "T" + id, Duration: 10, CoverID: "c" + id}
}

func TestStoreEditsSurviveReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "playlists.db")
	s, err := Open(ctx, path)
	require.NoError(t, err)
	p, err := s.Create(ctx, " Mix\x07 ", []source.Track{tr("local", "a"), tr("srv", "b")})
	require.NoError(t, err)
	assert.Equal(t, "Mix", p.Name)

	require.NoError(t, s.Add(ctx, p.ID, []source.Track{tr("srv", "c"), tr("local", "d")}, 1))
	require.NoError(t, s.Move(ctx, p.ID, 3, 0))
	require.NoError(t, s.Remove(ctx, p.ID, []int{1}))
	require.NoError(t, s.Rename(ctx, p.ID, "Road"))
	require.NoError(t, s.SetCover(ctx, p.ID, Cover{Type: "shape", Accent: "#aabbcc", Shape: "clover"}))
	require.Error(t, s.Remove(ctx, p.ID, []int{9}))
	require.Error(t, s.Rename(ctx, p.ID, " "))
	require.NoError(t, s.Close())

	s, err = Open(ctx, path)
	require.NoError(t, err)
	defer s.Close()
	got, err := s.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"srv:b", "srv:c", "local:d"}, ids(got.Tracks))
	assert.Equal(t, "Road", got.Name)
	assert.Equal(t, 3, got.TrackCount)
	assert.Equal(t, 30.0, got.Duration)
	assert.Equal(t, Cover{Type: "shape", Accent: "#aabbcc", Shape: "clover"}, got.Cover)
	assert.Equal(t, []CoverRef{{"srv", "cb"}, {"srv", "cc"}, {"local", "cd"}}, got.Mosaic)
	assert.Equal(t, "Tb", got.Tracks[0].Title)

	require.NoError(t, s.Delete(ctx, p.ID))
	list, err := s.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
}

// Each edit starts from a, b, c, d; a refused edit leaves the list as it was.
func TestStoreEdits(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "playlists.db"))
	require.NoError(t, err)
	defer s.Close()
	x := []source.Track{tr("l", "x")}
	tests := []struct {
		name    string
		edit    func(id string) error
		want    []string
		wantErr bool
	}{
		{"add at start", func(id string) error { return s.Add(ctx, id, x, 0) }, []string{"l:x", "l:a", "l:b", "l:c", "l:d"}, false},
		{"add past the end appends", func(id string) error { return s.Add(ctx, id, x, 9) }, []string{"l:a", "l:b", "l:c", "l:d", "l:x"}, false},
		{"move down", func(id string) error { return s.Move(ctx, id, 0, 2) }, []string{"l:b", "l:c", "l:a", "l:d"}, false},
		{"move up", func(id string) error { return s.Move(ctx, id, 3, 1) }, []string{"l:a", "l:d", "l:b", "l:c"}, false},
		{"move out of range", func(id string) error { return s.Move(ctx, id, 0, 4) }, []string{"l:a", "l:b", "l:c", "l:d"}, true},
		{"remove several", func(id string) error { return s.Remove(ctx, id, []int{3, 0}) }, []string{"l:b", "l:c"}, false},
		{"remove one bad position", func(id string) error { return s.Remove(ctx, id, []int{1, 4}) }, []string{"l:a", "l:b", "l:c", "l:d"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := s.Create(ctx, "P", []source.Track{tr("l", "a"), tr("l", "b"), tr("l", "c"), tr("l", "d")})
			require.NoError(t, err)
			err = tt.edit(p.ID)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			got, err := s.Get(ctx, p.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.want, ids(got.Tracks))
		})
	}
	require.Error(t, s.Add(ctx, "missing", x, 0))
}

func TestCoverValidation(t *testing.T) {
	for _, c := range []Cover{{Type: "auto"}, {Type: "art", Library: "l", ID: "t"}, {Type: "shape", Accent: "#00ff00", Shape: "pill"}} {
		assert.NoError(t, c.validate(), c)
	}
	for _, c := range []Cover{{}, {Type: "art", Library: "l"}, {Type: "shape", Accent: "red", Shape: "pill"},
		{Type: "shape", Accent: "#00ff00", Shape: "a b"}, {Type: "auto", ID: "x"}} {
		assert.Error(t, c.validate(), c)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		data string
		want []Entry
	}{
		{"extended m3u with BOM and CRLF", "\xef\xbb\xbf#EXTM3U\r\n#EXTINF:123,Ann - One\r\nA/01.flac\r\n\r\n# comment\r\n/abs/02.mp3\r\n",
			[]Entry{{Path: "/music/A/01.flac", Title: "Ann - One", Duration: 123}, {Path: "/abs/02.mp3"}}},
		{"urls", "file:///x/y%20z.ogg\nhttps://radio.example/stream\n../up.wav\n",
			[]Entry{{Path: "/x/y z.ogg"}, {URL: "https://radio.example/stream"}, {Path: "/up.wav"}}},
		{"windows separators", "Sub\\b.mp3\n", []Entry{{Path: "/music/Sub/b.mp3"}}},
		{"pls out of order", "[playlist]\nTitle2=Two\nFile2=/b.mp3\nFile1=a.mp3\nLength1=61\nFile3=\nNumberOfEntries=3\nVersion=2\n",
			[]Entry{{Path: "/music/a.mp3", Duration: 61}, {Path: "/b.mp3", Title: "Two"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.data), "/music")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatsParseBack(t *testing.T) {
	entries := []Entry{{Path: "/m/a.flac", Title: "Ann - One", Duration: 61}, {Path: "/m/b c.mp3", Title: "Two", Duration: 2}}
	for name, data := range map[string][]byte{"m3u8": FormatM3U(entries), "pls": FormatPLS(entries)} {
		got, err := Parse(data, "/elsewhere")
		require.NoError(t, err, name)
		assert.Equal(t, entries, got, name)
	}
}

// Import reads only regular files; export writes only playlist files.
func TestImportExportPaths(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe.m3u")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	_, err := readPlaylistFile(fifo)
	assert.Error(t, err, "a FIFO fails at once")

	s := &Service{}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "d.m3u8"), 0o700))
	for _, path := range []string{filepath.Join(dir, "notes.txt"), filepath.Join(dir, "d.m3u8")} {
		_, err := s.Export(context.Background(), "p", path, "m3u8")
		assert.Error(t, err, path)
	}
}

// Playlists stored before the artist index existed are indexed on open.
func TestArtistIndexMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "playlists.db")
	old, err := library.OpenDB(ctx, path, migrations[:1])
	require.NoError(t, err)
	_, err = old.Exec(`INSERT INTO playlists (id, name, created, updated) VALUES ('p', 'Old', 0, 0);
		INSERT INTO items (playlist_id, pos, library_id, track_id, track) VALUES ('p', 0, 'loc', 'x', '{"artistId":"bob"}')`)
	require.NoError(t, err)
	require.NoError(t, old.Close())

	s, err := Open(ctx, path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	m, err := s.ArtistMatches(ctx, "loc", "bob", "")
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"p": 1}, m)
}
