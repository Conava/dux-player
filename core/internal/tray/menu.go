package tray

import (
	"errors"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	menuPath  = dbus.ObjectPath("/MenuBar")
	ifaceMenu = "com.canonical.dbusmenu"
)

const (
	idRoot int32 = iota
	idOpen
	idToggle
	idNext
	idPrevious
	idSeparator
	idQuit
)

var errUnknownID = errors.New("no such menu item")

// defaultLabels are the menu's texts. The daemon has no translations; a UI
// can replace them by key once it pushes its own.
var defaultLabels = map[string]string{
	"open": "Open Dux Player", "play": "Play", "pause": "Pause",
	"next": "Next", "previous": "Previous", "quit": "Quit Dux Player",
}

// menuItem is dbusmenu's (ia{sv}av) layout node.
type menuItem struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

type itemProps struct {
	ID    int32
	Props map[string]dbus.Variant
}

type menuEvent struct {
	ID    int32
	Event string
	Data  dbus.Variant
	Time  uint32
}

// items returns the menu entries' properties in order.
func (t *Tray) items() []itemProps {
	t.mu.Lock()
	toggle := t.labels["play"]
	if t.playing {
		toggle = t.labels["pause"]
	}
	t.mu.Unlock()
	label := func(id int32, text string) itemProps {
		return itemProps{id, map[string]dbus.Variant{"label": dbus.MakeVariant(text)}}
	}
	return []itemProps{
		label(idOpen, t.labels["open"]),
		label(idToggle, toggle),
		label(idNext, t.labels["next"]),
		label(idPrevious, t.labels["previous"]),
		{idSeparator, map[string]dbus.Variant{"type": dbus.MakeVariant("separator")}},
		label(idQuit, t.labels["quit"]),
	}
}

func (t *Tray) layout(depth int32) menuItem {
	root := menuItem{ID: idRoot, Props: map[string]dbus.Variant{"children-display": dbus.MakeVariant("submenu")},
		Children: []dbus.Variant{}}
	if depth == 0 {
		return root
	}
	for _, it := range t.items() {
		root.Children = append(root.Children, dbus.MakeVariant(menuItem{it.ID, it.Props, []dbus.Variant{}}))
	}
	return root
}

func (t *Tray) clicked(id int32) {
	switch id {
	case idOpen:
		go t.cfg.Show()
	case idToggle:
		t.run("toggle", t.cfg.Player.Toggle)
	case idNext:
		t.run("next", t.cfg.Player.Next)
	case idPrevious:
		t.run("previous", t.cfg.Player.Previous)
	case idQuit:
		go t.cfg.Quit()
	}
}

func (t *Tray) exportMenu() error {
	ro := func(v any) *prop.Prop { return &prop.Prop{Value: v, Emit: prop.EmitFalse} }
	props, err := prop.Export(t.conn, menuPath, prop.Map{ifaceMenu: {
		"Version":       ro(uint32(3)),
		"TextDirection": ro("ltr"),
		"Status":        ro("normal"),
		"IconThemePath": ro([]string{}),
	}})
	if err != nil {
		return err
	}
	if err := t.conn.Export(menuMethods{t}, menuPath, ifaceMenu); err != nil {
		return err
	}
	node := &introspect.Node{Name: string(menuPath), Interfaces: []introspect.Interface{
		introspect.IntrospectData, prop.IntrospectData,
		{Name: ifaceMenu, Methods: introspect.Methods(menuMethods{}), Properties: props.Introspection(ifaceMenu),
			Signals: []introspect.Signal{
				{Name: "LayoutUpdated", Args: []introspect.Arg{{Name: "revision", Type: "u"}, {Name: "parent", Type: "i"}}},
				{Name: "ItemsPropertiesUpdated", Args: []introspect.Arg{{Name: "updatedProps", Type: "a(ia{sv})"}, {Name: "removedProps", Type: "a(ias)"}}},
			}},
	}}
	return t.conn.Export(introspect.NewIntrospectable(node), menuPath, "org.freedesktop.DBus.Introspectable")
}

// menuMethods is com.canonical.dbusmenu. The menu is flat and small, so
// property name filters are ignored and every item is sent whole.
type menuMethods struct{ t *Tray }

func (m menuMethods) GetLayout(parent, depth int32, _ []string) (uint32, menuItem, *dbus.Error) {
	m.t.mu.Lock()
	rev := m.t.rev
	m.t.mu.Unlock()
	if parent == idRoot {
		return rev, m.t.layout(depth), nil
	}
	for _, it := range m.t.items() {
		if it.ID == parent {
			return rev, menuItem{it.ID, it.Props, []dbus.Variant{}}, nil
		}
	}
	return 0, menuItem{}, dbus.MakeFailedError(errUnknownID)
}

func (m menuMethods) GetGroupProperties(ids []int32, _ []string) ([]itemProps, *dbus.Error) {
	all := m.t.items()
	if len(ids) == 0 {
		return all, nil
	}
	out := []itemProps{}
	for _, it := range all {
		for _, id := range ids {
			if it.ID == id {
				out = append(out, it)
			}
		}
	}
	return out, nil
}

func (m menuMethods) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	for _, it := range m.t.items() {
		if v, ok := it.Props[name]; ok && it.ID == id {
			return v, nil
		}
	}
	return dbus.Variant{}, dbus.MakeFailedError(errUnknownID)
}

func (m menuMethods) Event(id int32, event string, _ dbus.Variant, _ uint32) *dbus.Error {
	if event == "clicked" {
		m.t.clicked(id)
	}
	return nil
}

func (m menuMethods) EventGroup(events []menuEvent) ([]int32, *dbus.Error) {
	for _, e := range events {
		_ = m.Event(e.ID, e.Event, e.Data, e.Time)
	}
	return []int32{}, nil
}

func (menuMethods) AboutToShow(int32) (bool, *dbus.Error) { return false, nil }

func (menuMethods) AboutToShowGroup([]int32) ([]int32, []int32, *dbus.Error) {
	return []int32{}, []int32{}, nil
}
