pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Modules
import qs.DCommon.Widgets
import qs.Widgets

// The editor of one now-playing surface: its zones of elements, the side panel's "Follow the bottom bar", its
// problems, and an options page per element.
Item {
    id: root

    required property string surfaceId
    property string title: ""
    property Component preview: null
    property size previewSize: Qt.size(1, 1)
    // the element whose options page is open
    property string optionsId: ""
    // opening another surface (Customize from a view) starts on its editor, not on the last options page
    onSurfaceIdChanged: optionsId = ""
    property int revision: 0

    readonly property var surface: UiConfig.surface(surfaceId) ?? ({
            "zones": {}
        })
    readonly property var zoneMeta: UiConfig.zonesOf(surfaceId)
    readonly property var problems: UiConfig.problems(surfaceId)
    readonly property bool following: surfaceId === "side" && surface.panesFollowBar
    readonly property var target: findInstance(optionsId)
    // reads revision so a refused move rebuilds the rows
    readonly property var editorZones: revision >= 0 ? zoneMeta.map(z => zoneModel(z)) : []

    signal back

    function zoneInfo(zoneId) {
        return zoneMeta.find(z => z.id === zoneId) ?? null;
    }

    // what the zone accepts, by type: {type: {group, options}}
    function accepted(zoneId) {
        const out = {};
        for (const t of UiConfig.typesFor(surfaceId, zoneId))
            out[t.type] = t;
        return out;
    }

    function placeable(zone, entry) {
        return zone.overflows && entry !== undefined && ["transport", "buttons", "actions"].indexOf(entry.group) >= 0;
    }

    function hasOptions(zone, entry) {
        return placeable(zone, entry) || Object.keys(entry?.options ?? {}).some(k => Array.isArray(entry.options[k].values));
    }

    function summary(zone, entry, inst) {
        const parts = [];
        if (placeable(zone, entry))
            parts.push(optionsPage.placementLabel(inst.placement));
        for (const key of Object.keys(inst.options ?? {}))
            if (Array.isArray(entry?.options?.[key]?.values))
                parts.push(UiConfig.optionLabel(inst.options[key]));
        return parts.join(" · ");
    }

    function rowOf(zone, types, inst) {
        if (!UiConfig.isKnown(inst.type))
            return {
                "title": inst.type,
                "icon": "help",
                "subtitle": I18n.tr("Not available in this version", "an element saved by a newer version of the app"),
                "dimmed": true,
                "hideable": false
            };
        const entry = types[inst.type];
        return {
            "title": UiConfig.elementLabel(inst.type),
            "icon": UiConfig.elementIcon(inst.type),
            "subtitle": summary(zone, entry, inst),
            "hidden": inst.hidden,
            "hideable": inst.type !== "spacer",
            "hasOptions": hasOptions(zone, entry)
        };
    }

    function zoneModel(zone) {
        if (zone.id === "panes" && following) {
            const panes = UiConfig.sidePanes;
            return {
                "id": zone.id,
                "label": zone.label,
                "note": I18n.tr("Follows the bottom bar: %1", "side panel panes zone note, %1 lists the panes in order").arg(panes.map(p => UiConfig.elementLabel(p)).join(", ")),
                "disabled": true,
                "addable": false,
                "rows": panes.map(p => ({
                            "title": UiConfig.elementLabel(p),
                            "icon": UiConfig.elementIcon(p),
                            "removable": false
                        }))
            };
        }
        const types = accepted(zone.id);
        const followed = following && UiConfig.sideFollowedZones.indexOf(zone.id) >= 0;
        return {
            "id": zone.id,
            "label": zone.label,
            "note": followed ? I18n.tr("Follows the bottom bar", "side panel editor: a zone taken from the bar") : "",
            "disabled": followed,
            "addable": !followed,
            "rows": (surface.zones[zone.id] ?? []).map(inst => rowOf(zone, types, inst))
        };
    }

    function findInstance(id) {
        if (id === "")
            return null;
        for (const zone of zoneMeta) {
            const list = surface.zones[zone.id] ?? [];
            const index = list.findIndex(i => i.id === id);
            if (index >= 0)
                return {
                    "zone": zone,
                    "index": index,
                    "instance": list[index],
                    "entry": accepted(zone.id)[list[index].type]
                };
        }
        return null;
    }

    function zonesCopy() {
        return JSON.parse(JSON.stringify(surface.zones));
    }

    function write(zones, follow) {
        const value = {
            "zones": zones
        };
        if (surfaceId === "side")
            value.panesFollowBar = follow ?? surface.panesFollowBar;
        UiConfig.setSurface(surfaceId, value);
    }

    function move(fromZone, fromIndex, toZone, toIndex) {
        const zones = zonesCopy();
        const inst = zones[fromZone]?.[fromIndex];
        if (!inst)
            return;
        if (fromZone !== toZone && !UiConfig.accepts(surfaceId, toZone, zones[toZone] ?? [], inst.type)) {
            revision++;
            Router.notify(I18n.tr("%1 can't go in %2", "toast when an element is dropped on a zone that doesn't take it; %1 is the element, %2 the zone").arg(UiConfig.elementLabel(inst.type)).arg(zoneInfo(toZone)?.label ?? toZone));
            return;
        }
        zones[fromZone].splice(fromIndex, 1);
        zones[toZone].splice(toIndex, 0, inst);
        write(zones);
    }

    function setHidden(zone, index, hidden) {
        const zones = zonesCopy();
        if (!zones[zone]?.[index])
            return;
        zones[zone][index].hidden = hidden;
        write(zones);
    }

    function updateInstance(inst) {
        const found = findInstance(inst.id);
        if (!found)
            return;
        const zones = zonesCopy();
        zones[found.zone.id][found.index] = inst;
        write(zones);
    }

    // follows Settings → General's removing setting: an Undo toast, a confirm dialog, or neither
    function remove(zone, index) {
        const before = zonesCopy();
        const inst = before[zone]?.[index];
        if (!inst)
            return;
        const label = UiConfig.isKnown(inst.type) ? UiConfig.elementLabel(inst.type) : inst.type;
        const follow = surface.panesFollowBar;
        const run = () => {
            const zones = zonesCopy();
            zones[zone].splice(index, 1);
            write(zones);
            Router.offerUndo(I18n.tr("Removed %1", "toast after removing an element from a now-playing surface, %1 is the element").arg(label), () => write(before, follow));
        };
        if (UiConfig.removeMode !== "confirm") {
            run();
            return;
        }
        removeDialog.title = I18n.tr("Remove %1?", "remove an element from a now-playing surface confirmation title, %1 is the element").arg(label);
        removeDialog.run = run;
        removeDialog.ask();
    }

    function openPicker(zoneId) {
        const types = UiConfig.typesFor(surfaceId, zoneId);
        if (zoneId === "panes") {
            const present = (surface.zones.panes ?? []).map(i => i.type);
            addSheet.sections = [
                {
                    "title": zoneInfo(zoneId)?.label ?? "",
                    "items": types.map(t => ({
                                "label": t.label,
                                "icon": t.icon,
                                "value": t.type,
                                "done": present.indexOf(t.type) >= 0
                            }))
                }
            ];
        } else {
            const groups = [["trackInfo", I18n.tr("Track info", "element picker group")], ["transport", I18n.tr("Playback", "element picker group: transport and seek")], ["buttons", I18n.tr("Buttons", "element picker group")], ["actions", I18n.tr("Track actions", "element picker group")], ["layout", I18n.tr("Layout", "element picker group: spacer and divider")]];
            addSheet.sections = groups.map(g => ({
                        "title": g[1],
                        "items": types.filter(t => (t.group === "seek" ? "transport" : t.group) === g[0]).map(t => ({
                                    "label": t.label,
                                    "icon": t.icon,
                                    "value": t.type
                                }))
                    }));
        }
        addSheet.zoneId = zoneId;
        addSheet.opened = true;
    }

    function add(zoneId, type) {
        const zones = zonesCopy();
        zones[zoneId] = (zones[zoneId] ?? []).concat([
            {
                "type": type,
                "hidden": false,
                "placement": "auto",
                "options": {}
            }
        ]);
        write(zones);
    }

    // later: clearing it now would re-evaluate target inside its own update
    onTargetChanged: {
        if (!target && optionsId !== "")
            Qt.callLater(() => optionsId = "");
    }

    SettingsPage {
        id: page
        visible: root.target === null

        SettingsBackRow {
            text: I18n.tr("Now playing", "settings sidebar tab label")
            onClicked: root.back()
        }

        SectionLabel {
            text: root.title

            actions: DButton {
                text: I18n.tr("Reset", "button that restores the defaults")
                iconName: "restart_alt"
                backgroundColor: "transparent"
                textColor: Theme.primary
                onClicked: resetDialog.ask()
            }
        }

        SurfaceCard.Preview {
            width: parent.width
            height: Math.min(implicitHeight, root.height / 3)
            preview: root.preview
            previewSize: root.previewSize
        }

        SettingsCard {
            visible: root.problems.length > 0

            Repeater {
                model: root.problems

                SettingsRow {
                    required property string modelData
                    title: modelData
                    iconName: "warning"
                    iconColor: Theme.error
                }
            }
        }

        SettingsCard {
            visible: root.surfaceId === "side"

            SettingsToggleRow {
                text: I18n.tr("Follow the bottom bar", "side panel editor setting")
                description: I18n.tr("Takes the title, controls and buttons from the bar, and the tabs in the order of their buttons there.", "side panel editor setting description")
                checked: root.following
                onToggled: value => root.write(root.zonesCopy(), value)
            }
        }

        SettingsPageIntro {
            text: I18n.tr("Drag the handle to reorder or move between zones. Click an element for its options.", "now playing editor hint")
        }

        DZoneEditor {
            flickable: page
            rowColor: SettingsMetrics.rowColor
            zones: root.editorZones
            onMoved: (fromZone, fromIndex, toZone, toIndex) => root.move(fromZone, fromIndex, toZone, toIndex)
            onRemoved: (zone, index) => root.remove(zone, index)
            onHiddenToggled: (zone, index, hidden) => root.setHidden(zone, index, hidden)
            onOptionsRequested: (zone, index) => root.optionsId = root.surface.zones[zone][index].id
            onAddRequested: zone => root.openPicker(zone)
        }
    }

    PickerSheet {
        id: addSheet
        property string zoneId: ""
        title: I18n.tr("Add element", "button under a zone in a layout editor")
        doneFormat: I18n.tr("%1 (already in this zone)", "picker item that is already in the zone")
        onPicked: value => root.add(addSheet.zoneId, value)
    }

    ConfirmDialog {
        id: resetDialog
        host: root
        title: I18n.tr("Reset %1?", "reset one now-playing surface confirmation title, %1 is the surface").arg(root.title)
        supportingText: I18n.tr("Its elements return to their defaults.", "reset a now-playing surface confirmation")
        confirmText: I18n.tr("Reset", "button that restores the defaults")
        onConfirmed: UiConfig.resetSurface(root.surfaceId)
    }

    ConfirmDialog {
        id: removeDialog
        property var run: null
        host: root
        confirmText: I18n.tr("Remove", "button that removes an item")
        onConfirmed: run?.()
    }

    ElementOptionsPage {
        id: optionsPage
        visible: root.target !== null
        instance: root.target?.instance ?? null
        placeable: root.target ? root.placeable(root.target.zone, root.target.entry) : false
        schema: root.target?.entry?.options ?? ({})
        backText: root.title
        onEdited: inst => root.updateInstance(inst)
        onBack: root.optionsId = ""
    }
}
