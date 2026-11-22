import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// A grid or carousel tile for an album, artist, playlist, genre or track. The overlay is fixed (play, ⋮, heart);
// the ⋮ and right-click menus are the type's.
StyledButton {
    id: root

    property string kind: "album"
    required property var item
    property var lines: []
    property var lineOptions: ({})
    // the page's tile size, "s", "m" or "l": the buttons and caption follow it
    property string tileSize: "m"
    readonly property bool large: tileSize === "l"
    readonly property string info: UiConfig.infoText(item, lines, lineOptions)
    readonly property bool menuOpened: menu.opened || trackMenu.opened
    // the queue came from this album or playlist and a track is loaded
    readonly property bool playing: (kind === "album" || kind === "playlist") && PlayerService.current !== null && PlayerService.playContext.type === kind && PlayerService.playContext.id === item?.id
    readonly property bool active: hovered || visualFocus || playButton.visualFocus || moreButton.visualFocus || menuOpened

    // false at creation and while a carousel holds the tile in its pool, so a value set then lands at once
    property bool placed: false
    readonly property bool animates: Theme.animationsEnabled && placed
    readonly property bool showsPlayButton: active && kind !== "artist" && kind !== "genre"
    // overlays grow in from this scale
    readonly property real overlayScale: 0.8
    readonly property bool showsBadge: playing && !active

    // an overlay fades and grows in on the quick effects row and leaves on the exit row
    component OverlayFade: NumberAnimation {
        property bool show: true
        duration: show ? Theme.expressiveDurations.expressiveFastEffects : Theme.expressiveDurations.expressiveEffects
        easing.type: Easing.BezierSpline
        easing.bezierCurve: show ? Theme.expressiveCurves.expressiveFastEffects : Theme.expressiveCurves.emphasizedAccel
    }

    function openMenu(anchor, x, y) {
        if (kind === "track")
            trackMenu.open(anchor, x, y);
        else
            menu.open(anchor, x, y);
    }

    signal activated

    implicitWidth: PlayerMetrics.gridCellWidth
    implicitHeight: width + (large ? PlayerMetrics.gridCaptionHeightL : PlayerMetrics.gridCaptionHeight)
    radius: Theme.cornerRadiusL
    color: playing ? Theme.primaryContainer : "transparent"
    Accessible.name: playing ? Phrases.named(Phrases.named(item?.title ?? item?.name ?? "", info), I18n.tr("Playing", "tile state: the queue plays from this album or playlist")) : Phrases.named(item?.title ?? item?.name ?? "", info)

    Component.onCompleted: placed = true
    ListView.onPooled: placed = false
    ListView.onReused: Qt.callLater(() => placed = true)

    Behavior on color {
        enabled: root.animates
        ColorAnimation {
            duration: Theme.expressiveDurations.expressiveEffects
        }
    }

    // tracks without an album tag sit under the placeholder "Unknown Album", which isn't a page to open
    onClicked: {
        if (kind !== "track")
            Router.openItem(kind, item);
        else if (SettingsData.trackTileOpensAlbum && (item?.albumId ?? "") !== "" && item.album !== "Unknown Album")
            Router.go("album", item.albumId);
        else
            activated();
    }

    StateLayer {
        control: root
        stateColor: root.playing ? Theme.onPrimaryContainer : Theme.onSurface
        // stays highlighted while its menu is open
        hovered: root.hovered || root.menuOpened
    }

    DPressFeedback {
        control: root
    }

    TapHandler {
        acceptedButtons: Qt.RightButton
        onTapped: eventPoint => root.openMenu(root, eventPoint.position.x, eventPoint.position.y)
    }

    Keys.onPressed: event => {
        if (event.key !== Qt.Key_Menu)
            return;
        openMenu(moreButton);
        event.accepted = true;
    }

    MediaMenu {
        id: menu
        kind: root.kind
        item: root.kind === "track" ? null : root.item
    }

    TrackMenu {
        id: trackMenu
        tracks: root.kind === "track" && root.item ? [root.item] : []
    }

    MediaArt {
        id: art
        x: PlayerMetrics.tileInset
        y: PlayerMetrics.tileInset
        width: parent.width - PlayerMetrics.tileInset * 2
        height: width
        kind: root.kind
        item: root.item
        radius: Theme.cornerRadiusL

        DIconButton {
            id: playButton
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            anchors.margins: PlayerMetrics.tileInset
            buttonSize: root.tileSize === "s" ? Theme.buttonHeightS : PlayerMetrics.tilePlayButtonSize
            width: buttonSize
            variant: "filled"
            iconName: "play_arrow"
            iconFilled: true
            visible: root.showsPlayButton || opacity > 0
            opacity: root.showsPlayButton ? 1 : 0
            scale: root.showsPlayButton ? 1 : root.overlayScale
            Behavior on opacity {
                enabled: root.animates
                OverlayFade {
                    show: root.showsPlayButton
                }
            }
            Behavior on scale {
                enabled: root.animates
                OverlayFade {
                    show: root.showsPlayButton
                }
            }
            Accessible.name: I18n.tr("Play %1", "play button, %1 is the album or playlist").arg(root.item?.title ?? root.item?.name ?? "")
            onClicked: {
                if (root.kind === "album")
                    PlayerService.playAlbum(root.item.id, false);
                else if (root.kind === "playlist")
                    PlayerService.playPlaylist(root.item.id, false);
                else
                    root.activated();
            }
        }

        DIconButton {
            id: moreButton
            anchors.right: parent.right
            anchors.top: parent.top
            anchors.margins: PlayerMetrics.tileInset
            visible: root.active || opacity > 0
            opacity: root.active ? 1 : 0
            scale: root.active ? 1 : root.overlayScale
            Behavior on opacity {
                enabled: root.animates
                OverlayFade {
                    show: root.active
                }
            }
            Behavior on scale {
                enabled: root.animates
                OverlayFade {
                    show: root.active
                }
            }
            buttonSize: root.tileSize === "s" ? Theme.buttonHeightXS : Theme.buttonHeightS
            iconSize: root.tileSize === "s" ? Theme.iconSizeSmall : Theme.iconSize
            variant: "tonal"
            iconName: "more_vert"
            tooltipText: I18n.tr("More", "track row menu button")
            Accessible.name: tooltipText
            onClicked: root.openMenu(moreButton)
        }

        Rectangle {
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            anchors.margins: PlayerMetrics.tileInset
            width: Theme.buttonHeightXS
            height: width
            radius: width / 2
            color: Theme.primary
            visible: root.showsBadge || opacity > 0
            opacity: root.showsBadge ? 1 : 0
            scale: root.showsBadge ? 1 : root.overlayScale
            Behavior on opacity {
                enabled: root.animates
                OverlayFade {
                    show: root.showsBadge
                }
            }
            Behavior on scale {
                enabled: root.animates
                OverlayFade {
                    show: root.showsBadge
                }
            }

            DIcon {
                anchors.centerIn: parent
                name: "equalizer"
                filled: true
                size: Theme.iconSizeSmall
                color: Theme.onPrimary
            }
        }

        DIcon {
            anchors.left: parent.left
            anchors.top: parent.top
            anchors.margins: PlayerMetrics.tileInset
            visible: root.item?.favorite === true
            name: "favorite"
            filled: true
            size: Theme.iconSizeSmall
            color: Theme.primary
        }
    }

    Column {
        anchors.left: art.left
        anchors.right: art.right
        anchors.top: art.bottom
        anchors.topMargin: PlayerMetrics.tileInset
        spacing: Theme.spacingXXS

        StyledText {
            width: parent.width
            text: root.item?.title ?? root.item?.name ?? ""
            font.pixelSize: root.large ? Theme.fontSizeLarge : Theme.fontSizeMedium
            font.weight: Theme.fontWeightMedium
            color: root.playing ? Theme.onPrimaryContainer : Theme.surfaceText
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
            horizontalAlignment: root.kind === "artist" ? Text.AlignHCenter : Text.AlignLeft
        }

        StyledText {
            width: parent.width
            visible: root.info !== ""
            text: root.info
            font.pixelSize: root.large ? Theme.fontSizeMedium : Theme.fontSizeSmall
            color: root.playing ? Theme.onPrimaryContainer : Theme.surfaceVariantText
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
            horizontalAlignment: root.kind === "artist" ? Text.AlignHCenter : Text.AlignLeft
        }
    }

    // tracks dragged onto a playlist are added to it
    TrackDropArea {
        enabled: root.kind === "playlist"
        onDropped: PlayerService.reportSkipped(PlayerService.addToPlaylist(root.item.id, Router.dragTracks))
    }
}
