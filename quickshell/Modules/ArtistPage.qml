pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import "../Services/Catalog.js" as Catalog

// An artist: image, favorite, biography, playlists holding their tracks, similar artists, then their albums
// or tracks (the page's sections: the first one shown is where it opens).
Item {
    id: root

    readonly property string artistId: Router.itemId
    readonly property var catalogArtist: CatalogService.artist(artistId)
    // browse.artist's answer; null until it arrives or when it fails
    property var info: null
    property var fetchedAlbums: null
    property var artistTracks: []
    property var playlistIds: []
    property string imageUrl: ""
    readonly property var options: UiConfig.pageOptions("artist", artistId)
    readonly property bool editing: Router.editing
    readonly property var shownSections: (options.sections ?? []).filter(s => !s.hidden).map(s => s.type)
    // the section picked on the page; runtime only, a new artist opens on the first one shown
    property string pickedView: ""
    readonly property string view: shownSections.indexOf(pickedView) >= 0 ? pickedView : shownSections[0] ?? "albums"
    readonly property real tileWidth: options.tileSize === "s" ? PlayerMetrics.tileSizeS : options.tileSize === "l" ? PlayerMetrics.tileSizeL : PlayerMetrics.gridCellWidth
    property bool biographyOpen: false
    property string filterText: ""
    readonly property bool filtering: filterText.trim() !== ""
    readonly property var shownAlbums: CatalogService.refine(Catalog.filterText(albums, filterText), "albums", {
        "sort": options.sort,
        "descending": options.descending
    })
    // the tracks most played first, with the Tracks page's info lines
    readonly property var shownTracks: CatalogService.refine(Catalog.filterText(artistTracks, filterText), "tracks", {
        "sort": "playCount",
        "descending": true
    })

    readonly property var artist: {
        if (!catalogArtist && !info)
            return null;
        return CatalogService.withArtistMarks(Object.assign({}, catalogArtist ?? {}, info ?? {}));
    }
    readonly property var albums: fetchedAlbums ?? CatalogService.artistAlbums(artistId)
    readonly property var playlists: playlistIds.map(id => PlaylistService.playlist(id)).filter(p => p)
    readonly property string biography: (info?.biography ?? "").replace(/<[^>]*>/g, "").replace(/[ \t]+/g, " ").trim()
    readonly property int trackCount: info?.trackCount || artistTracks.length
    readonly property var context: ({
            "type": "artist",
            "id": root.artistId,
            "name": root.artist?.name ?? ""
        })

    // also runs once at creation: the id differs from the default then
    onArtistIdChanged: load()

    function load() {
        const id = artistId;
        info = null;
        fetchedAlbums = null;
        artistTracks = [];
        playlistIds = [];
        imageUrl = "";
        biographyOpen = false;
        filterText = "";
        pickedView = "";
        if (id.indexOf(":") < 0)
            return;
        CatalogService.artistInfo(id, a => {
            if (root?.artistId !== id)
                return;
            root.info = a;
            const who = root.artist;
            if (!who)
                return;
            CatalogService.artistImage(who, url => {
                if (root?.artistId === id)
                    root.imageUrl = url;
            });
            if (!root.catalogArtist)
                root.loadPlaylists(who);
        });
        CatalogService.artistAlbumList(id, list => {
            if (root?.artistId === id && list.length > 0)
                root.fetchedAlbums = list;
        });
        CatalogService.artistTracks(id, list => {
            if (root?.artistId === id)
                root.artistTracks = list;
        });
        // the slowest answer: asked on its own, the section shows when it comes
        if (catalogArtist)
            loadPlaylists(catalogArtist);
    }

    // the offset to put back once the list has redone its rows; -1 when none
    property real pendingOffset: -1
    readonly property int rowCount: list.view.count
    readonly property real listOrigin: list.view.originY
    readonly property bool listMoving: list.view.moving

    // A new track count resets the list to the top after the switch; the header above the switch is the same in
    // both views, so the offset is put back once the rows are redone.
    function setView(next) {
        pendingOffset = list.view.contentY - list.view.originY;
        pickedView = next;
        Qt.callLater(restoreOffset);
    }

    function restoreOffset() {
        if (pendingOffset < 0)
            return;
        const lv = list.view;
        lv.forceLayout();
        lv.contentY = lv.originY + Math.max(0, Math.min(pendingOffset, lv.contentHeight + lv.bottomMargin - lv.height));
    }

    // the rows and the header (the album grid comes and goes) settle a moment after the switch
    onRowCountChanged: Qt.callLater(restoreOffset)
    onListOriginChanged: Qt.callLater(restoreOffset)
    onListMovingChanged: {
        if (listMoving)
            pendingOffset = -1;
    }
    onFilterTextChanged: pendingOffset = -1

    function loadPlaylists(who) {
        const id = artistId;
        CatalogService.artistPlaylists(who, ids => {
            if (root?.artistId === id)
                root.playlistIds = ids;
        });
    }

    Connections {
        target: CatalogService

        // playlists can match by name across libraries, so any library's update counts
        function onArtistPlaylistsChanged() {
            if (root.catalogArtist)
                root.loadPlaylists(root.catalogArtist);
        }
    }

    component SectionTitle: StyledText {
        width: parent?.width ?? 0
        font.pixelSize: Theme.fontSizeLarge
        font.weight: Theme.fontWeightMedium
        color: Theme.surfaceText
        horizontalAlignment: Text.AlignLeft
    }

    TrackList {
        id: list
        anchors.fill: parent
        anchors.leftMargin: PlayerMetrics.pagePadding
        anchors.rightMargin: PlayerMetrics.pagePadding
        tracks: root.view === "tracks" ? root.shownTracks : []
        context: root.context
        lines: UiConfig.pageOptions("tracks", "").lines

        header: Component {
            Column {
                width: list.width
                spacing: Theme.spacingL
                bottomPadding: Theme.spacingL

                PageHeader {
                    title: root.editing ? root.artist?.name ?? "" : ""
                    editable: true
                    editing: root.editing
                    onEditRequested: Router.editing = true
                    onDone: Router.editing = false
                    onResetRequested: editor.reset()
                    searchPlaceholder: I18n.tr("Search in %1", "a page's own search field, %1 is the page, playlist or artist").arg(root.artist?.name ?? "")
                    searchText: root.filterText
                    onSearchEdited: text => root.filterText = text

                    DIconButton {
                        id: moreButton
                        anchors.verticalCenter: parent.verticalCenter
                        visible: root.artist !== null
                        iconName: "more_vert"
                        tooltipText: I18n.tr("More", "track row menu button")
                        Accessible.name: tooltipText
                        onClicked: artistMenu.open(moreButton)
                    }
                }

                PageEditor {
                    id: editor
                    visible: root.editing
                    sheetHost: root
                    kind: "artist"
                    itemId: root.artistId
                    type: "albums"
                    scopeThis: I18n.tr("This artist", "page edit scope")
                    scopeAll: I18n.tr("All artists", "page edit scope")
                    // layout, sort and lines are the albums'; the tracks follow the Tracks page
                    layouts: root.view === "albums" ? ["grid", "list", "compact"] : []
                    sorts: root.view === "albums" ? UiConfig.sortChoices("albums") : []
                    linesEditable: root.view === "albums"
                    itemType: root.view === "albums" ? "album" : "track"
                }

                DetailHero {
                    visible: !root.editing
                    eyebrow: I18n.tr("Artist", "track list column")
                    title: root.artist?.name ?? ""
                    cover: AlbumArt {
                        anchors.fill: parent
                        radius: Theme.fullRadius(width, height)
                        item: root.artist
                        imageUrl: root.imageUrl
                    }

                    Row {
                        spacing: Theme.spacingS

                        LibraryBadge {
                            anchors.verticalCenter: parent.verticalCenter
                            libraryId: root.artist?.library ?? ""
                        }

                        StyledText {
                            anchors.verticalCenter: parent.verticalCenter
                            text: Phrases.join(Phrases.albums(root.albums.length), Phrases.tracks(root.trackCount))
                            font.pixelSize: Theme.fontSizeMedium
                            color: Theme.surfaceVariantText
                        }
                    }

                    PlayButtons {
                        topPadding: Theme.spacingS
                        enabled: root.artist !== null
                        onPlay: shuffle => {
                            // a search plays the tracks it found
                            if (root.filtering && shuffle)
                                PlayerService.shuffleTracks(root.shownTracks, root.context, true);
                            else if (root.filtering)
                                PlayerService.playTracks(root.shownTracks, 0, root.context);
                            else
                                PlayerService.playArtist(root.artist, shuffle);
                        }

                        FavoriteButton {
                            anchors.verticalCenter: parent.verticalCenter
                            kind: "artist"
                            item: root.artist
                        }
                    }

                    KeepOfflineRow {
                        kind: "artist"
                        item: root.artist
                    }
                }

                Column {
                    width: parent.width
                    spacing: Theme.spacingXS
                    visible: root.biography !== "" && !root.editing

                    StyledText {
                        id: biographyText
                        width: parent.width
                        text: root.biography
                        font.pixelSize: Theme.fontSizeMedium
                        color: Theme.surfaceVariantText
                        wrapMode: Text.WordWrap
                        elide: root.biographyOpen ? Text.ElideNone : Text.ElideRight
                        horizontalAlignment: Text.AlignLeft

                        Binding on maximumLineCount {
                            value: PlayerMetrics.biographyLines
                            when: !root.biographyOpen
                        }
                    }

                    // pulled out by the text button's padding so its label lines up with the text
                    Row {
                        x: -moreLess.leftPadding
                        spacing: Theme.spacingS

                        DButton {
                            id: moreLess
                            visible: biographyText.truncated || root.biographyOpen
                            text: root.biographyOpen ? I18n.tr("Show less", "collapses an artist's biography") : I18n.tr("Show more", "expands an artist's biography")
                            buttonHeight: Theme.buttonHeightXS
                            backgroundColor: "transparent"
                            textColor: Theme.primary
                            onClicked: root.biographyOpen = !root.biographyOpen
                        }

                        LinkText {
                            anchors.verticalCenter: parent.verticalCenter
                            visible: (root.info?.biographySource?.name ?? "") !== ""
                            text: I18n.tr("From %1", "where an artist's biography comes from, %1 is the source").arg(root.info?.biographySource?.name ?? "")
                            linked: (root.info?.biographySource?.url ?? "").startsWith("https://")
                            font.pixelSize: Theme.fontSizeSmall
                            color: Theme.surfaceVariantText
                            onActivated: Qt.openUrlExternally(root.info.biographySource.url)
                        }
                    }
                }

                SectionTitle {
                    visible: root.playlists.length > 0 && !root.editing
                    text: I18n.tr("Playlists with this artist", "artist page section")
                }

                DHorizontalList {
                    width: parent.width
                    visible: root.playlists.length > 0 && !root.editing
                    height: visible ? PlayerMetrics.tileSizeS + Theme.spacingS * 2 + PlayerMetrics.gridCaptionHeight : 0
                    spacing: Theme.spacingXS
                    model: root.playlists

                    delegate: MediaTile {
                        required property var modelData
                        width: PlayerMetrics.tileSizeS + Theme.spacingS * 2
                        kind: "playlist"
                        item: modelData
                        lines: UiConfig.pageOptions("playlists", "").lines
                    }
                }

                SectionTitle {
                    visible: (root.info?.similar ?? []).length > 0 && !root.editing
                    text: I18n.tr("Similar artists", "artist page section")
                }

                DHorizontalList {
                    width: parent.width
                    visible: (root.info?.similar ?? []).length > 0 && !root.editing
                    height: visible ? PlayerMetrics.tileSizeS + Theme.spacingS * 2 + PlayerMetrics.gridCaptionHeight : 0
                    spacing: Theme.spacingXS
                    model: root.info?.similar ?? []

                    delegate: MediaTile {
                        required property var modelData
                        width: PlayerMetrics.tileSizeS + Theme.spacingS * 2
                        kind: "artist"
                        item: modelData
                    }
                }

                DButtonGroup {
                    visible: root.shownSections.length > 1
                    size: "small"
                    model: root.shownSections.map(t => UiConfig.typeLabel(t))
                    currentIndex: root.shownSections.indexOf(root.view)
                    onSelectionChanged: (index, selected) => {
                        if (selected)
                            root.setView(root.shownSections[index]);
                    }
                }

                StyledText {
                    width: parent.width
                    visible: root.filtering && (root.view === "albums" ? root.shownAlbums : root.shownTracks).length === 0
                    text: I18n.tr("Nothing matches \"%1\".", "search page empty result").arg(root.filterText.trim())
                    font.pixelSize: Theme.fontSizeMedium
                    color: Theme.surfaceVariantText
                    horizontalAlignment: Text.AlignLeft
                }

                Flow {
                    id: albumFlow
                    readonly property bool grid: root.options.layout === "grid"
                    readonly property int columnCount: Math.max(1, Math.floor(width / root.tileWidth))
                    width: parent.width
                    visible: root.view === "albums"
                    spacing: grid ? 0 : Theme.groupedListGap

                    Repeater {
                        model: root.view === "albums" && albumFlow.grid ? root.shownAlbums : []

                        MediaTile {
                            required property var modelData
                            width: Math.floor(albumFlow.width / albumFlow.columnCount)
                            kind: "album"
                            item: modelData
                            lines: root.options.lines
                        }
                    }

                    Repeater {
                        model: root.view === "albums" && !albumFlow.grid ? root.shownAlbums : []

                        MediaRow {
                            required property var modelData
                            required property int index
                            width: albumFlow.width
                            kind: "album"
                            item: modelData
                            lines: root.options.lines
                            compact: root.options.layout === "compact"
                            firstInGroup: index === 0
                            lastInGroup: index === root.shownAlbums.length - 1
                        }
                    }
                }
            }
        }
    }

    MediaMenu {
        id: artistMenu
        kind: "artist"
        item: root.artist
    }
}
