package source

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverColor(t *testing.T) {
	grey, red := color.RGBA{128, 128, 128, 255}, color.RGBA{200, 30, 30, 255}
	tests := []struct {
		name    string
		redPart int // columns of 100
		want    string
	}{
		{name: "a colorful part wins over a grey majority", redPart: 30, want: "#c81e1e"},
		{name: "a grey cover stays grey", redPart: 0, want: "#808080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, 100, 100))
			for y := range 100 {
				for x := range 100 {
					img.Set(x, y, grey)
					if x < tt.redPart {
						img.Set(x, y, red)
					}
				}
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "cover.png")
			f, err := os.Create(path)
			require.NoError(t, err)
			require.NoError(t, png.Encode(f, img))
			require.NoError(t, f.Close())
			assert.Equal(t, tt.want, CoverColor(filepath.Join(dir, "colors"), path))
			assert.Equal(t, tt.want, CoverColor(filepath.Join(dir, "colors"), path), "stored value")
		})
	}
}
