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
    // a disc header per disc, when the album has more than one
    readonly property bool byDisc: options.group === "disc" && tracks.some(t => t.disc !== tracks[0].disc)
    readonly property var shownTracks: byDisc ? tracks.map(t => Object.assign({}, t, {
            "discLabel": I18n.tr("Disc %1", "album page section header, %1 is the disc number").arg(t.disc)
        })) : tracks
    // the album may load after the page opens (catalog still coming in)
    readonly property string loadKey: album?.id ?? ""

    onLoadKeyChanged: PlayerService.loadAlbum(loadKey)

    TrackList {
        id: list
        anchors.fill: parent
        anchors.leftMargin: PlayerMetrics.pagePadding
        anchors.rightMargin: PlayerMetrics.pagePadding
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

        header: Component {
            Column {
                width: list.width
                spacing: Theme.spacingL
                bottomPadding: Theme.spacingL

                PageHeader {
                    title: root.editing ? root.album?.title ?? "" : ""
                    editable: true
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
                        onClicked: albumMenu.open(moreButton)
                    }
                }

                PageEditor {
                    id: editor
                    visible: root.editing
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

                DetailHero {
                    visible: !root.editing
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
                        spacing: Theme.spacingS

                        LibraryBadge {
                            anchors.verticalCenter: parent.verticalCenter
                            libraryId: root.album?.library ?? ""
                        }

                        StyledText {
                            anchors.verticalCenter: parent.verticalCenter
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
                        onPlay: shuffle => PlayerService.playAlbum(root.album.id, shuffle)

                        DIconButton {
                            anchors.verticalCenter: parent.verticalCenter
                            size: "m"
                            variant: "tonal"
                            iconName: "playlist_add"
                            tooltipText: I18n.tr("Add to playlist", "track row action")
                            Accessible.name: tooltipText
                            onClicked: Router.pickPlaylist(root.tracks)
                        }
                    }

                    KeepOfflineRow {
                        kind: "album"
                        item: root.album
                    }
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
