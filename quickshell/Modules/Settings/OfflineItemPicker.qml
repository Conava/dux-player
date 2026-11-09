pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

// Picks a playlist, album or artist of one server library, or a local playlist, to keep on this device.
WindowDialog {
    id: root

    // "playlist" | "album" | "artist"
    property string kind: "playlist"
    // a server library, or "" for local playlists
    property string scope: ""
    property string filterText: ""

    readonly property var matches: {
        if (!opened)
            return [];
        const all = kind === "playlist" ? PlaylistService.playlists.filter(p => p.kind === (scope === "" ? "local" : "server")) : kind === "album" ? CatalogService.albums : CatalogService.artists;
        const needle = filterText.trim().toLowerCase();
        return all.filter(i => i.library === scope && (needle === "" || label(i).toLowerCase().indexOf(needle) >= 0));
    }
    readonly property var keptRules: !opened ? [] : scope === "" ? SettingsService.cacheRules().filter(r => r.local) : SettingsService.rulesFor(scope)
    readonly property var items: matches.slice(0, PlayerMetrics.offlinePickerLimit).map(i => ({
                "item": i,
                "kept": keptRules.some(r => r.kind === kind && r.id === i.rawId)
            }))

    function label(i) {
        return kind === "album" ? Phrases.join(i.title, i.artist) : i.name;
    }

    function open(kind, scope) {
        root.kind = kind;
        root.scope = scope;
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
            model: root.items

            DListItem {
                id: row
                required property var modelData
                required property int index
                width: parent.width
                enabled: !modelData.kept
                surfaceColor: SettingsMetrics.rowColor
                firstInGroup: index === 0
                lastInGroup: index === root.items.length - 1
                Accessible.name: rowLabel.text
                onClicked: {
                    SettingsService.addCacheRule(row.modelData.item.library, root.kind, row.modelData.item.rawId);
                    root.opened = false;
                }

                DIcon {
                    id: icon
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.spacingL
                    anchors.verticalCenter: parent.verticalCenter
                    name: row.modelData.kept ? "offline_pin" : root.kind === "playlist" ? (root.scope === "" ? "queue_music" : "cloud") : root.kind === "album" ? "album" : "artist"
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
        text: root.matches.length === 0 ? I18n.tr("Nothing matches.", "offline rule picker empty result") : root.matches.length > root.items.length ? I18n.tr("Showing %1 of %2 matches", "offline rule picker overflow").arg(root.items.length).arg(root.matches.length) : ""
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
