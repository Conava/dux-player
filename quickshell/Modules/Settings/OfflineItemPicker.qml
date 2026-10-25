pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

// Picks a playlist, album or artist of one server library to keep on this device.
WindowDialog {
    id: root

    property string libraryId: ""
    // "playlist" | "album" | "artist"
    property string kind: "playlist"
    property string filterText: ""

    readonly property var items: {
        if (!opened)
            return [];
        const lib = libraryId;
        const all = kind === "playlist" ? PlaylistService.playlists.filter(p => p.library === lib && p.kind === "server") : kind === "album" ? CatalogService.albums.filter(a => a.library === lib) : CatalogService.artists.filter(a => a.library === lib);
        const needle = filterText.trim().toLowerCase();
        const kept = SettingsService.cacheRules(lib).filter(r => r.kind === kind).map(r => r.id);
        return all.filter(i => needle === "" || label(i).toLowerCase().indexOf(needle) >= 0).map(i => ({
                    "item": i,
                    "kept": kept.indexOf(i.rawId) >= 0
                }));
    }

    function label(i) {
        return kind === "album" ? Phrases.join(i.title, i.artist) : i.name;
    }

    function open(libraryId, kind) {
        root.libraryId = libraryId;
        root.kind = kind;
        filterText = "";
        search.text = "";
        opened = true;
        Qt.callLater(search.forceActiveFocus);
    }

    maximumHeight: parent ? parent.height - Theme.spacingXL * 2 : Infinity
    title: kind === "playlist" ? I18n.tr("Add playlist", "offline rule picker title") : kind === "album" ? I18n.tr("Add album", "offline rule picker title") : I18n.tr("Add artist", "offline rule picker title")
    acceptEnabled: false
    onRejected: opened = false

    DSearchField {
        id: search
        width: parent.width
        placeholderText: I18n.tr("Search", "offline rule picker search field")
        onTextChanged: root.filterText = text
    }

    Column {
        width: parent.width
        spacing: Theme.groupedListGap

        Repeater {
            model: root.items.slice(0, PlayerMetrics.offlinePickerLimit)

            DListItem {
                id: row
                required property var modelData
                required property int index
                width: parent.width
                enabled: !modelData.kept
                surfaceColor: SettingsMetrics.rowColor
                firstInGroup: index === 0
                lastInGroup: index === Math.min(root.items.length, PlayerMetrics.offlinePickerLimit) - 1
                Accessible.name: rowLabel.text
                onClicked: {
                    SettingsService.addCacheRule(root.libraryId, root.kind, row.modelData.item.rawId);
                    root.opened = false;
                }

                DIcon {
                    id: icon
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.spacingL
                    anchors.verticalCenter: parent.verticalCenter
                    name: row.modelData.kept ? "offline_pin" : root.kind === "playlist" ? "cloud" : root.kind === "album" ? "album" : "artist"
                    size: Theme.iconSize
                    color: row.contentColor
                }

                StyledText {
                    id: rowLabel
                    anchors.left: icon.right
                    anchors.leftMargin: Theme.spacingL
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.spacingL
                    anchors.verticalCenter: parent.verticalCenter
                    text: row.modelData.kept ? I18n.tr("%1 (already on this device)", "offline rule picker item that is already kept").arg(root.label(row.modelData.item)) : root.label(row.modelData.item)
                    font.pixelSize: Theme.fontSizeMedium
                    color: row.contentColor
                    elide: Text.ElideRight
                    wrapMode: Text.NoWrap
                    horizontalAlignment: Text.AlignLeft
                }
            }
        }
    }

    StyledText {
        width: parent.width
        visible: text !== ""
        text: root.items.length === 0 ? I18n.tr("Nothing matches.", "offline rule picker empty result") : root.items.length > PlayerMetrics.offlinePickerLimit ? I18n.tr("Showing %1 of %2 matches", "offline rule picker overflow").arg(PlayerMetrics.offlinePickerLimit).arg(root.items.length) : ""
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.surfaceVariantText
        wrapMode: Text.WordWrap
    }

    actions: [
        DButton {
            text: I18n.tr("Cancel", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.rejected()
        }
    ]
}
