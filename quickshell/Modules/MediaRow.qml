import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// A list row for an album, artist, playlist or genre; tracks use TrackRow. Its end, hover buttons and menus are
// the type's (Edit page → Rows and menus).
DListItem {
    id: root

    property string kind: "album"
    required property var item
    property var lines: []
    property bool compact: false
    readonly property string info: UiConfig.infoText(item, lines)

    implicitHeight: compact ? Theme.listItemHeight : Theme.listItemTwoLineHeight
    Accessible.name: Phrases.named(item?.title ?? item?.name ?? "", info)
    // stays highlighted while its menu is open
    isHovered: hovered || menu.opened
    onClicked: Router.openItem(kind, item)
    onContextMenuRequested: (x, y) => menu.open(root, x, y)
    Keys.onPressed: event => {
        if (event.key !== Qt.Key_Menu)
            return;
        menu.open(trailing.moreButton ?? root);
        event.accepted = true;
    }

    MediaMenu {
        id: menu
        kind: root.kind
        item: root.item
    }

    MediaArt {
        id: art
        anchors.left: parent.left
        anchors.leftMargin: Theme.spacingL
        anchors.verticalCenter: parent.verticalCenter
        width: root.compact ? Theme.iconSize : PlayerMetrics.rowArtSize
        height: width
        kind: root.kind
        item: root.item
        radius: root.compact ? Theme.cornerRadiusXS : Theme.cornerRadiusS
    }

    Column {
        anchors.left: art.right
        anchors.leftMargin: Theme.spacingL
        anchors.right: trailing.left
        anchors.rightMargin: Theme.spacingM
        anchors.verticalCenter: parent.verticalCenter
        spacing: Theme.spacingXXS

        StyledText {
            width: parent.width
            text: root.compact && root.info !== "" ? Phrases.join(root.item?.title ?? root.item?.name ?? "", root.info) : root.item?.title ?? root.item?.name ?? ""
            font.pixelSize: Theme.fontSizeMedium
            font.weight: Theme.fontWeightMedium
            color: root.contentColor
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
            horizontalAlignment: Text.AlignLeft
        }

        StyledText {
            width: parent.width
            visible: !root.compact && root.info !== ""
            text: root.info
            font.pixelSize: Theme.fontSizeSmall
            color: root.supportingContentColor
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
            horizontalAlignment: Text.AlignLeft
        }
    }

    RowEnd {
        id: trailing
        anchors.right: parent.right
        anchors.rightMargin: Theme.spacingS
        anchors.verticalCenter: parent.verticalCenter
        actions: menu
        item: root.item
        hovering: root.hovered || pointerInside || menu.opened
        textColor: root.supportingContentColor
    }

    // tracks dragged onto a playlist are added to it
    TrackDropArea {
        enabled: root.kind === "playlist"
        onDropped: PlayerService.reportSkipped(PlayerService.addToPlaylist(root.item.id, Router.dragTracks))
    }
}
