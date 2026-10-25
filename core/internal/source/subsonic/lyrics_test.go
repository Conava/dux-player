package subsonic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shaped like Navidrome's getLyricsBySongId: an unsynced set first, then a
// synced one with an offset.
const lyricsReply = `{"structuredLyrics":[
 {"lang":"xxx","synced":false,"line":[{"value":"plain one"},{"value":"plain two"}]},
 {"lang":"eng","synced":true,"offset":500,"line":[{"start":1500,"value":"first"},{"start":4000,"value":"second"}]}]}`

func TestPickLyricsPrefersSyncedAndAppliesOffset(t *testing.T) {
	var out struct {
		StructuredLyrics list[structuredLyrics] `json:"structuredLyrics"`
	}
	require.NoError(t, json.Unmarshal([]byte(lyricsReply), &out))
	l := pickLyrics(out.StructuredLyrics)
	require.Len(t, l.Synced, 2)
	assert.InDelta(t, 1.0, float64(l.Synced[0].Start), 0.001)
	assert.Equal(t, "second", l.Synced[1].Text)
	assert.Equal(t, "first\nsecond", l.Plain)
	assert.True(t, pickLyrics(nil).Empty())
}

// A server that doesn't parse an LRC tag sends it as one unsynced set.
func TestPickLyricsParsesLRCSentAsPlain(t *testing.T) {
	var out struct {
		StructuredLyrics list[structuredLyrics] `json:"structuredLyrics"`
	}
	require.NoError(t, json.Unmarshal([]byte(`{"structuredLyrics":[{"synced":false,"line":[{"value":"[00:01.50]first"},{"value":"[00:04.00]second"}]}]}`), &out))
	l := pickLyrics(out.StructuredLyrics)
	require.Len(t, l.Synced, 2)
	assert.InDelta(t, 1.5, float64(l.Synced[0].Start), 0.001)
	assert.Equal(t, "second", l.Synced[1].Text)
}
