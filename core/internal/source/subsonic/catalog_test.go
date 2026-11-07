package subsonic

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

func reply(body string) string { return strings.Replace(okEnvelope, "%s", ","+body, 1) }

func TestCatalogChanged(t *testing.T) {
	for _, tt := range []struct {
		name, stamp, reply string
		want               bool
		wantNext           string
	}{
		{"first time", "", `"indexes":{"lastModified":1700000000000}`, true, "1700000000000"},
		{"nothing since", "1700000000000", `"indexes":{"lastModified":1700000000000}`, false, "1700000000000"},
		{"moved on", "1700000000000", `"indexes":{"lastModified":1700000000999}`, true, "1700000000999"},
		{"no element is Airsonic's unchanged", "1700000000000", `"other":1`, false, "1700000000000"},
		{"zero can't be gated", "1700000000000", `"indexes":{"lastModified":0}`, false, "1700000000000"},
		{"missing can't be gated", "1700000000000", `"indexes":{}`, false, "1700000000000"},
		{"zero again", "0", `"indexes":{"lastModified":0}`, false, "0"},
		{"zero at the first sync still syncs", "", `"indexes":{"lastModified":0}`, true, "0"},
		{"a value after zero", "0", `"indexes":{"lastModified":1700000000000}`, true, "1700000000000"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, "demo", map[string]string{"ping": strings.Replace(okEnvelope, "%s", "", 1), "getIndexes": reply(tt.reply)})
			changed, next, err := newSource(t, f, "demo").CatalogChanged(context.Background(), tt.stamp)
			require.NoError(t, err)
			assert.Equal(t, tt.want, changed)
			assert.Equal(t, tt.wantNext, next)
			assert.Equal(t, tt.stamp, f.query("getIndexes").Get("ifModifiedSince"))
		})
	}
}

// A walk reads the count once; each page asking would cost a request, and on
// a server without a count a pass over the album list.
func TestWalkReadsTheCountOnce(t *testing.T) {
	for _, tt := range []struct {
		name    string
		details bool
		offset  int
		asked   bool
	}{
		{"a walk's first page", true, 0, true},
		{"a walk's later page", true, 500, false},
		{"a page for browsing", false, 500, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFake(t, "demo", map[string]string{"ping": strings.Replace(okEnvelope, "%s", "", 1),
				"search3":       reply(`"searchResult3":{"song":[{"id":"s","title":"T"}]}`),
				"getScanStatus": reply(`"scanStatus":{"scanning":false,"count":10}`)})
			page, err := newSource(t, f, "demo").TrackPage(context.Background(), source.TrackQuery{Offset: tt.offset, Limit: 500, Details: tt.details})
			require.NoError(t, err)
			assert.Equal(t, tt.asked, f.query("getScanStatus") != nil)
			assert.True(t, page.Estimated)
		})
	}
}

func TestCatalogBusy(t *testing.T) {
	for _, tt := range []struct {
		reply string
		want  bool
	}{
		{`"scanStatus":{"scanning":true,"count":10}`, true},
		{`"scanStatus":{"scanning":false,"count":10}`, false},
	} {
		f := newFake(t, "demo", map[string]string{"ping": strings.Replace(okEnvelope, "%s", "", 1), "getScanStatus": reply(tt.reply)})
		assert.Equal(t, tt.want, newSource(t, f, "demo").CatalogBusy(context.Background()), tt.reply)
	}
}

// A sync sweeps what it didn't list, so a listing may not stop short of a
// large library.
func TestAlbumsReachPastFiveThousand(t *testing.T) {
	var page strings.Builder
	for i := range 500 {
		if i > 0 {
			page.WriteByte(',')
		}
		fmt.Fprintf(&page, `{"id":"a%d","name":"A"}`, i)
	}
	full := reply(`"albumList2":{"album":[` + page.String() + `]}`)
	f := newFake(t, "demo", map[string]string{"ping": strings.Replace(okEnvelope, "%s", "", 1), "getAlbumList2": full})
	albums, err := newSource(t, f, "demo").Albums(context.Background(), "")
	require.NoError(t, err)
	assert.Greater(t, len(albums), 5000)
}
