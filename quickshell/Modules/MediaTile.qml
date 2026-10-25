import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// A grid or carousel tile for an album, artist, playlist, genre or track. The overlay is fixed (play, ⋮, heart);
// the ⋮ and right-click menus are the type's.
StyledButton {
    id: root

    property string kind: "album"
    required property var item
    property var lines: []
    readonly property string info: UiConfig.infoText(item, lines)
    readonly property bool menuOpened: menu.opened || trackMenu.opened
    readonly property bool active: hovered || activeFocus || menuOpened

    function openMenu(anchor, x, y) {
        if (kind === "track")
            trackMenu.open(anchor, x, y);
        else
            menu.open(anchor, x, y);
    }

    signal activated

    implicitWidth: PlayerMetrics.gridCellWidth
    implicitHeight: width + PlayerMetrics.gridCaptionHeight
    radius: Theme.cornerRadiusL
    color: "transparent"
    Accessible.name: Phrases.named(item?.title ?? item?.name ?? "", info)
    onClicked: kind === "track" ? activated() : Router.openItem(kind, item)

    StateLayer {
        control: root
        stateColor: Theme.onSurface
        // stays highlighted while its menu is open
        hovered: root.hovered || root.menuOpened
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
        x: Theme.spacingS
        y: Theme.spacingS
        width: parent.width - Theme.spacingS * 2
        height: width
        kind: root.kind
        item: root.item
        radius: Theme.cornerRadiusL

        DIconButton {
            anchors.right: parent.right
            anchors.bottom: parent.bottom
            anchors.margins: Theme.spacingS
            size: "m"
            variant: "filled"
            iconName: "play_arrow"
            iconFilled: true
            visible: (root.active || activeFocus) && root.kind !== "artist" && root.kind !== "genre"
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
            anchors.margins: Theme.spacingS
            visible: root.active || activeFocus
            variant: "tonal"
            iconName: "more_vert"
            tooltipText: I18n.tr("More", "track row menu button")
            Accessible.name: tooltipText
            onClicked: root.openMenu(moreButton)
        }

        DIcon {
            anchors.left: parent.left
            anchors.top: parent.top
            anchors.margins: Theme.spacingS
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
        anchors.topMargin: Theme.spacingS
        spacing: Theme.spacingXXS

        StyledText {
            width: parent.width
            text: root.item?.title ?? root.item?.name ?? ""
            font.pixelSize: Theme.fontSizeMedium
            font.weight: Theme.fontWeightMedium
            color: Theme.surfaceText
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
            horizontalAlignment: root.kind === "artist" ? Text.AlignHCenter : Text.AlignLeft
        }

        StyledText {
            width: parent.width
            visible: root.info !== ""
            text: root.info
            font.pixelSize: Theme.fontSizeSmall
            color: Theme.surfaceVariantText
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
