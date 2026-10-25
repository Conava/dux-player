// Package tray shows a StatusNotifierItem tray icon with a dbusmenu menu.
// It lives in the daemon because the UI process ends with the window.
package tray

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/duxlabs-dev/dux-go/log"
	"github.com/duxlabs-dev/dux-go/paths"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"

	"github.com/Conava/dux-player/core/internal/player"
)

const (
	AppID       = "io.github.conava.DuxPlayer"
	watcherName = "org.kde.StatusNotifierWatcher"
	itemPath    = dbus.ObjectPath("/StatusNotifierItem")
	ifaceItem   = "org.kde.StatusNotifierItem"
	callTimeout = 5 * time.Second
)

// icon.png is the app icon rendered for hosts that can't find it in the
// icon theme (a build run from the source tree).
//
//go:generate rsvg-convert -w 64 -h 64 -o icon.png ../../../assets/io.github.conava.DuxPlayer.svg
//go:embed icon.png
var iconPNG []byte

// Player is what the tray drives.
type Player interface {
	Toggle(ctx context.Context) error
	Next(ctx context.Context) error
	Previous(ctx context.Context) error
	SetVolume(ctx context.Context, v float64, relative bool) error
}

type Config struct {
	Player Player
	Show   func()
	Quit   func()
}

type Tray struct {
	conn   *dbus.Conn
	props  *prop.Properties
	cfg    Config
	ctx    context.Context
	name   string
	labels map[string]string

	mu      sync.Mutex
	on      bool
	playing bool
	tip     string
	rev     uint32
}

type pixmap struct {
	W, H int32
	Data []byte
}

type toolTip struct {
	Icon    string
	Pixmaps []pixmap
	Title   string
	Text    string
}

// Start exports the item and its menu on conn. It shows nothing until
// SetEnabled(true).
func Start(ctx context.Context, conn *dbus.Conn, cfg Config) (*Tray, error) {
	t := &Tray{conn: conn, cfg: cfg, ctx: ctx, labels: defaultLabels,
		name: fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())}
	// A host prefers IconName, so name the icon only when the theme has it.
	iconName := ""
	if themeHasIcon() {
		iconName = AppID
	}
	icon, err := argb(iconPNG)
	if err != nil {
		return nil, err
	}
	ro := func(v any) *prop.Prop { return &prop.Prop{Value: v, Emit: prop.EmitFalse} }
	props, err := prop.Export(conn, itemPath, prop.Map{ifaceItem: {
		"Category":   ro("ApplicationStatus"),
		"Id":         ro(AppID),
		"Title":      ro("Dux Player"),
		"Status":     ro("Active"),
		"WindowId":   ro(int32(0)),
		"IconName":   ro(iconName),
		"IconPixmap": ro([]pixmap{icon}),
		// The shell keys hidden and ordered items by Id and tooltip title,
		// so the title stays fixed and the track goes in the text.
		"ToolTip":    ro(toolTip{Title: "Dux Player", Pixmaps: []pixmap{}}),
		"ItemIsMenu": ro(false),
		"Menu":       ro(menuPath),
	}})
	if err != nil {
		return nil, err
	}
	t.props = props
	if err := conn.Export(itemMethods{t}, itemPath, ifaceItem); err != nil {
		return nil, err
	}
	sig := func(name string) introspect.Signal { return introspect.Signal{Name: name} }
	node := &introspect.Node{Name: string(itemPath), Interfaces: []introspect.Interface{
		introspect.IntrospectData, prop.IntrospectData,
		{Name: ifaceItem, Methods: introspect.Methods(itemMethods{}), Properties: props.Introspection(ifaceItem),
			Signals: []introspect.Signal{sig("NewToolTip"), sig("NewStatus")}},
	}}
	if err := conn.Export(introspect.NewIntrospectable(node), itemPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, err
	}
	if err := t.exportMenu(); err != nil {
		return nil, err
	}
	if err := conn.AddMatchSignal(dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, watcherName)); err != nil {
		return nil, err
	}
	ch := make(chan *dbus.Signal, 4)
	conn.Signal(ch)
	go t.watch(ch)
	return t, nil
}

// watch registers again whenever a watcher (the shell's tray) starts.
func (t *Tray) watch(ch chan *dbus.Signal) {
	for {
		select {
		case <-t.ctx.Done():
			return
		case s, ok := <-ch:
			if !ok {
				return
			}
			if s.Name != "org.freedesktop.DBus.NameOwnerChanged" || len(s.Body) != 3 {
				continue
			}
			if owner, _ := s.Body[2].(string); owner != "" {
				t.register()
			}
		}
	}
}

// SetEnabled shows or removes the icon. Releasing the name is what makes
// the watcher drop the item.
func (t *Tray) SetEnabled(on bool) {
	t.mu.Lock()
	if on == t.on {
		t.mu.Unlock()
		return
	}
	if !on {
		t.on = false
		t.mu.Unlock()
		if _, err := t.conn.ReleaseName(t.name); err != nil {
			log.Debugf("tray: release name: %v", err)
		}
		return
	}
	reply, err := t.conn.RequestName(t.name, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.mu.Unlock()
		log.Warnf("tray: can't own %s: %v", t.name, err)
		return
	}
	t.on = true
	t.mu.Unlock()
	t.register()
}

// register tells the watcher about the item. No watcher means no tray
// right now; watch tries again when one appears.
func (t *Tray) register() {
	t.mu.Lock()
	on := t.on
	t.mu.Unlock()
	if !on {
		return
	}
	ctx, cancel := context.WithTimeout(t.ctx, callTimeout)
	defer cancel()
	err := t.conn.Object(watcherName, "/StatusNotifierWatcher").
		CallWithContext(ctx, watcherName+".RegisterStatusNotifierItem", 0, t.name).Err
	if err != nil {
		log.Debugf("tray: no watcher: %v", err)
	}
}

// Update follows a player status: the tooltip shows the track, the menu's
// Play/Pause entry the state. Signals go out only on a change.
func (t *Tray) Update(st player.Status) {
	tip := ""
	if st.Track != nil {
		tip = st.Track.Title
		if st.Track.Artist != "" {
			tip += " — " + st.Track.Artist
		}
	}
	playing := st.State == player.StatePlaying
	t.mu.Lock()
	tipChanged, playChanged := tip != t.tip, playing != t.playing
	t.tip, t.playing = tip, playing
	if playChanged {
		t.rev++
	}
	rev := t.rev
	t.mu.Unlock()
	if tipChanged {
		t.props.SetMust(ifaceItem, "ToolTip", toolTip{Title: "Dux Player", Pixmaps: []pixmap{}, Text: tip})
		t.emit(itemPath, ifaceItem+".NewToolTip")
	}
	if playChanged {
		t.emit(menuPath, ifaceMenu+".LayoutUpdated", rev, int32(0))
	}
}

func (t *Tray) emit(path dbus.ObjectPath, name string, args ...any) {
	if err := t.conn.Emit(path, name, args...); err != nil {
		log.Debugf("tray %s: %v", name, err)
	}
}

// run sends a command to the player off the D-Bus goroutine.
func (t *Tray) run(name string, f func(ctx context.Context) error) {
	go func() {
		ctx, cancel := context.WithTimeout(t.ctx, 10*time.Second)
		defer cancel()
		if err := f(ctx); err != nil {
			log.Debugf("tray %s: %v", name, err)
		}
	}()
}

type itemMethods struct{ t *Tray }

func (m itemMethods) Activate(x, y int32) *dbus.Error {
	go m.t.cfg.Show()
	return nil
}

func (m itemMethods) SecondaryActivate(x, y int32) *dbus.Error {
	m.t.run("toggle", m.t.cfg.Player.Toggle)
	return nil
}

// Scroll changes the volume by 5 per wheel step, whatever the host's delta
// unit is.
func (m itemMethods) Scroll(delta int32, orientation string) *dbus.Error {
	if delta == 0 || !strings.EqualFold(orientation, "vertical") {
		return nil
	}
	step := 5.0
	if delta < 0 {
		step = -step
	}
	m.t.run("volume", func(ctx context.Context) error { return m.t.cfg.Player.SetVolume(ctx, step, true) })
	return nil
}

func themeHasIcon() bool {
	dirs := []string{paths.XDGDataHome()}
	if v := os.Getenv("XDG_DATA_DIRS"); v != "" {
		dirs = append(dirs, filepath.SplitList(v)...)
	} else {
		dirs = append(dirs, "/usr/local/share", "/usr/share")
	}
	for _, d := range dirs {
		if m, _ := filepath.Glob(filepath.Join(d, "icons", "hicolor", "*", "apps", AppID+".*")); len(m) > 0 {
			return true
		}
	}
	return false
}

// argb turns a PNG into the item's pixmap format: ARGB32, network byte order.
func argb(data []byte) (pixmap, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return pixmap{}, fmt.Errorf("tray icon: %w", err)
	}
	b := img.Bounds()
	out := make([]byte, 0, b.Dx()*b.Dy()*4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			out = append(out, c.A, c.R, c.G, c.B)
		}
	}
	return pixmap{W: int32(b.Dx()), H: int32(b.Dy()), Data: out}, nil
}
