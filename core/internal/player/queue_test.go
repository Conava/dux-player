package player

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

func tracks(ids ...string) []source.Track {
	out := make([]source.Track, len(ids))
	for i, id := range ids {
		out[i] = source.Track{Library: "l", ID: id}
	}
	return out
}

func ids(q *Queue) []string {
	var out []string
	for _, it := range q.Items() {
		out = append(out, it.Track.ID)
	}
	return out
}

func current(q *Queue) string {
	it, ok := q.Current()
	if !ok {
		return ""
	}
	return it.Track.ID
}

func TestQueueEdits(t *testing.T) {
	tests := []struct {
		name    string
		start   int
		edit    func(q *Queue) error
		want    []string
		current string
	}{
		{
			name:    "add next inserts after current",
			start:   1,
			edit:    func(q *Queue) error { q.Add(tracks("x", "y"), true); return nil },
			want:    []string{"a", "b", "x", "y", "c"},
			current: "b",
		},
		{
			name:    "add appends",
			start:   0,
			edit:    func(q *Queue) error { q.Add(tracks("x"), false); return nil },
			want:    []string{"a", "b", "c", "x"},
			current: "a",
		},
		{
			name:    "remove before current keeps current",
			start:   2,
			edit:    func(q *Queue) error { _, err := q.Remove(0); return err },
			want:    []string{"b", "c"},
			current: "c",
		},
		{
			name:    "remove current selects the following item",
			start:   1,
			edit:    func(q *Queue) error { _, err := q.Remove(1); return err },
			want:    []string{"a", "c"},
			current: "c",
		},
		{
			name:    "remove last current selects the new last",
			start:   2,
			edit:    func(q *Queue) error { _, err := q.Remove(2); return err },
			want:    []string{"a", "b"},
			current: "b",
		},
		{
			name:    "move current item follows it",
			start:   0,
			edit:    func(q *Queue) error { return q.Move(0, 2) },
			want:    []string{"b", "c", "a"},
			current: "a",
		},
		{
			name:    "keep only the current item",
			start:   1,
			edit:    func(q *Queue) error { q.KeepOnly(q.currentUID()); return nil },
			want:    []string{"b"},
			current: "b",
		},
		{
			name:    "move across current keeps current",
			start:   1,
			edit:    func(q *Queue) error { return q.Move(2, 0) },
			want:    []string{"c", "a", "b"},
			current: "b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQueue()
			q.Replace(tracks("a", "b", "c"), tt.start, Origin{Kind: "album"})
			require.NoError(t, tt.edit(q))
			assert.Equal(t, tt.want, ids(q))
			assert.Equal(t, tt.current, current(q))
		})
	}
}

func TestQueueRejectsOutOfRange(t *testing.T) {
	q := NewQueue()
	q.Replace(tracks("a", "b"), 0, Origin{})
	_, err := q.Remove(2)
	assert.Error(t, err)
	assert.Error(t, q.Move(0, 2))
	assert.Error(t, q.Jump(-1))
	assert.Equal(t, []string{"a", "b"}, ids(q))
}

func TestQueueAdvanceAndPeek(t *testing.T) {
	q := NewQueue()
	q.Add(tracks("a", "b"), false)
	assert.Equal(t, "a", current(q), "an empty queue selects the first added item")
	next, ok := q.Peek()
	require.True(t, ok)
	assert.Equal(t, "b", next.Track.ID)
	require.True(t, q.Advance())
	_, ok = q.Peek()
	assert.False(t, ok)
	assert.False(t, q.Advance(), "advancing past the end stays on the last item")
	assert.Equal(t, "b", current(q))
}

func TestShuffleKeepsCurrentAndRestoresOrder(t *testing.T) {
	q := NewQueue()
	q.Replace(tracks("a", "b", "c", "d", "e", "f"), 2, Origin{Kind: "album"})
	q.SetShuffle(true)
	assert.Equal(t, "c", current(q))
	assert.Equal(t, 0, q.Index(), "the current track leads the shuffled queue")
	assert.ElementsMatch(t, []string{"a", "b", "c", "d", "e", "f"}, ids(q))

	q.Add(tracks("n"), true)
	q.SetShuffle(false)
	assert.Equal(t, []string{"a", "b", "c", "n", "d", "e", "f"}, ids(q), "play next while shuffled lands after the current track")
	assert.Equal(t, "c", current(q))
}

// State version 1 saved a shuffle mode; both ways of on restore as on.
func TestOldShuffleModesRestoreAsOn(t *testing.T) {
	for raw, want := range map[string]bool{`"tracks"`: true, `"albums"`: true, `"off"`: false, `true`: true, `false`: false} {
		var st QueueState
		require.NoError(t, json.Unmarshal([]byte(`{"items":[{"uid":1,"track":{"library":"l","id":"a"}},{"uid":2,"track":{"library":"l","id":"b"}}],"index":0,"shuffle":`+raw+`}`), &st))
		q := NewQueue()
		q.Restore(st)
		assert.Equal(t, want, q.Shuffle(), raw)
		q.SetShuffle(false)
		assert.Equal(t, []string{"a", "b"}, ids(q), raw)
	}
}

func TestRepeatAllWraps(t *testing.T) {
	q := NewQueue()
	q.Replace(tracks("a", "b"), 1, Origin{})
	require.NoError(t, q.SetRepeat(RepeatAll))
	next, ok := q.Peek()
	require.True(t, ok)
	assert.Equal(t, "a", next.Track.ID)
	require.True(t, q.Advance())
	assert.Equal(t, "a", current(q))
	assert.Error(t, q.SetRepeat("sometimes"))
}

func TestOriginTurnsModifiedOnEdits(t *testing.T) {
	q := NewQueue()
	q.Replace(tracks("a", "b"), 0, Origin{Kind: "playlist", ID: "p1"})
	q.SetShuffle(true)
	assert.False(t, q.Origin().Modified, "shuffling is not an edit")
	q.Add(tracks("c"), true)
	assert.True(t, q.Origin().Modified)
	assert.Equal(t, "p1", q.Origin().ID)
}

func TestQueueStateRoundTrip(t *testing.T) {
	q := NewQueue()
	q.Replace(tracks("a", "b", "c", "d"), 1, Origin{Kind: "album", ID: "x"})
	q.SetShuffle(true)
	require.NoError(t, q.SetRepeat(RepeatOne))

	r := NewQueue()
	r.Restore(q.State())
	assert.Equal(t, ids(q), ids(r))
	assert.Equal(t, current(q), current(r))
	assert.Equal(t, q.Origin(), r.Origin())
	assert.Equal(t, RepeatOne, r.Repeat())
	r.Add(tracks("e"), false)
	assert.NotContains(t, ids(q), "e")
	assert.Equal(t, 5, len(r.State().Orig), "uids keep counting past the restored ones")
	r.SetShuffle(false)
	assert.Equal(t, []string{"a", "b", "c", "d", "e"}, ids(r), "unshuffle works after a restore")

	bad := q.State()
	bad.Orig = bad.Orig[:1]
	bad.Repeat = "nonsense"
	r.Restore(bad)
	assert.Equal(t, RepeatOff, r.Repeat())
	r.SetShuffle(false)
	assert.ElementsMatch(t, []string{"a", "b", "c", "d"}, ids(r), "a broken unshuffle order loses no tracks")
}

// Items put in by hand carry Added, through a save and restore; a growing
// shuffle or a new queue doesn't.
func TestQueueMarksTracksAddedByHand(t *testing.T) {
	tests := []struct {
		name string
		edit func(q *Queue)
		want []string // per item: "" none, "hand" Added, "auto" Auto
	}{
		{"new queue", func(q *Queue) {}, []string{"", ""}},
		{"play next", func(q *Queue) { q.Add(tracks("x"), true) }, []string{"", "hand", ""}},
		{"add to queue", func(q *Queue) { q.Add(tracks("x"), false) }, []string{"", "", "hand"}},
		{"growing shuffle", func(q *Queue) { q.Extend(tracks("x")) }, []string{"", "", ""}},
		{"grow", func(q *Queue) { q.Grow(tracks("w"), tracks("x")) }, []string{"", "", "", ""}},
		{"continue", func(q *Queue) { q.ExtendAuto(tracks("x")) }, []string{"", "", "auto"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newQueue(nil)
			q.Replace(tracks("a", "b"), 0, Origin{})
			tt.edit(q)
			data, err := json.Marshal(q.State())
			require.NoError(t, err)
			var saved QueueState
			require.NoError(t, json.Unmarshal(data, &saved))
			r := newQueue(nil)
			r.Restore(saved)
			var got []string
			for _, it := range r.Items() {
				require.False(t, it.Added && it.Auto)
				switch {
				case it.Added:
					got = append(got, "hand")
				case it.Auto:
					got = append(got, "auto")
				default:
					got = append(got, "")
				}
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// A shuffled list that grows in the background shuffles the new tracks in and
// still restores the list order when shuffle goes off.
func TestShuffledQueueGrowsShuffled(t *testing.T) {
	list := make([]string, 60)
	for i := range list {
		list[i] = "t" + pad(i)
	}
	tests := []struct {
		name          string
		head          [2]int // the part of the list that played first
		before, after [2]int
	}{
		{"grows after the head", [2]int{0, 5}, [2]int{0, 0}, [2]int{5, 60}},
		{"grows on both sides", [2]int{20, 25}, [2]int{0, 20}, [2]int{25, 60}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewQueue()
			q.SetShuffle(true)
			q.Replace(tracks(list[tt.head[0]:tt.head[1]]...), -1, Origin{})
			cur := current(q)
			q.Grow(tracks(list[tt.before[0]:tt.before[1]]...), tracks(list[tt.after[0]:tt.after[1]]...))
			assert.Equal(t, cur, current(q))
			assert.Equal(t, 0, q.Index())
			got := ids(q)
			assert.ElementsMatch(t, list, got)
			tail := got[len(got)-(tt.after[1]-tt.after[0]):]
			assert.NotEqual(t, list[tt.after[0]:tt.after[1]], tail, "the grown tracks were appended in list order")
			q.SetShuffle(false)
			assert.Equal(t, list, ids(q))
			assert.Equal(t, cur, current(q))
		})
	}
}

// The Shuffle button of one album or playlist mixes its tracks whatever
// Shuffle by says.
func TestShuffleOfOneListIgnoresAlbums(t *testing.T) {
	var ts []source.Track
	for n := range 60 {
		ts = append(ts, source.Track{Library: "l", ID: fmt.Sprint(n), AlbumID: "x", TrackNo: n})
	}
	q := NewQueue()
	q.albums = true
	q.SetShuffle(true)
	q.Replace(ts, -1, Origin{})
	got := q.Items()
	assert.False(t, slices.IsSortedFunc(got, func(a, b Item) int { return a.Track.TrackNo - b.Track.TrackNo }))
}

// Untagged tracks share a made-up album, which is no reason to keep them together.
func TestShuffleByAlbumSplitsUntaggedTracks(t *testing.T) {
	var ts []source.Track
	for n := range 60 {
		ts = append(ts, source.Track{Library: "l", ID: fmt.Sprint(n), Album: source.UnknownAlbum, AlbumID: "x", TrackNo: n})
	}
	q := NewQueue()
	q.albums = true
	q.Replace(ts, -1, Origin{})
	q.SetShuffle(true)
	assert.False(t, slices.IsSortedFunc(q.Items(), func(a, b Item) int { return a.Track.TrackNo - b.Track.TrackNo }))
}

func TestShuffleByAlbum(t *testing.T) {
	var ts []source.Track
	for _, album := range []string{"x", "y", "z"} {
		for n := 5; n >= 1; n-- {
			ts = append(ts, source.Track{Library: "l", ID: fmt.Sprint(album, n), AlbumID: album, TrackNo: n})
		}
	}
	ts = append(ts, source.Track{Library: "l", ID: "single"})
	q := NewQueue()
	q.albums = true
	q.Replace(ts, 7, Origin{})
	q.SetShuffle(true)

	items := q.Items()
	require.Len(t, items, len(ts))
	cur, ok := q.Current()
	require.True(t, ok)
	assert.Equal(t, ts[7].ID, cur.Track.ID)
	assert.Equal(t, ts[7].AlbumID, items[0].Track.AlbumID, "the current album comes first")
	seen := map[string]bool{}
	for i, it := range items {
		assert.False(t, seen[it.Track.ID], "every track once")
		seen[it.Track.ID] = true
		if i == 0 {
			continue
		}
		prev := items[i-1].Track
		if prev.AlbumID == it.Track.AlbumID {
			assert.Equal(t, prev.TrackNo+1, it.Track.TrackNo, "track order inside an album")
		} else {
			assert.True(t, it.Track.AlbumID == "" || it.Track.TrackNo == 1, "an album starts at its first track")
		}
	}
	q.SetShuffle(false)
	assert.Equal(t, ts[0].ID, q.Items()[0].Track.ID, "shuffle off restores the order")
}

func TestQueueDropLibrary(t *testing.T) {
	mixed := func(cur int) *Queue {
		q := NewQueue()
		ts := tracks("a", "b", "c", "d", "e")
		for _, i := range []int{1, 2} {
			ts[i].Library = "gone"
		}
		q.Replace(ts, cur, Origin{Kind: "album"})
		return q
	}
	tests := []struct {
		name                  string
		cur                   int
		wantIDs               []string
		wantCur               string
		curDropped, noneAfter bool
	}{
		{"current from another library stays", 3, []string{"a", "d", "e"}, "d", false, false},
		{"dropped current moves to the next remaining", 1, []string{"a", "d", "e"}, "d", true, false},
		{"dropped current in a run moves to the next remaining", 2, []string{"a", "d", "e"}, "d", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := mixed(tt.cur)
			removed, dropped, none := q.DropLibrary("gone")
			assert.True(t, removed)
			assert.Equal(t, tt.curDropped, dropped)
			assert.Equal(t, tt.noneAfter, none)
			assert.Equal(t, tt.wantIDs, ids(q))
			assert.Equal(t, tt.wantCur, current(q))
			assert.True(t, q.Origin().Modified)
		})
	}

	t.Run("only earlier items remain", func(t *testing.T) {
		q := NewQueue()
		ts := tracks("a", "b", "c")
		ts[1].Library, ts[2].Library = "gone", "gone"
		q.Replace(ts, 2, Origin{Kind: "album"})
		_, dropped, none := q.DropLibrary("gone")
		assert.True(t, dropped)
		assert.True(t, none)
		assert.Equal(t, "a", current(q))
	})

	t.Run("nothing from the library is a no-op", func(t *testing.T) {
		q := mixed(0)
		removed, _, _ := q.DropLibrary("other")
		assert.False(t, removed)
		assert.Equal(t, []string{"a", "b", "c", "d", "e"}, ids(q))
		assert.False(t, q.Origin().Modified)
	})

	t.Run("shuffled order stays consistent", func(t *testing.T) {
		q := mixed(0)
		q.SetShuffle(true)
		q.DropLibrary("gone")
		assert.ElementsMatch(t, []string{"a", "d", "e"}, ids(q))
		assert.Equal(t, "a", current(q))
		q.SetShuffle(false)
		assert.Equal(t, []string{"a", "d", "e"}, ids(q))
	})

	t.Run("everything dropped leaves an empty queue", func(t *testing.T) {
		q := NewQueue()
		ts := tracks("a")
		ts[0].Library = "gone"
		q.Replace(ts, 0, Origin{})
		_, dropped, none := q.DropLibrary("gone")
		assert.True(t, dropped)
		assert.True(t, none)
		assert.Equal(t, 0, q.Len())
		_, ok := q.Current()
		assert.False(t, ok)
	})
}
