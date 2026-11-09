package devices

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The combine must create streams only for our output filters. A rule that
// also matched its own streams fed back until the child ran out of fds.
func TestCombineRuleMatchesOnlyOutputFilters(t *testing.T) {
	m := regexp.MustCompile(`matches = \[ \{ media\.class = "Audio/Sink/Internal" node\.name = "~(.*)" \} \]`).FindStringSubmatch(combineConf)
	require.NotNil(t, m, "the rule must constrain the media class and the node name")
	re := regexp.MustCompile(strings.ReplaceAll(m[1], `\\`, `\`))
	for name, want := range map[string]bool{
		outPrefix + token("alsa_output.pci-0000_0e_00.4.iec958-stereo"): true,
		outPrefix + token("bluez_output.AA_BB.1"):                       true,
		outPrefix + token("x") + ".play":                                false,
		"output.dplayer_multi_" + outPrefix + token("x"):                false,
		combineNode:      false,
		eqNode:           false,
		eqNode + ".play": false,
	} {
		assert.Equal(t, want, re.MatchString(name), name)
	}
}

func TestNormalize(t *testing.T) {
	sinks := []string{"a", "b", "c"}
	assert.Equal(t, map[string]float64{"a": 0.05, "b": 0, "c": 0.4}, normalize(sinks, map[string]int{"a": 150, "b": 100, "c": 500}))
	assert.Equal(t, map[string]float64{"a": 0, "b": 0, "c": 0.12}, normalize(sinks, map[string]int{"c": 120}))
}

func TestLocalDevices(t *testing.T) {
	raw := json.RawMessage(`[
		{"name":"auto","description":"Autoselect device"},
		{"name":"pipewire","description":"Default (pipewire)"},
		{"name":"pipewire/alsa_output.usb-Elgato_Wave_3.analog-stereo","description":"Elgato Wave 3"},
		{"name":"pipewire/alsa_output.pci-0000_0c_00.1.hdmi-stereo","description":"Navi 31 HDMI/DP Audio"},
		{"name":"pipewire/bluez_output.AA_BB_CC.1","description":"WH-1000XM4"},
		{"name":"pipewire/alsa_output.pci-0000_0e_00.4.analog-stereo","description":"Headphones"},
		{"name":"pipewire/dplayer_multi","description":"Dux Player (several outputs)"},
		{"name":"pipewire/evil\"name","description":"x"},
		{"name":"pulse/alsa_output.usb-Elgato_Wave_3.analog-stereo","description":"Elgato Wave 3"},
		{"name":"alsa/sysdefault","description":"Default Audio Device"}
	]`)
	assert.Equal(t, []Device{
		{ID: "alsa_output.usb-Elgato_Wave_3.analog-stereo", Name: "Elgato Wave 3", Kind: "usb", Available: true},
		{ID: "alsa_output.pci-0000_0c_00.1.hdmi-stereo", Name: "Navi 31 HDMI/DP Audio", Kind: "hdmi", Available: true},
		{ID: "bluez_output.AA_BB_CC.1", Name: "WH-1000XM4", Kind: "bluetooth", Available: true},
		{ID: "alsa_output.pci-0000_0e_00.4.analog-stereo", Name: "Headphones", Kind: "headset", Available: true},
	}, localDevices(raw))
}

type fakePlayer struct {
	mu      sync.Mutex
	outputs []string
	env     []string
	pauses  int
	builtin bool
}

const speakerList = `[{"name":"pipewire/speaker","description":"Speaker"}]`

func (f *fakePlayer) AudioDevices(context.Context) (json.RawMessage, error) {
	return json.RawMessage(speakerList), nil
}

func (f *fakePlayer) SetAudioOutput(_ context.Context, device string, env []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outputs, f.env = append(f.outputs, device), env
	return nil
}

func (f *fakePlayer) Builtin() bool { return f.builtin }

func (f *fakePlayer) PauseDisconnected(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pauses++
	return nil
}

func (f *fakePlayer) paused() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pauses
}

func (f *fakePlayer) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.outputs) == 0 {
		return ""
	}
	return f.outputs[len(f.outputs)-1]
}

func run(t *testing.T, dir string) (*Service, *fakePlayer) {
	p := &fakePlayer{}
	s := New(p, dir, t.TempDir(), EQ{}, fakeCast{}, func(State) {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)
	require.Eventually(t, func() bool { return p.last() != "" }, time.Second, 5*time.Millisecond)
	return s, p
}

func TestChoiceSurvivesRestartUnlessResetOnStart(t *testing.T) {
	for _, reset := range []bool{true, false} {
		dir := t.TempDir()
		s, _ := run(t, dir)
		require.NoError(t, s.SetResetOnStart(context.Background(), reset))
		require.NoError(t, s.Select(context.Background(), []string{"speaker"}))

		_, p := run(t, dir)
		want := "pipewire/speaker"
		if reset {
			want = "auto"
		}
		assert.Equal(t, want, p.last(), "resetOnStart=%v", reset)
	}
}

func TestSelectRejects(t *testing.T) {
	s, _ := run(t, t.TempDir())
	for _, sel := range [][]string{nil, {`bad"name`}, {Default, "speaker"}} {
		assert.Error(t, s.Select(context.Background(), sel), "%v", sel)
	}
	assert.Equal(t, []string{Default}, s.State().Selected)
}

func TestBuiltinPlayerSkipsInternalSinks(t *testing.T) {
	p := &fakePlayer{builtin: true}
	s := New(p, t.TempDir(), t.TempDir(), EQ{On: true}, fakeCast{}, func(State) {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)
	require.Eventually(t, func() bool { return p.last() != "" }, time.Second, 5*time.Millisecond)

	assert.Equal(t, "auto", p.last())
	require.Eventually(t, func() bool { return s.State().Equalizer == "needsMpv" }, time.Second, 5*time.Millisecond)
	assert.ErrorIs(t, s.Select(ctx, []string{"speaker", "headset"}), errNeedsMPV)
}

func TestDefaultOutputGoingAwayPauses(t *testing.T) {
	const both = `[{"name":"pipewire/speaker","description":"Speaker"},{"name":"pipewire/headset","description":"Headset"}]`
	for _, tc := range []struct {
		name    string
		on      string
		pause   bool
		enabled bool
		pauses  int
	}{
		{name: "the default left", on: "headset", enabled: true, pauses: 1},
		{name: "another output left", on: "speaker", enabled: true},
		{name: "turned off", on: "headset"},
	} {
		s, p := run(t, t.TempDir())
		require.NoError(t, s.SetPauseOnDisconnect(context.Background(), tc.enabled))
		s.defaults.set(tc.on)
		s.OnDeviceList(json.RawMessage(both))
		require.Eventually(t, func() bool { return len(s.State().Devices) >= 3 }, time.Second, 5*time.Millisecond, tc.name)
		s.OnDeviceList(json.RawMessage(speakerList))
		pauses := func() int {
			p.mu.Lock()
			defer p.mu.Unlock()
			return p.pauses
		}
		if tc.pauses > 0 {
			require.Eventually(t, func() bool { return pauses() > 0 }, time.Second, 5*time.Millisecond, tc.name)
		} else {
			time.Sleep(50 * time.Millisecond)
		}
		assert.Equal(t, tc.pauses, pauses(), tc.name)
	}
}

func TestDefaultSinkLine(t *testing.T) {
	for line, want := range map[string]string{
		`update: id:0 key:'default.audio.sink' value:'{"name":"alsa_output.usb"}' type:'Spa:String:JSON'`:               "alsa_output.usb",
		`update: id:0 key:'default.configured.audio.sink' value:'{ "name": "alsa_output.usb" }' type:'Spa:String:JSON'`: "",
		`update: id:0 key:'default.audio.source' value:'{"name":"alsa_input"}' type:'Spa:String:JSON'`:                  "",
		`Found "default" metadata 39`: "",
	} {
		assert.Equal(t, want, defaultSinkIn(line), line)
	}
}

func TestMissingChoiceStaysListed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, fileName), []byte(`{"selected":["bluez_output.gone"],"resetOnStart":false}`), 0o600))
	s, _ := run(t, dir)
	gone := Device{ID: "bluez_output.gone", Name: "bluez_output.gone", Kind: "bluetooth", OnDisconnect: DisconnectPause}
	assert.Eventually(t, func() bool { return slices.Contains(s.State().Devices, gone) }, time.Second, 5*time.Millisecond)
}

func TestDeviceKey(t *testing.T) {
	for node, want := range map[string]string{
		"bluez_output.AA_BB_CC_DD_EE_FF.1":         "bluez:AA_BB_CC_DD_EE_FF",
		"bluez_output.aa_bb_cc_dd_ee_ff.a2dp-sink": "bluez:AA_BB_CC_DD_EE_FF",
		"bluez_output.AA_BB_CC_DD_EE_FF":           "bluez:AA_BB_CC_DD_EE_FF",
		"alsa_output.pci-0000_0e_00.4.iec958":      "alsa_output.pci-0000_0e_00.4.iec958",
	} {
		assert.Equal(t, want, deviceKey(node), node)
	}
}

// A Bluetooth speaker's delay comes back with it, whatever profile suffix
// its node has next time, and across restarts.
func TestDelayFollowsBluetoothDevice(t *testing.T) {
	dir := t.TempDir()
	s, _ := run(t, dir)
	require.NoError(t, s.SetDelay(context.Background(), "bluez_output.AA_BB_CC_DD_EE_FF.1", 120))
	s, _ = run(t, dir)
	assert.Equal(t, map[string]int{"bluez_output.AA_BB_CC_DD_EE_FF.2": 120, "speaker": 0},
		s.prefs.delays([]string{"bluez_output.AA_BB_CC_DD_EE_FF.2", "speaker"}))
}

// One output follows the global choice; the per-output choice counts only
// in several-outputs mode.
func TestDisconnect(t *testing.T) {
	for _, pause := range []bool{true, false} {
		s, p := run(t, t.TempDir())
		ctx := context.Background()
		require.NoError(t, s.SetOnDisconnect(ctx, "speaker", DisconnectContinue))
		require.NoError(t, s.SetPauseOnDisconnect(ctx, pause))
		require.NoError(t, s.Select(ctx, []string{"speaker"}))
		assert.Equal(t, pause, p.env != nil, "pause=%v: mpv waits instead of falling back", pause)

		s.OnDeviceList(json.RawMessage(`[]`))
		require.Eventually(t, func() bool { return !s.State().Devices[1].Available }, time.Second, 5*time.Millisecond)
		want := 0
		if pause {
			want = 1
		}
		assert.Equal(t, want, p.paused(), "pause=%v", pause)
		assert.Equal(t, pause, s.State().PauseOnDisconnect)
	}
	p := prefs{PauseOnDisconnect: false, OnDisconnect: map[string]string{"b": DisconnectContinue}}
	assert.True(t, p.pauses("a", []string{"a", "b"}), "several outputs: per output, pause by default")
	assert.False(t, p.pauses("b", []string{"a", "b"}))
	assert.False(t, p.pauses("a", []string{"a"}))
}

// A devices.json from before the global choice pauses, whatever per-device
// mode it kept.
func TestOldDevicesFilePausesOnDisconnect(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, fileName), []byte(`{"selected":["speaker"],"resetOnStart":false,"onDisconnect":{"speaker":"continue"}}`), 0o600))
	s, p := run(t, dir)
	assert.True(t, s.State().PauseOnDisconnect)
	assert.NotNil(t, p.env, "mpv waits for the output")
}

type fakeCast struct{}

func (fakeCast) Devices() []Device {
	return []Device{{ID: "cast:tv", Name: "TV", Kind: "cast", Available: true}}
}
func (fakeCast) Select(context.Context, string) error { return nil }

func TestCastEndReturnsToLocal(t *testing.T) {
	s, p := run(t, t.TempDir())
	ctx := context.Background()
	require.NoError(t, s.Select(ctx, []string{"speaker"}))
	require.NoError(t, s.Select(ctx, []string{"cast:tv"}))
	assert.Equal(t, []string{"cast:tv"}, s.State().Selected)
	p.mu.Lock()
	p.outputs = nil
	p.mu.Unlock()
	s.ExtraEnded()
	require.Eventually(t, func() bool { return slices.Equal(s.State().Selected, []string{"speaker"}) }, time.Second, 5*time.Millisecond)
	assert.Equal(t, "pipewire/speaker", p.last())
}

// A chosen Bluetooth output that comes back under another suffix is played
// on again and stays the choice, also for the next start.
func TestSelectionFollowsBluetoothDevice(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, fileName),
		[]byte(`{"selected":["bluez_output.AA_BB_CC_DD_EE_FF.1"],"resetOnStart":false}`), 0o600))
	s, p := run(t, dir)
	assert.Equal(t, "pipewire/bluez_output.AA_BB_CC_DD_EE_FF.1", p.last())

	s.OnDeviceList(json.RawMessage(`[{"name":"pipewire/bluez_output.AA_BB_CC_DD_EE_FF.2","description":"Speaker"}]`))
	require.Eventually(t, func() bool {
		return slices.Equal(s.State().Selected, []string{"bluez_output.AA_BB_CC_DD_EE_FF.2"})
	}, time.Second, 5*time.Millisecond)
	assert.Equal(t, "pipewire/bluez_output.AA_BB_CC_DD_EE_FF.2", p.last())
	assert.True(t, s.State().Devices[1].Available)
	assert.False(t, slices.ContainsFunc(s.State().Devices, func(d Device) bool { return d.ID == "bluez_output.AA_BB_CC_DD_EE_FF.1" }),
		"the old name isn't listed as a missing device")

	data, err := os.ReadFile(filepath.Join(dir, fileName))
	require.NoError(t, err)
	assert.Contains(t, string(data), "EE_FF.2")
}

// The equalizer's stream follows the default output unless it has a target,
// and waits for a vanished target only when asked to.
func TestEQConfTarget(t *testing.T) {
	for _, tc := range []struct {
		sink       string
		wait       bool
		want, deny []string
	}{
		{sink: "", deny: []string{"target.object", "dont-fallback", "dont-move"}},
		{sink: "spk", want: []string{`target.object = "spk"`}, deny: []string{"dont-fallback"}},
		{sink: combineNode, wait: true, want: []string{`target.object = "dplayer_multi"`, "node.dont-fallback = true node.linger = true"}},
	} {
		play := eqConf(tc.sink, tc.wait, [10]float64{})
		play = play[strings.Index(play, "playback.props"):]
		for _, w := range tc.want {
			assert.Contains(t, play, w, tc.sink)
		}
		for _, d := range tc.deny {
			assert.NotContains(t, play, d, tc.sink)
		}
	}
}

// A change goes to the muted copy of the bands first, never the heard one
// (PipeWire's biquads click when retuned), then the mixer crossfades to it.
func TestEQTune(t *testing.T) {
	want, quit := make(chan [10]float64), make(chan struct{})
	var mu sync.Mutex
	var writes []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		tune(want, quit, func(bands string, fade []string) error {
			mu.Lock()
			writes = append(append(writes, bands), fade...)
			mu.Unlock()
			return nil
		}, func() uint64 { return 1 })
	}()
	want <- [10]float64{6}
	want <- [10]float64{-3}
	want <- [10]float64{}
	close(quit)
	<-done

	mu.Lock()
	defer mu.Unlock()
	mix := regexp.MustCompile(`"mix:Gain 1" ([0-9.]+) "mix:Gain 2" ([0-9.]+)`)
	heard := 0
	for _, w := range writes {
		if m := mix.FindStringSubmatch(w); m != nil {
			a, _ := strconv.ParseFloat(m[1], 64)
			b, _ := strconv.ParseFloat(m[2], 64)
			assert.InDelta(t, 1, a+b, 1e-9, w)
			if a == 0 || b == 0 {
				heard = map[bool]int{true: 0, false: 1}[b == 0]
			}
			continue
		}
		assert.NotContains(t, w, `"`+eqChains[heard], "a write to the heard copy clicks: %s", w)
	}
	require.GreaterOrEqual(t, len(writes), 2*(1+eqFadeSteps))
	assert.Equal(t, chainParams(1, [10]float64{6}), writes[0])
	assert.Equal(t, mixParams(0, eqFadeSteps), writes[eqFadeSteps])
	assert.InDelta(t, 0.501, preamp([10]float64{6, -3}), 0.001, "boosts are cut ahead of the bands")
}

// The equalizer's status says when it can't apply: no PipeWire, or a cast.
func TestEQStatus(t *testing.T) {
	p := &fakePlayer{}
	s := New(p, t.TempDir(), t.TempDir(), EQ{On: true}, fakeCast{}, func(State) {})
	s.canMulti = false
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Run(ctx)
	require.Eventually(t, func() bool { return s.State().Equalizer == "unavailable" }, time.Second, 5*time.Millisecond)
	assert.Equal(t, []Reason{eqNoPipeWire}, s.State().Errors, "the UI shows why")
	assert.Equal(t, "auto", p.last(), "without PipeWire mpv plays straight to the output")
	require.NoError(t, s.Select(ctx, []string{"cast:tv"}))
	assert.Equal(t, "casting", s.State().Equalizer)
	s.OnEqualizer(EQ{})
	require.Eventually(t, func() bool { return s.State().Equalizer == "off" }, time.Second, 5*time.Millisecond)
}

// A restarted equalizer child comes up with copy a heard; the next change
// must go to b, whatever was heard before the crash.
func TestEQRetuneAfterRestart(t *testing.T) {
	want, quit := make(chan [10]float64), make(chan struct{})
	var gen atomic.Uint64
	gen.Store(1)
	writes := make(chan string, 100)
	done := make(chan struct{})
	go func() {
		defer close(done)
		tune(want, quit, func(bands string, _ []string) error { writes <- bands; return nil }, gen.Load)
	}()
	want <- [10]float64{6}
	assert.Equal(t, chainParams(1, [10]float64{6}), <-writes)
	gen.Add(1)
	want <- [10]float64{2}
	assert.Equal(t, chainParams(1, [10]float64{2}), <-writes, "after the restart a is heard")
	close(quit)
	<-done
}

// With PipeWire's tools installed but no PipeWire running, an equalizer
// that is on says so at once, and the error goes once it is off.
func TestEQWithoutPipeWireServer(t *testing.T) {
	bin := t.TempDir()
	for name, script := range map[string]string{
		"pw-cli":   "#!/bin/sh\necho 'Error: \"failed to connect: Host is down\"'\nexit 255\n",
		"pipewire": "#!/bin/sh\nexit 1\n",
		"pw-play":  "#!/bin/sh\nexit 1\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700))
	}
	t.Setenv("PATH", bin)
	s, _ := run(t, t.TempDir())
	require.True(t, s.canMulti)
	s.OnEqualizer(EQ{On: true})
	require.Eventually(t, func() bool { return s.State().Equalizer == "unavailable" }, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, []Reason{eqNoServer}, s.State().Errors)
	s.OnEqualizer(EQ{})
	require.Eventually(t, func() bool { return s.State().Equalizer == "off" }, time.Second, 5*time.Millisecond)
	assert.Empty(t, s.State().Error)
}

// One change runs one pw-cli: the bands, then every crossfade step, in
// order. A bands line that pw-cli ran before it knew the node goes again;
// its connect-time errors don't count.
func TestEQApplyOneProcess(t *testing.T) {
	for _, failFirst := range []bool{false, true} {
		bin := t.TempDir()
		logFile := filepath.Join(bin, "log")
		script := strings.NewReplacer("LOG", logFile, "FAIL", strconv.FormatBool(failFirst)).Replace(`#!/bin/sh
echo "start $*" >> "LOG"
if [ "$1" = info ]; then echo "id: 42"; exit 0; fi
echo 'Error: "unsupported type PipeWire:Interface:SecurityContext"'
echo ">> remote 0 is named 'pipewire-0'"
fail=FAIL
while read -r l; do
  echo "$l" >> "LOG"
  if [ "$fail" = true ]; then echo "Error: \"set-param: unknown global '42'\""; fail=false; fi
done
`)
		require.NoError(t, os.WriteFile(filepath.Join(bin, "pw-cli"), []byte(script), 0o700))
		t.Setenv("PATH", bin)

		e := &eqChild{sink: "spk", quit: make(chan struct{})}
		id := ""
		bands := chainParams(1, [10]float64{6})
		require.NoError(t, e.apply(&id, bands, fadeParams(0)), "failFirst=%v", failFirst)

		lookup, set := "start info "+eqNode, "set-param 42 Props "
		want := []string{lookup, "start ", set + bands}
		if failFirst {
			want = append(want, lookup, set+bands)
		}
		for k := 1; k <= eqFadeSteps; k++ {
			want = append(want, set+mixParams(0, k))
		}
		want = append(want, "quit")
		data, err := os.ReadFile(logFile)
		require.NoError(t, err)
		assert.Equal(t, want, strings.Split(strings.TrimSuffix(string(data), "\n"), "\n"), "failFirst=%v", failFirst)
	}
}

// A crossfade that stops partway must not count as done: the next change
// still goes to copy b. Quitting stops a running change at once.
func TestEQApplyInterrupted(t *testing.T) {
	sleepBin, err := exec.LookPath("sleep")
	require.NoError(t, err)
	for _, tc := range []struct {
		name, session string
		quit          bool
	}{
		{name: "pw-cli exits mid-fade", session: `n=0
while read -r l; do echo "$l" >> "LOG"; n=$((n+1)); [ $n -ge 4 ] && exit 0; done`},
		{name: "quit mid-fade", quit: true, session: "exec " + sleepBin + " 30"},
	} {
		bin := t.TempDir()
		logFile := filepath.Join(bin, "log")
		script := strings.ReplaceAll(`#!/bin/sh
if [ "$1" = info ]; then echo "id: 42"; exit 0; fi
echo ">> remote 0 is named 'pipewire-0'"
`+tc.session+"\n", "LOG", logFile)
		require.NoError(t, os.WriteFile(filepath.Join(bin, "pw-cli"), []byte(script), 0o700))
		t.Setenv("PATH", bin)
		e := &eqChild{sink: "spk", quit: make(chan struct{})}

		if tc.quit {
			id := ""
			returned := make(chan error, 1)
			go func() { returned <- e.apply(&id, chainParams(1, [10]float64{6}), fadeParams(0)) }()
			time.Sleep(eqSettle + 3*eqStepEvery)
			close(e.quit)
			select {
			case err := <-returned:
				assert.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("apply kept running after quit")
			}
			continue
		}

		want, done := make(chan [10]float64, 1), make(chan struct{})
		go func() {
			defer close(done)
			id := ""
			tune(want, e.quit, func(bands string, fade []string) error { return e.apply(&id, bands, fade) }, func() uint64 { return 1 })
		}()
		bandsLines := func() []string {
			data, _ := os.ReadFile(logFile)
			return regexp.MustCompile(`(?m)^set-param 42 Props \{ params = \[ "(a|b)pre`).FindAllString(string(data), -1)
		}
		want <- [10]float64{6}
		require.Eventually(t, func() bool { return len(bandsLines()) == 1 }, 2*time.Second, 5*time.Millisecond)
		want <- [10]float64{2}
		require.Eventually(t, func() bool { return len(bandsLines()) == 2 }, 2*time.Second, 5*time.Millisecond)
		close(e.quit)
		<-done
		for _, l := range bandsLines() {
			assert.Contains(t, l, `"bpre`, "%s: a half-done crossfade left copy a heard", tc.name)
		}
	}
}
