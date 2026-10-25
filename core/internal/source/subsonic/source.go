package subsonic

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Conava/dux-player/core/internal/source"
)

const (
	Kind = "subsonic"

	// Albums with no artist filter come from getAlbumList2, which pages at
	// 500; this caps how far it pages.
	maxAlbums = 5000
)

type Source struct {
	lib      source.Library
	client   *Client
	coverDir string
	// albumPaging is set once search3 with an empty query turned out to
	// return nothing, so track pages come from the album list instead.
	albumPaging atomic.Bool
}

var (
	_ source.Browser           = (*Source)(nil)
	_ source.TrackPager        = (*Source)(nil)
	_ source.CoverArt          = (*Source)(nil)
	_ source.Playlists         = (*Source)(nil)
	_ source.PlaylistEditor    = (*Source)(nil)
	_ source.Pinger            = (*Source)(nil)
	_ source.Favorites         = (*Source)(nil)
	_ source.Ratings           = (*Source)(nil)
	_ source.RandomTracks      = (*Source)(nil)
	_ source.PlaylistReorderer = (*Source)(nil)
	_ source.Scrobbler         = (*Source)(nil)
	_ source.CapabilityFilter  = (*Source)(nil)
	_ source.Notices           = (*Source)(nil)
	_ source.RecentPlays       = (*Source)(nil)
	_ source.ArtistDetailer    = (*Source)(nil)
)

func Factory(lib source.Library, env source.Env) (source.Source, error) {
	addr, _ := lib.Config["url"].(string)
	user, _ := lib.Config["username"].(string)
	auth, _ := lib.Config["auth"].(string)
	if user == "" && auth != AuthAPIKey {
		return nil, errors.New("an (Open)Subsonic library needs a username")
	}
	password, err := env.Secrets.Get(lib.ID, "password")
	if err != nil {
		return nil, fmt.Errorf("password for %s: %w", lib.Name, err)
	}
	state := filepath.Join(env.CacheDir, "servers", lib.ID+".json")
	client, err := NewClient(addr, user, password, auth, state, nil)
	if err != nil {
		return nil, err
	}
	return &Source{lib: lib, client: client, coverDir: filepath.Join(env.CacheDir, "covers", lib.ID)}, nil
}

func (s *Source) Library() source.Library { return s.lib }

func (s *Source) Close() error { return nil }

func (s *Source) Ping(ctx context.Context) (source.ServerInfo, error) {
	st, err := s.client.Probe(ctx)
	if err != nil {
		return source.ServerInfo{}, err
	}
	return source.ServerInfo{Type: st.Type, APIVersion: st.Version, ServerVersion: st.ServerVersion,
		OpenSubsonic: st.OpenSubsonic, Extensions: st.Extensions, Auth: st.Auth}, nil
}

// needs maps capabilities to the calls they rely on; a server that
// answered one of them with "not implemented" loses the capability.
var needs = map[string][]string{
	source.CapRatings:         {"setRating"},
	source.CapFavorites:       {"star", "unstar", "getStarred2"},
	source.CapRandomTracks:    {"getRandomSongs"},
	source.CapPlaylists:       {"getPlaylists", "getPlaylist"},
	source.CapPlaylistEdit:    {"getPlaylists", "createPlaylist", "updatePlaylist"},
	source.CapPlaylistReorder: {"getPlaylist", "createPlaylist"},
	source.CapPlaylistDelete:  {"deletePlaylist"},
	source.CapScrobble:        {"scrobble"},
}

func (s *Source) FilterCapabilities(caps []string) []string {
	st := s.client.state()
	out := caps[:0:0]
	for _, c := range caps {
		if !slices.ContainsFunc(needs[c], func(m string) bool { return slices.Contains(st.Unsupported, m) }) {
			out = append(out, c)
		}
	}
	// Plain Subsonic playlists have no art of their own; OpenSubsonic
	// servers (Navidrome, LMS, Ampache) send a coverArt per playlist.
	if st.OpenSubsonic && slices.Contains(out, source.CapPlaylists) {
		out = append(out, source.CapPlaylistCovers)
	}
	return out
}

func (s *Source) Notices() []source.Notice {
	var out []source.Notice
	if n, ok := source.PlainHTTPNotice(s.client.base); ok {
		out = append(out, n)
	}
	if s.client.state().Auth == AuthPassword {
		out = append(out, source.Notice{Code: source.NoticePasswordSignIn,
			Message: "Signs in with the password itself; this server doesn't accept token sign-in."})
	}
	return out
}

func (s *Source) track(x Song) source.Track {
	albumArtist := x.DisplayAlbumArtist
	if albumArtist == "" {
		albumArtist = x.Artist
	}
	return source.Track{
		Library: s.lib.ID, ID: string(x.ID), Title: source.Clean(x.Title), Artist: source.Clean(x.Artist),
		ArtistID: string(x.ArtistID), Album: source.Clean(x.Album), AlbumID: string(x.AlbumID),
		AlbumArtist: source.Clean(albumArtist), TrackNo: x.Track.int(), DiscNo: x.DiscNumber.int(), Year: x.Year.int(),
		Genre: source.Clean(x.Genre), Duration: max(float64(x.Duration), 0), CoverID: string(x.CoverArt),
		Favorite: x.Starred != "", Rating: clampRating(x.UserRating.int()), ReplayGain: x.ReplayGain.values(),
		PlayCount: x.PlayCount.int(), LastPlayed: unixTime(x.Played), Artists: artistRefs(x.Artists, x.Artist, string(x.ArtistID)),
	}
}

// artistRefs is OpenSubsonic's artist list, or the display name as one
// artist when the server sends none.
func artistRefs(list []idName, joined, id string) []source.ArtistRef {
	if len(list) == 0 {
		return source.SingleArtist(joined, id)
	}
	out := make([]source.ArtistRef, 0, len(list))
	for _, a := range list {
		if name := source.Clean(a.Name); name != "" {
			out = append(out, source.ArtistRef{ID: string(a.ID), Name: name})
		}
	}
	return out
}

func (s *Source) tracks(songs []Song) []source.Track {
	out := make([]source.Track, len(songs))
	for i, x := range songs {
		out[i] = s.track(x)
	}
	return out
}

func (s *Source) album(a AlbumEntry) source.Album {
	return source.Album{
		Library: s.lib.ID, ID: string(a.ID), Name: source.Clean(a.Name), Artist: source.Clean(a.Artist),
		ArtistID: string(a.ArtistID), Year: a.Year.int(), TrackCount: a.SongCount.int(), Duration: max(float64(a.Duration), 0),
		CoverID: string(a.CoverArt), Tracks: s.tracks(a.Song), Favorite: a.Starred != "",
		Rating: clampRating(a.UserRating.int()), Genre: source.Clean(a.Genre), Added: unixTime(a.Created),
		PlayCount: a.PlayCount.int(), LastPlayed: unixTime(a.Played), Artists: artistRefs(a.Artists, a.Artist, string(a.ArtistID)),
	}
}

func unixTime(iso string) int64 {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func (s *Source) albums(list []AlbumEntry) []source.Album {
	out := make([]source.Album, len(list))
	for i, a := range list {
		out[i] = s.album(a)
	}
	return out
}

func (s *Source) artists(list []ArtistEntry) []source.Artist {
	out := make([]source.Artist, len(list))
	for i, a := range list {
		out[i] = source.Artist{Library: s.lib.ID, ID: string(a.ID), Name: source.Clean(a.Name),
			AlbumCount: a.AlbumCount.int(), CoverID: string(a.CoverArt), Favorite: a.Starred != "",
			Rating: clampRating(a.UserRating.int())}
	}
	return out
}

func (s *Source) Track(ctx context.Context, id string) (source.Track, error) {
	x, err := s.client.Song(ctx, id)
	if err != nil {
		return source.Track{}, err
	}
	return s.track(x), nil
}

func (s *Source) StreamURL(ctx context.Context, id string) (string, error) {
	return s.client.StreamURL(ctx, id)
}

func (s *Source) Artists(ctx context.Context) ([]source.Artist, error) {
	list, err := s.client.Artists(ctx)
	return s.artists(list), err
}

func (s *Source) Albums(ctx context.Context, artistID string) ([]source.Album, error) {
	if artistID != "" {
		a, err := s.client.Artist(ctx, artistID)
		return s.albums(a.Album), err
	}
	var all []AlbumEntry
	for offset := 0; offset < maxAlbums; offset += 500 {
		page, err := s.client.AlbumList(ctx, 500, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < 500 {
			break
		}
	}
	return s.albums(all), nil
}

func (s *Source) Album(ctx context.Context, id string) (source.Album, error) {
	a, err := s.client.Album(ctx, id)
	return s.album(a), err
}

func (s *Source) Search(ctx context.Context, query string, limit int) (source.SearchResult, error) {
	r, err := s.client.Search(ctx, query, limit)
	if err != nil {
		return source.SearchResult{}, err
	}
	return source.SearchResult{Artists: s.artists(r.Artist), Albums: s.albums(r.Album), Tracks: s.tracks(r.Song)}, nil
}

func (s *Source) playlist(p PlaylistEntry) source.Playlist {
	return source.Playlist{
		Library: s.lib.ID, ID: string(p.ID), Name: source.Clean(p.Name), TrackCount: p.SongCount.int(),
		Duration: max(float64(p.Duration), 0), Owner: source.Clean(p.Owner), ReadOnly: bool(p.ReadOnly),
		CoverID: string(p.CoverArt), Tracks: s.tracks(p.Entry),
	}
}

func (s *Source) Playlists(ctx context.Context) ([]source.Playlist, error) {
	list, err := s.client.Playlists(ctx)
	out := make([]source.Playlist, len(list))
	for i, p := range list {
		out[i] = s.playlist(p)
	}
	return out, err
}

func (s *Source) Playlist(ctx context.Context, id string) (source.Playlist, error) {
	p, err := s.client.Playlist(ctx, id)
	return s.playlist(p), err
}

func (s *Source) CreatePlaylist(ctx context.Context, name string, trackIDs []string) (source.Playlist, error) {
	p, err := s.client.CreatePlaylist(ctx, "", name, trackIDs)
	return s.playlist(p), err
}

// MovePlaylistTrack rewrites the playlist in the new order: Subsonic has
// no move call, but createPlaylist with a playlistId replaces the songs.
func (s *Source) MovePlaylistTrack(ctx context.Context, id string, from, to int) error {
	p, err := s.client.Playlist(ctx, id)
	if err != nil {
		return err
	}
	ids := make([]string, len(p.Entry))
	for i, e := range p.Entry {
		ids[i] = string(e.ID)
	}
	if from < 0 || from >= len(ids) || to < 0 || to >= len(ids) {
		return fmt.Errorf("positions must be 0 to %d", len(ids)-1)
	}
	moved := ids[from]
	ids = slices.Insert(slices.Delete(ids, from, from+1), to, moved)
	_, err = s.client.CreatePlaylist(ctx, id, "", ids)
	return err
}

func (s *Source) NowPlaying(ctx context.Context, id string) error {
	return s.client.Scrobble(ctx, id, time.Time{}, false)
}

func (s *Source) Scrobble(ctx context.Context, id string, playedAt time.Time) error {
	return s.client.Scrobble(ctx, id, playedAt, true)
}

func (s *Source) DeletePlaylist(ctx context.Context, id string) error {
	_, err := s.client.call(ctx, "deletePlaylist", url.Values{"id": {id}}, nil)
	return err
}

func (s *Source) UpdatePlaylist(ctx context.Context, id string, edit source.PlaylistEdit) error {
	return s.client.UpdatePlaylist(ctx, id, edit.Name, edit.Add, edit.RemoveIndexes)
}

func (s *Source) Cover(ctx context.Context, coverID string, size int) (string, error) {
	return source.CachedCover(s.coverDir, coverID, size, func() ([]byte, string, error) {
		return s.client.CoverArt(ctx, coverID, size)
	})
}

func clampRating(r int) int { return min(max(r, 0), 5) }

func (s *Source) SetFavorite(ctx context.Context, kind, id string, favorite bool) error {
	if !source.ValidItemKind(kind) {
		return fmt.Errorf("%q is not track, album or artist", kind)
	}
	return s.client.Star(ctx, kind, id, favorite)
}

func (s *Source) SetRating(ctx context.Context, kind, id string, rating int) error {
	if !source.ValidItemKind(kind) || rating < 0 || rating > 5 {
		return fmt.Errorf("rating must be 0 to 5 on a track, album or artist")
	}
	return s.client.SetRating(ctx, id, rating)
}

func (s *Source) Favorites(ctx context.Context) (source.SearchResult, error) {
	r, err := s.client.Starred(ctx)
	if err != nil {
		return source.SearchResult{}, err
	}
	return source.SearchResult{Artists: s.artists(r.Artist), Albums: s.albums(r.Album), Tracks: s.tracks(r.Song)}, nil
}

func (s *Source) RandomTracks(ctx context.Context, n int) ([]source.Track, error) {
	songs, err := s.client.RandomSongs(ctx, n)
	return s.tracks(songs), err
}

// TrackCount uses getScanStatus, whose count Navidrome reports as the number
// of songs. Servers without it (or mid-scan) fall back to summing the song
// counts of the album list, and failing that to one getRandomSongs batch.
func (s *Source) TrackCount(ctx context.Context) (int, error) {
	if n, err := s.client.ScanCount(ctx); err == nil && n > 0 {
		return n, nil
	}
	total := 0
	for offset := 0; offset < maxAlbums; offset += 500 {
		page, err := s.client.AlbumList(ctx, 500, offset)
		if err != nil {
			break
		}
		for _, a := range page {
			total += max(a.SongCount.int(), 0)
		}
		if len(page) < 500 {
			break
		}
	}
	if total > 0 {
		return total, nil
	}
	return 500, nil
}

// TrackPage comes from search3 in the server's own order. Servers that
// answer an empty search with nothing are paged album by album instead.
func (s *Source) TrackPage(ctx context.Context, q source.TrackQuery) (source.TrackPage, error) {
	if !s.albumPaging.Load() {
		songs, err := s.client.Songs(ctx, max(q.Offset, 0), min(max(q.Limit, 1), 500))
		if err != nil {
			return source.TrackPage{}, err
		}
		if len(songs) > 0 || q.Offset > 0 {
			total := s.songTotal(ctx)
			return source.TrackPage{Total: max(total, q.Offset+len(songs)), Tracks: s.tracks(songs), Sort: source.SortNone}, nil
		}
		s.albumPaging.Store(true)
	}
	return s.pageByAlbums(ctx, q)
}

func (s *Source) songTotal(ctx context.Context) int {
	if n, err := s.client.ScanCount(ctx); err == nil && n > 0 {
		return n
	}
	total := 0
	_ = s.eachAlbum(ctx, func(a AlbumEntry) bool { total += max(a.SongCount.int(), 0); return true })
	return total
}

func (s *Source) eachAlbum(ctx context.Context, f func(AlbumEntry) bool) error {
	for offset := 0; offset < maxAlbums; offset += 500 {
		page, err := s.client.AlbumList(ctx, 500, offset)
		if err != nil {
			return err
		}
		for _, a := range page {
			if !f(a) {
				return nil
			}
		}
		if len(page) < 500 {
			return nil
		}
	}
	return nil
}

// pageByAlbums walks the album list (sorted by name) by song counts and
// fetches only the albums the page overlaps.
func (s *Source) pageByAlbums(ctx context.Context, q source.TrackQuery) (source.TrackPage, error) {
	limit := min(max(q.Limit, 1), 500)
	var ids []string
	total, firstSkip := 0, 0
	err := s.eachAlbum(ctx, func(a AlbumEntry) bool {
		n := max(a.SongCount.int(), 0)
		if total+n > q.Offset && total < q.Offset+limit {
			if len(ids) == 0 {
				firstSkip = q.Offset - total
			}
			ids = append(ids, string(a.ID))
		}
		total += n
		return true
	})
	if err != nil {
		return source.TrackPage{}, err
	}
	var tracks []source.Track
	for _, id := range ids {
		a, err := s.client.Album(ctx, id)
		if err != nil {
			return source.TrackPage{}, err
		}
		tracks = append(tracks, s.tracks(a.Song)...)
	}
	return source.TrackPage{Total: total, Tracks: source.Window(tracks, firstSkip, limit), Sort: source.SortAlbum}, nil
}

// recentAlbums is how many of the last played albums RecentPlays reads.
const recentAlbums = 20

// RecentPlays reads the songs of the last played albums: their counts
// changed since the last full read.
func (s *Source) RecentPlays(ctx context.Context) ([]source.Track, error) {
	list, err := s.client.AlbumListOf(ctx, "recent", recentAlbums, 0)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(list))
	for i, a := range list {
		ids[i] = string(a.ID)
	}
	albums := make([][]Song, len(ids))
	errs := make([]error, len(ids))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, id := range ids {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-sem; wg.Done() }()
			a, err := s.client.Album(ctx, id)
			albums[i], errs[i] = a.Song, err
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	var out []source.Track
	for _, songs := range albums {
		for _, x := range songs {
			if x.PlayCount.int() > 0 || x.Played != "" {
				out = append(out, s.track(x))
			}
		}
	}
	return out, nil
}

// ArtistInfo is getArtist plus getArtistInfo2 (biography, similar artists),
// which a plain Subsonic server may not have.
func (s *Source) ArtistInfo(ctx context.Context, id string) (source.ArtistInfo, error) {
	a, err := s.client.Artist(ctx, id)
	if err != nil {
		return source.ArtistInfo{}, err
	}
	info := source.ArtistInfo{Artist: s.artists([]ArtistEntry{a})[0], Similar: []source.Artist{}}
	info.AlbumCount = max(info.AlbumCount, len(a.Album))
	for _, al := range a.Album {
		info.TrackCount += max(al.SongCount.int(), 0)
	}
	extra, err := s.client.ArtistInfo(ctx, id)
	if err != nil {
		return info, nil
	}
	if info.Biography = source.PlainText(extra.Biography); info.Biography != "" {
		info.BiographySource = &source.Link{Name: s.lib.Name}
	}
	info.MusicBrainzID = source.Clean(extra.MusicBrainzID)
	for _, sim := range s.artists(extra.SimilarArtist) {
		if sim.ID != "" {
			info.Similar = append(info.Similar, sim)
		}
	}
	return info, nil
}
