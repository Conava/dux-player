package mpris

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

type fakePlayer struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakePlayer) rec(c string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	return nil
}

func (f *fakePlayer) has(c string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.calls {
		if x == c {
			return true
		}
	}
	return false
}

func (f *fakePlayer) Resume(context.Context) error   { return f.rec("resume") }
func (f *fakePlayer) Pause(context.Context) error    { return f.rec("pause") }
func (f *fakePlayer) Toggle(context.Context) error   { return f.rec("toggle") }
func (f *fakePlayer) Stop(context.Context) error     { return f.rec("stop") }
func (f *fakePlayer) Next(context.Context) error     { return f.rec("next") }
func (f *fakePlayer) Previous(context.Context) error { return f.rec("previous") }
func (f *fakePlayer) Seek(_ context.Context, pos float64, rel bool) error {
	if rel {
		return f.rec("seek+" + ftoa(pos))
	}
	return f.rec("seek=" + ftoa(pos))
}
func (f *fakePlayer) SetVolume(_ context.Context, v float64, _ bool) error {
	return f.rec("volume=" + ftoa(v))
}
func (f *fakePlayer) SetShuffle(_ context.Context, on bool) error {
	return f.rec(fmt.Sprintf("shuffle=%t", on))
}
func (f *fakePlayer) SetRepeat(_ context.Context, m string) error { return f.rec("repeat=" + m) }

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// privateBus starts a dbus-daemon of its own, without service activation,
// so the test never reaches the desktop's session or starts a service.
func privateBus(t *testing.T) string {
	bin, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon not installed")
	}
	dir := t.TempDir()
	conf := filepath.Join(dir, "bus.conf")
	require.NoError(t, os.WriteFile(conf, []byte(`<busconfig><type>session</type><listen>unix:dir=`+dir+`</listen>
<policy context="default"><allow send_destination="*" eavesdrop="true"/><allow eavesdrop="true"/><allow own="*"/></policy></busconfig>`), 0o600))
	cmd := exec.Command(bin, "--config-file="+conf, "--nofork", "--nopidfile", "--print-address=1")
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr, err := bufio.NewReader(out).ReadString('\n')
	require.NoError(t, err)
	return strings.TrimSpace(addr)
}

func connect(t *testing.T, addr string) *dbus.Conn {
	c, err := dbus.Connect(addr)
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })
	return c
}

func TestMpris(t *testing.T) {
	addr := privateBus(t)
	fp := &fakePlayer{}
	srv, err := Start(context.Background(), connect(t, addr), Config{Player: fp,
		Cover: func(context.Context, source.Track) (string, error) { return "/tmp/cover art.jpg", nil }})
	require.NoError(t, err)

	client := connect(t, addr)
	obj := client.Object(BusName, path)
	get := func(name string) any {
		v, err := obj.GetProperty(ifacePlayer + "." + name)
		require.NoError(t, err)
		return v.Value()
	}
	require.NoError(t, client.AddMatchSignal(dbus.WithMatchInterface(ifacePlayer), dbus.WithMatchMember("Seeked")))
	signals := make(chan *dbus.Signal, 10)
	client.Signal(signals)

	track := &source.Track{Library: "lib", ID: "1", Title: "Dawn", Artist: "Alpha", Album: "Day", TrackNo: 2, Duration: 200, CoverID: "c1"}
	srv.Update(player.Status{State: player.StatePlaying, Position: 10, Duration: 200, Volume: 40, Index: 0, Length: 2,
		Track: track, Shuffle: false, Repeat: player.RepeatOne})

	assert.Equal(t, "Playing", get("PlaybackStatus"))
	assert.Equal(t, "Track", get("LoopStatus"))
	assert.Equal(t, false, get("Shuffle"))
	assert.InDelta(t, 0.4, get("Volume"), 1e-9)
	assert.Equal(t, true, get("CanGoNext"))
	assert.Equal(t, int64(10e6), get("Position"))
	require.Eventually(t, func() bool {
		md := get("Metadata").(map[string]dbus.Variant)
		return md["mpris:artUrl"].Value() == "file:///tmp/cover%20art.jpg"
	}, time.Second, 10*time.Millisecond)
	md := get("Metadata").(map[string]dbus.Variant)
	assert.Equal(t, "Dawn", md["xesam:title"].Value())
	assert.Equal(t, []string{"Alpha"}, md["xesam:artist"].Value())
	assert.Equal(t, int64(200e6), md["mpris:length"].Value())
	id := md["mpris:trackid"].Value().(dbus.ObjectPath)

	// Steady progress is not a seek; a jump is.
	srv.UpdatePosition(10.5)
	srv.UpdatePosition(120)
	select {
	case sig := <-signals:
		assert.Equal(t, int64(120e6), sig.Body[0])
	case <-time.After(time.Second):
		t.Fatal("no Seeked signal")
	}
	assert.Empty(t, signals)

	call := func(m string, args ...any) { require.NoError(t, obj.Call(ifacePlayer+"."+m, 0, args...).Err) }
	call("PlayPause")
	call("Seek", int64(-5e6))
	call("SetPosition", id, int64(30e6))
	call("SetPosition", dbus.ObjectPath("/stale"), int64(40e6))
	call("SetPosition", id, int64(300e6))
	require.NoError(t, obj.SetProperty(ifacePlayer+".LoopStatus", dbus.MakeVariant("Playlist")))
	require.NoError(t, obj.SetProperty(ifacePlayer+".Shuffle", dbus.MakeVariant(true)))
	require.NoError(t, obj.SetProperty(ifacePlayer+".Volume", dbus.MakeVariant(0.25)))
	for _, c := range []string{"toggle", "seek+-5", "seek=30", "repeat=all", "shuffle=true", "volume=25"} {
		assert.Eventually(t, func() bool { return fp.has(c) }, time.Second, 5*time.Millisecond, c)
	}
	assert.False(t, fp.has("seek=40"), "stale track id")
	assert.False(t, fp.has("seek=300"), "past the end")
}
