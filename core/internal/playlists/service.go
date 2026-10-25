package playlists

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Conava/dux-player/core/internal/atomicfile"
	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
	"github.com/Conava/dux-player/core/internal/source/local"
)

const (
	Topic = "localPlaylists"

	// An import reports at most this many entries it couldn't place.
	maxUnresolved = 50
)

// Service is local playlists as the daemon offers them: tracks resolved
// through the libraries, edits announced on the event bus.
type Service struct {
	Store     *Store
	Libraries *library.Manager
	Player    *player.Player
	Publish   func(topic string, data any)
}

func (s *Service) changed(id string) {
	s.Publish(Topic, map[string]any{"type": "changed", "id": id})
}

// Get returns a playlist whose tracks say whether they can play now. Tracks
// of indexed libraries are read fresh from the index; the others are the
// copies stored when they were added.
func (s *Service) Get(ctx context.Context, id string) (Playlist, error) {
	p, err := s.Store.Get(ctx, id)
	if err != nil {
		return p, err
	}
	idx := s.Libraries.Store()
	for i, t := range p.Tracks {
		if fresh, err := idx.Track(ctx, t.Library, t.ID); err == nil {
			p.Tracks[i] = fresh.Track
		}
		if p.Tracks[i].Unavailable == "" {
			r := s.Libraries.Unavailable(ctx, t.Ref())
			p.Tracks[i].Unavailable, p.Tracks[i].UnavailableCode = r.Message, r.Code
		}
	}
	return p, nil
}

func (s *Service) Create(ctx context.Context, name string, refs []source.Ref) (Playlist, error) {
	tracks, err := s.Libraries.Tracks(ctx, refs)
	if err != nil {
		return Playlist{}, err
	}
	p, err := s.Store.Create(ctx, name, tracks)
	if err == nil {
		s.changed(p.ID)
	}
	return p, err
}

func (s *Service) Add(ctx context.Context, id string, refs []source.Ref, at int) error {
	tracks, err := s.Libraries.Tracks(ctx, refs)
	if err != nil {
		return err
	}
	return s.done(id, s.Store.Add(ctx, id, tracks, at))
}

func (s *Service) Rename(ctx context.Context, id, name string) error {
	return s.done(id, s.Store.Rename(ctx, id, name))
}

func (s *Service) SetCover(ctx context.Context, id string, c Cover) error {
	return s.done(id, s.Store.SetCover(ctx, id, c))
}

func (s *Service) Remove(ctx context.Context, id string, positions []int) error {
	return s.done(id, s.Store.Remove(ctx, id, positions))
}

func (s *Service) Move(ctx context.Context, id string, from, to int) error {
	return s.done(id, s.Store.Move(ctx, id, from, to))
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.done(id, s.Store.Delete(ctx, id))
}

func (s *Service) done(id string, err error) error {
	if err == nil {
		s.changed(id)
	}
	return err
}

// Duplicate copies a local playlist, or a server playlist when library is
// set, into a new local one.
func (s *Service) Duplicate(ctx context.Context, lib, id, name string) (Playlist, error) {
	var src source.Playlist
	if lib == "" {
		p, err := s.Store.Get(ctx, id)
		if err != nil {
			return Playlist{}, err
		}
		src = source.Playlist{Name: p.Name, Tracks: p.Tracks}
	} else {
		lsrc, err := s.Libraries.Get(lib)
		if err != nil {
			return Playlist{}, err
		}
		pls, ok := lsrc.(source.Playlists)
		if !ok {
			return Playlist{}, fmt.Errorf("library %s has no playlists", lib)
		}
		if src, err = pls.Playlist(ctx, id); err != nil {
			return Playlist{}, err
		}
	}
	if name == "" {
		name = src.Name
	}
	p, err := s.Store.Create(ctx, name, src.Tracks)
	if err == nil {
		s.changed(p.ID)
	}
	return p, err
}

// Play replaces the queue with the playlist from index; tracks that can't
// play are passed over by the queue.
func (s *Service) Play(ctx context.Context, id string, index int, shuffle bool) error {
	p, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	origin := player.Origin{Kind: "playlist", ID: p.ID, Name: p.Name}
	if shuffle {
		return s.Player.PlayShuffled(ctx, p.Tracks, origin)
	}
	return s.Player.Play(ctx, p.Tracks, index, origin)
}

// ExportResult says what an export wrote and left out.
type ExportResult struct {
	Path    string `json:"path"`
	Written int    `json:"written"`
	// Skipped tracks have no file a playlist file could point at (server
	// tracks: their stream address carries credentials).
	Skipped int `json:"skipped"`
}

// Export writes the playlist as .m3u8 or .pls (by format, else by the file
// extension) to an absolute path.
func (s *Service) Export(ctx context.Context, id, path, format string) (ExportResult, error) {
	if !filepath.IsAbs(path) {
		return ExportResult{}, errors.New("the export path must be absolute")
	}
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".m3u" && ext != ".m3u8" && ext != ".pls" {
		return ExportResult{}, errors.New("the export file must end in .m3u8, .m3u or .pls")
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return ExportResult{}, fmt.Errorf("%s exists and is not a playlist file", path)
	}
	if format == "" {
		format = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	}
	p, err := s.Store.Get(ctx, id)
	if err != nil {
		return ExportResult{}, err
	}
	res := ExportResult{Path: path}
	var entries []Entry
	for _, t := range p.Tracks {
		file := s.file(ctx, t.Ref())
		if file == "" || strings.ContainsAny(file, "\r\n") {
			res.Skipped++
			continue
		}
		entries = append(entries, Entry{Path: file, Title: title(t), Duration: t.Duration})
	}
	var data []byte
	switch format {
	case "m3u8", "m3u":
		data = FormatM3U(entries)
	case "pls":
		data = FormatPLS(entries)
	default:
		return ExportResult{}, errors.New("format must be m3u8 or pls")
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return ExportResult{}, err
	}
	// A user document, not app state.
	if err := atomicfile.WriteFile(path, data, 0o644); err != nil {
		return ExportResult{}, err
	}
	res.Written = len(entries)
	return res, nil
}

func title(t source.Track) string {
	if t.Artist != "" {
		return t.Artist + " - " + t.Title
	}
	return t.Title
}

// file is the local file of a track, if it has one.
func (s *Service) file(ctx context.Context, ref source.Ref) string {
	if ref.Library == local.FilesLibrary {
		return ref.ID
	}
	p, _ := s.Libraries.Store().Path(ctx, ref.Library, ref.ID)
	return p
}

// readPlaylistFile reads a regular file of bounded size. Non-blocking, so a
// FIFO fails instead of hanging the request.
func readPlaylistFile(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a playlist file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err == nil && len(data) > maxFileBytes {
		err = fmt.Errorf("the playlist file is larger than %d MB", maxFileBytes>>20)
	}
	return data, err
}

// ImportResult is the new playlist and the entries that matched no track.
type ImportResult struct {
	Playlist   Playlist `json:"playlist"`
	Unresolved []string `json:"unresolved"`
	// Missed counts all unresolved entries; Unresolved lists the first ones.
	Missed int `json:"missed"`
}

// Import reads an .m3u/.m3u8/.pls file into a new playlist. Each path
// becomes the indexed track of that file, or a loose file when no library
// has it; URLs and files that don't exist are reported.
func (s *Service) Import(ctx context.Context, path, name string) (ImportResult, error) {
	if !filepath.IsAbs(path) {
		return ImportResult{}, errors.New("the playlist path must be absolute")
	}
	data, err := readPlaylistFile(path)
	if err != nil {
		return ImportResult{}, err
	}
	entries, err := Parse(data, filepath.Dir(path))
	if err != nil {
		return ImportResult{}, err
	}
	res := ImportResult{Unresolved: []string{}}
	var tracks []source.Track
	for _, e := range entries {
		t, ok := s.resolve(ctx, e)
		if !ok {
			res.Missed++
			if len(res.Unresolved) < maxUnresolved {
				res.Unresolved = append(res.Unresolved, source.Clean(e.String()))
			}
			continue
		}
		tracks = append(tracks, t)
	}
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if res.Playlist, err = s.Store.Create(ctx, name, tracks); err != nil {
		return res, err
	}
	s.changed(res.Playlist.ID)
	return res, nil
}

func (s *Service) resolve(ctx context.Context, e Entry) (source.Track, bool) {
	if e.Path == "" {
		return source.Track{}, false
	}
	ref, ok := s.Libraries.Store().TrackByPath(ctx, e.Path)
	if !ok {
		ref = source.Ref{Library: local.FilesLibrary, ID: e.Path}
	}
	tracks, err := s.Libraries.Tracks(ctx, []source.Ref{ref})
	if err != nil || len(tracks) == 0 {
		return source.Track{}, false
	}
	return tracks[0], true
}
