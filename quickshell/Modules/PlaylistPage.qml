pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import "../Common/Format.js" as Format
import "../Services/Catalog.js" as Catalog

Item {
    id: root

    property bool reordering: false
    // after "Reorder tracks" the list shows the playlist's order until the user sorts or leaves; the saved sort
    // stays
    property bool playlistOrder: false
    property string filterText: ""
    readonly property bool filtering: filterText.trim() !== ""

    readonly property string playlistId: Router.itemId
    readonly property var playlist: PlayerService.playlist(playlistId)
    readonly property bool server: playlist?.kind === "server"
    // in playlist order, each with its `position`, so a sorted view still edits the right entries
    readonly property var tracks: {
        const items = playlist?.items ?? [];
        const out = [];
        for (let i = 0; i < items.length; i++) {
            const t = CatalogService.track(items[i]);
            if (t)
                out.push(Object.assign({}, t, {
                    "position": i
                }));
        }
        return out;
    }
    readonly property var shownTracks: {
        const tracks = Catalog.filterText(root.tracks, filterText);
        if (sortKey === "position")
            return sortDescending ? tracks.slice().reverse() : tracks;
        return CatalogService.refine(tracks, "tracks", {
            "sort": sortKey,
            "descending": sortDescending
        });
    }
    readonly property int libraryCount: tracks.map(t => t.library).filter((id, i, all) => all.indexOf(id) === i).length
    readonly property string serverName: PlayerService.libraryName(playlist?.library ?? "")
    // each playlist remembers its own sort and view
    readonly property var options: UiConfig.pageOptions("playlist", playlistId)
    readonly property string sortKey: playlistOrder ? "position" : options.sort
    readonly property bool sortDescending: !playlistOrder && options.descending
    readonly property bool editing: Router.editing
    readonly property string reorderBlocker: PlayerService.whyNot("playlistReorder", playlist)

    // playlists fetch their tracks when opened
    onPlaylistIdChanged: {
        filterText = "";
        playlistOrder = false;
        PlayerService.loadPlaylist(playlistId);
    }

    function sortBy(key) {
        const descending = key === sortKey && !sortDescending;
        playlistOrder = false;
        UiConfig.setPageOptions("playlist", playlistId, Object.assign({}, options, {
            "sort": key,
            "descending": descending
        }));
    }

    // drop tracks dragged from any list to add them here
    TrackDropArea {
        radius: PlayerMetrics.paneRadius
        enabled: root.playlist !== null && Router.dragSource !== root.playlist?.id
        onDropped: PlayerService.reportSkipped(PlayerService.addToPlaylist(root.playlist.id, Router.dragTracks))
    }

    component Header: Column {
        width: parent?.width ?? 0
        spacing: Theme.spacingL
        bottomPadding: Theme.spacingL

        PageHeader {
            title: root.editing ? root.playlist?.name ?? "" : ""
            subtitle: root.editing ? Phrases.tracks(root.tracks.length) : ""
            // reordering edits the whole list
            searchPlaceholder: root.reordering ? "" : I18n.tr("Search in %1", "a page's own search field, %1 is the page, playlist or artist").arg(root.playlist?.name ?? "")
            searchText: root.filterText
            onSearchEdited: text => root.filterText = text
            editable: !root.reordering
            editing: root.editing
            onEditRequested: Router.editing = true
            onDone: Router.editing = false
            onResetRequested: editor.reset()

            DIconButton {
                id: moreButton
                anchors.verticalCenter: parent.verticalCenter
                iconName: "more_vert"
                tooltipText: I18n.tr("More", "track row menu button")
                Accessible.name: tooltipText
                onClicked: menu.open(moreButton)
            }
        }

        PageEditor {
            id: editor
            visible: root.editing
            sheetHost: root
            kind: "playlist"
            itemId: root.playlistId
            type: "tracks"
            scopeThis: I18n.tr("This playlist", "page edit scope")
            scopeAll: I18n.tr("All playlists", "page edit scope")
            layouts: ["list", "compact"]
            sorts: ["position"].concat(UiConfig.sortChoices("tracks"))
            itemType: "track"
        }

        DetailHero {
            visible: !root.editing
            eyebrow: root.server ? I18n.tr("Playlist on %1", "playlist kind label for a server playlist, %1 is the library").arg(root.serverName) : I18n.tr("Local playlist", "playlist kind label")
            title: root.playlist?.name ?? ""
            cover: PlaylistCover {
                anchors.fill: parent
                radius: Theme.cornerRadiusXL
                playlist: root.playlist
            }

            StyledText {
                width: parent.width
                text: {
                    const facts = Phrases.join(Phrases.tracks(root.tracks.length), Format.duration(root.tracks.reduce((sum, t) => sum + t.duration, 0)));
                    if (!root.server && root.libraryCount > 1)
                        return Phrases.join(facts, I18n.tr("from %1 libraries", "playlist info line").arg(root.libraryCount));
                    return facts;
                }
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceVariantText
            }

            PlayButtons {
                topPadding: Theme.spacingS
                enabled: root.tracks.length > 0
                onPlay: shuffle => {
                    // a search plays what it found
                    const ctx = {
                        "type": "playlist",
                        "id": root.playlist.id
                    };
                    if (root.filtering && shuffle)
                        PlayerService.shuffleTracks(root.shownTracks, ctx, true);
                    else if (root.filtering)
                        PlayerService.playTracks(root.shownTracks, 0, ctx);
                    else
                        PlayerService.playPlaylist(root.playlist.id, shuffle);
                }
            }

            KeepOfflineRow {
                kind: "playlist"
                item: root.playlist
            }
        }

        StyledText {
            width: parent.width
            visible: root.tracks.length === 0 || root.shownTracks.length === 0
            text: root.tracks.length === 0 ? I18n.tr("Add tracks from their menu or drag them here.", "empty playlist hint") : I18n.tr("Nothing matches \"%1\".", "search page empty result").arg(root.filterText.trim())
            font.pixelSize: Theme.fontSizeMedium
            color: Theme.surfaceVariantText
            wrapMode: Text.WordWrap
        }

        Row {
            visible: root.reordering
            spacing: Theme.spacingM

            StyledText {
                anchors.verticalCenter: parent.verticalCenter
                text: root.server ? I18n.tr("Drag to reorder. Saved to %1.", "playlist reorder hint for a server playlist").arg(root.serverName) : I18n.tr("Drag to reorder.", "playlist reorder hint")
                font.pixelSize: Theme.fontSizeMedium
                color: Theme.surfaceVariantText
            }

            DButton {
                text: I18n.tr("Done", "edit mode finish button")
                iconName: "check"
                buttonHeight: Theme.buttonHeightXS
                onClicked: root.reordering = false
            }
        }
    }

    TrackList {
        anchors.fill: parent
        anchors.leftMargin: PlayerMetrics.pagePadding
        anchors.rightMargin: PlayerMetrics.pagePadding
        visible: !root.reordering
        tracks: root.shownTracks
        lines: root.options.lines
        layout: root.options.layout
        playlistId: root.playlist?.id ?? ""
        showArt: false
        numbered: true
        sortable: true
        orderKey: "position"
        sortKey: root.sortKey
        descending: root.sortDescending
        onSortRequested: key => root.sortBy(key)
        header: Component {
            Header {}
        }
    }

    DFlickable {
        id: reorderFlick
        anchors.fill: parent
        anchors.leftMargin: PlayerMetrics.pagePadding
        anchors.rightMargin: PlayerMetrics.pagePadding
        visible: root.reordering
        contentWidth: width
        contentHeight: reorderColumn.height + PlayerMetrics.pagePadding
        clip: true

        Column {
            id: reorderColumn
            width: reorderFlick.width

            Header {}

            DReorderList {
                id: reorder
                width: parent.width
                flickable: reorderFlick
                model: root.reordering ? root.tracks : []
                onReordered: indices => PlayerService.reorderPlaylist(root.playlist.id, indices)

                delegate: ReorderRow {
                    id: entry
                    required property var modelData
                    view: reorder
                    label: modelData.title

                    StyledText {
                        anchors.left: entry.handle.right
                        anchors.leftMargin: Theme.spacingS
                        anchors.right: parent.right
                        anchors.rightMargin: Theme.spacingL
                        anchors.verticalCenter: parent.verticalCenter
                        text: Phrases.join(entry.modelData.title, entry.modelData.artist)
                        font.pixelSize: Theme.fontSizeMedium
                        color: entry.contentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                    }
                }
            }
        }
    }

    MediaMenu {
        id: menu
        kind: "playlist"
        item: root.playlist
        extra: [menu.entry("cover", I18n.tr("Change cover", "playlist menu action"), "image"), menu.entry("reorder", I18n.tr("Reorder tracks", "playlist menu action"), "swap_vert", root.reorderBlocker)]
        onExtraTriggered: id => {
            if (id === "cover")
                coverDialog.opened = true;
            else if (id === "reorder") {
                Router.editing = false;
                root.playlistOrder = true;
                root.filterText = "";
                root.reordering = true;
            }
        }
    }

    CoverDialog {
        id: coverDialog
        host: root
        playlist: root.playlist
    }
}
