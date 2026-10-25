// Package mpris shows the player on D-Bus as org.mpris.MediaPlayer2.dplayer,
// for media keys and the shell's media widget.
package mpris

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"net/url"
	"reflect"
	"sync"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"

	"github.com/Conava/dux-player/core/internal/player"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	BusName     = "org.mpris.MediaPlayer2.dplayer"
	path        = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	ifaceRoot   = "org.mpris.MediaPlayer2"
	ifacePlayer = "org.mpris.MediaPlayer2.Player"
	noTrack     = dbus.ObjectPath("/org/mpris/MediaPlayer2/TrackList/NoTrack")
	// A position this far off where steady playback would be is a seek.
	seekSlack  = 1.5
	cmdTimeout = 10 * time.Second
)

// Player is what the MPRIS controls drive.
type Player interface {
	Resume(ctx context.Context) error
	Pause(ctx context.Context) error
	Toggle(ctx context.Context) error
	Stop(ctx context.Context) error
	Next(ctx context.Context) error
	Previous(ctx context.Context) error
	Seek(ctx context.Context, pos float64, relative bool) error
	SetVolume(ctx context.Context, v float64, relative bool) error
	SetShuffle(ctx context.Context, on bool) error
	SetRepeat(ctx context.Context, mode string) error
}

type Config struct {
	Player Player
	// Cover returns a local file for the track's art, or "".
	Cover func(ctx context.Context, t source.Track) (string, error)
	Raise func()
	Quit  func()
}

type Server struct {
	conn  *dbus.Conn
	props *prop.Properties
	cfg   Config
	ctx   context.Context

	mu      sync.Mutex
	setMu   sync.Mutex
	trackID dbus.ObjectPath
	track   *source.Track
	artURL  string
	length  float64
	pos     float64
	posAt   time.Time
	playing bool
}

// Start exports the player on conn and claims the bus name. Player
// commands run on ctx.
func Start(ctx context.Context, conn *dbus.Conn, cfg Config) (*Server, error) {
	s := &Server{conn: conn, cfg: cfg, ctx: ctx, trackID: noTrack, posAt: time.Now()}
	ro := func(v any) *prop.Prop { return &prop.Prop{Value: v, Emit: prop.EmitTrue} }
	rw := func(v any, cb func(*prop.Change) *dbus.Error) *prop.Prop {
		return &prop.Prop{Value: v, Writable: true, Emit: prop.EmitTrue, Callback: cb}
	}
	props, err := prop.Export(conn, path, prop.Map{
		ifaceRoot: {
			"Identity":            ro("Dux Player"),
			"DesktopEntry":        ro("com.marlonkranz.player"),
			"CanQuit":             ro(cfg.Quit != nil),
			"CanRaise":            ro(cfg.Raise != nil),
			"HasTrackList":        ro(false),
			"SupportedUriSchemes": ro([]string{}),
			"SupportedMimeTypes":  ro([]string{}),
		},
		ifacePlayer: {
			"PlaybackStatus": ro("Stopped"),
			"LoopStatus":     rw("None", s.setLoop),
			"Shuffle":        rw(false, s.setShuffle),
			"Volume":         rw(1.0, s.setVolume),
			"Metadata":       ro(map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(noTrack)}),
			// Clients poll the position and follow Seeked; it changes too often to signal.
			"Position":      &prop.Prop{Value: int64(0), Emit: prop.EmitFalse},
			"Rate":          rw(1.0, func(c *prop.Change) *dbus.Error { return rateOne(c) }),
			"MinimumRate":   ro(1.0),
			"MaximumRate":   ro(1.0),
			"CanGoNext":     ro(false),
			"CanGoPrevious": ro(false),
			// playerctl skips players with CanPlay false when it looks for metadata.
			"CanPlay":    ro(false),
			"CanPause":   ro(false),
			"CanSeek":    ro(false),
			"CanControl": ro(true),
		},
	})
	if err != nil {
		return nil, err
	}
	s.props = props
	if err := conn.Export(rootMethods{s}, path, ifaceRoot); err != nil {
		return nil, err
	}
	// Go vet wants a method named Seek to be io.Seeker's.
	if err := conn.ExportWithMap(playerMethods{s}, map[string]string{"SeekBy": "Seek"}, path, ifacePlayer); err != nil {
		return nil, err
	}
	methods := introspect.Methods(playerMethods{})
	for i := range methods {
		if methods[i].Name == "SeekBy" {
			methods[i].Name = "Seek"
		}
	}
	node := &introspect.Node{Name: string(path), Interfaces: []introspect.Interface{
		introspect.IntrospectData, prop.IntrospectData,
		{Name: ifaceRoot, Methods: introspect.Methods(rootMethods{}), Properties: props.Introspection(ifaceRoot)},
		{Name: ifacePlayer, Methods: methods, Properties: props.Introspection(ifacePlayer),
			Signals: []introspect.Signal{{Name: "Seeked", Args: []introspect.Arg{{Name: "Position", Type: "x"}}}}},
	}}
	if err := conn.Export(introspect.NewIntrospectable(node), path, "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, err
	}
	reply, err := conn.RequestName(BusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return nil, err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return nil, errors.New("another process owns " + BusName)
	}
	return s, nil
}

// run sends a command to the player off the D-Bus goroutine. Property
// callbacks hold the property lock, and the player may be publishing a
// status that Update needs that lock for.
func (s *Server) run(name string, f func(ctx context.Context) error) {
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, cmdTimeout)
		defer cancel()
		if err := f(ctx); err != nil {
			log.Debugf("mpris %s: %v", name, err)
		}
	}()
}

// Update mirrors a player status.
func (s *Server) Update(st player.Status) {
	status := map[player.State]string{player.StatePlaying: "Playing", player.StatePaused: "Paused"}[st.State]
	if status == "" {
		status = "Stopped"
	}
	id := trackID(st.Track)
	s.mu.Lock()
	newTrack := id != s.trackID
	if newTrack {
		s.trackID, s.track, s.artURL = id, st.Track, ""
		s.pos, s.posAt = st.Position, time.Now()
	}
	s.length = st.Duration
	seeked := s.moveTo(st.Position, st.State == player.StatePlaying, !newTrack)
	md := metadata(s.trackID, s.track, s.length, s.artURL)
	s.mu.Unlock()

	s.set("PlaybackStatus", status)
	s.set("LoopStatus", loopStatus(st.Repeat))
	s.set("Shuffle", st.Shuffle)
	s.set("Volume", st.Volume/100)
	s.set("Metadata", md)
	// Next at the end of the queue continues it.
	s.set("CanGoNext", st.Length > 0)
	s.set("CanGoPrevious", st.Length > 0)
	s.set("CanPlay", st.Length > 0)
	s.set("CanPause", st.Length > 0)
	s.set("CanSeek", st.Track != nil && st.Duration > 0)
	s.setPosition(st.Position, seeked)
	if newTrack && st.Track != nil && st.Track.CoverID != "" && s.cfg.Cover != nil {
		go s.fetchCover(id, *st.Track)
	}
}

func loopStatus(repeat string) string {
	switch repeat {
	case player.RepeatOne:
		return "Track"
	case player.RepeatAll:
		return "Playlist"
	}
	return "None"
}

// UpdatePosition takes the player's periodic position event.
func (s *Server) UpdatePosition(pos float64) {
	s.mu.Lock()
	seeked := s.moveTo(pos, s.playing, true)
	s.mu.Unlock()
	s.setPosition(pos, seeked)
}

// moveTo records a new position and says whether it is a jump rather than
// playback moving on. Callers hold mu.
func (s *Server) moveTo(pos float64, playing, sameTrack bool) bool {
	expect := s.pos
	if s.playing {
		expect += time.Since(s.posAt).Seconds()
	}
	s.pos, s.posAt, s.playing = pos, time.Now(), playing
	return sameTrack && math.Abs(pos-expect) > seekSlack
}

func (s *Server) setPosition(pos float64, seeked bool) {
	us := int64(pos * 1e6)
	s.props.SetMust(ifacePlayer, "Position", us)
	if seeked {
		if err := s.conn.Emit(path, ifacePlayer+".Seeked", us); err != nil {
			log.Debugf("mpris seeked: %v", err)
		}
	}
}

func (s *Server) fetchCover(id dbus.ObjectPath, t source.Track) {
	ctx, cancel := context.WithTimeout(s.ctx, cmdTimeout)
	defer cancel()
	file, err := s.cfg.Cover(ctx, t)
	if err != nil || file == "" {
		if err != nil {
			log.Debugf("mpris cover: %v", err)
		}
		return
	}
	s.mu.Lock()
	if s.trackID != id {
		s.mu.Unlock()
		return
	}
	s.artURL = (&url.URL{Scheme: "file", Path: file}).String()
	md := metadata(s.trackID, s.track, s.length, s.artURL)
	s.mu.Unlock()
	s.set("Metadata", md)
}

// set changes a property only when it differs, so clients see few signals.
// Update and the cover fetch call it from different goroutines; setMu makes
// the compare and the write one step.
func (s *Server) set(name string, v any) {
	s.setMu.Lock()
	defer s.setMu.Unlock()
	if cur, err := s.props.Get(ifacePlayer, name); err == nil && reflect.DeepEqual(cur.Value(), v) {
		return
	}
	s.props.SetMust(ifacePlayer, name, v)
}

func trackID(t *source.Track) dbus.ObjectPath {
	if t == nil {
		return noTrack
	}
	h := fnv.New64a()
	h.Write([]byte(t.Library + "\x00" + t.ID))
	return dbus.ObjectPath(fmt.Sprintf("/com/marlonkranz/player/track/%016x", h.Sum64()))
}

func metadata(id dbus.ObjectPath, t *source.Track, length float64, art string) map[string]dbus.Variant {
	md := map[string]dbus.Variant{"mpris:trackid": dbus.MakeVariant(id)}
	if t == nil {
		return md
	}
	add := func(k string, v any, ok bool) {
		if ok {
			md[k] = dbus.MakeVariant(v)
		}
	}
	add("xesam:title", t.Title, t.Title != "")
	add("xesam:artist", []string{t.Artist}, t.Artist != "")
	add("xesam:album", t.Album, t.Album != "")
	add("xesam:albumArtist", []string{t.AlbumArtist}, t.AlbumArtist != "")
	add("xesam:genre", []string{t.Genre}, t.Genre != "")
	add("xesam:trackNumber", int32(t.TrackNo), t.TrackNo > 0)
	add("xesam:discNumber", int32(t.DiscNo), t.DiscNo > 0)
	add("xesam:userRating", float64(t.Rating)/5, t.Rating > 0)
	add("mpris:length", int64(length*1e6), length > 0)
	add("mpris:artUrl", art, art != "")
	return md
}

func (s *Server) setLoop(c *prop.Change) *dbus.Error {
	mode, ok := map[string]string{"None": player.RepeatOff, "Track": player.RepeatOne, "Playlist": player.RepeatAll}[c.Value.(string)]
	if !ok {
		return prop.ErrInvalidArg
	}
	s.run("loop", func(ctx context.Context) error { return s.cfg.Player.SetRepeat(ctx, mode) })
	return nil
}

func (s *Server) setShuffle(c *prop.Change) *dbus.Error {
	on := c.Value.(bool)
	s.run("shuffle", func(ctx context.Context) error { return s.cfg.Player.SetShuffle(ctx, on) })
	return nil
}

func (s *Server) setVolume(c *prop.Change) *dbus.Error {
	v := max(c.Value.(float64), 0) * 100
	s.run("volume", func(ctx context.Context) error { return s.cfg.Player.SetVolume(ctx, v, false) })
	return nil
}

func rateOne(c *prop.Change) *dbus.Error {
	if c.Value.(float64) != 1 {
		return prop.ErrInvalidArg
	}
	return nil
}

type rootMethods struct{ s *Server }

func (m rootMethods) Raise() *dbus.Error {
	if m.s.cfg.Raise != nil {
		go m.s.cfg.Raise()
	}
	return nil
}

func (m rootMethods) Quit() *dbus.Error {
	if m.s.cfg.Quit != nil {
		go m.s.cfg.Quit()
	}
	return nil
}

type playerMethods struct{ s *Server }

func (m playerMethods) Play() *dbus.Error  { m.s.run("play", m.s.cfg.Player.Resume); return nil }
func (m playerMethods) Pause() *dbus.Error { m.s.run("pause", m.s.cfg.Player.Pause); return nil }
func (m playerMethods) PlayPause() *dbus.Error {
	m.s.run("playpause", m.s.cfg.Player.Toggle)
	return nil
}
func (m playerMethods) Stop() *dbus.Error { m.s.run("stop", m.s.cfg.Player.Stop); return nil }
func (m playerMethods) Next() *dbus.Error { m.s.run("next", m.s.cfg.Player.Next); return nil }
func (m playerMethods) Previous() *dbus.Error {
	m.s.run("previous", m.s.cfg.Player.Previous)
	return nil
}

// SeekBy is MPRIS's Seek: a relative seek in microseconds.
func (m playerMethods) SeekBy(offset int64) *dbus.Error {
	m.s.run("seek", func(ctx context.Context) error { return m.s.cfg.Player.Seek(ctx, float64(offset)/1e6, true) })
	return nil
}

// SetPosition ignores a stale track id or a position past the end, as the
// spec asks.
func (m playerMethods) SetPosition(id dbus.ObjectPath, us int64) *dbus.Error {
	m.s.mu.Lock()
	ok := id == m.s.trackID && id != noTrack && us >= 0 && float64(us)/1e6 <= m.s.length
	m.s.mu.Unlock()
	if ok {
		m.s.run("setposition", func(ctx context.Context) error { return m.s.cfg.Player.Seek(ctx, float64(us)/1e6, false) })
	}
	return nil
}

func (playerMethods) OpenUri(string) *dbus.Error {
	return dbus.MakeFailedError(errors.New("opening URIs is not supported"))
}
