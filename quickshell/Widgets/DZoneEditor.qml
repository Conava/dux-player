pragma ComponentBehavior: Bound

import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// Titled zones of DInstanceRows. Drag reorders within and across zones; the row menu (right-click, Menu key)
// moves up, down or to another zone. Emits edits only: the consumer changes the model, and decides undo and
// confirmation for removals.
// zones: [{id, label, note?, disabled?, addable?, rows: [{title, icon?, subtitle?, warning?, hidden?,
//          hideable?, hideEnabled?, removable?, hasOptions?, dimmed?}]}]
Column {
    id: root

    property var zones: []
    property var flickable: null
    property string addText: I18n.tr("Add element", "button under a zone in a layout editor")
    property string emptyText: I18n.tr("Drop elements here", "now playing zone empty state")
    property color rowColor: Style.foregroundColor(Style.chipSurface, Style.isFloatingWindow(root))

    // toIndex is the position in toZone after the element left fromZone
    signal moved(string fromZone, int fromIndex, string toZone, int toIndex)
    signal removed(string zone, int index)
    signal hiddenToggled(string zone, int index, bool hidden)
    signal optionsRequested(string zone, int index)
    signal addRequested(string zone)

    property string menuZone: ""
    property int menuIndex: -1

    function zoneById(id) {
        return zones.find(z => z.id === id) ?? null;
    }

    function openMenu(zoneId, index, anchor, x, y) {
        const zone = zoneById(zoneId);
        const row = zone?.rows?.[index];
        if (!row || zone.disabled)
            return;
        const forward = I18n.isRtl ? "arrow_back" : "arrow_forward";
        const entries = [
            {
                "id": "up",
                "text": I18n.tr("Move up", "editor row menu"),
                "icon": "arrow_upward",
                "enabled": index > 0
            },
            {
                "id": "down",
                "text": I18n.tr("Move down", "editor row menu"),
                "icon": "arrow_downward",
                "enabled": index < zone.rows.length - 1
            }
        ];
        const targets = zones.filter(z => z.id !== zoneId && !z.disabled);
        if (targets.length > 0)
            entries.push({
                "divider": true
            });
        for (const z of targets)
            entries.push({
                "id": "zone:" + z.id,
                "text": I18n.tr("Move to %1", "editor row menu, %1 is a zone").arg(z.label),
                "icon": forward
            });
        if (row.removable !== false)
            entries.push({
                "divider": true
            }, {
                "id": "remove",
                "text": I18n.tr("Remove", "button that removes an item"),
                "icon": "close"
            });
        menuZone = zoneId;
        menuIndex = index;
        rowMenu.model = entries;
        rowMenu.open(anchor, x, y);
    }

    function runMenu(id) {
        const zone = zoneById(menuZone);
        if (!zone || menuIndex < 0 || menuIndex >= zone.rows.length)
            return;
        switch (id) {
        case "up":
            moved(menuZone, menuIndex, menuZone, menuIndex - 1);
            return;
        case "down":
            moved(menuZone, menuIndex, menuZone, menuIndex + 1);
            return;
        case "remove":
            removed(menuZone, menuIndex);
            return;
        }
        const target = zoneById(id.slice(5));
        if (target)
            moved(menuZone, menuIndex, target.id, target.rows.length);
    }

    // a drag reorder is one element moving; find which from the permutation
    function reordered(zoneId, indices) {
        let first = 0;
        while (first < indices.length && indices[first] === first)
            first++;
        let last = indices.length - 1;
        while (last > first && indices[last] === last)
            last--;
        if (first >= last)
            return;
        if (indices[first] === last)
            moved(zoneId, last, zoneId, first);
        else
            moved(zoneId, first, zoneId, last);
    }

    width: parent?.width ?? 0
    spacing: Style.spacingL
    LayoutMirroring.enabled: I18n.isRtl
    LayoutMirroring.childrenInherit: true

    DReorderGroup {
        id: reorderGroup
        coordinateItem: root
        onTransferred: (source, sourceIndex, target, targetIndex) => root.moved(source.groupKey, sourceIndex, target.groupKey, targetIndex)
    }

    DMenu {
        id: rowMenu
        onTriggered: id => root.runMenu(id)
    }

    Repeater {
        model: root.zones

        Column {
            id: zone

            required property var modelData
            readonly property string zoneId: modelData.id
            readonly property var rows: modelData.rows ?? []

            width: root.width
            spacing: Style.spacingS
            enabled: !modelData.disabled
            z: list.draggingIndex >= 0 ? 1 : 0

            StyledText {
                width: parent.width
                topPadding: Style.spacingS
                text: zone.modelData.label ?? ""
                font.pixelSize: Style.fontSizeMedium
                font.weight: Style.fontWeightMedium
                color: zone.enabled ? Style.primary : Style.onSurfaceVariant
                wrapMode: Text.WordWrap
                horizontalAlignment: Text.AlignLeft
                Accessible.role: Accessible.Heading
                Accessible.name: text
            }

            StyledText {
                width: parent.width
                visible: text !== ""
                text: zone.modelData.note ?? ""
                font.pixelSize: Style.fontSizeSmall
                color: Style.onSurfaceVariant
                wrapMode: Text.WordWrap
                horizontalAlignment: Text.AlignLeft
            }

            StyledText {
                width: parent.width
                visible: zone.rows.length === 0 && zone.enabled
                text: root.emptyText
                font.pixelSize: Style.fontSizeSmall
                color: Style.onSurfaceVariant
                horizontalAlignment: Text.AlignLeft
            }

            DReorderList {
                id: list
                width: parent.width
                model: zone.rows
                group: reorderGroup
                groupKey: zone.zoneId
                dropArea: zone
                flickable: root.flickable
                onReordered: indices => root.reordered(zone.zoneId, indices)

                delegate: DInstanceRow {
                    id: row

                    required property var modelData

                    view: list
                    surfaceColor: root.rowColor
                    iconName: modelData.icon ?? ""
                    title: modelData.title ?? ""
                    subtitle: modelData.subtitle ?? ""
                    warning: modelData.warning ?? ""
                    hidden: modelData.hidden ?? false
                    dimmed: modelData.dimmed ?? hidden
                    hideable: modelData.hideable ?? true
                    hideEnabled: modelData.hideEnabled ?? true
                    removable: modelData.removable ?? true
                    hasOptions: modelData.hasOptions ?? false
                    onOptionsRequested: root.optionsRequested(zone.zoneId, index)
                    onHiddenToggled: hidden => root.hiddenToggled(zone.zoneId, index, hidden)
                    onRemoveRequested: root.removed(zone.zoneId, index)
                    onContextMenuRequested: (x, y) => root.openMenu(zone.zoneId, index, row, x, y)
                }
            }

            DButton {
                anchors.horizontalCenter: parent.horizontalCenter
                visible: zone.modelData.addable ?? true
                text: root.addText
                iconName: "add"
                backgroundColor: Style.secondaryContainer
                textColor: Style.onSecondaryContainer
                onClicked: root.addRequested(zone.zoneId)
            }
        }
    }
}
