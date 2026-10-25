// Package local is the source kind for folders on disk, indexed into the
// library database.
package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	"go.senan.xyz/taglib"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/loudness"
	"github.com/Conava/dux-player/core/internal/source"
	"github.com/Conava/dux-player/core/internal/tagwrite"
)

const Kind = "local"

var folderCovers = []string{"cover.jpg", "cover.png", "folder.jpg", "folder.png", "front.jpg", "front.png"}

type Source struct {
	lib      source.Library
	store    *library.Store
	coverDir string
	meter    *loudness.Meter
	measure  *loudness.Loop
	ctx      context.Context
	cancel   context.CancelFunc
	// gone holds the library folders that are missing right now.
	gone        atomic.Pointer[map[string]bool]
	watchNotice atomic.Value
}

var (
	_ source.Browser      = (*Source)(nil)
	_ source.Scanner      = (*Source)(nil)
	_ source.CoverArt     = (*Source)(nil)
	_ source.Loudness     = (*Source)(nil)
	_ source.Notices      = (*Source)(nil)
	_ source.RandomTracks = (*Source)(nil)
	_ source.TrackPager   = (*Source)(nil)
	_ source.Ratings      = (*Source)(nil)
	_ source.Favorites    = (*Source)(nil)
	_ tagwrite.Files      = (*Source)(nil)

	_ library.Availability = (*Source)(nil)
)

// Factory builds local sources on top of the shared index.
func Factory(store *library.Store) source.Factory {
	return func(lib source.Library, env source.Env) (source.Source, error) {
		if len(Folders(lib)) == 0 {
			return nil, errors.New("a local library needs at least one folder")
		}
		ctx, cancel := context.WithCancel(context.Background())
		s := &Source{lib: lib, store: store, coverDir: filepath.Join(env.CacheDir, "covers", lib.ID),
			meter: env.Meter, ctx: ctx, cancel: cancel}
		s.measure = &loudness.Loop{Meter: env.Meter, Pending: s.pendingMeasure, Save: s.saveMeasure}
		s.checkRoots(ctx)
		go s.watch()
		// Picks up measurements an earlier run didn't finish.
		s.startMeasuring()
		return s, nil
	}
}

// Folders reads the library's folder list from its config.
func Folders(lib source.Library) []string {
	raw, _ := lib.Config["folders"].([]any)
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok && filepath.IsAbs(s) {
			out = append(out, filepath.Clean(s))
		}
	}
	return out
}

func (s *Source) Library() source.Library { return s.lib }

func (s *Source) Close() error {
	s.cancel()
	return nil
}

func (s *Source) Track(ctx context.Context, id string) (source.Track, error) {
	t, err := s.store.Track(ctx, s.lib.ID, id)
	return t.Track, err
}

func (s *Source) StreamURL(ctx context.Context, id string) (string, error) {
	t, err := s.store.Track(ctx, s.lib.ID, id)
	if err != nil {
		return "", err
	}
	return t.Path, nil
}

func (s *Source) Artists(ctx context.Context) ([]source.Artist, error) {
	return s.store.Artists(ctx, s.lib.ID)
}

func (s *Source) Albums(ctx context.Context, artistID string) ([]source.Album, error) {
	return s.store.Albums(ctx, s.lib.ID, artistID)
}

func (s *Source) Album(ctx context.Context, id string) (source.Album, error) {
	return s.store.Album(ctx, s.lib.ID, id)
}

func (s *Source) TrackPage(ctx context.Context, q source.TrackQuery) (source.TrackPage, error) {
	return s.store.TrackPage(ctx, s.lib.ID, q)
}

func (s *Source) Search(ctx context.Context, query string, limit int) (source.SearchResult, error) {
	return s.store.Search(ctx, s.lib.ID, query, limit)
}

func (s *Source) Cover(ctx context.Context, coverID string, _ int) (string, error) {
	return s.store.Cover(ctx, s.lib.ID, coverID)
}

// Scan walks every folder and brings the index up to date. Files whose size
// and mtime are unchanged are not reread. A folder that looks gone keeps its
// tracks, marked missing: an unmounted drive must not empty the library.
func (s *Source) Scan(ctx context.Context, progress func(done, total int)) (source.ScanStats, error) {
	var stats source.ScanStats
	var files, gone []string
	for _, root := range Folders(s.lib) {
		if s.looksGone(ctx, root) {
			gone = append(gone, root)
			continue
		}
		found, err := Collect(ctx, root)
		if err != nil {
			return stats, err
		}
		files = append(files, found...)
	}
	return s.index(ctx, files, gone, nil, progress, nil)
}

// looksGone is true for a folder that doesn't exist, or that is empty while
// the index has tracks in it: an unmounted drive leaves its empty mount point.
func (s *Source) looksGone(ctx context.Context, dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return true
	}
	defer f.Close()
	if _, err := f.Readdirnames(1); err == nil {
		return false
	}
	return s.store.HasTracksUnder(ctx, s.lib.ID, dir)
}

// index stores files, keeps the tracks under gone as missing, and drops the
// rest of the rows it didn't see: in the whole library, or only below scope.
// index stores files and drops what's gone below scope; d, when set, gets
// the ids of the tracks it added, changed and removed.
func (s *Source) index(ctx context.Context, files, gone, scope []string, progress func(done, total int), d *library.Delta) (source.ScanStats, error) {
	var stats source.ScanStats
	stamps, err := s.store.FileStamps(ctx, s.lib.ID)
	if err != nil {
		return stats, err
	}
	batch, err := s.store.Begin(ctx, s.lib.ID, time.Now().UnixNano())
	if err != nil {
		return stats, err
	}
	for _, dir := range gone {
		if err := batch.KeepMissing(ctx, dir); err != nil {
			batch.Abort()
			return stats, err
		}
	}
	covered := make(map[string]bool)
	kept := make(map[string]bool, len(files))
	for i, path := range files {
		if err := ctx.Err(); err != nil {
			batch.Abort()
			return stats, err
		}
		if progress != nil && i%50 == 0 {
			progress(i, len(files))
		}
		stats.Seen++
		info, err := os.Stat(path)
		if err != nil {
			stats.Failed++
			continue
		}
		stamp := [2]int64{info.Size(), info.ModTime().UnixNano()}
		if old, ok := stamps[path]; ok && old == stamp {
			if err := batch.Touch(ctx, path); err != nil {
				batch.Abort()
				return stats, err
			}
			kept[path] = true
			continue
		}
		t, err := readTrack(path)
		if err != nil {
			stats.Failed++
			continue
		}
		t.Library = s.lib.ID
		t.ID = hashID(path)
		cover := ""
		if !covered[t.AlbumID] {
			cover = s.findCover(path, t.AlbumID)
			covered[t.AlbumID] = cover != ""
		}
		t.Size, t.MTime = stamp[0], stamp[1]
		if err := batch.PutTrack(ctx, t, cover); err != nil {
			batch.Abort()
			return stats, err
		}
		kept[path] = true
		stats.Updated++
		if d != nil {
			if _, ok := stamps[path]; ok {
				d.Changed = append(d.Changed, t.ID)
			} else {
				d.Added = append(d.Added, t.ID)
			}
		}
	}
	stats.Removed, err = batch.Finish(ctx, scope...)
	if d != nil && err == nil {
		for path := range stamps {
			if !kept[path] && underAny(scope, path) {
				d.Removed = append(d.Removed, hashID(path))
			}
		}
	}
	if progress != nil {
		progress(len(files), len(files))
	}
	if err == nil {
		s.startMeasuring()
	}
	return stats, err
}

// startMeasuring measures tracks in the background, one at a time, after
// the scan has stored their tags: their loudness when they have no
// ReplayGain tags, and their silence while skipping silence is on.
func (s *Source) startMeasuring() {
	s.measure.Start(s.ctx)
}

func (s *Source) pendingMeasure(ctx context.Context, silence bool) ([]loudness.Job, error) {
	pending, err := s.store.PendingMeasure(ctx, s.lib.ID, silence, 50)
	jobs := make([]loudness.Job, 0, len(pending))
	for _, t := range pending {
		jobs = append(jobs, loudness.Job{ID: t.ID, Path: t.Path})
	}
	return jobs, err
}

func (s *Source) saveMeasure(ctx context.Context, j loudness.Job, r loudness.Result, err error) error {
	if err != nil {
		log.Debugf("loudness of %s: %v", j.Path, err)
		return s.store.SetMeasured(ctx, s.lib.ID, j.ID, nil)
	}
	return s.store.SetMeasured(ctx, s.lib.ID, j.ID, &r)
}

func (s *Source) LoudnessGain(ctx context.Context, id string) (float64, bool) {
	return s.store.LoudnessGain(ctx, s.lib.ID, id)
}

func (s *Source) Silence(ctx context.Context, id string) loudness.Silence {
	return s.store.Silence(ctx, s.lib.ID, id)
}

func (s *Source) Notices() []source.Notice {
	var out []source.Notice
	if !s.meter.Available() {
		out = append(out, source.Notice{Code: source.NoticeNoFFmpeg,
			Message: "ffmpeg is not installed, so tracks without ReplayGain tags play without loudness correction"})
	}
	if n, ok := s.watchNotice.Load().(source.Notice); ok {
		out = append(out, n)
	}
	if gone := s.goneRoots(); len(gone) > 0 && len(gone) < len(Folders(s.lib)) {
		for _, root := range Folders(s.lib) {
			if gone[root] {
				out = append(out, source.Notice{Code: source.NoticeFolderMissing, Message: folderMissing(root).Message, Detail: root})
			}
		}
	}
	return out
}

func folderMissing(root string) library.Reason {
	return library.Reason{Code: library.CodeFolderMissing, Message: "Folder not found: " + root + ". It was removed or its drive isn't mounted."}
}

// Unavailable is set while every folder of the library is gone.
func (s *Source) Unavailable() library.Reason {
	roots := Folders(s.lib)
	if gone := s.goneRoots(); len(gone) == 0 || len(gone) < len(roots) {
		return library.Reason{}
	}
	if len(roots) == 1 {
		return folderMissing(roots[0])
	}
	return library.Reason{Code: library.CodeFoldersMissing, Message: "Folders not found. They were removed or their drive isn't mounted."}
}

func (s *Source) TrackUnavailable(ctx context.Context, id string) library.Reason {
	if s.store.Missing(ctx, s.lib.ID, id) {
		return library.TrackMissing
	}
	return library.Reason{}
}

func (s *Source) TrackCount(ctx context.Context) (int, error) {
	return s.store.TrackCount(ctx, s.lib.ID)
}

func (s *Source) RandomTracks(ctx context.Context, n int) ([]source.Track, error) {
	return s.store.RandomTracks(ctx, s.lib.ID, n)
}

// Favorites and ratings live in the index; tagwrite copies them into the
// files when the library turns that on.
func (s *Source) SetFavorite(ctx context.Context, kind, id string, favorite bool) error {
	return s.store.SetMark(ctx, s.lib.ID, kind, id, &favorite, nil)
}

func (s *Source) SetRating(ctx context.Context, kind, id string, rating int) error {
	return s.store.SetMark(ctx, s.lib.ID, kind, id, nil, &rating)
}

func (s *Source) Favorites(ctx context.Context) (source.SearchResult, error) {
	return s.store.Favorites(ctx, s.lib.ID)
}

// FilterCapabilities adds writeMarks: source can't name tagwrite.Files
// without an import cycle.
func (s *Source) FilterCapabilities(caps []string) []string {
	return append(caps, source.CapWriteMarks)
}

func (s *Source) TrackFile(ctx context.Context, id string) (tagwrite.Item, error) {
	t, err := s.store.Track(ctx, s.lib.ID, id)
	return tagwrite.Item{Path: t.Path, Mark: tagwrite.Mark{Favorite: t.Favorite, Rating: t.Rating}}, err
}

// MarkedFiles lists the files of the tracks with a favorite or a rating.
func (s *Source) MarkedFiles(ctx context.Context) ([]tagwrite.Item, error) {
	page, err := s.store.TrackPage(ctx, s.lib.ID, source.TrackQuery{Sort: source.SortAlbum})
	if err != nil {
		return nil, err
	}
	var out []tagwrite.Item
	for _, t := range page.Tracks {
		if !t.Favorite && t.Rating == 0 {
			continue
		}
		if path, ok := s.store.Path(ctx, s.lib.ID, t.ID); ok {
			// Only what is set here: a rating another player wrote stays.
			out = append(out, tagwrite.Item{Path: path, Mark: tagwrite.Mark{Favorite: t.Favorite, Rating: t.Rating},
				Fields: tagwrite.Fields{Favorite: t.Favorite, Rating: t.Rating > 0}})
		}
	}
	return out, nil
}

// findCover prefers a cover file next to the track, then embedded art copied
// into the cache.
func (s *Source) findCover(path, albumID string) string {
	dir := filepath.Dir(path)
	for _, name := range folderCovers {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p
		}
	}
	img, err := taglib.ReadImage(path)
	if err != nil || len(img) == 0 || len(img) > source.MaxCover {
		return ""
	}
	ext := ".jpg"
	if http.DetectContentType(img) == "image/png" {
		ext = ".png"
	}
	if err := os.MkdirAll(s.coverDir, 0o700); err != nil {
		return ""
	}
	out := filepath.Join(s.coverDir, albumID+ext)
	if err := os.WriteFile(out, img, 0o600); err != nil {
		return ""
	}
	return out
}

// Collect returns the audio files under root, sorted. Hidden files and
// directories are skipped (tag writes stage their copies as hidden files);
// symlinked directories are not followed.
func Collect(ctx context.Context, root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !IsAudio(root) {
			return nil, fmt.Errorf("%s is not an audio file", root)
		}
		return []string{root}, nil
	}
	var out []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && IsAudio(p) {
			out = append(out, p)
		}
		return nil
	})
	slices.Sort(out)
	return out, err
}

func underAny(dirs []string, path string) bool {
	for _, d := range dirs {
		if path == d || strings.HasPrefix(path, d+"/") {
			return true
		}
	}
	return false
}
