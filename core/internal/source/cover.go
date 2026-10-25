package source

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Conava/dux-player/core/internal/atomicfile"
)

// CachedCover returns the cached file for a remote cover, fetching it on a
// miss. Cover ids are opaque server strings, so they are hashed into the
// file name; fetch returns the image and its content type.
func CachedCover(dir, coverID string, size int, fetch func() ([]byte, string, error)) (string, error) {
	if coverID == "" {
		return "", ErrNotFound
	}
	h := sha256.Sum256([]byte(coverID))
	name := fmt.Sprintf("%x-%d", h[:8], size)
	for _, ext := range []string{".jpg", ".png", ".webp", ".gif"} {
		p := filepath.Join(dir, name+ext)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	data, ct, err := fetch()
	if err != nil {
		return "", err
	}
	ext := ".jpg"
	switch ct {
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	}
	p := filepath.Join(dir, name+ext)
	return p, atomicfile.WriteFile(p, data, 0o600)
}
