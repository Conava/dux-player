pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// One track. Click plays; Ctrl/Shift-click or the leading checkbox selects; dragging the art moves the track
// (or the selection) onto the queue or a playlist. The end of the row, the hover buttons and both menus are the
// track type's (Edit page → Rows and menus); the Menu key opens the ⋮ menu.
DListItem {
    id: root

    required property var track
    property int number: track?.number ?? 0
    property bool showArt: false
    // the wide layout: info lines become columns under a sortable header
    property bool columns: false
    property bool compact: false
    property var lines: ["artist", "album"]
    property var lineOptions: ({})
    // the wide layout's columns, widths and title share, worked out once by the list
    property var columnSpecs: []
    property real titleWeight: PlayerMetrics.titleColumnWeight
    property real timeWidth: PlayerMetrics.columnTimeWidth
    property bool selecting: false
    property bool selected: false
    // tracks a drag or the menu acts on; the selection when this row is part of it
    property var actionTracks: [track]
    property string playlistId: ""
    property var playlistIndexes: []

    readonly property bool isCurrent: track !== null && PlayerService.currentKey === PlayerService.key(track)
    readonly property bool hovering: root.hovered || leadHover.hovered || rowEnd.pointerInside || menu.opened
    // the end of the row shows duration and library
    readonly property var infoLines: lines.filter(l => l !== "duration" && l !== "library")
    readonly property string info: UiConfig.infoText(track ?? {}, infoLines, lineOptions)
    // why it can't play right now; the row stays, greyed, and the queue passes over it
    readonly property string unavailable: PlayerService.unavailableReason(track)
    readonly property string subtitle: unavailable !== "" ? unavailable : info
    // the subtitle as artist links plus the other facts
    readonly property bool artistLinks: !compact && !columns && unavailable === "" && infoLines.indexOf("artist") >= 0 && (track?.artists?.length ?? 0) > 0
    readonly property string otherInfo: UiConfig.infoText(track ?? {}, infoLines.filter(l => l !== "artist"), lineOptions)
    property int pressModifiers: Qt.NoModifier

    signal activated
    // modifiers: Qt.ControlModifier toggles, Qt.ShiftModifier extends, Qt.NoModifier toggles from the checkbox
    signal selectRequested(int modifiers)

    implicitHeight: compact || subtitle === "" || (columns && unavailable === "") ? PlayerMetrics.listRowHeight : PlayerMetrics.listRowTwoLineHeight
    surfaceColor: PlayerMetrics.listRowColor
    isSelected: selected
    // stays highlighted while its menu is open
    isHovered: hovered || menu.opened
    Accessible.name: Phrases.named(track?.title ?? "", track?.artist ?? "")
    onClicked: {
        if (pressModifiers & (Qt.ControlModifier | Qt.ShiftModifier)) {
            selectRequested(pressModifiers & (Qt.ControlModifier | Qt.ShiftModifier));
            return;
        }
        if (selecting) {
            selectRequested(Qt.ControlModifier);
            return;
        }
        activated();
    }
    onContextMenuRequested: (x, y) => menu.open(root, x, y)
    Keys.onPressed: event => {
        if (event.key !== Qt.Key_Menu)
            return;
        menu.open(rowEnd.moreButton ?? root);
        event.accepted = true;
    }

    TapHandler {
        acceptedButtons: Qt.LeftButton
        onPressedChanged: {
            if (pressed)
                root.pressModifiers = point.modifiers;
        }
    }

    TrackMenu {
        id: menu
        tracks: root.actionTracks
        playlistId: root.playlistId
        playlistIndexes: root.playlistIndexes
    }

    RowLayout {
        anchors.fill: parent
        opacity: root.unavailable !== "" ? Theme.disabledOpacity : 1
        anchors.leftMargin: Theme.spacingL
        anchors.rightMargin: Theme.spacingS
        spacing: Theme.spacingM

        Item {
            id: lead
            Layout.preferredWidth: root.showArt && !root.compact ? PlayerMetrics.rowArtSize : PlayerMetrics.columnNumberWidth
            Layout.preferredHeight: root.compact ? Theme.iconSize : PlayerMetrics.rowArtSize
            Layout.alignment: Qt.AlignVCenter

            readonly property bool checkbox: root.selecting || leadHover.hovered

            HoverHandler {
                id: leadHover
                cursorShape: Qt.PointingHandCursor
            }

            TapHandler {
                onTapped: root.selectRequested(Qt.NoModifier)
            }

            DragHandler {
                id: dragHandler
                target: null
                onActiveChanged: {
                    if (active) {
                        Router.dragTracks = root.actionTracks;
                        Router.dragSource = root.playlistId;
                    } else
                        ghost.Drag.drop();
                }
            }

            AlbumArt {
                anchors.fill: parent
                visible: root.showArt && !root.compact && !lead.checkbox
                item: root.track
                radius: Theme.cornerRadiusS
            }

            StyledText {
                anchors.centerIn: parent
                visible: !root.showArt && !root.isCurrent && !lead.checkbox
                text: root.number > 0 ? root.number : ""
                font.pixelSize: Theme.fontSizeMedium
                color: root.supportingContentColor
            }

            DIcon {
                anchors.centerIn: parent
                visible: root.isCurrent && !lead.checkbox && !(root.showArt && !root.compact)
                name: "graphic_eq"
                size: Theme.iconSize
                color: PlayerService.playing ? Theme.primary : Theme.surfaceVariantText
                filled: true
            }

            DIcon {
                anchors.centerIn: parent
                visible: lead.checkbox
                name: root.selected ? "check_box" : "check_box_outline_blank"
                filled: root.selected
                size: Theme.iconSize
                color: root.selected ? Theme.primary : root.supportingContentColor
                Accessible.role: Accessible.CheckBox
                Accessible.name: I18n.tr("Select", "track row checkbox")
                Accessible.checked: root.selected
            }

            // the drag image: a chip with the title or the count
            Rectangle {
                id: ghost
                parent: root.Window.window?.contentItem ?? lead
                visible: dragHandler.active
                z: Theme.zDrag
                x: dragHandler.active ? lead.mapToItem(parent, dragHandler.centroid.position.x, dragHandler.centroid.position.y).x : 0
                y: dragHandler.active ? lead.mapToItem(parent, dragHandler.centroid.position.x, dragHandler.centroid.position.y).y : 0
                width: ghostText.implicitWidth + Theme.spacingL * 2
                height: Theme.buttonHeightXS
                radius: Theme.fullRadius(width, height)
                color: Theme.secondaryContainer
                Drag.active: dragHandler.active
                Drag.keys: ["dplayer/tracks"]
                Drag.hotSpot.x: 0
                Drag.hotSpot.y: 0

                StyledText {
                    id: ghostText
                    anchors.centerIn: parent
                    text: root.actionTracks.length === 1 ? (root.track?.title ?? "") : Phrases.tracks(root.actionTracks.length)
                    font.pixelSize: Theme.fontSizeSmall
                    color: Theme.onSecondaryContainer
                }
            }
        }

        Column {
            Layout.fillWidth: true
            Layout.preferredWidth: root.titleWeight
            Layout.minimumWidth: PlayerMetrics.columnTitleMinWidth
            Layout.alignment: Qt.AlignVCenter
            spacing: Theme.spacingXXS

            StyledText {
                width: parent.width
                text: root.compact && root.subtitle !== "" ? Phrases.join(root.track?.title ?? "", root.subtitle) : root.track?.title ?? ""
                font.pixelSize: Theme.fontSizeMedium
                font.weight: Theme.fontWeightMedium
                color: root.isCurrent ? Theme.primary : root.contentColor
                elide: Text.ElideRight
                wrapMode: Text.NoWrap
                horizontalAlignment: Text.AlignLeft
            }

            Row {
                width: parent.width
                visible: root.artistLinks

                ArtistLinks {
                    id: subtitleArtists
                    width: Math.min(implicitWidth, parent.width - Math.min(otherText.implicitWidth, parent.width / 2))
                    artists: root.track?.artists ?? []
                    color: root.supportingContentColor
                }

                StyledText {
                    id: otherText
                    // not parent.width - x: a mirrored Row sets x from the width, which never settles
                    width: parent.width - subtitleArtists.width
                    visible: root.otherInfo !== ""
                    text: Phrases.join("", root.otherInfo)
                    font.pixelSize: Theme.fontSizeSmall
                    color: root.supportingContentColor
                    elide: Text.ElideRight
                    wrapMode: Text.NoWrap
                    horizontalAlignment: Text.AlignLeft
                }
            }

            StyledText {
                width: parent.width
                visible: !root.compact && !root.artistLinks && (root.unavailable !== "" || (!root.columns && root.info !== ""))
                text: root.subtitle
                font.pixelSize: Theme.fontSizeSmall
                color: root.supportingContentColor
                elide: Text.ElideRight
                wrapMode: Text.NoWrap
                horizontalAlignment: Text.AlignLeft
            }
        }

        Repeater {
            // a count, so a drag that changes the widths keeps the cells
            model: root.columnSpecs.length

            StyledText {
                required property int index
                readonly property var modelData: root.columnSpecs[index] ?? ({})
                Layout.preferredWidth: modelData.width ?? 0
                text: modelData.line === "artist" && (root.track?.artists?.length ?? 0) > 0 ? "" : UiConfig.infoText(root.track ?? {}, [modelData.line], root.lineOptions)
                font.pixelSize: Theme.fontSizeMedium
                color: root.supportingContentColor
                elide: Text.ElideRight
                wrapMode: Text.NoWrap
                horizontalAlignment: Text.AlignLeft

                ArtistLinks {
                    anchors.left: parent.left
                    anchors.verticalCenter: parent.verticalCenter
                    width: Math.min(implicitWidth, parent.width)
                    visible: parent.modelData.line === "artist" && parent.text === ""
                    artists: root.track?.artists ?? []
                    fontSize: Theme.fontSizeMedium
                    color: root.supportingContentColor
                }
            }
        }

        RowEnd {
            id: rowEnd
            Layout.alignment: Qt.AlignVCenter
            actions: menu
            item: root.track
            hovering: root.hovering
            fixedLibrary: root.columns
            timeWidth: root.timeWidth
            narrow: !root.columns && root.width < PlayerMetrics.fullRowWidth
            textColor: root.supportingContentColor
        }
    }
}
