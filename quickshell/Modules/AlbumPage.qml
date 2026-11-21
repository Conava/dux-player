pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Modules.Settings
import "../Common/Format.js" as Format

Item {
    id: root

    readonly property var album: PlayerService.album(Router.itemId)
    readonly property var tracks: album ? PlayerService.albumTracks(album.id) : []
    readonly property var options: UiConfig.pageOptions("album", album?.id ?? "")
    readonly property bool editing: Router.editing
    readonly property bool notFound: PlayerService.albumMissing(Router.itemId)
    // a disc header per disc, when the album has more than one
    readonly property bool byDisc: options.group === "disc" && tracks.some(t => t.disc !== tracks[0].disc)
    readonly property var shownTracks: byDisc ? tracks.map(t => Object.assign({}, t, {
            "discLabel": I18n.tr("Disc %1", "album page section header, %1 is the disc number").arg(t.disc)
        })) : tracks
    // the album may load after the page opens (catalog still coming in)
    readonly property string loadKey: album?.id ?? ""
    // the cover card beside the tracks; narrow windows and edit mode stack
    readonly property bool sideBySide: UiConfig.albumSideBySide && width >= PlayerMetrics.albumSideBySideMinWidth && !editing && !notFound
    readonly property real cardWidth: Math.min(PlayerMetrics.albumPaneMaxWidth, Math.round((width - PlayerMetrics.pagePadding * 2) * PlayerMetrics.albumPaneRatio))

    onLoadKeyChanged: PlayerService.loadAlbum(loadKey)

    PageTint {
        seed: root.editing || root.notFound ? "" : PlayerService.coverColor(root.album)
        // fixed side by side, where the header row and the card don't scroll
        scroll: root.sideBySide ? 0 : list.view.contentY - list.view.originY
    }

    TrackList {
        id: list
        anchors.fill: parent
        anchors.topMargin: root.sideBySide ? fixedHeader.height : 0
        anchors.leftMargin: PlayerMetrics.pagePadding + (root.sideBySide && !I18n.isRtl ? root.cardWidth + Theme.spacingL : 0)
        anchors.rightMargin: PlayerMetrics.pagePadding + (root.sideBySide && I18n.isRtl ? root.cardWidth + Theme.spacingL : 0)
        tracks: root.shownTracks
        sectionProperty: root.byDisc ? "discLabel" : ""
        layout: root.options.layout
        context: ({
                "type": "album",
                "id": root.album?.id ?? ""
            })
        showArt: false
        numbered: true
        lines: root.options.lines
        lineOptions: root.options.lineOptions ?? ({})

        header: Component {
            Column {
                width: list.width
                spacing: PlayerMetrics.sectionSpacing
                bottomPadding: root.sideBySide ? 0 : PlayerMetrics.sectionSpacing

                Loader {
                    width: parent.width
                    active: !root.sideBySide
                    visible: active
                    sourceComponent: headerRow
                    // Reset only shows while editing, which always stacks
                    onLoaded: (item as PageHeader).resetRequested.connect(() => (editorLoader.item as PageEditor)?.reset())
                }

                // built only while editing
                Loader {
                    id: editorLoader
                    width: parent.width
                    active: root.editing
                    visible: active

                    sourceComponent: PageEditor {
                        id: editor
                        sheetHost: root
                        kind: "album"
                        itemId: root.album?.id ?? ""
                        type: "tracks"
                        scopeThis: I18n.tr("This album", "page edit scope")
                        scopeAll: I18n.tr("All albums", "page edit scope")
                        layouts: ["list", "compact"]
                        itemType: "track"

                        SettingsToggleRow {
                            text: I18n.tr("Group by disc", "page edit mode: a header per disc on an album page")
                            checked: editor.options.group === "disc"
                            onToggled: value => editor.apply({
                                    "group": value ? "disc" : ""
                                })
                        }
                    }
                }

                EmptyState {
                    width: parent.width
                    visible: root.notFound
                    iconName: "search_off"
                    title: I18n.tr("Not found", "page title when an album, artist or playlist doesn't exist")
                }

                Loader {
                    width: parent.width
                    active: !root.sideBySide && !root.editing && !root.notFound
                    visible: active
                    sourceComponent: hero
                }
            }
        }
    }

    // side by side: the header row stays put above the card and the tracks
    Loader {
        id: fixedHeader
        x: PlayerMetrics.pagePadding
        width: root.width - PlayerMetrics.pagePadding * 2
        active: root.sideBySide
        visible: active
        sourceComponent: headerRow
    }

    // built only side by side
    Loader {
        x: I18n.isRtl ? root.width - PlayerMetrics.pagePadding - width : PlayerMetrics.pagePadding
        y: fixedHeader.height
        width: root.cardWidth
        active: root.sideBySide
        visible: active

        sourceComponent: Rectangle {
            height: Math.min(cardHero.height + Theme.spacingXL * 2, root.height - fixedHeader.height - PlayerMetrics.pagePadding)
            color: Theme.surfaceContainerHigh
            radius: Theme.cornerRadiusXL

            DFlickable {
                anchors.fill: parent
                anchors.margins: Theme.spacingXL
                clip: true
                contentHeight: cardHero.height
                interactive: contentHeight > height
                showScrollBar: interactive

                Loader {
                    id: cardHero
                    width: parent.width
                    sourceComponent: hero
                }
            }
        }
    }

    Component {
        id: headerRow

        PageHeader {
            title: root.editing ? root.album?.title ?? "" : ""
            editable: true
            editing: root.editing
            onEditRequested: Router.editing = true
            onDone: Router.editing = false

            DIconButton {
                id: moreButton
                visible: !root.notFound
                anchors.verticalCenter: parent.verticalCenter
                iconName: "more_vert"
                tooltipText: I18n.tr("More", "track row menu button")
                Accessible.name: tooltipText
                onClicked: albumMenu.open(moreButton)
            }
        }
    }

    Component {
        id: hero

        DetailHero {
            card: root.sideBySide
            eyebrow: I18n.tr("Album", "an album: page label, list column, ReplayGain mode")
            title: root.album?.title ?? ""
            cover: AlbumArt {
                anchors.fill: parent
                radius: Theme.cornerRadiusXL
                item: root.album
            }

            ArtistLinks {
                width: Math.min(implicitWidth, parent.width)
                artists: root.album?.artists ?? []
                fontSize: Theme.fontSizeLarge
                fontWeight: Theme.fontWeightMedium
                color: Theme.primary
            }

            Row {
                id: facts
                width: parent.width
                spacing: Theme.spacingS

                LibraryBadge {
                    id: factsBadge
                    anchors.verticalCenter: parent.verticalCenter
                    libraryId: root.album?.library ?? ""
                }

                StyledText {
                    anchors.verticalCenter: parent.verticalCenter
                    width: Math.min(implicitWidth, facts.width - (factsBadge.visible ? factsBadge.width + facts.spacing : 0))
                    elide: Text.ElideRight
                    wrapMode: Text.NoWrap
                    text: Phrases.joinAll([root.album?.year || "", root.album?.genre ?? "", Phrases.tracks(root.tracks.length), Format.duration(root.album?.duration ?? 0)])
                    font.pixelSize: Theme.fontSizeMedium
                    color: Theme.surfaceVariantText
                }
            }

            Row {
                spacing: Theme.spacingS

                FavoriteButton {
                    anchors.verticalCenter: parent.verticalCenter
                    kind: "album"
                    item: root.album
                }

                RatingStars {
                    anchors.verticalCenter: parent.verticalCenter
                    kind: "album"
                    item: root.album
                }
            }

            PlayButtons {
                topPadding: Theme.spacingS
                note: download.blocker
                onPlay: shuffle => PlayerService.playAlbum(root.album.id, shuffle)

                DIconButton {
                    size: "m"
                    variant: "tonal"
                    iconName: "playlist_add"
                    tooltipText: I18n.tr("Add to playlist", "track row action")
                    Accessible.name: tooltipText
                    onClicked: Router.pickPlaylist(root.tracks)
                }

                DownloadButton {
                    id: download
                    kind: "album"
                    item: root.album
                }
            }
        }
    }

    MediaMenu {
        id: albumMenu
        kind: "album"
        item: root.album
    }
}
