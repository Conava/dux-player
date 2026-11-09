package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/source"
)

const (
	Kind = "jellyfin"

	pageSize = 500
	// detailsPage keeps a page with the slower fields inside CallTimeout.
	detailsPage = 200
	// maxItems caps how far a full listing pages.
	maxItems = 200_000
	ticks    = 1e7
	// stampSkew covers a server clock that differs from ours.
	stampSkew = time.Hour
)

// Jellyfin has favorites but no star ratings, so Source has no SetRating
// and the library reports no ratings capability.
type Source struct {
	lib      source.Library
	client   *Client
	coverDir string
	// quickOff is set when the last read of the server's Quick Connect
	// setting said it is off.
	quickOff atomic.Bool
}

var (
	_ source.Browser           = (*Source)(nil)
	_ source.TrackPager        = (*Source)(nil)
	_ source.CoverArt          = (*Source)(nil)
	_ source.Pinger            = (*Source)(nil)
	_ source.Playlists         = (*Source)(nil)
	_ source.PlaylistEditor    = (*Source)(nil)
	_ source.PlaylistReorderer = (*Source)(nil)
	_ source.Favorites         = (*Source)(nil)
	_ source.RandomTracks      = (*Source)(nil)
	_ source.SimilarTracks     = (*Source)(nil)
	_ source.Scrobbler         = (*Source)(nil)
	_ source.CapabilityFilter  = (*Source)(nil)
	_ source.Notices           = (*Source)(nil)
	_ source.PlayStats         = (*Source)(nil)
	_ source.ArtistDetailer    = (*Source)(nil)
	_ source.CatalogSync       = (*Source)(nil)
	_ source.RecentPlays       = (*Source)(nil)
)

// Factory builds the source without network I/O. The token from the last
// sign-in is kept in the keyring next to the password, so a restart
// doesn't open a new session on the server.
func Factory(lib source.Library, env source.Env) (source.Source, error) {
	addr, _ := lib.Config["url"].(string)
	user, _ := lib.Config["username"].(string)
	auth, _ := lib.Config["auth"].(string)
	quick := auth == authQuickConnect
	if user == "" && !quick {
		return nil, errors.New("a jellyfin library needs a username")
	}
	var password string
	if !quick {
		var err error
		if password, err = env.Secrets.Get(lib.ID, "password"); err != nil {
			return nil, fmt.Errorf("password for %s: %w", lib.Name, err)
		}
	}
	version := env.Version
	if version == "" {
		version = "dev"
	}
	c, err := NewClient(addr, user, password, "dplayer-"+lib.ID, version, nil)
	if err != nil {
		return nil, err
	}
	c.canRenew = !quick
	// A stored session is used only for the address and user it was made
	// for, so editing either signs in again.
	owner := func(user string) string { return addr + "\x00" + user }
	// An unreadable keyring is no missing session; stop at the first such
	// error, since each read of a locked one can raise an unlock prompt.
	unreadable := func(err error) bool { return quick && err != nil && !errors.Is(err, source.ErrNoSecret) }
	token, terr := env.Secrets.Get(lib.ID, "token")
	if unreadable(terr) {
		return nil, fmt.Errorf("sign-in for %s: %w", lib.Name, terr)
	}
	uid, uerr := env.Secrets.Get(lib.ID, "userId")
	if unreadable(uerr) {
		return nil, fmt.Errorf("sign-in for %s: %w", lib.Name, uerr)
	}
	made, ferr := env.Secrets.Get(lib.ID, "sessionFor")
	if unreadable(ferr) {
		return nil, fmt.Errorf("sign-in for %s: %w", lib.Name, ferr)
	}
	if terr == nil && uerr == nil && ferr == nil && made == owner(user) {
		c.setSession(token, uid)
	}
	c.onToken = func(token, uid, user string) {
		if err := errors.Join(env.Secrets.Set(lib.ID, "token", token), env.Secrets.Set(lib.ID, "userId", uid),
			env.Secrets.Set(lib.ID, "sessionFor", owner(user))); err != nil {
			log.Warnf("library %s: keep sign-in: %v", lib.Name, err)
		}
	}
	return &Source{lib: lib, client: c, coverDir: filepath.Join(env.CacheDir, "covers", lib.ID)}, nil
}

// SignInRefused implements source.SignInReporter.
func (s *Source) SignInRefused() bool { return s.client.refused.Load() }

func (s *Source) Library() source.Library { return s.lib }

func (s *Source) Close() error { return nil }

func (s *Source) Notices() []source.Notice {
	if n, ok := source.PlainHTTPNotice(s.client.base); ok {
		return []source.Notice{n}
	}
	return nil
}

// FilterCapabilities adds playlistCovers (Jellyfin playlists have their
// own primary image) and drops quickConnect when the server has it off.
func (s *Source) FilterCapabilities(caps []string) []string {
	if s.quickOff.Load() {
		caps = slices.DeleteFunc(slices.Clone(caps), func(c string) bool { return c == source.CapQuickConnect })
	}
	return append(caps, source.CapPlaylistCovers)
}

type userData struct {
	IsFavorite     bool   `json:"IsFavorite"`
	PlayCount      int    `json:"PlayCount"`
	LastPlayedDate string `json:"LastPlayedDate"`
}

func (u userData) lastPlayed() int64 {
	t, err := time.Parse(time.RFC3339Nano, u.LastPlayedDate)
	if err != nil {
		return 0
	}
	return t.Unix()
}

type nameID struct {
	Name string `json:"Name"`
	ID   string `json:"Id"`
}

type item struct {
	ID                   string            `json:"Id"`
	Name                 string            `json:"Name"`
	Type                 string            `json:"Type"`
	Album                string            `json:"Album"`
	AlbumID              string            `json:"AlbumId"`
	AlbumArtist          string            `json:"AlbumArtist"`
	AlbumArtists         []nameID          `json:"AlbumArtists"`
	Artists              []string          `json:"Artists"`
	ArtistItems          []nameID          `json:"ArtistItems"`
	AlbumPrimaryImageTag string            `json:"AlbumPrimaryImageTag"`
	ImageTags            map[string]string `json:"ImageTags"`
	IndexNumber          int               `json:"IndexNumber"`
	ParentIndexNumber    int               `json:"ParentIndexNumber"`
	ProductionYear       int               `json:"ProductionYear"`
	Genres               []string          `json:"Genres"`
	RunTimeTicks         int64             `json:"RunTimeTicks"`
	ChildCount           int               `json:"ChildCount"`
	AlbumCount           int               `json:"AlbumCount"`
	NormalizationGain    *float64          `json:"NormalizationGain"`
	PlaylistItemID       string            `json:"PlaylistItemId"`
	DateCreated          string            `json:"DateCreated"`
	Container            string            `json:"Container"`
	UserData             userData          `json:"UserData"`
	Overview             string            `json:"Overview"`
	ProviderIDs          map[string]string `json:"ProviderIds"`
	MediaSources         []mediaSource     `json:"MediaSources"`
	People               []person          `json:"People"`
}

type person struct {
	Name string `json:"Name"`
	Type string `json:"Type"`
}

func (it item) composer() string {
	var names []string
	for _, p := range it.People {
		if name := source.Clean(p.Name); p.Type == "Composer" && name != "" {
			names = append(names, name)
		}
	}
	return source.Clean(strings.Join(names, ", "))
}

func (it item) bitrate() int {
	if len(it.MediaSources) == 0 {
		return 0
	}
	return it.MediaSources[0].kbits()
}

type itemsReply struct {
	Items            []item `json:"Items"`
	TotalRecordCount int    `json:"TotalRecordCount"`
}

const itemFields = "Genres,ChildCount,ParentId,DateCreated"

func (it item) coverID() string {
	if it.ImageTags["Primary"] != "" {
		return it.ID
	}
	if it.AlbumPrimaryImageTag != "" {
		return it.AlbumID
	}
	return ""
}

func (it item) added() int64 {
	t, err := time.Parse(time.RFC3339Nano, it.DateCreated)
	if err != nil {
		return 0
	}
	// Jellyfin's empty date is 0001-01-01, which overflows as nanoseconds.
	return max(t.Unix(), 0)
}

func duration(t int64) float64 { return max(float64(t)/ticks, 0) }

func (s *Source) track(it item) source.Track {
	t := source.Track{
		Library: s.lib.ID, ID: it.ID, Title: source.Clean(it.Name), Album: source.Clean(it.Album), AlbumID: it.AlbumID,
		AlbumArtist: source.Clean(it.AlbumArtist), TrackNo: it.IndexNumber, DiscNo: it.ParentIndexNumber,
		Year: it.ProductionYear, Duration: duration(it.RunTimeTicks), CoverID: it.coverID(),
		Favorite: it.UserData.IsFavorite, PlayCount: it.UserData.PlayCount, LastPlayed: it.UserData.lastPlayed(),
		Format: strings.ToUpper(source.Clean(it.Container)), Added: it.added(), Composer: it.composer(), Bitrate: it.bitrate(),
	}
	if len(it.Artists) > 0 {
		t.Artist = source.Clean(strings.Join(it.Artists, ", "))
	}
	if len(it.ArtistItems) > 0 {
		t.ArtistID = it.ArtistItems[0].ID
	}
	t.Artists = artistRefs(it.ArtistItems, it.Artists, t.ArtistID)
	if t.AlbumArtist == "" {
		t.AlbumArtist = t.Artist
	}
	if len(it.Genres) > 0 {
		t.Genre = source.Clean(it.Genres[0])
	}
	// Jellyfin's loudness normalization gain (to -18 LUFS) plays the role
	// of a ReplayGain track gain.
	if g := it.NormalizationGain; g != nil && *g != 0 {
		t.ReplayGain = &source.ReplayGain{Track: g}
	}
	return t
}

func (s *Source) tracks(items []item) []source.Track {
	out := make([]source.Track, 0, len(items))
	for _, it := range items {
		out = append(out, s.track(it))
	}
	return out
}

func (s *Source) album(it item) source.Album {
	a := source.Album{
		Library: s.lib.ID, ID: it.ID, Name: source.Clean(it.Name), Artist: source.Clean(it.AlbumArtist),
		Year: it.ProductionYear, TrackCount: it.ChildCount, Duration: duration(it.RunTimeTicks), CoverID: it.coverID(),
		Favorite: it.UserData.IsFavorite, PlayCount: it.UserData.PlayCount, LastPlayed: it.UserData.lastPlayed(),
	}
	if len(it.Genres) > 0 {
		a.Genre = source.Clean(it.Genres[0])
	}
	a.Added = it.added()
	switch {
	case len(it.AlbumArtists) > 0:
		a.ArtistID = it.AlbumArtists[0].ID
	case len(it.ArtistItems) > 0:
		a.ArtistID = it.ArtistItems[0].ID
	}
	if a.Artist == "" {
		a.Artist = source.Clean(strings.Join(it.Artists, ", "))
	}
	a.Artists = artistRefs(it.AlbumArtists, []string{a.Artist}, a.ArtistID)
	return a
}

// artistRefs prefers the items with ids; names alone come without.
func artistRefs(items []nameID, names []string, id string) []source.ArtistRef {
	var out []source.ArtistRef
	for _, a := range items {
		if name := source.Clean(a.Name); name != "" {
			out = append(out, source.ArtistRef{ID: a.ID, Name: name})
		}
	}
	if len(out) > 0 {
		return out
	}
	if len(names) == 1 {
		return source.SingleArtist(names[0], id)
	}
	for _, n := range names {
		if name := source.Clean(n); name != "" {
			out = append(out, source.ArtistRef{Name: name})
		}
	}
	return out
}

func (s *Source) albums(items []item) []source.Album {
	out := make([]source.Album, 0, len(items))
	for _, it := range items {
		out = append(out, s.album(it))
	}
	return out
}

func (s *Source) artist(it item) source.Artist {
	return source.Artist{Library: s.lib.ID, ID: it.ID, Name: source.Clean(it.Name),
		AlbumCount: max(it.AlbumCount, it.ChildCount), CoverID: it.coverID(), Favorite: it.UserData.IsFavorite}
}

func (s *Source) artists(items []item) []source.Artist {
	out := make([]source.Artist, 0, len(items))
	for _, it := range items {
		out = append(out, s.artist(it))
	}
	return out
}

// items queries /Users/{user}/Items, paging until limit (0: everything
// up to maxItems).
func (s *Source) items(ctx context.Context, path string, q url.Values, limit int) ([]item, error) {
	if q.Get("Fields") == "" {
		q.Set("Fields", itemFields)
	}
	want := limit
	if want <= 0 {
		want = maxItems
	}
	var out []item
	for len(out) < want {
		page := min(pageSize, want-len(out))
		q.Set("StartIndex", strconv.Itoa(len(out)))
		q.Set("Limit", strconv.Itoa(page))
		var r itemsReply
		if err := s.client.get(ctx, path, q, &r); err != nil {
			return nil, err
		}
		out = append(out, r.Items...)
		if len(r.Items) < page {
			break
		}
	}
	return out, nil
}

func musicQuery(types string) url.Values {
	return url.Values{"IncludeItemTypes": {types}, "Recursive": {"true"}}
}

func (s *Source) Ping(ctx context.Context) (source.ServerInfo, error) {
	var info struct {
		ProductName string `json:"ProductName"`
		Version     string `json:"Version"`
	}
	if err := s.client.do(ctx, http.MethodGet, "/System/Info/Public", nil, nil, &info, ""); err != nil {
		return source.ServerInfo{}, err
	}
	// A failed read keeps what was known; the sign-in check below must not
	// hide this answer, since a refused library signs in again with it.
	if on, err := s.QuickConnectEnabled(ctx); err == nil {
		s.quickOff.Store(!on)
	}
	// Checks the sign-in too, looking again after an earlier refusal.
	s.client.refused.Store(false)
	if err := s.client.get(ctx, "/Users/{user}", nil, nil); err != nil {
		return source.ServerInfo{}, err
	}
	return source.ServerInfo{Type: info.ProductName, ServerVersion: info.Version, Auth: "token"}, nil
}

func (s *Source) Track(ctx context.Context, id string) (source.Track, error) {
	var it item
	if err := s.client.get(ctx, "/Users/{user}/Items/"+url.PathEscape(id), url.Values{"Fields": {itemFields}}, &it); err != nil {
		return source.Track{}, err
	}
	if it.Type != "Audio" {
		return source.Track{}, source.ErrNotFound
	}
	return s.track(it), nil
}

func (s *Source) StreamURL(ctx context.Context, id string) (string, error) {
	return s.client.StreamURL(ctx, id)
}

func (s *Source) StreamURLFor(ctx context.Context, id string, containers []string) (string, error) {
	return s.client.StreamURLFor(ctx, id, containers)
}

func (s *Source) Artists(ctx context.Context) ([]source.Artist, error) {
	list, err := s.items(ctx, "/Artists/AlbumArtists", url.Values{"UserId": {userParam}, "SortBy": {"SortName"}}, 0)
	return s.artists(list), err
}

func (s *Source) Albums(ctx context.Context, artistID string) ([]source.Album, error) {
	q := musicQuery("MusicAlbum")
	q.Set("SortBy", "SortName")
	if artistID != "" {
		q.Set("AlbumArtistIds", artistID)
		q.Set("SortBy", "ProductionYear,SortName")
	}
	list, err := s.items(ctx, "/Users/{user}/Items", q, 0)
	return s.albums(list), err
}

func (s *Source) Album(ctx context.Context, id string) (source.Album, error) {
	var it item
	if err := s.client.get(ctx, "/Users/{user}/Items/"+url.PathEscape(id), url.Values{"Fields": {itemFields}}, &it); err != nil {
		return source.Album{}, err
	}
	if it.Type != "MusicAlbum" {
		return source.Album{}, source.ErrNotFound
	}
	q := musicQuery("Audio")
	q.Set("ParentId", id)
	q.Set("SortBy", "ParentIndexNumber,IndexNumber,SortName")
	list, err := s.items(ctx, "/Users/{user}/Items", q, 0)
	a := s.album(it)
	a.Tracks = s.tracks(list)
	return a, err
}

func (s *Source) Search(ctx context.Context, query string, limit int) (source.SearchResult, error) {
	res := source.SearchResult{Artists: []source.Artist{}, Albums: []source.Album{}, Tracks: []source.Track{}}
	q := url.Values{"SearchTerm": {query}, "UserId": {userParam}}
	artists, err := s.items(ctx, "/Artists", q, limit)
	if err != nil {
		return res, err
	}
	res.Artists = s.artists(artists)
	for _, kind := range []string{"MusicAlbum", "Audio"} {
		q := musicQuery(kind)
		q.Set("SearchTerm", query)
		list, err := s.items(ctx, "/Users/{user}/Items", q, limit)
		if err != nil {
			return res, err
		}
		if kind == "Audio" {
			res.Tracks = s.tracks(list)
		} else {
			res.Albums = s.albums(list)
		}
	}
	return res, nil
}

func (s *Source) Cover(ctx context.Context, coverID string, size int) (string, error) {
	return source.CachedCover(s.coverDir, coverID, size, func() ([]byte, string, error) {
		return s.client.Image(ctx, coverID, size)
	})
}

func (s *Source) playlist(it item) source.Playlist {
	return source.Playlist{Library: s.lib.ID, ID: it.ID, Name: source.Clean(it.Name), TrackCount: it.ChildCount,
		Duration: duration(it.RunTimeTicks), CoverID: it.coverID()}
}

func (s *Source) Playlists(ctx context.Context) ([]source.Playlist, error) {
	q := musicQuery("Playlist")
	q.Set("MediaTypes", "Audio")
	q.Set("SortBy", "SortName")
	list, err := s.items(ctx, "/Users/{user}/Items", q, 0)
	out := make([]source.Playlist, 0, len(list))
	for _, it := range list {
		out = append(out, s.playlist(it))
	}
	return out, err
}

func (s *Source) entries(ctx context.Context, id string) ([]item, error) {
	return s.items(ctx, "/Playlists/"+url.PathEscape(id)+"/Items", url.Values{"UserId": {userParam}}, 0)
}

func (s *Source) Playlist(ctx context.Context, id string) (source.Playlist, error) {
	var it item
	if err := s.client.get(ctx, "/Users/{user}/Items/"+url.PathEscape(id), url.Values{"Fields": {itemFields}}, &it); err != nil {
		return source.Playlist{}, err
	}
	if it.Type != "Playlist" {
		return source.Playlist{}, source.ErrNotFound
	}
	list, err := s.entries(ctx, id)
	p := s.playlist(it)
	p.Tracks = s.tracks(list)
	p.TrackCount = len(p.Tracks)
	return p, err
}

func (s *Source) CreatePlaylist(ctx context.Context, name string, trackIDs []string) (source.Playlist, error) {
	var res struct {
		ID string `json:"Id"`
	}
	uid, err := s.client.UserID(ctx)
	if err != nil {
		return source.Playlist{}, err
	}
	body := map[string]any{"Name": name, "Ids": append([]string{}, trackIDs...), "UserId": uid, "MediaType": "Audio"}
	if err := s.client.call(ctx, http.MethodPost, "/Playlists", nil, body, &res); err != nil {
		return source.Playlist{}, err
	}
	if res.ID == "" {
		return source.Playlist{}, errors.New("Playlists: server returned no playlist")
	}
	return source.Playlist{Library: s.lib.ID, ID: res.ID, Name: source.Clean(name), TrackCount: len(trackIDs)}, nil
}

// UpdatePlaylist removes by entry id (Jellyfin's playlist item id), so the
// positions are looked up first, before anything is added.
func (s *Source) UpdatePlaylist(ctx context.Context, id string, edit source.PlaylistEdit) error {
	path := "/Playlists/" + url.PathEscape(id)
	if len(edit.RemoveIndexes) > 0 {
		list, err := s.entries(ctx, id)
		if err != nil {
			return err
		}
		var entryIDs []string
		for _, i := range edit.RemoveIndexes {
			if i < 0 || i >= len(list) {
				return fmt.Errorf("position %d is not in the playlist", i)
			}
			entryIDs = append(entryIDs, entryID(list[i]))
		}
		if err := s.client.call(ctx, http.MethodDelete, path+"/Items", url.Values{"EntryIds": {strings.Join(entryIDs, ",")}}, nil, nil); err != nil {
			return err
		}
	}
	if len(edit.Add) > 0 {
		q := url.Values{"Ids": {strings.Join(edit.Add, ",")}, "UserId": {userParam}}
		if err := s.client.call(ctx, http.MethodPost, path+"/Items", q, nil, nil); err != nil {
			return err
		}
	}
	if edit.Name != nil {
		// Jellyfin 10.9+; Emby renames through the item editor instead.
		if err := s.client.call(ctx, http.MethodPost, path, nil, map[string]string{"Name": *edit.Name}, nil); err != nil {
			return err
		}
	}
	return nil
}

func entryID(it item) string {
	if it.PlaylistItemID != "" {
		return it.PlaylistItemID
	}
	return it.ID
}

// DeletePlaylist deletes the playlist item; the user needs delete rights.
func (s *Source) DeletePlaylist(ctx context.Context, id string) error {
	return s.client.call(ctx, http.MethodDelete, "/Items/"+url.PathEscape(id), nil, nil, nil)
}

func (s *Source) MovePlaylistTrack(ctx context.Context, id string, from, to int) error {
	list, err := s.entries(ctx, id)
	if err != nil {
		return err
	}
	if from < 0 || from >= len(list) || to < 0 || to >= len(list) {
		return fmt.Errorf("positions must be 0 to %d", len(list)-1)
	}
	path := fmt.Sprintf("/Playlists/%s/Items/%s/Move/%d", url.PathEscape(id), url.PathEscape(entryID(list[from])), to)
	return s.client.call(ctx, http.MethodPost, path, nil, nil, nil)
}

func (s *Source) SetFavorite(ctx context.Context, kind, id string, favorite bool) error {
	if !source.ValidItemKind(kind) {
		return fmt.Errorf("%q is not track, album or artist", kind)
	}
	method := http.MethodPost
	if !favorite {
		method = http.MethodDelete
	}
	return s.client.call(ctx, method, "/Users/{user}/FavoriteItems/"+url.PathEscape(id), nil, nil, nil)
}

func (s *Source) Favorites(ctx context.Context) (source.SearchResult, error) {
	res := source.SearchResult{Artists: []source.Artist{}, Albums: []source.Album{}, Tracks: []source.Track{}}
	q := musicQuery("MusicArtist,MusicAlbum,Audio")
	q.Set("Filters", "IsFavorite")
	q.Set("SortBy", "SortName")
	list, err := s.items(ctx, "/Users/{user}/Items", q, 0)
	for _, it := range list {
		switch it.Type {
		case "MusicArtist":
			res.Artists = append(res.Artists, s.artist(it))
		case "MusicAlbum":
			res.Albums = append(res.Albums, s.album(it))
		case "Audio":
			res.Tracks = append(res.Tracks, s.track(it))
		}
	}
	return res, err
}

func (s *Source) TrackCount(ctx context.Context) (int, error) {
	return s.total(ctx, musicQuery("Audio"))
}

func (s *Source) RandomTracks(ctx context.Context, n int) ([]source.Track, error) {
	q := musicQuery("Audio")
	q.Set("SortBy", "Random")
	list, err := s.items(ctx, "/Users/{user}/Items", q, min(max(n, 1), pageSize))
	return s.tracks(list), err
}

// SimilarTracks is the seed's Instant Mix.
func (s *Source) SimilarTracks(ctx context.Context, seed source.Track, n int) ([]source.Track, error) {
	if seed.ID == "" {
		return nil, nil
	}
	n = min(max(n, 1), source.MaxSimilarTracks)
	q := url.Values{"UserId": {userParam}, "Limit": {strconv.Itoa(n)}, "Fields": {itemFields}}
	var r itemsReply
	if err := s.client.get(ctx, "/Items/"+url.PathEscape(seed.ID)+"/InstantMix", q, &r); err != nil {
		return nil, err
	}
	if len(r.Items) > n {
		r.Items = r.Items[:n]
	}
	return s.tracks(r.Items), nil
}

// NowPlaying and Scrobble are Jellyfin's play reporting: a playing session
// for the dashboard, then the item marked played, which counts the play.
func (s *Source) NowPlaying(ctx context.Context, id string) error {
	return s.client.call(ctx, http.MethodPost, "/Sessions/Playing", nil,
		map[string]any{"ItemId": id, "CanSeek": true, "PlayMethod": "DirectPlay"}, nil)
}

func (s *Source) Scrobble(ctx context.Context, id string, playedAt time.Time) error {
	if err := s.client.call(ctx, http.MethodPost, "/Sessions/Playing/Stopped", nil, map[string]any{"ItemId": id}, nil); err != nil {
		log.Warnf("jellyfin: end play session: %v", err)
	}
	q := url.Values{"DatePlayed": {playedAt.UTC().Format(time.RFC3339)}}
	return s.client.call(ctx, http.MethodPost, "/Users/{user}/PlayedItems/"+url.PathEscape(id), q, nil, nil)
}

var trackSorts = map[string]string{
	source.SortDefault: "AlbumArtist,Album,ParentIndexNumber,IndexNumber",
	source.SortTitle:   "SortName",
	source.SortAlbum:   "Album,ParentIndexNumber,IndexNumber",
	source.SortYear:    "ProductionYear,AlbumArtist,Album,ParentIndexNumber,IndexNumber",
	source.SortAdded:   "DateCreated",
}

func (s *Source) TrackPage(ctx context.Context, q source.TrackQuery) (source.TrackPage, error) {
	by, ok := trackSorts[q.Sort]
	if !ok {
		return source.TrackPage{}, fmt.Errorf("unknown sort %q", q.Sort)
	}
	qv := musicQuery("Audio")
	qv.Set("SortBy", by)
	if q.Sort == source.SortAdded {
		qv.Set("SortOrder", "Descending")
	}
	qv.Set("Fields", itemFields)
	limit := pageSize
	if q.Details {
		qv.Set("Fields", itemFields+",MediaSources,People")
		limit = detailsPage
	}
	qv.Set("StartIndex", strconv.Itoa(max(q.Offset, 0)))
	qv.Set("Limit", strconv.Itoa(min(max(q.Limit, 1), limit)))
	var r itemsReply
	if err := s.client.get(ctx, "/Users/{user}/Items", qv, &r); err != nil {
		return source.TrackPage{}, err
	}
	return source.TrackPage{Total: r.TotalRecordCount, Tracks: s.tracks(r.Items), Sort: q.Sort}, nil
}

// PlayStats is every played track, most played first (up to maxItems).
func (s *Source) PlayStats(ctx context.Context) ([]source.Track, error) {
	q := musicQuery("Audio")
	q.Set("Filters", "IsPlayed")
	q.Set("SortBy", "PlayCount,DatePlayed")
	q.Set("SortOrder", "Descending")
	list, err := s.items(ctx, "/Users/{user}/Items", q, 0)
	return s.tracks(list), err
}

// recentPlays is how many of the last played tracks RecentPlays reads.
const recentPlays = 200

// RecentPlays is the last played tracks in one request: PlayStats reads all
// of them.
func (s *Source) RecentPlays(ctx context.Context) ([]source.Track, error) {
	q := musicQuery("Audio")
	q.Set("Filters", "IsPlayed")
	q.Set("SortBy", "DatePlayed")
	q.Set("SortOrder", "Descending")
	list, err := s.items(ctx, "/Users/{user}/Items", q, recentPlays)
	return s.tracks(list), err
}

// total counts the items of a query without reading any.
func (s *Source) total(ctx context.Context, q url.Values) (int, error) {
	q.Set("Limit", "0")
	var r itemsReply
	if err := s.client.get(ctx, "/Users/{user}/Items", q, &r); err != nil {
		return 0, err
	}
	return r.TotalRecordCount, nil
}

// CatalogChanged asks for items saved since the stamp and compares the track
// count, which is what shows deletions: nothing lists those. The stamp is our
// clock and that count.
func (s *Source) CatalogChanged(ctx context.Context, stamp string) (bool, string, error) {
	now := time.Now().UTC()
	count, err := s.TrackCount(ctx)
	if err != nil {
		return false, "", err
	}
	next := now.Format(time.RFC3339) + "|" + strconv.Itoa(count)
	at, oldCount, ok := strings.Cut(stamp, "|")
	since, err := time.Parse(time.RFC3339, at)
	if !ok || err != nil || oldCount != strconv.Itoa(count) {
		return true, next, nil
	}
	q := musicQuery("MusicArtist,MusicAlbum,Audio")
	q.Set("minDateLastSaved", since.Add(-stampSkew).Format(time.RFC3339))
	saved, err := s.total(ctx, q)
	if err != nil {
		return false, "", err
	}
	return saved > 0, next, nil
}

// CatalogBusy is false: Jellyfin keeps listing what it has during a scan.
func (s *Source) CatalogBusy(context.Context) bool { return false }

// ArtistInfo is the artist item (overview, MusicBrainz id), their albums for
// the counts and Jellyfin's similar artists.
func (s *Source) ArtistInfo(ctx context.Context, id string) (source.ArtistInfo, error) {
	var it item
	q := url.Values{"Fields": {itemFields + ",Overview,ProviderIds"}}
	if err := s.client.get(ctx, "/Users/{user}/Items/"+url.PathEscape(id), q, &it); err != nil {
		return source.ArtistInfo{}, err
	}
	if it.Type != "MusicArtist" {
		return source.ArtistInfo{}, source.ErrNotFound
	}
	info := source.ArtistInfo{Artist: s.artist(it), MusicBrainzID: source.Clean(it.ProviderIDs["MusicBrainzArtist"]),
		Similar: []source.Artist{}}
	if info.Biography = source.PlainText(it.Overview); info.Biography != "" {
		info.BiographySource = &source.Link{Name: s.lib.Name}
	}
	albums, err := s.Albums(ctx, id)
	if err != nil {
		return info, err
	}
	info.AlbumCount = len(albums)
	for _, a := range albums {
		info.TrackCount += a.TrackCount
	}
	similar, err := s.items(ctx, "/Artists/"+url.PathEscape(id)+"/Similar", url.Values{"UserId": {userParam}}, 12)
	if err == nil {
		info.Similar = append(info.Similar, s.artists(similar)...)
	}
	return info, nil
}
