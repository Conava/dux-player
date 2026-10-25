package source

import (
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	"github.com/Conava/dux-player/core/internal/atomicfile"
)

// maxColorPixels skips images too large to decode for one color (16 Mpx
// is about 64 MB decoded); real covers stay far below it.
const maxColorPixels = 4096 * 4096

// CoverColor is the dominant color of an image file as #rrggbb, computed
// once per file version and kept under dir. Empty when the format can't be
// decoded (WebP) or the image has no pixels.
func CoverColor(dir, path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(fmt.Appendf(nil, "%s|%d|%d", path, st.Size(), st.ModTime().UnixNano()))
	stored := filepath.Join(dir, fmt.Sprintf("%x", h[:12]))
	if data, err := os.ReadFile(stored); err == nil {
		return string(data)
	}
	c := fileColor(path)
	_ = atomicfile.WriteFile(stored, []byte(c), 0o600)
	return c
}

func fileColor(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width*cfg.Height > maxColorPixels {
		return ""
	}
	if _, err := f.Seek(0, 0); err != nil {
		return ""
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return ""
	}
	r, g, b, ok := dominant(img)
	if !ok {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// dominant is the mean color of the most common 3-bit-per-channel bucket,
// preferring colorful pixels: a scheme seeded from grey or black looks
// dead, so those only win when the cover has hardly any color.
func dominant(img image.Image) (r, g, b uint8, ok bool) {
	type bucket struct{ n, r, g, b int }
	var all, vivid [512]bucket
	bounds := img.Bounds()
	step := max(1, max(bounds.Dx(), bounds.Dy())/96)
	samples, vividN := 0, 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			cr, cg, cb, ca := img.At(x, y).RGBA()
			if ca < 0x8000 {
				continue
			}
			pr, pg, pb := int(cr>>8), int(cg>>8), int(cb>>8)
			k := (pr>>5)<<6 | (pg>>5)<<3 | pb>>5
			add := func(bs *[512]bucket) { bs[k].n++; bs[k].r += pr; bs[k].g += pg; bs[k].b += pb }
			add(&all)
			samples++
			hi, lo := max(pr, pg, pb), min(pr, pg, pb)
			if hi > 40 && hi < 245 && hi-lo > hi/4 {
				add(&vivid)
				vividN++
			}
		}
	}
	if samples == 0 {
		return 0, 0, 0, false
	}
	set := &all
	if vividN*20 >= samples {
		set = &vivid
	}
	best := 0
	for i := range set {
		if set[i].n > set[best].n {
			best = i
		}
	}
	bk := set[best]
	return uint8(bk.r / bk.n), uint8(bk.g / bk.n), uint8(bk.b / bk.n), true
}
