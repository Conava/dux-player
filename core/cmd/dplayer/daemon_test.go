package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/similar"
	"github.com/Conava/dux-player/core/internal/source"
)

// A seed far into a long playlist still finds it, with the full length.
func TestPlaylistSetMatchesPastTheSample(t *testing.T) {
	seed := source.Track{Library: "L", ID: "seed"}
	var long []source.Track
	for i := range 400 {
		long = append(long, source.Track{Library: "L", ID: fmt.Sprintf("t%d", i)})
	}
	long[350] = seed

	set := newPlaylistSet()
	set.add("long", long)
	set.add("other", long[:10])
	out := map[string]similar.Playlist{}
	set.match([]source.Ref{seed.Ref()}, 100, "L\x00", out)

	require.Contains(t, out, "L\x00long")
	assert.NotContains(t, out, "L\x00other")
	pl := out["L\x00long"]
	assert.Equal(t, 400, pl.Len)
	assert.Equal(t, []source.Ref{seed.Ref()}, pl.Seeds)
	assert.Len(t, pl.Tracks, 100)
}
