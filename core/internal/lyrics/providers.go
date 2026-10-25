package lyrics

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sync"

	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"

	"github.com/Conava/dux-player/core/internal/atomicfile"
)

const providersFile = "lyrics.json"

// Provider is one row of the lyrics settings: the list order is the
// priority order.
type Provider struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

var providerNames = map[duxlyrics.Provider]string{
	ServerSource:           "Server",
	duxlyrics.BetterLyrics: "BetterLyrics",
	duxlyrics.Unison:       "Unison",
	duxlyrics.LyricsPlus:   "LyricsPlus",
	duxlyrics.KuGou:        "KuGou",
	duxlyrics.LRCLIB:       "LRCLIB",
	duxlyrics.YouTubeMusic: "YouTube Music",
}

// defaultProviders is the track's own server, then dux-go's order with
// YouTube Music off, as the shell ships it.
func defaultProviders() []Provider {
	out := []Provider{{ID: ServerSource, Name: providerNames[ServerSource], Enabled: true}}
	for _, id := range duxlyrics.DefaultProviders() {
		out = append(out, Provider{ID: string(id), Name: providerNames[id], Enabled: id != duxlyrics.YouTubeMusic})
	}
	return out
}

type providerStore struct {
	path string
	mu   sync.Mutex
	list []Provider
}

// openProviders reads the saved order. Unknown ids are dropped and
// providers missing from the file are appended, so a dux-go update that
// adds one shows it. A file from before the server was a row gets it first,
// where it always was.
func openProviders(path string) *providerStore {
	s := &providerStore{path: path, list: defaultProviders()}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var saved struct {
		Providers []Provider `json:"providers"`
	}
	if json.Unmarshal(data, &saved) == nil {
		s.list = normalize(saved.Providers)
	}
	return s
}

func normalize(in []Provider) []Provider {
	var out []Provider
	seen := map[string]bool{}
	for _, p := range in {
		name, ok := providerNames[duxlyrics.Provider(p.ID)]
		if !ok || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		out = append(out, Provider{ID: p.ID, Name: name, Enabled: p.Enabled})
	}
	for _, p := range defaultProviders() {
		switch {
		case seen[p.ID]:
		case p.ID == ServerSource:
			out = append([]Provider{p}, out...)
		default:
			out = append(out, p)
		}
	}
	return out
}

func (s *providerStore) get() []Provider {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.list)
}

// enabled is the priority order of what is on, ServerSource included:
// non-nil, so all off means sidecar files only.
func (s *providerStore) enabled() []duxlyrics.Provider {
	out := []duxlyrics.Provider{}
	for _, p := range s.get() {
		if p.Enabled {
			out = append(out, duxlyrics.Provider(p.ID))
		}
	}
	return out
}

// set saves a new order; every id must be known.
func (s *providerStore) set(list []Provider) ([]Provider, error) {
	for _, p := range list {
		if _, ok := providerNames[duxlyrics.Provider(p.ID)]; !ok {
			return nil, fmt.Errorf("%w: %q", errUnknownProvider, p.ID)
		}
	}
	next := normalize(list)
	data, err := json.MarshalIndent(map[string]any{"providers": next}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := atomicfile.WriteFile(s.path, append(data, '\n'), 0o600); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.list = next
	s.mu.Unlock()
	return slices.Clone(next), nil
}
