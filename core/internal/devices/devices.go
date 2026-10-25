// Package devices picks where playback goes: the default output, one or
// several local outputs (PipeWire children), or a device another feature adds
// (Chromecast). The equalizer sits between mpv and the outputs.
package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/atomicfile"
)

const (
	Topic    = "devices"
	Default  = "default"
	fileName = "devices.json"
	// MaxDelayMs is the largest delay one output can get.
	MaxDelayMs = int(maxDelay * 1000)

	// What one of several outputs going away does: pause playback, or let
	// the others keep playing. A single output follows PauseOnDisconnect.
	DisconnectPause    = "pause"
	DisconnectContinue = "continue"
)

type Device struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Kind is default, speaker, usb, hdmi, bluetooth, headset or cast.
	Kind      string `json:"kind"`
	Available bool   `json:"available"`
	// OnDisconnect is pause or continue for several-outputs mode; local
	// outputs only.
	OnDisconnect string `json:"onDisconnect,omitempty"`
}

// State is what devices.list returns and the devices event carries.
type State struct {
	Devices []Device `json:"devices"`
	// Selected is one id, or several local outputs played at once.
	Selected []string `json:"selected"`
	// Delays are the user's per-output delays in ms, keyed by output id.
	Delays       map[string]int `json:"delays"`
	ResetOnStart bool           `json:"resetOnStart"`
	// PauseOnDisconnect pauses when the one output played on goes away;
	// off, the default output takes over.
	PauseOnDisconnect bool `json:"pauseOnDisconnect"`
	// CanMulti is false without PipeWire's tools.
	CanMulti bool `json:"canMulti"`
	TestTone bool `json:"testTone"`
	// Equalizer is off, on, casting (it can't reach a cast device) or
	// unavailable (no PipeWire, or it failed: Error says why).
	Equalizer string `json:"equalizer"`
	// Errors are the current problems; Error is their messages joined.
	Errors []Reason `json:"errors,omitempty"`
	Error  string   `json:"error,omitempty"`
}

// Reason is a problem: a stable Code the UI words, and Message in plain
// English. The underlying error goes to the log.
type Reason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Codes of Reason.
const (
	CodeOutputFailed = "outputFailed"
	CodeEQNoPipeWire = "eqNoPipeWire"
	CodeEQFailed     = "eqFailed"
	CodeEQNoServer   = "eqNoServer"
)

var (
	outputFailed = Reason{CodeOutputFailed, "Couldn't switch outputs"}
	eqNoPipeWire = Reason{CodeEQNoPipeWire, "Requires PipeWire (pipewire and pw-cli)"}
	eqFailed     = Reason{CodeEQFailed, "PipeWire filter failed to start"}
	eqNoServer   = Reason{CodeEQNoServer, "PipeWire isn't running"}
)

// Player is the part of the player this package drives.
type Player interface {
	AudioDevices(ctx context.Context) (json.RawMessage, error)
	SetAudioOutput(ctx context.Context, device string, env []string) error
	Pause(ctx context.Context) error
}

// Extra adds playback targets that aren't mpv outputs (the cast feature).
// Device ids must not collide with PipeWire node names (the cast feature
// uses "cast:<id>").
type Extra interface {
	Devices() []Device
	// Select hands playback to id, or takes it back with "" when the user
	// picks a local output again.
	Select(ctx context.Context, id string) error
}

// ExtrasChanged says the extra's device list changed. It never blocks.
func (s *Service) ExtrasChanged() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func (s *Service) extraFor(id string) Extra {
	if s.extra == nil {
		return nil
	}
	for _, d := range s.extra.Devices() {
		if d.ID == id {
			return s.extra
		}
	}
	return nil
}

// Delays and OnDisconnect are keyed by deviceKey, so they survive a
// Bluetooth device coming back under another profile suffix. Files from
// before PauseOnDisconnect lack it and load with the default, on.
type prefs struct {
	Selected          []string          `json:"selected"`
	ResetOnStart      bool              `json:"resetOnStart"`
	PauseOnDisconnect bool              `json:"pauseOnDisconnect"`
	Delays            map[string]int    `json:"delays"`
	OnDisconnect      map[string]string `json:"onDisconnect"`
}

var bluezNode = regexp.MustCompile(`^bluez_output\.([0-9A-Fa-f]{2}(?:_[0-9A-Fa-f]{2}){5})(?:\.|$)`)

// deviceKey is a node name that stays the same across sessions: a Bluetooth
// sink's name ends in a profile or index that can change, its address can't.
func deviceKey(node string) string {
	if m := bluezNode.FindStringSubmatch(node); m != nil {
		return "bluez:" + strings.ToUpper(m[1])
	}
	return node
}

func (p prefs) delays(ids []string) map[string]int {
	out := map[string]int{}
	for _, id := range ids {
		if id != Default {
			out[id] = p.Delays[deviceKey(id)]
		}
	}
	return out
}

// pauses says whether id going away pauses playback on sel: the global
// choice for one output, the output's own for several.
func (p prefs) pauses(id string, sel []string) bool {
	if len(sel) > 1 {
		return p.onDisconnect(id) == DisconnectPause
	}
	return p.PauseOnDisconnect
}

func (p prefs) onDisconnect(id string) string {
	if p.OnDisconnect[deviceKey(id)] == DisconnectContinue {
		return DisconnectContinue
	}
	return DisconnectPause
}

type Service struct {
	player   Player
	path     string
	publish  func(State)
	canMulti bool

	ops      chan func()
	listCh   chan json.RawMessage
	testDone chan struct{}
	ended    chan struct{}
	eqCh     chan EQ
	changed  chan struct{}
	extra    Extra

	// Owned by the loop.
	prefs    prefs
	local    []Device
	names    map[string]string
	selected []string
	casting  Extra
	multi    *multi
	eq       EQ
	eqc      *eqChild
	testTone bool
	err      Reason
	// eqErr says why the equalizer is on but not in the path.
	eqErr Reason

	mu    sync.Mutex
	state State
}

// New loads the saved choice. runDir holds the PipeWire child configs; eq is
// the equalizer as the settings have it; extra, when not nil, adds its
// devices to the list.
func New(p Player, configDir, runDir string, eq EQ, extra Extra, publish func(State)) *Service {
	s := &Service{
		player:   p,
		path:     filepath.Join(configDir, fileName),
		publish:  publish,
		canMulti: pipewireAvailable(),
		ops:      make(chan func()),
		listCh:   make(chan json.RawMessage, 1),
		testDone: make(chan struct{}, 1),
		ended:    make(chan struct{}, 1),
		eqCh:     make(chan EQ, 1),
		changed:  make(chan struct{}, 1),
		extra:    extra,
		eq:       eq,
		names:    map[string]string{},
		multi:    newMulti(runDir),
		prefs:    prefs{Selected: []string{Default}, ResetOnStart: true, PauseOnDisconnect: true, Delays: map[string]int{}, OnDisconnect: map[string]string{}},
	}
	if err := s.load(); err != nil {
		log.Warnf("devices: %v; using the default output", err)
	}
	return s
}

// Run applies the saved choice (or the default when reset on start is on)
// and serves until ctx ends, then stops the PipeWire children.
func (s *Service) Run(ctx context.Context) {
	defer s.multi.stop()
	defer s.stopEQ()
	if raw, err := s.player.AudioDevices(ctx); err == nil {
		s.onList(raw)
	}
	sel := s.current(s.prefs.Selected)
	if s.prefs.ResetOnStart {
		sel = []string{Default}
	}
	s.selected = []string{Default}
	if err := s.apply(ctx, sel); err != nil {
		log.Warnf("devices: %v", err)
		s.err = outputFailed
	}
	s.update()
	for {
		select {
		case <-ctx.Done():
			return
		case op := <-s.ops:
			op()
		case raw := <-s.listCh:
			if s.onList(raw) {
				if err := s.player.Pause(ctx); err != nil {
					log.Warnf("devices: pause on disconnect: %v", err)
				}
			}
			s.follow(ctx)
			s.update()
		case <-s.ended:
			if s.casting != nil {
				s.casting = nil
				if err := s.apply(ctx, s.prefs.Selected); err != nil {
					log.Warnf("devices: back from casting: %v", err)
				}
			}
			s.update()
		case e := <-s.eqCh:
			s.setEQ(ctx, e)
			s.update()
		case <-s.changed:
			s.update()
		case <-s.testDone:
			s.testTone = false
			s.update()
		}
	}
}

// OnDeviceList takes mpv's audio-device-list. It never blocks: the player
// loop publishes it, and the loop here may be waiting on the player.
func (s *Service) OnDeviceList(raw json.RawMessage) {
	select {
	case <-s.listCh:
	default:
	}
	s.listCh <- raw
}

// ExtraEnded says a cast (or other extra device) stopped by itself; the
// selection goes back to the saved local one. Like OnDeviceList it never
// blocks, since the cast feature may publish while this loop waits on it.
func (s *Service) ExtraEnded() {
	select {
	case s.ended <- struct{}{}:
	default:
	}
}

// OnEqualizer takes the equalizer settings after a change. Like
// OnDeviceList it never blocks; the newest settings win.
func (s *Service) OnEqualizer(e EQ) {
	select {
	case <-s.eqCh:
	default:
	}
	s.eqCh <- e
}

// setEQ retunes a running equalizer live; turning it on or off reroutes.
func (s *Service) setEQ(ctx context.Context, e EQ) {
	was := s.eq
	s.eq = e
	if was.On != e.On {
		if s.casting == nil {
			if err := s.apply(ctx, s.selected); err != nil {
				log.Warnf("devices: equalizer: %v", err)
				s.err = outputFailed
			}
		}
		return
	}
	if s.eqc != nil {
		s.eqc.retune(e.Gains)
	}
}

func (s *Service) stopEQ() {
	if s.eqc != nil {
		s.eqc.close()
		s.eqc = nil
	}
}

// viaEQ puts the equalizer in front of sink ("" is the system default) and
// returns the device and env mpv plays to then. With the equalizer off, or
// when it fails, mpv plays to device with env.
func (s *Service) viaEQ(ctx context.Context, sink, device string, env []string) (string, []string) {
	if !s.eq.On {
		return device, env
	}
	if !s.canMulti {
		s.eqErr = eqNoPipeWire
		return device, env
	}
	wait := env != nil
	if s.eqc == nil || s.eqc.sink != sink || s.eqc.wait != wait {
		// A new target is a new child; mpv's stream waits for it meanwhile.
		s.stopEQ()
		e, err := startEQ(ctx, s.multi.dir, sink, wait, s.eq.Gains)
		if err != nil {
			log.Warnf("devices: equalizer: %v", err)
			s.eqErr = eqFailed
			if errors.Is(err, errNoServer) {
				s.eqErr = eqNoServer
			}
			return device, env
		}
		s.eqc = e
	}
	s.eqc.retune(s.eq.Gains)
	return EQDevice, WaitEnv
}

func (s *Service) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Service) do(ctx context.Context, fn func(ctx context.Context) error) error {
	errc := make(chan error, 1)
	select {
	case s.ops <- func() {
		err := fn(ctx)
		s.update()
		errc <- err
	}:
	case <-ctx.Done():
		return ctx.Err()
	}
	return <-errc
}

// Select plays on ids: one device, or several local outputs at once.
func (s *Service) Select(ctx context.Context, ids []string) error {
	return s.do(ctx, func(ctx context.Context) error {
		if err := s.apply(ctx, ids); err != nil {
			return err
		}
		s.err = Reason{}
		if s.casting == nil {
			s.prefs.Selected = slices.Clone(s.selected)
			return s.save()
		}
		return nil
	})
}

// SetDelay sets how much later one output plays, applied live.
func (s *Service) SetDelay(ctx context.Context, id string, ms int) error {
	return s.do(ctx, func(ctx context.Context) error {
		if !validSink.MatchString(id) {
			return fmt.Errorf("%q is not a local output", id)
		}
		s.prefs.Delays[deviceKey(id)] = min(max(ms, 0), MaxDelayMs)
		if s.multi.running() {
			if err := s.multi.set(ctx, s.selected, normalize(s.selected, s.prefs.delays(s.selected))); err != nil {
				return err
			}
		}
		return s.save()
	})
}

// SetOnDisconnect picks what id going away does while it is one of several
// outputs: DisconnectPause or DisconnectContinue.
func (s *Service) SetOnDisconnect(ctx context.Context, id, mode string) error {
	return s.do(ctx, func(context.Context) error {
		if !validSink.MatchString(id) {
			return fmt.Errorf("%q is not a local output", id)
		}
		if mode != DisconnectPause && mode != DisconnectContinue {
			return fmt.Errorf("mode must be %s or %s", DisconnectPause, DisconnectContinue)
		}
		s.prefs.OnDisconnect[deviceKey(id)] = mode
		return s.save()
	})
}

// SetPauseOnDisconnect picks whether the one output played on going away
// pauses playback or lets the default output take over.
func (s *Service) SetPauseOnDisconnect(ctx context.Context, on bool) error {
	return s.do(ctx, func(ctx context.Context) error {
		s.prefs.PauseOnDisconnect = on
		if err := s.save(); err != nil {
			return err
		}
		// It decides whether mpv waits for the output or falls back.
		if s.casting == nil && len(s.selected) == 1 && s.selected[0] != Default {
			return s.apply(ctx, s.selected)
		}
		return nil
	})
}

func (s *Service) SetResetOnStart(ctx context.Context, on bool) error {
	return s.do(ctx, func(context.Context) error {
		s.prefs.ResetOnStart = on
		return s.save()
	})
}

// TestTone plays clicks on every output at once, so the delays can be
// matched by ear.
func (s *Service) TestTone(ctx context.Context) error {
	return s.do(ctx, func(ctx context.Context) error {
		if !s.multi.running() {
			return errors.New("the test tone is for playing on several outputs")
		}
		if s.testTone {
			return nil
		}
		wav := filepath.Join(s.multi.dir, "clicks.wav")
		if err := os.WriteFile(wav, clicks(), 0o600); err != nil {
			return err
		}
		s.testTone = true
		go func() {
			if out, err := pwRun(context.Background(), "pw-play", "--target", combineNode,
				"-P", "{ node.dont-fallback = true node.linger = true }", wav); err != nil {
				log.Warnf("test tone: %v: %s", err, strings.TrimSpace(out))
			}
			s.testDone <- struct{}{}
		}()
		return nil
	})
}

// apply switches playback to sel. Several outputs start the PipeWire
// children before mpv moves; leaving them moves mpv first, as the children's
// nodes are what mpv plays into.
func (s *Service) apply(ctx context.Context, sel []string) error {
	if len(sel) == 0 {
		return errors.New("pick at least one output")
	}
	if len(sel) == 1 {
		if e := s.extraFor(sel[0]); e != nil {
			if err := e.Select(ctx, sel[0]); err != nil {
				return err
			}
			s.casting, s.selected = e, slices.Clone(sel)
			return nil
		}
	}
	for _, id := range sel {
		if id != Default && !validSink.MatchString(id) || id == Default && len(sel) > 1 {
			return fmt.Errorf("%q can't be played on together with other outputs", id)
		}
	}
	if s.casting != nil {
		if err := s.casting.Select(ctx, ""); err != nil {
			log.Warnf("devices: stop casting: %v", err)
		}
		s.casting = nil
	}
	if len(sel) == 1 {
		sink, device, env := "", "auto", []string(nil)
		if sel[0] != Default {
			sink, device = sel[0], "pipewire/"+sel[0]
			// Waiting instead of falling back keeps a paused-on-disconnect
			// track from blaring out of the default output first.
			if s.prefs.PauseOnDisconnect {
				env = WaitEnv
			}
		}
		device, env = s.viaEQ(ctx, sink, device, env)
		if err := s.player.SetAudioOutput(ctx, device, env); err != nil {
			return err
		}
		s.multi.stop()
		if device != EQDevice {
			s.stopEQ()
		}
		s.selected = slices.Clone(sel)
		return nil
	}
	if !s.canMulti {
		return errNoPipeWire
	}
	if err := s.multi.set(ctx, sel, normalize(sel, s.prefs.delays(sel))); err != nil {
		s.fallBack(ctx)
		return err
	}
	device, env := s.viaEQ(ctx, combineNode, MultiDevice, WaitEnv)
	if err := s.player.SetAudioOutput(ctx, device, env); err != nil {
		s.fallBack(ctx)
		return err
	}
	if device != EQDevice {
		s.stopEQ()
	}
	s.selected = slices.Clone(sel)
	return nil
}

func (s *Service) fallBack(ctx context.Context) {
	if err := s.player.SetAudioOutput(ctx, "auto", nil); err != nil {
		log.Warnf("devices: back to the default output: %v", err)
	}
	s.multi.stop()
	s.stopEQ()
	s.selected = []string{Default}
}

type mpvDevice struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// localDevices keeps mpv's PipeWire sinks, minus our own nodes.
func localDevices(raw json.RawMessage) []Device {
	var list []mpvDevice
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	var out []Device
	for _, d := range list {
		name, ok := strings.CutPrefix(d.Name, "pipewire/")
		if !ok || name == "" || strings.HasPrefix(name, "dplayer_") || !validSink.MatchString(name) {
			continue
		}
		out = append(out, Device{ID: name, Name: d.Description, Kind: kindOf(name, d.Description), Available: true})
	}
	return out
}

func kindOf(name, desc string) string {
	n, d := strings.ToLower(name), strings.ToLower(desc)
	switch {
	case strings.HasPrefix(n, "bluez_"):
		return "bluetooth"
	case strings.Contains(n, "hdmi") || strings.Contains(d, "hdmi") || strings.Contains(d, "displayport"):
		return "hdmi"
	case strings.Contains(d, "headset") || strings.Contains(d, "headphone"):
		return "headset"
	case strings.Contains(n, "usb"):
		return "usb"
	}
	return "speaker"
}

// onList takes a new device list and says whether a chosen output that
// was there is gone and set to pause.
func (s *Service) onList(raw json.RawMessage) bool {
	was := map[string]bool{}
	for _, d := range s.local {
		was[d.ID] = true
	}
	s.local = localDevices(raw)
	now := map[string]bool{}
	for _, d := range s.local {
		s.names[d.ID] = d.Name
		now[d.ID] = true
	}
	if s.casting != nil {
		return false
	}
	for _, id := range s.selected {
		if was[id] && !now[id] && s.prefs.pauses(id, s.selected) {
			return true
		}
	}
	return false
}

// current maps chosen outputs that are gone to a present output with the
// same device key: a Bluetooth sink back under another suffix.
func (s *Service) current(ids []string) []string {
	out := slices.Clone(ids)
	for i, id := range out {
		if slices.ContainsFunc(s.local, func(d Device) bool { return d.ID == id }) {
			continue
		}
		for _, d := range s.local {
			if deviceKey(d.ID) == deviceKey(id) {
				out[i] = d.ID
				break
			}
		}
	}
	return out
}

// follow moves playback to a chosen output that came back under a new
// node name, and remembers the new name.
func (s *Service) follow(ctx context.Context) {
	if saved := s.current(s.prefs.Selected); !slices.Equal(saved, s.prefs.Selected) {
		s.prefs.Selected = saved
		if err := s.save(); err != nil {
			log.Warnf("devices: %v", err)
		}
	}
	if s.casting != nil {
		return
	}
	if sel := s.current(s.selected); !slices.Equal(sel, s.selected) {
		if err := s.apply(ctx, sel); err != nil {
			log.Warnf("devices: follow %v: %v", sel, err)
		}
	}
}

// update rebuilds the published state. A chosen output that is gone stays
// listed as unavailable; in several-outputs mode its stream waits and
// relinks by itself when the device returns.
func (s *Service) update() {
	devs := []Device{{ID: Default, Name: "System default", Kind: Default, Available: true}}
	for _, d := range s.local {
		d.OnDisconnect = s.prefs.onDisconnect(d.ID)
		devs = append(devs, d)
	}
	for _, id := range append(slices.Clone(s.selected), s.prefs.Selected...) {
		if id == Default || slices.ContainsFunc(devs, func(d Device) bool { return d.ID == id }) || s.extraFor(id) != nil {
			continue
		}
		name := s.names[id]
		if name == "" {
			name = id
		}
		devs = append(devs, Device{ID: id, Name: name, Kind: kindOf(id, name), OnDisconnect: s.prefs.onDisconnect(id)})
	}
	if s.extra != nil {
		devs = append(devs, s.extra.Devices()...)
	}
	eq := "off"
	switch {
	case !s.eq.On:
	case s.casting != nil:
		eq = "casting"
	case s.eqc != nil:
		eq = "on"
	default:
		eq = "unavailable"
	}
	var errs []Reason
	if s.err.Code != "" {
		errs = append(errs, s.err)
	}
	if eq == "unavailable" && s.eqErr.Code != "" {
		errs = append(errs, s.eqErr)
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Message
	}
	st := State{Devices: devs, Selected: slices.Clone(s.selected), Delays: s.prefs.delays(s.selected), ResetOnStart: s.prefs.ResetOnStart,
		PauseOnDisconnect: s.prefs.PauseOnDisconnect, CanMulti: s.canMulti, TestTone: s.testTone, Equalizer: eq, Errors: errs, Error: strings.Join(msgs, "; ")}
	s.mu.Lock()
	same := reflect.DeepEqual(st, s.state)
	s.state = st
	s.mu.Unlock()
	if !same {
		s.publish(st)
	}
}

func (s *Service) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	p := s.prefs
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("%s: %w", s.path, err)
	}
	if len(p.Selected) == 0 {
		p.Selected = []string{Default}
	}
	// Older files keyed delays by node name; rekeying keeps them.
	delays, modes := map[string]int{}, map[string]string{}
	for k, v := range p.Delays {
		delays[deviceKey(k)] = v
	}
	for k, v := range p.OnDisconnect {
		modes[deviceKey(k)] = v
	}
	p.Delays, p.OnDisconnect = delays, modes
	s.prefs = p
	return nil
}

func (s *Service) save() error {
	data, err := json.MarshalIndent(s.prefs, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(s.path, append(data, '\n'), 0o600)
}

// clicks is a 3 s, 48 kHz stereo WAV with a 2 ms click every half second.
func clicks() []byte {
	const rate, seconds = 48000, 3
	n := rate * seconds
	data := make([]byte, 44+n*4)
	le := func(off int, v uint32, size int) {
		for i := range size {
			data[off+i] = byte(v >> (8 * i))
		}
	}
	copy(data, "RIFF")
	le(4, uint32(36+n*4), 4)
	copy(data[8:], "WAVEfmt ")
	le(16, 16, 4)
	le(20, 1, 2)
	le(22, 2, 2)
	le(24, rate, 4)
	le(28, rate*4, 4)
	le(32, 4, 2)
	le(34, 16, 2)
	copy(data[36:], "data")
	le(40, uint32(n*4), 4)
	for i := range n {
		if i%(rate/2) < rate/500 {
			v := uint32(uint16(int16(8000)))
			le(44+i*4, v, 2)
			le(46+i*4, v, 2)
		}
	}
	return data
}
