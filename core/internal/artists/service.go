// Package artists answers the artist page: info, image and the playlists
// holding an artist's tracks. The library comes first; online services fill
// in an image or biography it lacks, when the user allows it.
package artists

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/playlists"
	"github.com/Conava/dux-player/core/internal/source"
)

// Providers that need the user's API key.
const (
	ProviderFanartTV = "fanarttv"
	ProviderLastFM   = "lastfm"
)

const (
	// onlineWait bounds the online part of one request.
	onlineWait = 20 * time.Second
)

type Config struct {
	// Get returns a library's working source.
	Get func(lib string) (source.Source, error)
	// Local are the local playlists; nil until they are open.
	Local func() *playlists.Store
	// Online says whether online lookups are on (the artistInfoOnline setting).
	Online   func() bool
	Secrets  source.Secrets
	CacheDir string
	Version  string
	// Lang is the system language ("de"), for Wikipedia.
	Lang string
	// Publish sends TopicPlaylists when a library's playlist index changed.
	Publish func(topic string, data any)
}

type Service struct {
	cfg    Config
	online *online

	mu   sync.Mutex
	keys map[string]string
	// index is each server library's playlists by the artists in them.
	index map[string]*plIndex
}

func New(cfg Config) *Service {
	s := &Service{cfg: cfg, keys: map[string]string{}, index: map[string]*plIndex{}}
	s.online = newOnline(filepath.Join(cfg.CacheDir, "artists"), cfg.Version, cfg.Lang, s.key)
	return s
}

func secretID(provider string) string { return "artistinfo-" + provider }

// key is the user's API key for a provider, read from the keyring once.
func (s *Service) key(provider string) string {
	s.mu.Lock()
	k, ok := s.keys[provider]
	s.mu.Unlock()
	if ok {
		return k
	}
	k, _ = s.cfg.Secrets.Get(secretID(provider), "apiKey")
	s.mu.Lock()
	s.keys[provider] = k
	s.mu.Unlock()
	return k
}

func (s *Service) SetKey(provider, key string) error {
	if provider != ProviderFanartTV && provider != ProviderLastFM {
		return fmt.Errorf("provider must be %s or %s", ProviderFanartTV, ProviderLastFM)
	}
	key = strings.TrimSpace(key)
	if len(key) > 128 || strings.ContainsFunc(key, func(r rune) bool { return r <= ' ' }) {
		return errors.New("that doesn't look like an API key")
	}
	var err error
	if key == "" {
		err = s.cfg.Secrets.Delete(secretID(provider))
	} else {
		err = s.cfg.Secrets.Set(secretID(provider), "apiKey", key)
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.keys[provider] = key
	s.mu.Unlock()
	return nil
}

type Provider struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Uses     []string `json:"uses"`
	NeedsKey bool     `json:"needsKey"`
	HasKey   bool     `json:"hasKey"`
}

type Status struct {
	Online    bool       `json:"online"`
	Providers []Provider `json:"providers"`
}

func (s *Service) Status() Status {
	return Status{Online: s.cfg.Online(), Providers: []Provider{
		{ID: "musicbrainz", Name: "MusicBrainz", Uses: []string{"ids"}},
		{ID: "wikimedia", Name: "Wikidata, Wikipedia and Wikimedia Commons", Uses: []string{"image", "biography"}},
		{ID: ProviderFanartTV, Name: "fanart.tv", Uses: []string{"image"}, NeedsKey: true, HasKey: s.key(ProviderFanartTV) != ""},
		{ID: ProviderLastFM, Name: "Last.fm", Uses: []string{"biography"}, NeedsKey: true, HasKey: s.key(ProviderLastFM) != ""},
	}}
}

// Info is the library's artist info, with an online biography when the
// library has none.
func (s *Service) Info(ctx context.Context, lib, id string) (source.ArtistInfo, error) {
	src, err := s.cfg.Get(lib)
	if err != nil {
		return source.ArtistInfo{}, err
	}
	d, ok := src.(source.ArtistDetailer)
	if !ok {
		return source.ArtistInfo{}, fmt.Errorf("library %s does not support artist info", lib)
	}
	info, err := d.ArtistInfo(ctx, id)
	if err != nil {
		return info, err
	}
	if _, ok := src.(source.Ratings); !ok {
		info.Rating = 0
	}
	if info.Biography != "" || !s.cfg.Online() || info.Name == "" {
		return info, nil
	}
	octx, cancel := context.WithTimeout(ctx, onlineWait)
	defer cancel()
	e, err := s.online.lookup(octx, info.Name, info.MusicBrainzID, false)
	if err != nil {
		log.Infof("biography of %s: %v", info.Name, err)
	}
	info.Biography, info.BiographySource = e.Bio, e.BioSource
	if info.MusicBrainzID == "" {
		info.MusicBrainzID = e.MBID
	}
	return info, nil
}

// Image is the artist's image file and where it came from: the library's
// own (the server's coverId, files next to the music), then online.
func (s *Service) Image(ctx context.Context, lib, id, name, coverID string, size int) (path, from string, err error) {
	src, err := s.cfg.Get(lib)
	if err != nil {
		return "", "", err
	}
	if c, ok := src.(source.CoverArt); ok && coverID != "" {
		if path, err := c.Cover(ctx, coverID, size); err == nil {
			return path, "library", nil
		}
	}
	if im, ok := src.(source.ArtistImager); ok {
		if path, err := im.ArtistImage(ctx, id); err == nil {
			return path, "library", nil
		}
	}
	if name = source.Clean(name); name == "" || !s.cfg.Online() {
		return "", "", errors.New("no image")
	}
	octx, cancel := context.WithTimeout(ctx, onlineWait)
	defer cancel()
	e, err := s.online.lookup(octx, name, "", true)
	if err != nil {
		return "", "", fmt.Errorf("no image: %w", err)
	}
	if e.Image == "" {
		return "", "", errors.New("no image")
	}
	path = filepath.Join(s.online.dir, e.Image)
	if _, err := os.Stat(path); err != nil {
		return "", "", errors.New("no image")
	}
	return path, e.ImageSource, nil
}
