package artists

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/atomicfile"
	"github.com/Conava/dux-player/core/internal/source"
)

// TopicPlaylists says a server library's playlists were indexed anew and
// browse.artistPlaylists may answer differently: {library}.
const TopicPlaylists = "artistPlaylists"

const (
	// maxPlaylists bounds the server playlists indexed per library.
	maxPlaylists = 200
	// listEvery is how old a library's playlist list may get before a
	// visit reads it again, in the background.
	listEvery = 5 * time.Minute
	// playlistTTL refetches a playlist whose size stayed the same, for
	// edits that keep it.
	playlistTTL = 24 * time.Hour
	// indexWait is how long a visit waits for a running refresh before
	// answering with what is indexed.
	indexWait = time.Second
	// failRetry is how long a failed refresh waits before a visit tries
	// again.
	failRetry      = time.Minute
	refreshTimeout = 2 * time.Minute
	fetchers       = 2
)

// PlaylistHit is a playlist holding Matches tracks of the artist; Library
// is empty for a local playlist.
type PlaylistHit struct {
	Library    string `json:"library"`
	ID         string `json:"id"`
	Name       string `json:"name"`
	TrackCount int    `json:"trackCount"`
	CoverID    string `json:"coverId,omitempty"`
	Matches    int    `json:"matches"`
}

// plEntry is one server playlist by its artists: IDs counts tracks per
// artist id, Names (lowercased) only the tracks that carry no artist id, so
// no track counts twice.
type plEntry struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	TrackCount int            `json:"trackCount"`
	Duration   float64        `json:"duration"`
	CoverID    string         `json:"coverId,omitempty"`
	Fetched    int64          `json:"fetched"`
	IDs        map[string]int `json:"ids"`
	Names      map[string]int `json:"names"`
}

// plIndex is a server library's playlists by artist, kept on disk so a
// restart doesn't fetch every playlist again.
type plIndex struct {
	Listed    int64     `json:"listed"`
	Playlists []plEntry `json:"playlists"`
	// running closes when the refresh in flight ends; stale are playlists
	// changed through this player, refetched by the next refresh.
	running chan struct{}
	stale   map[string]bool
	failed  time.Time
}

// Playlists finds the local playlists and the library's server playlists
// with tracks of the artist: by id in that library, and by name (when
// given) in any library a local playlist holds tracks of. It reads only
// indexes; a stale server index is refreshed in the background and
// TopicPlaylists follows when that changed anything.
func (s *Service) Playlists(ctx context.Context, lib, id, name string) ([]PlaylistHit, error) {
	out, err := s.localHits(ctx, lib, id, name)
	if err != nil {
		return nil, err
	}
	return append(out, s.serverHits(ctx, lib, id, name)...), nil
}

func (s *Service) localHits(ctx context.Context, lib, id, name string) ([]PlaylistHit, error) {
	out := []PlaylistHit{}
	if s.cfg.Local == nil {
		return out, nil
	}
	store := s.cfg.Local()
	if store == nil {
		return out, nil
	}
	matches, err := store.ArtistMatches(ctx, lib, id, name)
	if err != nil || len(matches) == 0 {
		return out, err
	}
	list, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if n := matches[p.ID]; n > 0 {
			out = append(out, PlaylistHit{ID: p.ID, Name: p.Name, TrackCount: p.TrackCount, Matches: n})
		}
	}
	return out, nil
}

func (s *Service) serverHits(ctx context.Context, lib, id, name string) []PlaylistHit {
	src, err := s.cfg.Get(lib)
	if err != nil {
		return nil
	}
	pls, ok := src.(source.Playlists)
	if !ok {
		return nil
	}
	if done := s.refresh(lib, pls); done != nil {
		t := time.NewTimer(indexWait)
		defer t.Stop()
		select {
		case <-done:
		case <-t.C:
		case <-ctx.Done():
		}
	}
	name = strings.ToLower(source.Clean(name))
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []PlaylistHit
	for _, e := range s.index[lib].Playlists {
		n := e.Names[name]
		if id != "" {
			n += e.IDs[id]
		}
		if n > 0 {
			out = append(out, PlaylistHit{Library: lib, ID: e.ID, Name: e.Name, TrackCount: e.TrackCount, CoverID: e.CoverID, Matches: n})
		}
	}
	return out
}

// Invalidate marks a server playlist changed through this player.
func (s *Service) Invalidate(lib, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.indexLocked(lib)
	idx.stale[id] = true
	idx.Listed = 0
}

func (s *Service) indexPath(lib string) string {
	h := sha256.Sum256([]byte(lib))
	return filepath.Join(s.cfg.CacheDir, "artists", "playlists", hex.EncodeToString(h[:8])+".json")
}

// indexLocked returns a library's index, read from disk the first time.
func (s *Service) indexLocked(lib string) *plIndex {
	if idx := s.index[lib]; idx != nil {
		return idx
	}
	idx := &plIndex{}
	if data, err := os.ReadFile(s.indexPath(lib)); err == nil && json.Unmarshal(data, idx) != nil {
		idx = &plIndex{}
	}
	if len(idx.Playlists) > maxPlaylists {
		idx.Playlists = idx.Playlists[:maxPlaylists]
	}
	idx.stale = map[string]bool{}
	s.index[lib] = idx
	return idx
}

// refresh starts reading a library's playlists again when its index is
// older than listEvery. While nothing is indexed yet it returns a channel
// that closes when the running refresh ends, else nil: an old index answers
// at once.
func (s *Service) refresh(lib string, pls source.Playlists) <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.indexLocked(lib)
	if idx.running == nil && time.Since(time.Unix(idx.Listed, 0)) >= listEvery && time.Since(idx.failed) >= failRetry {
		idx.running = make(chan struct{})
		old, stale := idx.Playlists, idx.stale
		idx.stale = map[string]bool{}
		go s.reindex(lib, pls, old, stale, idx.running)
	}
	if len(idx.Playlists) > 0 {
		return nil
	}
	return idx.running
}

func (s *Service) reindex(lib string, pls source.Playlists, old []plEntry, stale map[string]bool, done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	next, err := buildIndex(ctx, pls, old, stale)
	s.mu.Lock()
	idx := s.index[lib]
	var data []byte
	changed := false
	if err != nil {
		log.Infof("playlists of %s: %v", lib, err)
		maps.Copy(idx.stale, stale)
		idx.failed = time.Now()
	} else {
		changed = !sameHits(idx.Playlists, next)
		idx.Playlists, idx.Listed, idx.failed = next, time.Now().Unix(), time.Time{}
		if len(idx.stale) > 0 {
			// Invalidated while this ran: the next visit refetches those.
			idx.Listed = 0
		}
		data, _ = json.Marshal(idx)
	}
	s.mu.Unlock()
	if data != nil {
		path := s.indexPath(lib)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
			_ = atomicfile.WriteFile(path, data, 0o600)
		}
	}
	// Ends only after the write, so a newer refresh can't be overwritten by
	// this one's older index.
	s.mu.Lock()
	idx.running = nil
	s.mu.Unlock()
	close(done)
	if changed && s.cfg.Publish != nil {
		s.cfg.Publish(TopicPlaylists, map[string]string{"library": lib})
	}
}

// buildIndex lists the playlists and fetches those that are new, changed
// size, were marked stale or are older than playlistTTL; the rest keep
// their entry.
func buildIndex(ctx context.Context, pls source.Playlists, old []plEntry, stale map[string]bool) ([]plEntry, error) {
	list, err := pls.Playlists(ctx)
	if err != nil {
		return nil, err
	}
	if len(list) > maxPlaylists {
		list = list[:maxPlaylists]
	}
	prev := make(map[string]plEntry, len(old))
	for _, e := range old {
		prev[e.ID] = e
	}
	now := time.Now()
	out := make([]plEntry, len(list))
	sem := make(chan struct{}, fetchers)
	var wg sync.WaitGroup
	for i, p := range list {
		out[i] = plEntry{ID: p.ID, Name: p.Name, TrackCount: p.TrackCount, Duration: p.Duration, CoverID: p.CoverID}
		o, had := prev[p.ID]
		if had {
			out[i].Fetched, out[i].IDs, out[i].Names = o.Fetched, o.IDs, o.Names
		}
		if had && !stale[p.ID] && o.TrackCount == p.TrackCount && o.Duration == p.Duration &&
			now.Sub(time.Unix(o.Fetched, 0)) < playlistTTL {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			full, err := pls.Playlist(ctx, p.ID)
			if err != nil {
				// The old entry stays; the next refresh tries again.
				return
			}
			out[i].IDs, out[i].Names = artistCounts(full.Tracks)
			out[i].Fetched = now.Unix()
		}()
	}
	wg.Wait()
	return out, nil
}

func artistCounts(tracks []source.Track) (ids, names map[string]int) {
	ids, names = map[string]int{}, map[string]int{}
	for _, t := range tracks {
		seen := map[string]bool{}
		for _, a := range append([]source.ArtistRef{{ID: t.ArtistID}}, t.Artists...) {
			if a.ID != "" && !seen[a.ID] {
				seen[a.ID] = true
				ids[a.ID]++
			}
		}
		if len(seen) > 0 {
			continue
		}
		refs := t.Artists
		if len(refs) == 0 {
			refs = source.SingleArtist(t.Artist, "")
		}
		for _, r := range refs {
			if n := strings.ToLower(source.Clean(r.Name)); n != "" && !seen[n] {
				seen[n] = true
				names[n]++
			}
		}
	}
	return ids, names
}

// sameHits says whether two indexes answer every artist the same way.
func sameHits(a, b []plEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Name != b[i].Name || a[i].TrackCount != b[i].TrackCount ||
			!maps.Equal(a[i].IDs, b[i].IDs) || !maps.Equal(a[i].Names, b[i].Names) {
			return false
		}
	}
	return true
}
