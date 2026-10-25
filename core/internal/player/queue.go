package player

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"

	"github.com/Conava/dux-player/core/internal/source"
)

// Item is one queue entry. UID stays the same while the item is moved, so an
// mpv playlist entry can be matched back to it after edits.
type Item struct {
	UID   uint64       `json:"uid"`
	Track source.Track `json:"track"`
	// Added marks an item the user put in by hand (play next, add to queue).
	Added bool `json:"added,omitempty"`
}

// Origin says what the queue was started from, for "Continue listening".
// Kind is track, album, playlist, library, favorites or adhoc. Modified turns
// true once the user edits the queue (play next, reorder, remove).
type Origin struct {
	Kind     string `json:"kind"`
	Library  string `json:"library,omitempty"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Modified bool   `json:"modified"`
}

const (
	RepeatOff = "off"
	RepeatAll = "all"
	RepeatOne = "one"
)

// Queue is the play order. cur is -1 when nothing is selected. While
// shuffled, orig keeps the unshuffled order as UIDs so shuffle off can
// restore it.
type Queue struct {
	items   []Item
	cur     int
	nextUID uint64
	origin  Origin
	shuffle bool
	repeat  string
	orig    []uint64
	// skip, when set, says which items can't play now; Peek passes them.
	skip func(Item) bool
}

func NewQueue() *Queue { return &Queue{cur: -1, repeat: RepeatOff} }

func newQueue(r Resolver) *Queue {
	q := NewQueue()
	if a, ok := r.(Availability); ok {
		q.skip = func(it Item) bool { return a.Skip(it.Track.Ref()) }
	}
	return q
}

func (q *Queue) wrap(tracks []source.Track) []Item {
	out := make([]Item, len(tracks))
	for i, t := range tracks {
		q.nextUID++
		out[i] = Item{UID: q.nextUID, Track: t}
	}
	return out
}

// Replace swaps the whole queue, selects start and shuffles around it when
// shuffle is on.
func (q *Queue) Replace(tracks []source.Track, start int, origin Origin) {
	q.items = q.wrap(tracks)
	q.cur = -1
	if start >= 0 && start < len(q.items) {
		q.cur = start
	}
	q.origin = origin
	q.orig = nil
	if q.shuffle {
		q.reshuffle()
	}
}

func insertAt[T any](s []T, at int, v ...T) []T {
	return append(s[:at], append(v, s[at:]...)...)
}

func (q *Queue) origIndex(uid uint64) int {
	for i, u := range q.orig {
		if u == uid {
			return i
		}
	}
	return -1
}

// Add appends tracks the user picked (marked Added), or inserts them right
// after the current item when next is set. An empty queue selects the first
// added item.
func (q *Queue) Add(tracks []source.Track, next bool) {
	q.add(tracks, next, true)
}

func (q *Queue) add(tracks []source.Track, next, byHand bool) {
	items := q.wrap(tracks)
	for i := range items {
		items[i].Added = byHand
	}
	at := len(q.items)
	if next && q.cur >= 0 {
		at = q.cur + 1
	}
	if q.orig != nil {
		uids := make([]uint64, len(items))
		for i, it := range items {
			uids[i] = it.UID
		}
		oat := len(q.orig)
		if cur, ok := q.Current(); ok && next {
			oat = q.origIndex(cur.UID) + 1
		}
		q.orig = insertAt(q.orig, oat, uids...)
	}
	q.items = insertAt(q.items, at, items...)
	if q.cur < 0 && len(q.items) > 0 {
		q.cur = 0
	}
	q.origin.Modified = q.origin.Modified || len(q.items) > len(items)
}

// Grow puts tracks of the list being played that weren't there yet in place:
// before ahead of the current item, after at the end. While shuffled they
// land at random places after the current item and keep their list place in
// the unshuffled order. Not an edit, and not added by hand.
func (q *Queue) Grow(before, after []source.Track) {
	if q.orig != nil {
		pre, post := q.wrap(before), q.wrap(after)
		orig := make([]uint64, 0, len(pre)+len(q.orig)+len(post))
		for _, it := range pre {
			orig = append(orig, it.UID)
		}
		orig = append(orig, q.orig...)
		for _, it := range post {
			orig = append(orig, it.UID)
		}
		q.orig = orig
		for _, it := range append(pre, post...) {
			q.items = insertAt(q.items, q.cur+1+rand.IntN(len(q.items)-q.cur), it)
		}
		if q.cur < 0 && len(q.items) > 0 {
			q.cur = 0
		}
		return
	}
	modified := q.origin.Modified
	items := q.wrap(before)
	q.items = insertAt(q.items, 0, items...)
	if q.cur >= 0 {
		q.cur += len(items)
	}
	q.add(after, false, false)
	q.origin.Modified = modified
}

// Extend appends tracks the way a growing shuffle does: not an edit, so the
// origin stays unmodified.
func (q *Queue) Extend(tracks []source.Track) {
	modified := q.origin.Modified
	q.add(tracks, false, false)
	q.origin.Modified = modified
}

// Remove drops item i and reports whether it was the current one. The
// following item then becomes current; past the end, the last one.
func (q *Queue) Remove(i int) (bool, error) {
	if i < 0 || i >= len(q.items) {
		return false, fmt.Errorf("queue index %d out of range", i)
	}
	if oi := q.origIndex(q.items[i].UID); oi >= 0 {
		q.orig = append(q.orig[:oi], q.orig[oi+1:]...)
	}
	q.items = append(q.items[:i], q.items[i+1:]...)
	wasCurrent := i == q.cur
	switch {
	case i < q.cur:
		q.cur--
	case wasCurrent && q.cur >= len(q.items):
		q.cur = len(q.items) - 1
	}
	q.origin.Modified = true
	return wasCurrent, nil
}

// Move puts item from at position to and keeps the current item selected.
func (q *Queue) Move(from, to int) error {
	n := len(q.items)
	if from < 0 || from >= n || to < 0 || to >= n {
		return fmt.Errorf("queue move %d→%d out of range", from, to)
	}
	if from == to {
		return nil
	}
	curUID := q.currentUID()
	it := q.items[from]
	q.items = append(q.items[:from], q.items[from+1:]...)
	q.items = insertAt(q.items, to, it)
	q.selectUID(curUID)
	q.origin.Modified = true
	return nil
}

func (q *Queue) currentUID() uint64 {
	if it, ok := q.Current(); ok {
		return it.UID
	}
	return 0
}

func (q *Queue) selectUID(uid uint64) {
	for i, x := range q.items {
		if x.UID == uid {
			q.cur = i
			return
		}
	}
}

// KeepOnly drops every item but uid, which stays current.
func (q *Queue) KeepOnly(uid uint64) {
	for _, it := range q.items {
		if it.UID == uid {
			q.items, q.cur = []Item{it}, 0
			if q.orig != nil {
				q.orig = []uint64{uid}
			}
			q.origin.Modified = true
			return
		}
	}
}

func (q *Queue) Clear() {
	q.items = nil
	q.orig = nil
	q.cur = -1
	q.origin = Origin{}
}

func (q *Queue) Jump(i int) error {
	if i < 0 || i >= len(q.items) {
		return fmt.Errorf("queue index %d out of range", i)
	}
	q.cur = i
	return nil
}

func (q *Queue) Current() (Item, bool) {
	if q.cur < 0 || q.cur >= len(q.items) {
		return Item{}, false
	}
	return q.items[q.cur], true
}

// Peek is the item that plays after the current one, passing over items
// that can't play; with repeat all the first item follows the last.
func (q *Queue) Peek() (Item, bool) {
	if q.cur < 0 {
		return Item{}, false
	}
	for i := 1; i <= len(q.items); i++ {
		j := q.cur + i
		if j >= len(q.items) {
			if q.repeat != RepeatAll {
				break
			}
			j %= len(q.items)
		}
		if q.skip == nil || !q.skip(q.items[j]) {
			return q.items[j], true
		}
	}
	return Item{}, false
}

// Back moves to the closest earlier item that can play; false when there
// is none.
func (q *Queue) Back() bool {
	for j := q.cur - 1; j >= 0; j-- {
		if q.skip == nil || !q.skip(q.items[j]) {
			q.cur = j
			return true
		}
	}
	return false
}

// Advance moves to the item Peek returns; false at the end of the queue.
func (q *Queue) Advance() bool {
	it, ok := q.Peek()
	if !ok {
		return false
	}
	q.selectUID(it.UID)
	return true
}

// Mark sets the favorite and/or rating of every entry of ref; false when the
// queue doesn't hold it or nothing changed.
func (q *Queue) Mark(ref source.Ref, favorite *bool, rating *int) bool {
	changed := false
	for i := range q.items {
		t := &q.items[i].Track
		if t.Library != ref.Library || t.ID != ref.ID {
			continue
		}
		if favorite != nil && t.Favorite != *favorite {
			t.Favorite, changed = *favorite, true
		}
		if rating != nil && t.Rating != *rating {
			t.Rating, changed = *rating, true
		}
	}
	return changed
}

func (q *Queue) Index() int { return q.cur }

func (q *Queue) Len() int { return len(q.items) }

func (q *Queue) Items() []Item { return append([]Item{}, q.items...) }

func (q *Queue) Origin() Origin { return q.origin }

func (q *Queue) Shuffle() bool { return q.shuffle }

func (q *Queue) Repeat() string { return q.repeat }

func (q *Queue) SetRepeat(mode string) error {
	switch mode {
	case RepeatOff, RepeatAll, RepeatOne:
		q.repeat = mode
		return nil
	}
	return fmt.Errorf("repeat mode %q is not off, all or one", mode)
}

// SetShuffle reorders the queue. The current item stays current; turning
// shuffle off restores the order the queue had before, edits included.
func (q *Queue) SetShuffle(on bool) {
	if q.orig != nil {
		q.unshuffle()
	}
	q.shuffle = on
	if on {
		q.reshuffle()
	}
}

func (q *Queue) unshuffle() {
	byUID := make(map[uint64]Item, len(q.items))
	for _, it := range q.items {
		byUID[it.UID] = it
	}
	curUID := q.currentUID()
	items := make([]Item, 0, len(q.items))
	for _, uid := range q.orig {
		if it, ok := byUID[uid]; ok {
			items = append(items, it)
		}
	}
	q.items = items
	q.orig = nil
	q.selectUID(curUID)
}

// reshuffle shuffles everything but keeps the current item first.
func (q *Queue) reshuffle() {
	q.orig = make([]uint64, len(q.items))
	for i, it := range q.items {
		q.orig[i] = it.UID
	}
	curUID := q.currentUID()
	rand.Shuffle(len(q.items), func(i, j int) { q.items[i], q.items[j] = q.items[j], q.items[i] })
	if curUID != 0 {
		q.selectUID(curUID)
		q.items[0], q.items[q.cur] = q.items[q.cur], q.items[0]
		q.cur = 0
	} else if len(q.items) > 0 {
		q.cur = 0
	}
}

// QueueState is the queue as it is saved and restored.
type QueueState struct {
	Items   []Item    `json:"items"`
	Index   int       `json:"index"`
	Origin  Origin    `json:"origin"`
	Shuffle shuffleOn `json:"shuffle"`
	Repeat  string    `json:"repeat"`
	Orig    []uint64  `json:"unshuffled,omitempty"`
}

// shuffleOn reads a saved shuffle: a bool, or the mode of state version 1,
// where "tracks" and "albums" were the two ways of on.
type shuffleOn bool

func (s *shuffleOn) UnmarshalJSON(b []byte) error {
	var mode string
	if json.Unmarshal(b, &mode) == nil {
		*s = shuffleOn(mode != "" && mode != "off")
		return nil
	}
	return json.Unmarshal(b, (*bool)(s))
}

func (q *Queue) State() QueueState {
	return QueueState{Items: q.Items(), Index: q.cur, Origin: q.origin, Shuffle: shuffleOn(q.shuffle), Repeat: q.repeat,
		Orig: append([]uint64(nil), q.orig...)}
}

// Restore loads a saved queue. Bad modes and indexes fall back to defaults.
func (q *Queue) Restore(s QueueState) {
	q.items = append([]Item{}, s.Items...)
	q.nextUID = 0
	for _, it := range q.items {
		q.nextUID = max(q.nextUID, it.UID)
	}
	q.cur = -1
	if s.Index >= 0 && s.Index < len(q.items) {
		q.cur = s.Index
	}
	q.origin = s.Origin
	q.shuffle, q.repeat, q.orig = bool(s.Shuffle), RepeatOff, nil
	_ = q.SetRepeat(s.Repeat)
	if !q.shuffle {
		return
	}
	if len(s.Orig) == len(q.items) {
		q.orig = append([]uint64{}, s.Orig...)
		return
	}
	q.orig = make([]uint64, len(q.items))
	for i, it := range q.items {
		q.orig[i] = it.UID
	}
}
