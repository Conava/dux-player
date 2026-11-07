package library

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

func serverStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "library.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	require.NoError(t, s.AddLibrary(context.Background(), source.Library{ID: "srv", Kind: "fake", Name: "Server", Enabled: true}))
	return s
}

func put(t *testing.T, s *Store, gen int64, artists []source.Artist, albums []source.Album, tracks []source.Track) {
	t.Helper()
	ctx := context.Background()
	b := s.BeginCatalog("srv", gen, 1<<40)
	for _, a := range artists {
		require.NoError(t, b.PutServerArtist(ctx, a, false))
	}
	for _, a := range albums {
		require.NoError(t, b.PutServerAlbum(ctx, a, false))
	}
	for _, x := range tracks {
		require.NoError(t, b.PutServerTrack(ctx, x, false))
	}
	_, err := b.Finish(ctx)
	require.NoError(t, err)
}

// An upgrade from the schema before server catalogs keeps every local row
// without a rescan, and server libraries stay live until their first sync.
func TestUpgradeToServerCatalogs(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "library.db")
	old, err := OpenDB(ctx, path, migrations[:7])
	require.NoError(t, err)
	_, err = old.ExecContext(ctx, `INSERT INTO libraries (id, kind, name, last_scan) VALUES ('loc', 'local', 'Music', 100), ('srv', 'jellyfin', 'Server', 0);
		INSERT INTO artists (library_id, id, name) VALUES ('loc', 'ar', 'Artist');
		INSERT INTO albums (library_id, id, name, artist, artist_id, cover) VALUES ('loc', 'al', 'Album', 'Artist', 'ar', '/m/cover.jpg');
		INSERT INTO tracks (library_id, id, path, title, artist, artist_id, album, album_id, album_artist, size, mtime, scan_gen)
			VALUES ('loc', 'a', '/m/a.flac', 'A', 'Artist', 'ar', 'Album', 'al', 'Artist', 10, 2000000000, 1);`)
	require.NoError(t, err)
	require.NoError(t, old.Close())

	s, err := Open(ctx, path)
	require.NoError(t, err)
	defer s.Close()

	stale, err := s.Stale(ctx)
	require.NoError(t, err)
	assert.Empty(t, stale, "no local rescan")
	got, err := s.Track(ctx, "loc", "a")
	require.NoError(t, err)
	assert.Equal(t, "/m/a.flac", got.Path)
	assert.Equal(t, "al", got.CoverID, "a local cover is still the album's")
	assert.Equal(t, int64(2), got.Added)
	assert.False(t, s.Synced(ctx, "loc"), "a scanned local library is no sync")
	assert.False(t, s.Synced(ctx, "srv"))
}

// What a server said comes back as it was, through the same reads local
// libraries use.
func TestServerRowsReadBack(t *testing.T) {
	ctx := context.Background()
	s := serverStore(t)
	s.SetPlayable([]string{"flac"})
	gain := -6.5
	put(t, s, 1,
		[]source.Artist{{ID: "ar", Name: "Artist", CoverID: "ar-cover", Favorite: true}},
		[]source.Album{{ID: "al", Name: "Album", Artist: "Artist", ArtistID: "ar", Year: 2001, CoverID: "al-cover", Added: 500,
			PlayCount: 7, LastPlayed: 70, Rating: 4, Artists: []source.ArtistRef{{ID: "ar", Name: "Artist"}}}},
		[]source.Track{{ID: "t", Title: "Song", Artist: "Artist", ArtistID: "ar", Album: "Album", AlbumID: "al", AlbumArtist: "Artist",
			TrackNo: 2, CoverID: "t-cover", Favorite: true, Rating: 3, PlayCount: 5, LastPlayed: 50, Composer: "Bach", Format: "OPUS",
			Bitrate: 192, Added: 400, ReplayGain: &source.ReplayGain{Track: &gain}, Duration: 100,
			Artists: []source.ArtistRef{{ID: "ar", Name: "Artist"}, {Name: "Guest without id"}}}})

	got, err := s.Track(ctx, "srv", "t")
	require.NoError(t, err)
	assert.Empty(t, got.UnavailableCode, "a server transcodes, so no format is out of reach")
	assert.Equal(t, "t-cover", got.CoverID)
	assert.Equal(t, int64(400), got.Added)
	assert.Equal(t, 5, got.PlayCount)
	assert.Equal(t, int64(50), got.LastPlayed)
	assert.Equal(t, "Bach", got.Composer)
	assert.Equal(t, 192, got.Bitrate)
	assert.True(t, got.Favorite)
	assert.Equal(t, 3, got.Rating)
	require.NotNil(t, got.ReplayGain)
	assert.Equal(t, gain, *got.ReplayGain.Track)
	assert.Equal(t, []source.ArtistRef{{ID: "ar", Name: "Artist"}}, got.Artists, "a ref without an id gets no row")
	assert.Empty(t, s.TrackReason(ctx, "srv", "t").Code)

	album, err := s.Album(ctx, "srv", "al")
	require.NoError(t, err)
	assert.Equal(t, "al-cover", album.CoverID)
	assert.Equal(t, 7, album.PlayCount)
	assert.Equal(t, int64(500), album.Added)
	assert.Equal(t, 4, album.Rating)
	assert.Len(t, album.Tracks, 1)

	artists, err := s.Artists(ctx, "srv")
	require.NoError(t, err)
	require.Len(t, artists, 1)
	assert.Equal(t, "ar-cover", artists[0].CoverID)
	assert.True(t, artists[0].Favorite)
	assert.Equal(t, 1, artists[0].AlbumCount)

	_, found := s.TrackByPath(ctx, "")
	assert.False(t, found, "an empty path is not a server row")
}

// A rename on the server reaches the index, a mark cleared there goes, and a
// walk that didn't see a row sweeps it.
func TestServerRowsFollowTheServer(t *testing.T) {
	ctx := context.Background()
	s := serverStore(t)
	put(t, s, 1, []source.Artist{{ID: "ar", Name: "Old", CoverID: "old", Favorite: true}},
		[]source.Album{{ID: "al", Name: "Old album", Artist: "Old", ArtistID: "ar", Artists: []source.ArtistRef{{ID: "ar", Name: "Old"}}}},
		[]source.Track{{ID: "t", Title: "Old song", AlbumID: "al", Album: "Old album", Favorite: true,
			Artists: []source.ArtistRef{{ID: "guest", Name: "Old guest"}}},
			{ID: "gone", Title: "Gone", AlbumID: "al", Album: "Old album"}})
	put(t, s, 2, []source.Artist{{ID: "ar", Name: "New", CoverID: "new"}},
		[]source.Album{{ID: "al", Name: "New album", Artist: "New", ArtistID: "ar", Artists: []source.ArtistRef{{ID: "ar", Name: "New"}}}},
		[]source.Track{{ID: "t", Title: "New song", AlbumID: "al", Album: "New album",
			Artists: []source.ArtistRef{{ID: "guest", Name: "New guest"}}}})

	album, err := s.Album(ctx, "srv", "al")
	require.NoError(t, err)
	assert.Equal(t, "New album", album.Name)
	require.Len(t, album.Tracks, 1)
	assert.Equal(t, "New song", album.Tracks[0].Title)
	assert.False(t, album.Tracks[0].Favorite)
	artists, err := s.Artists(ctx, "srv")
	require.NoError(t, err)
	require.Len(t, artists, 2)
	assert.Equal(t, "New", artists[0].Name)
	assert.Equal(t, "new", artists[0].CoverID)
	assert.False(t, artists[0].Favorite)
	assert.Equal(t, "New guest", artists[1].Name, "an artist known only from a track")
}

// A later migration's "reread every file" has no file to reread in a server
// row, so it must not make the server library look stale.
func TestServerRowsAreNeverStale(t *testing.T) {
	ctx := context.Background()
	s := serverStore(t)
	put(t, s, 1, nil, nil, []source.Track{{ID: "t", Title: "No date"}})
	stale, err := s.Stale(ctx)
	require.NoError(t, err)
	assert.Empty(t, stale)
}

// An id with control characters or none at all is never stored.
func TestHostileIDsAreRefused(t *testing.T) {
	ctx := context.Background()
	s := serverStore(t)
	put(t, s, 1, []source.Artist{{ID: "a\x00b", Name: "X"}}, []source.Album{{ID: "", Name: "Y"}},
		[]source.Track{{ID: "t\nx", Title: "Z"}, {ID: "ok", Title: "Fine", Artists: []source.ArtistRef{{ID: "x\ty", Name: "W"}}}})
	n, err := s.TrackCount(ctx, "srv")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	artists, err := s.Artists(ctx, "srv")
	require.NoError(t, err)
	assert.Empty(t, artists)
}

// Changing the account forgets everything read with the old one, and the
// library is live again.
func TestClearCatalog(t *testing.T) {
	ctx := context.Background()
	s := serverStore(t)
	put(t, s, 1, nil, nil, []source.Track{{ID: "t", Title: "Song", Favorite: true}})
	require.NoError(t, s.SetCatalogSynced(ctx, "srv", "stamp"))
	require.True(t, s.Synced(ctx, "srv"))

	require.NoError(t, s.ClearCatalog(ctx, "srv"))
	assert.False(t, s.Synced(ctx, "srv"))
	stamp, last, err := s.CatalogState(ctx, "srv")
	require.NoError(t, err)
	assert.Empty(t, stamp)
	assert.Zero(t, last)
	n, err := s.TrackCount(ctx, "srv")
	require.NoError(t, err)
	assert.Zero(t, n)
	var marks int
	require.NoError(t, s.db.QueryRowContext(ctx, "SELECT count(*) FROM marks").Scan(&marks))
	assert.Zero(t, marks, "another account's hearts must not come back with the same ids")
}

// A heart set here while the server's list was on its way isn't taken back
// by that list.
func TestApplyFavoritesKeepsNewerMarks(t *testing.T) {
	ctx := context.Background()
	s := serverStore(t)
	put(t, s, 1, nil, nil, []source.Track{{ID: "a", Title: "A"}})
	on := true
	require.NoError(t, s.SetMark(ctx, "srv", source.ItemTrack, "a", &on, nil))

	listed := source.SearchResult{Tracks: []source.Track{{ID: "b"}}}
	changes, err := s.ApplyFavorites(ctx, "srv", listed, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, []MarkChange{{source.ItemTrack, "b", true}}, changes)
	got, err := s.Track(ctx, "srv", "a")
	require.NoError(t, err)
	assert.True(t, got.Favorite)
}
