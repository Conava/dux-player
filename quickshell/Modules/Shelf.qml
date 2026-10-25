pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

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
            return savedView ? PlayerService.query(savedView.type, savedView.options) : [];
        }
        return [];
    }
    readonly property var lines: savedView?.options?.lines ?? UiConfig.pageOptions(kind + "s", "").lines
    readonly property string layout: shelf.options.layout ?? "carousel"
    readonly property string size: shelf.options.size ?? "m"
    readonly property real tileWidth: size === "s" ? PlayerMetrics.tileSizeS : size === "l" ? PlayerMetrics.tileSizeL : PlayerMetrics.shelfArtSize
    readonly property int listLimit: size === "s" ? 3 : size === "l" ? 10 : 5
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
        PlayerService.playTracks(PlayerService.refsOf(items), index, playContext);
    }

    // An album shelf shuffles the tracks of the albums it shows (up to shelfLimit), one
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
        const albums = items.slice(0, PlayerMetrics.shelfLimit);
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
    // albums arrive after the window opens; the order is drawn once they're in
    readonly property int albumCount: source.type === "randomAlbums" ? PlayerService.albums.length : 0
    onAlbumCountChanged: {
        if (randomOrder.length === 0)
            randomOrder = PlayerService.shuffled(PlayerService.albums.map(a => a.id));
    }

    Row {
        spacing: Theme.spacingS

        // the title opens the full page
        StyledButton {
            id: titleLink
            visible: root.source.type !== "continue"
            anchors.verticalCenter: parent.verticalCenter
            width: titleRow.implicitWidth + Theme.spacingS * 2
            height: Theme.buttonHeightXS
            radius: Theme.fullRadius(width, height)
            color: "transparent"
            Accessible.role: Accessible.Link
            Accessible.name: I18n.tr("%1, see all", "home shelf title link").arg(root.title)
            onClicked: Router.go(root.seeAll[0], root.seeAll[1])

            StateLayer {
                control: titleLink
                stateColor: Theme.onSurface
            }

            Row {
                id: titleRow
                anchors.centerIn: parent
                spacing: Theme.spacingXXS

                StyledText {
                    anchors.verticalCenter: parent.verticalCenter
                    text: root.title
                    font.pixelSize: Theme.fontSizeLarge
                    font.weight: Theme.fontWeightMedium
                    color: Theme.surfaceText
                }

                DIcon {
                    anchors.verticalCenter: parent.verticalCenter
                    name: "chevron_right"
                    rotation: I18n.isRtl ? 180 : 0
                    size: Theme.iconSize
                    color: Theme.surfaceText
                }
            }
        }

        StyledText {
            visible: root.source.type === "continue"
            anchors.verticalCenter: parent.verticalCenter
            text: root.title
            font.pixelSize: Theme.fontSizeLarge
            font.weight: Theme.fontWeightMedium
            color: Theme.surfaceText
        }

        DIconButton {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.shuffleable && root.items.length > (root.kind === "album" ? 0 : 1)
            iconName: "shuffle"
            variant: "tonal"
            tooltipText: I18n.tr("Shuffle", "button or mode that plays in random order")
            Accessible.name: tooltipText
            onClicked: root.shuffleAll()
        }
    }

    ContinueCard {
        visible: root.source.type === "continue" && PlayerService.current !== null
    }

    DHorizontalList {
        id: carousel
        width: parent.width
        visible: root.layout === "carousel" && root.source.type !== "continue"
        height: visible ? root.tileWidth + Theme.spacingS * 2 + PlayerMetrics.gridCaptionHeight : 0
        spacing: Theme.spacingXS
        model: visible ? root.items.slice(0, PlayerMetrics.shelfLimit) : []

        // ends the row where the user is looking
        footer: StyledButton {
            id: seeAllTile
            width: root.tileWidth + Theme.spacingS * 2
            height: carousel.height
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
                anchors.verticalCenterOffset: -PlayerMetrics.gridCaptionHeight / 2
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
            required property var modelData
            required property int index
            width: root.tileWidth + Theme.spacingS * 2
            kind: root.kind
            item: modelData
            lines: root.lines
            onActivated: root.play(index)
        }
    }

    Flow {
        id: flow
        width: parent.width
        visible: root.layout === "grid" && root.source.type !== "continue"
        readonly property int columns: Math.max(1, Math.floor(width / (root.tileWidth + Theme.spacingS * 2)))

        Repeater {
            model: flow.visible ? root.items.slice(0, flow.columns * root.gridRows) : []

            MediaTile {
                required property var modelData
                required property int index
                width: flow.width / flow.columns
                kind: root.kind
                item: modelData
                lines: root.lines
                onActivated: root.play(index)
            }
        }
    }

    Column {
        width: parent.width
        visible: root.layout === "list" && root.source.type !== "continue"
        spacing: Theme.groupedListGap

        Repeater {
            id: rows
            model: root.layout === "list" ? root.items.slice(0, root.listLimit) : []

            Loader {
                id: rowLoader
                required property var modelData
                required property int index
                width: parent?.width ?? 0
                sourceComponent: root.kind === "track" ? trackRow : mediaRow

                Component {
                    id: trackRow
                    TrackRow {
                        track: rowLoader.modelData
                        showArt: true
                        lines: root.lines
                        firstInGroup: rowLoader.index === 0
                        lastInGroup: rowLoader.index === rows.count - 1
                        onActivated: root.play(rowLoader.index)
                    }
                }

                Component {
                    id: mediaRow
                    MediaRow {
                        kind: root.kind
                        item: rowLoader.modelData
                        lines: root.lines
                        firstInGroup: rowLoader.index === 0
                        lastInGroup: rowLoader.index === rows.count - 1
                    }
                }
            }
        }
    }
}
