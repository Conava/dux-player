package tray

import (
	"context"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePlayer struct{ calls chan string }

func (f fakePlayer) Toggle(context.Context) error   { f.calls <- "toggle"; return nil }
func (f fakePlayer) Next(context.Context) error     { f.calls <- "next"; return nil }
func (f fakePlayer) Previous(context.Context) error { f.calls <- "previous"; return nil }
func (f fakePlayer) SetVolume(_ context.Context, v float64, rel bool) error {
	f.calls <- "volume"
	return nil
}

func newTestTray() (*Tray, chan string) {
	calls := make(chan string, 4)
	t := &Tray{ctx: context.Background(), labels: defaultLabels, cfg: Config{
		Player: fakePlayer{calls},
		Show:   func() { calls <- "show" },
		Quit:   func() { calls <- "quit" },
	}}
	return t, calls
}

func labels(root menuItem) []string {
	var out []string
	for _, c := range root.Children {
		it := c.Value().(menuItem)
		if l, ok := it.Props["label"]; ok {
			out = append(out, l.Value().(string))
		} else {
			out = append(out, "--")
		}
	}
	return out
}

func TestLayoutFollowsPlayState(t *testing.T) {
	tr, _ := newTestTray()
	m := menuMethods{tr}
	_, root, err := m.GetLayout(0, -1, nil)
	require.Nil(t, err)
	assert.Equal(t, []string{"Open Dux Player", "Play", "Next", "Previous", "--", "Quit Dux Player"}, labels(root))

	tr.playing = true
	_, root, _ = m.GetLayout(0, -1, nil)
	assert.Equal(t, "Pause", labels(root)[1])

	_, root, _ = m.GetLayout(0, 0, nil)
	assert.Empty(t, root.Children)
}

func TestMenuEventDispatch(t *testing.T) {
	for _, tc := range []struct {
		id   int32
		want string
	}{{idOpen, "show"}, {idToggle, "toggle"}, {idNext, "next"}, {idPrevious, "previous"}, {idQuit, "quit"}} {
		tr, calls := newTestTray()
		require.Nil(t, menuMethods{tr}.Event(tc.id, "clicked", dbus.MakeVariant(""), 0))
		select {
		case got := <-calls:
			assert.Equal(t, tc.want, got)
		case <-time.After(time.Second):
			t.Fatalf("item %d: nothing happened", tc.id)
		}
	}
}
