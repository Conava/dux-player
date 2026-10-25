package jellyfin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLyricsReplyInTicks(t *testing.T) {
	tests := []struct {
		reply  string
		synced int
		plain  string
	}{
		{`{"Metadata":{"IsSynced":true},"Lyrics":[{"Text":"one","Start":12000000},{"Text":"two","Start":35000000}]}`, 2, "one\ntwo"},
		{`{"Metadata":{"IsSynced":false},"Lyrics":[{"Text":"one"},{"Text":"two"}]}`, 0, "one\ntwo"},
	}
	for _, tt := range tests {
		var r lyricsReply
		require.NoError(t, json.Unmarshal([]byte(tt.reply), &r))
		l := r.lyrics()
		assert.Len(t, l.Synced, tt.synced)
		assert.Equal(t, tt.plain, l.Plain)
		if tt.synced > 0 {
			assert.InDelta(t, 1.2, float64(l.Synced[0].Start), 0.001)
		}
	}
}
