package pendingmarks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func fav(v bool) *bool { return &v }
func stars(v int) *int { return &v }

// What a server receives once it's back, and what still waits, after
// changes made while it was unreachable. The queue is reopened before the
// flush, as after a restart.
func TestQueuedMarksMergeAndFlush(t *testing.T) {
	refuse := errors.New("server error")
	tests := []struct {
		name    string
		changes []Change
		online  map[string]bool
		fail    error
		// during is added while the first change is being sent.
		during *Change
		sent   []Change
		left   []Change
	}{
		{
			name:    "latest favorite wins",
			changes: []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(true)}, {Library: "s", Kind: "track", ID: "1", Favorite: fav(false)}},
			online:  map[string]bool{"s": true},
			sent:    []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(false)}},
		},
		{
			name:    "favorite and rating of one item merge",
			changes: []Change{{Library: "s", Kind: "album", ID: "a", Favorite: fav(true)}, {Library: "s", Kind: "album", ID: "a", Rating: stars(4)}},
			online:  map[string]bool{"s": true},
			sent:    []Change{{Library: "s", Kind: "album", ID: "a", Favorite: fav(true), Rating: stars(4)}},
		},
		{
			name:    "still offline waits",
			changes: []Change{{Library: "s", Kind: "track", ID: "1", Rating: stars(2)}},
			online:  map[string]bool{"s": false},
			left:    []Change{{Library: "s", Kind: "track", ID: "1", Rating: stars(2)}},
		},
		{
			name:    "removed library is dropped",
			changes: []Change{{Library: "gone", Kind: "track", ID: "1", Rating: stars(2)}},
			online:  map[string]bool{},
		},
		{
			name:    "refused change stays and counts the try",
			changes: []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(true)}},
			online:  map[string]bool{"s": true},
			fail:    refuse,
			sent:    []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(true)}},
			left:    []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(true), Tries: 1}},
		},
		{
			name:    "server gone again counts no try",
			changes: []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(true)}},
			online:  map[string]bool{"s": true},
			fail:    fmt.Errorf("%w: timeout", ErrUnreachable),
			sent:    []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(true)}},
			left:    []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(true)}},
		},
		{
			name:    "change made during the send is kept",
			changes: []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(true)}},
			online:  map[string]bool{"s": true},
			during:  &Change{Library: "s", Kind: "track", ID: "1", Favorite: fav(false)},
			sent:    []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(true)}},
			left:    []Change{{Library: "s", Kind: "track", ID: "1", Favorite: fav(false)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			q := Open(path)
			for _, c := range tt.changes {
				require.NoError(t, q.Add(c))
			}
			q = Open(path)
			var sent []Change
			q.Flush(context.Background(), tt.online, func(_ context.Context, c Change) error {
				if tt.during != nil && len(sent) == 0 {
					require.NoError(t, q.Add(*tt.during))
				}
				c.seq = 0
				sent = append(sent, c)
				return tt.fail
			})
			require.Equal(t, tt.sent, sent)
			var left []Change
			for _, c := range Open(path).items {
				c.seq = 0
				left = append(left, c)
			}
			require.Equal(t, tt.left, left)
		})
	}
}

// Marks belong to the account they were made with: after an account change
// none reach the new one, not even from a flush that is already running.
func TestDropLibrary(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	q := Open(path)
	for _, c := range []Change{
		{Library: "old", Kind: "track", ID: "1", Favorite: fav(true)},
		{Library: "old", Kind: "track", ID: "2", Favorite: fav(true)},
		{Library: "other", Kind: "track", ID: "3", Favorite: fav(true)},
	} {
		require.NoError(t, q.Add(c))
	}
	var sent []string
	q.Flush(context.Background(), map[string]bool{"old": true}, func(_ context.Context, c Change) error {
		sent = append(sent, c.ID)
		q.DropLibrary("old")
		return nil
	})
	require.Len(t, sent, 1)

	require.NoError(t, q.Add(Change{Library: "old", Kind: "track", ID: "4", Favorite: fav(true)}))
	q.DropLibrary("old")
	reopened := Open(path)
	require.False(t, reopened.Has("old", "track", "4"))
}
