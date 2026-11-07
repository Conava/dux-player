pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// Now playing as a side panel: its panes (cover, lyrics, the queue) on top, the title and controls below.
Item {
    id: root

    // the arrangement to show; Settings passes a draft for its live preview
    property var zones: UiConfig.surface("side").zones
    readonly property var track: PlayerService.current
    readonly property var cover: zones.cover.find(i => i.type === "cover" && !i.hidden) ?? null
    readonly property var views: UiConfig.sidePanes
    readonly property bool m3e: UiConfig.styleOf("side") === "m3e"
    // what the lines above and below the seekbar may take; the panes keep the art's least height
    readonly property real lineSpare: height - (switcher.visible ? switcher.height + Theme.spacingL : 0) - Theme.spacingM - content.anchors.bottomMargin - controls.anchors.margins - PlayerMetrics.mediaArtMinSize - [titleZone, seekZone, controlsZone, buttonsZone].reduce((sum, block) => sum + (block.populated && block.visible ? block.height + controls.spacing : 0), -controls.spacing)
    readonly property var labels: ({
            "cover": {
                "text": I18n.tr("Playing", "side panel view: cover art"),
                "icon": "album"
            },
            "lyrics": {
                "text": I18n.tr("Lyrics", "song lyrics: page, toggle, settings tab"),
                "icon": "lyrics"
            },
            "queue": {
                "text": I18n.tr("Queue", "the play queue: panel, toggle"),
                "icon": "queue_music"
            }
        })

    // right-click anywhere the panel's own controls don't take it
    MouseArea {
        id: background
        anchors.fill: parent
        acceptedButtons: Qt.RightButton
        preventStealing: true
        onClicked: mouse => sideMenu.open(background, mouse.x, mouse.y)
    }

    // the same menu from the keyboard while a control in here has focus (rows take the Menu key first)
    Keys.onPressed: event => {
        if (event.key !== Qt.Key_Menu && !(event.key === Qt.Key_F10 && (event.modifiers & Qt.ShiftModifier)))
            return;
        sideMenu.open(root.Window.activeFocusItem ?? root);
        event.accepted = true;
    }

    DMenu {
        id: sideMenu
        model: [
            {
                "id": "customize",
                "text": I18n.tr("Customize side panel…", "now playing side panel context menu"),
                "icon": "tune"
            },
            {
                "id": "dock",
                "text": I18n.tr("Move to bottom bar", "button moving now playing out of the sidebar"),
                "icon": "bottom_panel_open"
            }
        ]
        onTriggered: id => {
            if (id === "customize")
                Router.customize("side");
            else
                Router.toggleDock();
        }
    }

    DButtonGroup {
        id: switcher
        anchors.top: parent.top
        anchors.topMargin: Theme.spacingL
        anchors.horizontalCenter: parent.horizontalCenter
        visible: root.views.length > 1
        size: "small"
        checkEnabled: false
        model: root.views.map(v => root.labels[v])
        currentIndex: root.views.indexOf(Router.sideTab)
        onSelectionChanged: (index, selected) => {
            if (selected)
                Router.setSideTab(root.views[index]);
        }
    }

    Item {
        id: content
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: switcher.visible ? switcher.bottom : parent.top
        anchors.bottom: controls.top
        anchors.topMargin: Theme.spacingM
        anchors.bottomMargin: Theme.spacingL

        PlayerArt {
            visible: Router.sideTab === "cover" && root.track !== null && root.cover !== null
            anchors.centerIn: parent
            width: Math.max(0, Math.min(parent.width - Theme.spacingL * 2, parent.height))
            height: width
            item: root.track
            shape: root.cover?.options?.shape ?? "rounded"
            rotate: root.cover?.options?.rotate ?? true
        }

        Loader {
            anchors.fill: parent
            active: Router.sideTab === "lyrics"
            sourceComponent: LyricsView {}
        }

        Loader {
            anchors.fill: parent
            active: Router.sideTab === "queue"
            sourceComponent: QueueList {}
        }

        StyledText {
            anchors.centerIn: parent
            visible: root.track === null
            text: I18n.tr("Nothing playing", "side panel empty state")
            font.pixelSize: Theme.fontSizeLarge
            color: Theme.surfaceVariantText
        }
    }

    // the title, the seek line, the transport, then the top and bottom zones
    Column {
        id: controls
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.margins: Theme.spacingL
        spacing: Theme.spacingL

        NowPlayingZone {
            id: titleZone
            width: parent.width
            instances: root.zones.title
            context: "side"
            textLook: root.m3e ? "m3e" : "side"
            centered: UiConfig.titleCentered("side")
            column: true
            opensView: true
        }

        NowPlayingLine {
            id: lineAbove
            anchors.horizontalCenter: parent.horizontalCenter
            width: lineWidth
            available: parent.width
            spare: root.lineSpare
            gap: controls.spacing
            other: lineBelow
            instances: root.zones.seekAbove
            context: "side"
            centered: true
        }

        NowPlayingZone {
            id: seekZone
            width: parent.width
            visible: populated
            instances: root.zones.seek
            context: "side"
            room: parent.width
            keepMoves: true
        }

        NowPlayingLine {
            id: lineBelow
            anchors.horizontalCenter: parent.horizontalCenter
            width: lineWidth
            available: parent.width
            spare: root.lineSpare
            gap: controls.spacing
            other: lineAbove
            lower: true
            instances: root.zones.seekBelow
            context: "side"
            centered: true
        }

        NowPlayingZone {
            id: controlsZone
            width: parent.width
            instances: root.zones.controls
            context: "side"
            size: "m"
            look: root.m3e ? "m3e" : "transport"
            fill: true
            room: width
        }

        NowPlayingZone {
            id: buttonsZone
            readonly property bool spread: instances.some(i => !i.hidden && i.type === "spacer" && i.options?.size === "fill")
            anchors.horizontalCenter: parent.horizontalCenter
            width: spread ? parent.width : fitWidth
            instances: root.zones.top.concat(root.zones.bottom)
            context: "side"
            room: parent.width
        }
    }
}
