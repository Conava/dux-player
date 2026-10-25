package subsonic

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/Conava/dux-player/core/internal/source"
)

type Song struct {
	ID                 str      `json:"id"`
	Title              string   `json:"title"`
	Album              string   `json:"album"`
	AlbumID            str      `json:"albumId"`
	Artist             string   `json:"artist"`
	ArtistID           str      `json:"artistId"`
	Track              num      `json:"track"`
	DiscNumber         num      `json:"discNumber"`
	Year               num      `json:"year"`
	Genre              string   `json:"genre"`
	Duration           num      `json:"duration"`
	CoverArt           str      `json:"coverArt"`
	DisplayAlbumArtist string   `json:"displayAlbumArtist"`
	Starred            string   `json:"starred"`
	UserRating         num      `json:"userRating"`
	ReplayGain         songGain `json:"replayGain"`
	PlayCount          num      `json:"playCount"`
	Played             string   `json:"played"`
	// For browse.details.
	Size          num              `json:"size"`
	Suffix        string           `json:"suffix"`
	ContentType   string           `json:"contentType"`
	BitRate       num              `json:"bitRate"`
	SamplingRate  num              `json:"samplingRate"`
	BitDepth      num              `json:"bitDepth"`
	ChannelCount  num              `json:"channelCount"`
	Path          string           `json:"path"`
	Created       string           `json:"created"`
	Comment       string           `json:"comment"`
	MusicBrainzID string           `json:"musicBrainzId"`
	Genres        list[namedEntry] `json:"genres"`
	// OpenSubsonic artist lists.
	Artists      list[idName] `json:"artists"`
	AlbumArtists list[idName] `json:"albumArtists"`
}

type idName struct {
	ID   str    `json:"id"`
	Name string `json:"name"`
}

// namedEntry is OpenSubsonic's {name} item (genres, labels).
type namedEntry struct {
	Name string `json:"name"`
}

type ArtistEntry struct {
	ID         str    `json:"id"`
	Name       string `json:"name"`
	AlbumCount num    `json:"albumCount"`
	CoverArt   str    `json:"coverArt"`
	Starred    string `json:"starred"`
	UserRating num    `json:"userRating"`
	// getArtist only.
	Album list[AlbumEntry] `json:"album"`
}

type AlbumEntry struct {
	ID         str        `json:"id"`
	Name       string     `json:"name"`
	Artist     string     `json:"artist"`
	ArtistID   str        `json:"artistId"`
	Year       num        `json:"year"`
	SongCount  num        `json:"songCount"`
	Duration   num        `json:"duration"`
	CoverArt   str        `json:"coverArt"`
	Starred    string     `json:"starred"`
	UserRating num        `json:"userRating"`
	Genre      string     `json:"genre"`
	Created    string     `json:"created"`
	PlayCount  num        `json:"playCount"`
	Played     string     `json:"played"`
	Song       list[Song] `json:"song"`
	// For browse.details.
	MusicBrainzID string           `json:"musicBrainzId"`
	Genres        list[namedEntry] `json:"genres"`
	Artists       list[idName]     `json:"artists"`
}

type PlaylistEntry struct {
	ID        str        `json:"id"`
	Name      string     `json:"name"`
	SongCount num        `json:"songCount"`
	Duration  num        `json:"duration"`
	Owner     string     `json:"owner"`
	ReadOnly  boolean    `json:"readonly"`
	CoverArt  str        `json:"coverArt"`
	Entry     list[Song] `json:"entry"`
}

type Extension struct {
	Name string `json:"name"`
}

func (c *Client) Artists(ctx context.Context) ([]ArtistEntry, error) {
	var out struct {
		Artists struct {
			Index list[struct {
				Artist list[ArtistEntry] `json:"artist"`
			}] `json:"index"`
		} `json:"artists"`
	}
	if _, err := c.call(ctx, "getArtists", nil, &out); err != nil {
		return nil, err
	}
	var artists []ArtistEntry
	for _, idx := range out.Artists.Index {
		artists = append(artists, idx.Artist...)
	}
	return artists, nil
}

// Artist is getArtist: the artist with their albums.
func (c *Client) Artist(ctx context.Context, id string) (ArtistEntry, error) {
	var out struct {
		Artist ArtistEntry `json:"artist"`
	}
	_, err := c.call(ctx, "getArtist", url.Values{"id": {id}}, &out)
	return out.Artist, err
}

type ArtistInfo struct {
	Biography     string            `json:"biography"`
	MusicBrainzID string            `json:"musicBrainzId"`
	SimilarArtist list[ArtistEntry] `json:"similarArtist"`
}

func (c *Client) ArtistInfo(ctx context.Context, id string) (ArtistInfo, error) {
	var out struct {
		Info ArtistInfo `json:"artistInfo2"`
	}
	_, err := c.call(ctx, "getArtistInfo2", url.Values{"id": {id}, "count": {"12"}}, &out)
	return out.Info, err
}

// AlbumList pages through all albums sorted by name.
func (c *Client) AlbumList(ctx context.Context, size, offset int) ([]AlbumEntry, error) {
	return c.AlbumListOf(ctx, "alphabeticalByName", size, offset)
}

// AlbumListOf is getAlbumList2 of a type: alphabeticalByName, frequent
// (most played first), recent (last played first), …
func (c *Client) AlbumListOf(ctx context.Context, typ string, size, offset int) ([]AlbumEntry, error) {
	var out struct {
		List struct {
			Album list[AlbumEntry] `json:"album"`
		} `json:"albumList2"`
	}
	params := url.Values{"type": {typ}, "size": {strconv.Itoa(size)}, "offset": {strconv.Itoa(offset)}}
	_, err := c.call(ctx, "getAlbumList2", params, &out)
	return out.List.Album, err
}

func (c *Client) Album(ctx context.Context, id string) (AlbumEntry, error) {
	var out struct {
		Album AlbumEntry `json:"album"`
	}
	_, err := c.call(ctx, "getAlbum", url.Values{"id": {id}}, &out)
	return out.Album, err
}

func (c *Client) Song(ctx context.Context, id string) (Song, error) {
	var out struct {
		Song Song `json:"song"`
	}
	_, err := c.call(ctx, "getSong", url.Values{"id": {id}}, &out)
	return out.Song, err
}

type SearchResult struct {
	Artist list[ArtistEntry] `json:"artist"`
	Album  list[AlbumEntry]  `json:"album"`
	Song   list[Song]        `json:"song"`
}

func (c *Client) Search(ctx context.Context, query string, limit int) (SearchResult, error) {
	var out struct {
		Result SearchResult `json:"searchResult3"`
	}
	n := strconv.Itoa(limit)
	params := url.Values{"query": {query}, "artistCount": {n}, "albumCount": {n}, "songCount": {n}}
	_, err := c.call(ctx, "search3", params, &out)
	return out.Result, err
}

// Songs pages through every song: search3 with an empty query, which
// Navidrome and most OpenSubsonic servers answer with all songs.
func (c *Client) Songs(ctx context.Context, offset, count int) ([]Song, error) {
	var out struct {
		Result SearchResult `json:"searchResult3"`
	}
	params := url.Values{"query": {""}, "artistCount": {"0"}, "albumCount": {"0"},
		"songCount": {strconv.Itoa(count)}, "songOffset": {strconv.Itoa(offset)}}
	_, err := c.call(ctx, "search3", params, &out)
	return out.Result.Song, err
}

func (c *Client) Playlists(ctx context.Context) ([]PlaylistEntry, error) {
	var out struct {
		Playlists struct {
			Playlist list[PlaylistEntry] `json:"playlist"`
		} `json:"playlists"`
	}
	_, err := c.call(ctx, "getPlaylists", nil, &out)
	return out.Playlists.Playlist, err
}

func (c *Client) Playlist(ctx context.Context, id string) (PlaylistEntry, error) {
	var out struct {
		Playlist PlaylistEntry `json:"playlist"`
	}
	_, err := c.call(ctx, "getPlaylist", url.Values{"id": {id}}, &out)
	return out.Playlist, err
}

// CreatePlaylist needs a 1.14+ server to get the playlist back; older ones
// reply with an empty envelope. With playlistID set it replaces that
// playlist's songs, which is the only way to reorder one.
func (c *Client) CreatePlaylist(ctx context.Context, playlistID, name string, songIDs []string) (PlaylistEntry, error) {
	var out struct {
		Playlist PlaylistEntry `json:"playlist"`
	}
	params := url.Values{"songId": songIDs}
	if playlistID != "" {
		params.Set("playlistId", playlistID)
	} else {
		params.Set("name", name)
	}
	_, err := c.call(ctx, "createPlaylist", params, &out)
	if err == nil && playlistID == "" && out.Playlist.ID == "" {
		return out.Playlist, fmt.Errorf("createPlaylist: server returned no playlist")
	}
	return out.Playlist, err
}

// Scrobble reports a play: submission false is "now playing".
func (c *Client) Scrobble(ctx context.Context, id string, at time.Time, submission bool) error {
	params := url.Values{"id": {id}, "submission": {strconv.FormatBool(submission)}}
	if submission {
		params.Set("time", strconv.FormatInt(at.UnixMilli(), 10))
	}
	_, err := c.call(ctx, "scrobble", params, nil)
	return err
}

func (c *Client) UpdatePlaylist(ctx context.Context, id string, name *string, add []string, removeIdx []int) error {
	params := url.Values{"playlistId": {id}}
	if name != nil {
		params.Set("name", *name)
	}
	for _, s := range add {
		params.Add("songIdToAdd", s)
	}
	for _, i := range removeIdx {
		params.Add("songIndexToRemove", strconv.Itoa(i))
	}
	_, err := c.call(ctx, "updatePlaylist", params, nil)
	return err
}

// starParams puts the id under the parameter its kind needs: id for songs,
// albumId and artistId for the others (ID3 browsing).
func starParams(kind, id string) url.Values {
	switch kind {
	case "album":
		return url.Values{"albumId": {id}}
	case "artist":
		return url.Values{"artistId": {id}}
	}
	return url.Values{"id": {id}}
}

func (c *Client) Star(ctx context.Context, kind, id string, star bool) error {
	method := "star"
	if !star {
		method = "unstar"
	}
	_, err := c.call(ctx, method, starParams(kind, id), nil)
	return err
}

// SetRating takes any item id; 0 removes the rating.
func (c *Client) SetRating(ctx context.Context, id string, rating int) error {
	_, err := c.call(ctx, "setRating", url.Values{"id": {id}, "rating": {strconv.Itoa(rating)}}, nil)
	return err
}

func (c *Client) Starred(ctx context.Context) (SearchResult, error) {
	var out struct {
		Result SearchResult `json:"starred2"`
	}
	_, err := c.call(ctx, "getStarred2", nil, &out)
	return out.Result, err
}

// RandomSongs returns up to size (max 500) random songs.
func (c *Client) RandomSongs(ctx context.Context, size int) ([]Song, error) {
	var out struct {
		Random struct {
			Song list[Song] `json:"song"`
		} `json:"randomSongs"`
	}
	_, err := c.call(ctx, "getRandomSongs", url.Values{"size": {strconv.Itoa(min(size, 500))}}, &out)
	return out.Random.Song, err
}

// songGain is OpenSubsonic's replayGain on songs.
type songGain struct {
	TrackGain *float64 `json:"trackGain"`
	AlbumGain *float64 `json:"albumGain"`
	TrackPeak *float64 `json:"trackPeak"`
	AlbumPeak *float64 `json:"albumPeak"`
}

// values drops zero gains: Navidrome sends 0 (with peak 1) for files without
// ReplayGain tags, and 0 dB changes nothing anyway.
func (g songGain) values() *source.ReplayGain {
	nonZero := func(v *float64) *float64 {
		if v == nil || *v == 0 {
			return nil
		}
		return v
	}
	rg := source.ReplayGain{Track: nonZero(g.TrackGain), Album: nonZero(g.AlbumGain)}
	if rg.Track == nil && rg.Album == nil {
		return nil
	}
	return &rg
}

// ScanCount is getScanStatus' count; 0 while the server is scanning.
func (c *Client) ScanCount(ctx context.Context) (int, error) {
	var out struct {
		Status struct {
			Scanning boolean `json:"scanning"`
			Count    num     `json:"count"`
		} `json:"scanStatus"`
	}
	if _, err := c.call(ctx, "getScanStatus", nil, &out); err != nil {
		return 0, err
	}
	if out.Status.Scanning {
		return 0, nil
	}
	return out.Status.Count.int(), nil
}
