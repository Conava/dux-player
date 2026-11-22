pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import "../Services/Catalog.js" as Catalog

// One home shelf: a source of items shown as a carousel row, a grid or a list. The title is the link to the
// full page; "Continue listening" is a single card. Items show the info lines of their page kind.
Column {
    id: root

    // a home shelf instance {id, type, hidden, options: {source?, layout, size}}
    required property var shelf
    required property string title
    readonly property var source: ({
            "type": shelf.type,
            "id": shelf.options.source ?? ""
        })
    readonly property var savedView: source.type === "view" ? UiConfig.effective.savedViews.find(v => v.id === source.id) ?? null : null
    // fixed per shelf so the row doesn't reshuffle on every change
    property var randomOrder: []
    // the order of a custom page set to Random, fixed per shelf
    readonly property int seed: Date.now() % 1000000007
    readonly property string kind: {
        switch (source.type) {
        case "favoriteTracks":
        case "recentTracks":
        case "mostPlayed":
        case "playlist":
            return "track";
        case "playlists":
            return "playlist";
        case "view":
            return UiConfig.kindOf(savedView?.type ?? "albums");
        }
        return "album";
    }
    readonly property var items: {
        switch (source.type) {
        case "favoriteTracks":
            return PlayerService.favoriteTracks();
        case "recentTracks":
            return PlayerService.recentTracks.map(t => PlayerService.track(t)).filter(t => PlayerService.enabledLibrary(t.library));
        case "mostPlayed":
            return PlayerService.mostPlayed();
        case "recentlyPlayed":
            return PlayerService.recentAlbums.map(a => PlayerService.album(a.id) ?? a).filter(a => PlayerService.enabledLibrary(a.library));
        case "recentlyAdded":
            return PlayerService.visibleAlbums().sort((a, b) => b.added - a.added);
        case "randomAlbums":
            return randomOrder.map(id => PlayerService.album(id)).filter(a => a && PlayerService.enabledLibrary(a.library));
        case "favoriteAlbums":
            return PlayerService.visibleAlbums().filter(a => a.favorite);
        case "playlists":
            return PlayerService.query("playlists", {
                "sort": "name"
            });
        case "playlist":
            return PlayerService.playlistTracks(source.id);
        case "genre":
            return PlayerService.query("albums", {
                "sort": "title",
                "filters": {
                    "genre": source.id
                }
            });
        case "library":
            return PlayerService.query("albums", {
                "sort": "title",
                "filters": {
                    "library": source.id
                }
            });
        case "view":
            {
                if (!savedView)
                    return [];
                const options = UiConfig.savedViewOptions(savedView);
                return Catalog.shape(PlayerService.query(savedView.type, options.sort === "random" ? Object.assign({}, options, {
                    "sort": ""
                }) : options), options, seed, "");
            }
        }
        return [];
    }
    readonly property var lines: (savedView ? UiConfig.savedViewOptions(savedView) : UiConfig.pageOptions(kind + "s", "")).lines
    readonly property var lineOptions: (savedView ? UiConfig.savedViewOptions(savedView) : UiConfig.pageOptions(kind + "s", "")).lineOptions ?? ({})
    readonly property string layout: shelf.options.layout ?? "carousel"
    readonly property string size: shelf.options.size ?? "m"
    readonly property real tileWidth: size === "s" ? PlayerMetrics.tileSizeS : size === "l" ? PlayerMetrics.tileSizeL : PlayerMetrics.shelfArtSize
    readonly property real captionHeight: size === "l" ? PlayerMetrics.gridCaptionHeightL : PlayerMetrics.gridCaptionHeight
    readonly property int listLimit: size === "s" ? 3 : size === "l" ? 10 : 5
    // "auto" fits as many columns of at least listColumnMinWidth as the width allows; the size stays rows per column
    readonly property int listColumns: shelf.options.columns === "auto" ? Math.max(1, Math.floor((width + Theme.spacingS) / (PlayerMetrics.listColumnMinWidth + Theme.spacingS))) : 1
    // counts, not sliced arrays: a width step then adds or removes the tiles at the end and rebuilds none
    readonly property int listCount: layout === "list" ? Math.min(items.length, listLimit * listColumns) : 0
    // a carousel holds enough tiles to fill its width; grid and list shuffle the shelfLimit they always did
    readonly property int shownLimit: layout === "carousel" ? Math.max(PlayerMetrics.shelfLimit, Math.ceil(carouselList.width / (tileWidth + PlayerMetrics.tileInset * 2 + Theme.spacingXS))) : PlayerMetrics.shelfLimit
    readonly property int carouselCount: Math.min(items.length, shownLimit)
    readonly property int gridRows: size === "s" ? 1 : size === "l" ? 3 : 2
    readonly property var seeAll: {
        switch (source.type) {
        case "favoriteTracks":
        case "favoriteAlbums":
            return ["favorites", ""];
        case "recentTracks":
            return ["history", ""];
        case "mostPlayed":
            return ["mostPlayed", ""];
        case "playlists":
            return ["playlists", ""];
        case "recentlyAdded":
            return ["albums", "recent"];
        case "playlist":
        case "genre":
        case "library":
        case "view":
            return [source.type, source.id];
        }
        return ["albums", ""];
    }

    readonly property var playContext: {
        switch (source.type) {
        case "favoriteTracks":
            return {
                "type": "favorites"
            };
        case "playlist":
        case "genre":
        case "library":
        case "view":
            return {
                "type": source.type,
                "id": source.id
            };
        case "recentTracks":
            return {
                "type": "history"
            };
        }
        return {
            "type": "tracks"
        };
    }
    readonly property bool shuffleable: source.type !== "continue" && (kind === "track" || kind === "album")

    function play(index) {
        if (kind === "track" && savedView && UiConfig.savedViewOptions(savedView).click === "one")
            PlayerService.playTracks(PlayerService.refsOf([items[index]]), 0, playContext);
        else
            PlayerService.playTracks(PlayerService.refsOf(items), index, playContext);
    }

    // An album shelf shuffles the tracks of the albums it shows (up to shownLimit), one
    // browse.album per album not loaded yet
    function shuffleAll() {
        // favorites and playlists shuffle in the daemon over all they hold; rankings shuffle what they show
        if (kind === "track") {
            PlayerService.shuffleTracks(items, playContext, source.type !== "favoriteTracks" && source.type !== "playlist");
            return;
        }
        if (source.type === "library" && CatalogService.hasCapability(source.id, "randomTracks")) {
            PlayerService.shuffleTracks([], playContext, false);
            return;
        }
        const albums = items.slice(0, shownLimit);
        let left = albums.length;
        let all = [];
        for (const a of albums)
            CatalogService.withAlbumTracks(a.id, list => {
                all = all.concat(list);
                if (--left === 0)
                    PlayerService.shuffleTracks(all, root.playContext, true);
            });
    }

    width: parent?.width ?? 0
    spacing: Theme.spacingS
    readonly property bool hasContent: source.type === "continue" ? PlayerService.current !== null : items.length > 0
    visible: hasContent
    // albums arrive after the window opens and with every scan; new ones go in at random places, the rest keep theirs
    readonly property int albumsRevision: source.type === "randomAlbums" ? CatalogService.albumsRevision : 0
    onAlbumsRevisionChanged: {
        const known = new Set(randomOrder);
        const order = randomOrder.filter(id => CatalogService.album(id));
        for (const a of CatalogService.albums)
            if (!known.has(a.id))
                order.splice(Math.floor(Math.random() * (order.length + 1)), 0, a.id);
        randomOrder = order;
    }

    // the header and tiles bleed into the page padding so their labels and art line up with the page title
    ShelfHeader {
        title: root.title
        seeAll: root.source.type === "continue" ? null : root.seeAll
        shuffleShown: root.shuffleable && root.items.length > (root.kind === "album" ? 0 : 1)
        carousel: carouselList.used ? carouselList : null
        tileStep: root.tileWidth + PlayerMetrics.tileInset * 2 + carouselList.spacing
        onShuffleRequested: root.shuffleAll()
    }

    ContinueCard {
        visible: root.source.type === "continue" && PlayerService.current !== null
    }

    DHorizontalList {
        id: carouselList
        x: -PlayerMetrics.tileInset
        width: parent.width + PlayerMetrics.tileInset * 2
        // not `visible`, which follows Home: hidden, it keeps its tiles for the way back
        readonly property bool used: root.layout === "carousel" && root.source.type !== "continue"
        visible: used
        height: used ? root.tileWidth + PlayerMetrics.tileInset * 2 + root.captionHeight : 0
        spacing: Theme.spacingXS
        model: used ? root.carouselCount : 0

        // ends the row where the user is looking
        footer: StyledButton {
            id: seeAllTile
            width: root.tileWidth + PlayerMetrics.tileInset * 2
            height: carouselList.height
            radius: Theme.cornerRadiusL
            color: "transparent"
            Accessible.role: Accessible.Link
            Accessible.name: I18n.tr("See all", "home shelf action")
            onClicked: Router.go(root.seeAll[0], root.seeAll[1])

            StateLayer {
                control: seeAllTile
                stateColor: Theme.onSurface
            }

            Column {
                anchors.centerIn: parent
                anchors.verticalCenterOffset: -root.captionHeight / 2
                spacing: Theme.spacingS

                Rectangle {
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: Theme.buttonHeightM
                    height: width
                    radius: Theme.fullRadius(width, height)
                    color: Theme.secondaryContainer

                    DIcon {
                        anchors.centerIn: parent
                        name: "arrow_forward"
                        rotation: I18n.isRtl ? 180 : 0
                        size: Theme.iconSize
                        color: Theme.onSecondaryContainer
                    }
                }

                StyledText {
                    anchors.horizontalCenter: parent.horizontalCenter
                    text: I18n.tr("See all", "home shelf action")
                    font.pixelSize: Theme.fontSizeMedium
                    font.weight: Theme.fontWeightMedium
                    color: Theme.surfaceText
                }
            }
        }

        delegate: MediaTile {
            required property int index
            width: root.tileWidth + PlayerMetrics.tileInset * 2
            tileSize: root.size
            kind: root.kind
            item: root.items[index]
            lines: root.lines
            lineOptions: root.lineOptions
            onActivated: root.play(index)
        }
    }

    Flow {
        id: flow
        x: -PlayerMetrics.tileInset
        width: parent.width + PlayerMetrics.tileInset * 2
        readonly property bool used: root.layout === "grid" && root.source.type !== "continue"
        visible: used
        readonly property int columns: Math.max(1, Math.floor(width / (root.tileWidth + PlayerMetrics.tileInset * 2)))
        readonly property int wanted: used ? Math.min(root.items.length, columns * root.gridRows) : 0
        // tiles are only ever added: a narrower width hides the extra ones, so docking a panel rebuilds none
        property int built: 0
        onWantedChanged: built = wanted === 0 ? 0 : Math.max(built, wanted)
        Component.onCompleted: built = wanted

        Repeater {
            model: Math.min(flow.built, root.items.length)

            MediaTile {
                required property int index
                visible: index < flow.wanted
                width: flow.width / flow.columns
                tileSize: root.size
                kind: root.kind
                item: root.items[index]
                lines: root.lines
                lineOptions: root.lineOptions
                onActivated: root.play(index)
            }
        }
    }

    Row {
        id: listRow
        width: parent.width
        visible: root.layout === "list" && root.source.type !== "continue"
        spacing: Theme.spacingS

        // each column is its own connected group, filled top to bottom before the next
        readonly property int columnCount: Math.max(1, Math.ceil(root.listCount / root.listLimit))
        readonly property real columnWidth: (width - spacing * (columnCount - 1)) / columnCount

        Repeater {
            model: root.layout === "list" ? listRow.columnCount : 0

            Column {
                id: listColumn
                required property int index
                readonly property int offset: index * root.listLimit
                width: listRow.columnWidth
                spacing: Theme.groupedListGap

                Repeater {
                    id: rows
                    model: Math.max(0, Math.min(root.listLimit, root.listCount - listColumn.offset))

                    Loader {
                        id: rowLoader
                        required property int index
                        readonly property int itemIndex: listColumn.offset + index
                        readonly property var modelData: root.items[itemIndex]
                        width: parent?.width ?? 0
                        sourceComponent: root.kind === "track" ? trackRow : mediaRow

                        Component {
                            id: trackRow
                            TrackRow {
                                track: rowLoader.modelData
                                showArt: true
                                lines: root.lines
                                lineOptions: root.lineOptions
                                firstInGroup: rowLoader.index === 0
                                lastInGroup: rowLoader.index === rows.count - 1
                                onActivated: root.play(rowLoader.itemIndex)
                            }
                        }

                        Component {
                            id: mediaRow
                            MediaRow {
                                kind: root.kind
                                item: rowLoader.modelData
                                lines: root.lines
                                lineOptions: root.lineOptions
                                firstInGroup: rowLoader.index === 0
                                lastInGroup: rowLoader.index === rows.count - 1
                            }
                        }
                    }
                }
            }
        }
    }
}
