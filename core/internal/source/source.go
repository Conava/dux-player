// Package source defines what a library backend must (Source) and may do
// (optional interfaces checked by type assertion), so the player and IPC
// never switch on the kind.
package source

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Conava/dux-player/core/internal/loudness"
)

const maxTextLen = 512

// NewID is a random id for a library or a local playlist.
func NewID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// UnknownAlbum is the album of tracks without an album tag. It is a
// placeholder, not an album: such tracks don't belong together.
const UnknownAlbum = "Unknown Album"

// ErrNotFound is returned when a track, album or playlist id is unknown.
var ErrNotFound = errors.New("not found")

// Library is a configured instance of a source kind.
type Library struct {
	ID      string         `json:"id"`
	Kind    string         `json:"kind"`
	Name    string         `json:"name"`
	Enabled bool           `json:"enabled"`
	Config  map[string]any `json:"config"`
}

// Ref addresses a track across libraries. Paths and URLs alone never identify
// a track, so the queue and local playlists can mix libraries.
type Ref struct {
	Library string `json:"library"`
	ID      string `json:"id"`
}

type Track struct {
	Library  string `json:"library"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	ArtistID string `json:"artistId,omitempty"`
	// Artists are every track artist in order; Artist is the display string.
	Artists     []ArtistRef `json:"artists,omitempty"`
	Album       string      `json:"album"`
	AlbumID     string      `json:"albumId,omitempty"`
	AlbumArtist string      `json:"albumArtist,omitempty"`
	TrackNo     int         `json:"trackNo,omitempty"`
	DiscNo      int         `json:"discNo,omitempty"`
	Year        int         `json:"year,omitempty"`
	Genre       string      `json:"genre,omitempty"`
	Duration    float64     `json:"duration"`
	CoverID     string      `json:"coverId,omitempty"`
	Favorite    bool        `json:"favorite"`
	Rating      int         `json:"rating"`
	// ReplayGain the server reported, for streams whose own tags may be gone
	// (transcoding). Local files carry theirs in the file.
	ReplayGain *ReplayGain `json:"replayGain,omitempty"`
	// PlayCount and LastPlayed (unix seconds): a server's own numbers for
	// its tracks, this device's play history for the others.
	PlayCount  int   `json:"playCount,omitempty"`
	LastPlayed int64 `json:"lastPlayed,omitempty"`
	// Unavailable says why the track can't play right now, in plain
	// English; empty when it can. UnavailableCode is the same for the UI to
	// word (library.Code*).
	Unavailable     string `json:"unavailable,omitempty"`
	UnavailableCode string `json:"unavailableCode,omitempty"`
}

// ReplayGain values in dB; nil when unknown.
type ReplayGain struct {
	Track *float64 `json:"track,omitempty"`
	Album *float64 `json:"album,omitempty"`
}

func (t Track) Ref() Ref { return Ref{Library: t.Library, ID: t.ID} }

type Artist struct {
	Library    string `json:"library"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	AlbumCount int    `json:"albumCount"`
	CoverID    string `json:"coverId,omitempty"`
	Favorite   bool   `json:"favorite"`
	Rating     int    `json:"rating"`
}

type Album struct {
	Library  string `json:"library"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Artist   string `json:"artist"`
	ArtistID string `json:"artistId,omitempty"`
	// Artists are the album artists in order.
	Artists    []ArtistRef `json:"artists,omitempty"`
	Year       int         `json:"year,omitempty"`
	TrackCount int         `json:"trackCount"`
	Duration   float64     `json:"duration"`
	CoverID    string      `json:"coverId,omitempty"`
	Favorite   bool        `json:"favorite"`
	Rating     int         `json:"rating"`
	Genre      string      `json:"genre,omitempty"`
	PlayCount  int         `json:"playCount,omitempty"`
	LastPlayed int64       `json:"lastPlayed,omitempty"`
	// Added is when the album came into the library, in unix seconds.
	Added  int64   `json:"added,omitempty"`
	Tracks []Track `json:"tracks,omitempty"`
}

type Playlist struct {
	Library    string  `json:"library"`
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	TrackCount int     `json:"trackCount"`
	Duration   float64 `json:"duration"`
	Owner      string  `json:"owner,omitempty"`
	ReadOnly   bool    `json:"readOnly"`
	CoverID    string  `json:"coverId,omitempty"`
	Tracks     []Track `json:"tracks,omitempty"`
}

type SearchResult struct {
	Artists []Artist `json:"artists"`
	Albums  []Album  `json:"albums"`
	Tracks  []Track  `json:"tracks"`
}

// Source is what every library kind must do: describe a track and say what
// mpv should open to play it.
type Source interface {
	Library() Library
	Track(ctx context.Context, id string) (Track, error)
	// StreamURL may carry credentials; it goes to mpv and nowhere else.
	StreamURL(ctx context.Context, id string) (string, error)
	Close() error
}

// Browser lists and searches the library.
type Browser interface {
	Artists(ctx context.Context) ([]Artist, error)
	Albums(ctx context.Context, artistID string) ([]Album, error)
	Album(ctx context.Context, id string) (Album, error)
	Search(ctx context.Context, query string, limit int) (SearchResult, error)
}

// TrackPager pages through all tracks of the library, for track lists
// that would otherwise fetch every album. A source that can't sort as
// asked returns its own order and says so in TrackPage.Sort.
type TrackPager interface {
	TrackPage(ctx context.Context, q TrackQuery) (TrackPage, error)
}

// Track sort keys. The default is album artist, album, disc, track.
const (
	SortDefault = ""
	SortTitle   = "title"
	SortAlbum   = "album"
	SortYear    = "year"
	// SortAdded is newest first.
	SortAdded = "added"
	// SortNone is the source's own order, when it can't sort.
	SortNone = "none"
)

func ValidTrackSort(s string) bool {
	switch s {
	case SortDefault, SortTitle, SortAlbum, SortYear, SortAdded:
		return true
	}
	return false
}

type TrackQuery struct {
	Offset, Limit int
	Sort          string
}

type TrackPage struct {
	Total  int     `json:"total"`
	Tracks []Track `json:"tracks"`
	// Sort is the order the page is in: the one asked for, or SortNone.
	Sort string `json:"sort"`
}

// CoverArt fetches cover art for a CoverID into a local file and returns its
// path, so the UI never sees a token-bearing URL.
type CoverArt interface {
	Cover(ctx context.Context, coverID string, size int) (string, error)
}

// Scanner rebuilds a library's index from its storage.
type Scanner interface {
	Scan(ctx context.Context, progress func(done, total int)) (ScanStats, error)
}

type ScanStats struct {
	Seen    int `json:"seen"`
	Updated int `json:"updated"`
	Removed int `json:"removed"`
	Failed  int `json:"failed"`
}

// Playlists lists the playlists the library itself stores (server playlists).
type Playlists interface {
	Playlists(ctx context.Context) ([]Playlist, error)
	Playlist(ctx context.Context, id string) (Playlist, error)
}

// PlaylistEditor creates and edits server playlists.
type PlaylistEditor interface {
	CreatePlaylist(ctx context.Context, name string, trackIDs []string) (Playlist, error)
	UpdatePlaylist(ctx context.Context, id string, edit PlaylistEdit) error
}

type PlaylistEdit struct {
	Name *string
	Add  []string
	// RemoveIndexes are positions in the playlist before this edit.
	RemoveIndexes []int
}

// PlaylistDeleter deletes a server playlist.
type PlaylistDeleter interface {
	DeletePlaylist(ctx context.Context, id string) error
}

// PlaylistReorderer moves one track within a server playlist; from and to
// are positions.
type PlaylistReorderer interface {
	MovePlaylistTrack(ctx context.Context, id string, from, to int) error
}

// RandomTracks feeds a shuffle over the whole library. TrackCount weighs
// the library against others; RandomTracks returns up to n random tracks
// (possibly fewer, and later calls may repeat earlier tracks).
type RandomTracks interface {
	TrackCount(ctx context.Context) (int, error)
	RandomTracks(ctx context.Context, n int) ([]Track, error)
}

// Loudness reports a measured gain in dB for a track whose file has no
// ReplayGain tags, and the track's measured silence.
type Loudness interface {
	LoudnessGain(ctx context.Context, id string) (float64, bool)
	Silence(ctx context.Context, id string) loudness.Silence
}

// Detailer tells what a source knows about one track or album beyond the
// Track or Album itself, for a details view. kind is track or album.
type Detailer interface {
	Details(ctx context.Context, kind, id string) (Details, error)
}

// Details are all optional. Path is the local file, or the file's path on
// the server; never a stream URL.
type Details struct {
	// Format is the container or file type, upper case: FLAC, MP3, M4A.
	Format string `json:"format,omitempty"`
	// Codec is what a container holds (aac, alac, opus) or the MIME type.
	Codec       string            `json:"codec,omitempty"`
	Bitrate     int               `json:"bitrate,omitempty"` // kbit/s
	SampleRate  int               `json:"sampleRate,omitempty"`
	BitDepth    int               `json:"bitDepth,omitempty"`
	Channels    int               `json:"channels,omitempty"`
	Size        int64             `json:"size,omitempty"` // bytes; an album's is its tracks' sum
	Path        string            `json:"path,omitempty"`
	Server      string            `json:"server,omitempty"` // address without credentials
	Comment     string            `json:"comment,omitempty"`
	Genres      []string          `json:"genres,omitempty"`
	DiscCount   int               `json:"discCount,omitempty"`
	ReplayGain  *GainDetails      `json:"replayGain,omitempty"`
	MusicBrainz map[string]string `json:"musicBrainz,omitempty"`
	// Tags are the file's own tags (local files), by TagLib's key names.
	Tags map[string][]string `json:"tags,omitempty"`
}

// GainDetails are ReplayGain values in dB (peaks linear) as tagged, plus the
// gain measured on this device for a file without them.
type GainDetails struct {
	TrackGain *float64 `json:"trackGain,omitempty"`
	TrackPeak *float64 `json:"trackPeak,omitempty"`
	AlbumGain *float64 `json:"albumGain,omitempty"`
	AlbumPeak *float64 `json:"albumPeak,omitempty"`
	Measured  *float64 `json:"measured,omitempty"`
}

// Notices are soft problems a library wants the user to see, such as an
// optional tool that is missing.
type Notices interface {
	Notices() []Notice
}

// Notice is a stable Code the UI words, Message in plain English for logs,
// the CLI and older UIs, and Detail (a folder) for the codes that name one.
type Notice struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

// Codes of Notice; folderMissing shares the library reason's code.
const (
	NoticeNoFFmpeg       = "noFFmpeg"
	NoticeWatchFailed    = "watchFailed"
	NoticeTooManyFolders = "tooManyFolders"
	NoticeFolderMissing  = "folderMissing"
	NoticePlainHTTP      = "plainHttp"
	NoticePasswordSignIn = "passwordSignIn"
)

// Kinds of items that take favorites and ratings.
const (
	ItemTrack  = "track"
	ItemAlbum  = "album"
	ItemArtist = "artist"
)

func ValidItemKind(kind string) bool {
	return kind == ItemTrack || kind == ItemAlbum || kind == ItemArtist
}

// Favorites sets favorites (heart) on tracks, albums and artists and lists
// them.
type Favorites interface {
	SetFavorite(ctx context.Context, kind, id string, favorite bool) error
	Favorites(ctx context.Context) (SearchResult, error)
}

// Ratings sets 1–5 star ratings on tracks, albums and artists; 0 clears.
type Ratings interface {
	SetRating(ctx context.Context, kind, id string, rating int) error
}

// PlayStats returns every track the server counts plays for, with its
// PlayCount and LastPlayed, most played first (the server sorts).
type PlayStats interface {
	PlayStats(ctx context.Context) ([]Track, error)
}

// RecentPlays returns the tracks of the server's most recently played
// albums with their play counts: the cheap refresh between full reads of a
// server that can't sort tracks by play count (it pages them with
// TrackPager instead).
type RecentPlays interface {
	RecentPlays(ctx context.Context) ([]Track, error)
}

// Scrobbler reports plays to the library's server, which counts them and
// may forward them (Navidrome to Last.fm/ListenBrainz). A track of such a
// library is never scrobbled anywhere else, so nothing counts twice.
type Scrobbler interface {
	NowPlaying(ctx context.Context, id string) error
	Scrobble(ctx context.Context, id string, playedAt time.Time) error
}

// CapabilityFilter adjusts the capabilities the interfaces suggest to what
// the source knows about its server: it drops what the server turned out
// not to support and adds flags no interface expresses (playlistCovers).
type CapabilityFilter interface {
	FilterCapabilities(caps []string) []string
}

// Pinger checks that a remote library is reachable and the credentials work.
type Pinger interface {
	Ping(ctx context.Context) (ServerInfo, error)
}

type ServerInfo struct {
	Type          string   `json:"type,omitempty"`
	Version       string   `json:"version,omitempty"`
	APIVersion    string   `json:"apiVersion,omitempty"`
	OpenSubsonic  bool     `json:"openSubsonic"`
	Extensions    []string `json:"extensions,omitempty"`
	ServerVersion string   `json:"serverVersion,omitempty"`
	// Auth is how the client signs in (token, password, apiKey, …).
	Auth string `json:"auth,omitempty"`
}

// Capability flags reported per library. The UI enables a feature only
// when its flag is listed. Names are part of the IPC protocol: never rename
// one, only add.
const (
	CapBrowse          = "browse"          // Browser: artists, albums, search
	CapCover           = "cover"           // CoverArt
	CapScan            = "scan"            // Scanner
	CapPing            = "ping"            // Pinger
	CapPlaylists       = "playlists"       // Playlists: read server playlists
	CapPlaylistEdit    = "playlistEdit"    // PlaylistEditor: create, rename, add, remove
	CapPlaylistReorder = "playlistReorder" // PlaylistReorderer
	CapPlaylistDelete  = "playlistDelete"  // PlaylistDeleter
	CapPlaylistCovers  = "playlistCovers"  // server playlists carry their own coverId
	CapRandomTracks    = "randomTracks"    // RandomTracks: track count + random batches for shuffle
	CapFavorites       = "favorites"       // Favorites: set and list hearts
	CapRatings         = "ratings"         // Ratings: 1–5 stars
	CapScrobble        = "scrobble"        // Scrobbler: plays reported to the server
	CapDetails         = "details"         // Detailer: file, format and tag facts for browse.details
	CapWriteMarks      = "writeMarks"      // ratings and favorites can be written to the files (tagwrite.Files)
)

// Capabilities names what src can do: the optional interfaces it
// implements, adjusted by its CapabilityFilter.
func Capabilities(src Source) []string {
	checks := []struct {
		name string
		ok   bool
	}{
		{CapBrowse, is[Browser](src)},
		{CapCover, is[CoverArt](src)},
		{CapScan, is[Scanner](src)},
		{CapPing, is[Pinger](src)},
		{CapPlaylists, is[Playlists](src)},
		{CapPlaylistEdit, is[PlaylistEditor](src)},
		{CapPlaylistReorder, is[PlaylistReorderer](src)},
		{CapPlaylistDelete, is[PlaylistDeleter](src)},
		{CapRandomTracks, is[RandomTracks](src)},
		{CapFavorites, is[Favorites](src)},
		{CapRatings, is[Ratings](src)},
		{CapScrobble, is[Scrobbler](src)},
		{CapDetails, is[Detailer](src)},
	}
	caps := []string{}
	for _, c := range checks {
		if c.ok {
			caps = append(caps, c.name)
		}
	}
	if f, ok := src.(CapabilityFilter); ok {
		caps = f.FilterCapabilities(caps)
	}
	return caps
}

func is[T any](src Source) bool {
	_, ok := src.(T)
	return ok
}

// Secrets stores credentials per library.
type Secrets interface {
	Get(libraryID, key string) (string, error)
	Set(libraryID, key, value string) error
	Delete(libraryID string) error
}

// Env is what a factory may use to build a source.
type Env struct {
	Secrets  Secrets
	CacheDir string
	// Meter measures loudness of local files; it may be unavailable.
	Meter *loudness.Meter
	// Version is the app version, for servers that want to know the client.
	Version string
}

type Factory func(lib Library, env Env) (Source, error)

// Registry maps a kind to its factory. A new kind is one entry.
type Registry map[string]Factory

func (r Registry) Build(lib Library, env Env) (Source, error) {
	f, ok := r[lib.Kind]
	if !ok {
		return nil, fmt.Errorf("unknown library kind %q", lib.Kind)
	}
	return f(lib, env)
}

func (r Registry) Kinds() []string {
	kinds := make([]string, 0, len(r))
	for k := range r {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}

// Clean makes text from tags or a server safe to store and show: no control
// characters, bounded length.
func Clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > maxTextLen {
		s = strings.ToValidUTF8(s[:maxTextLen], "")
	}
	return s
}
