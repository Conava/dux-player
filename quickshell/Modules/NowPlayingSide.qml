pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// Now playing as a side panel: its panes (cover, lyrics, the queue) on top, the title card and controls below.
Item {
    id: root

    // the arrangement to show; Settings passes a draft for its live preview
    property var zones: UiConfig.surface("side").zones
    readonly property var track: PlayerService.current
    readonly property var views: UiConfig.sidePanes
    readonly property bool m3e: UiConfig.nowPlaying.style === "m3e"
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
        anchors.bottomMargin: Theme.spacingM

        AlbumArt {
            visible: Router.sideTab === "cover"
            anchors.centerIn: parent
            width: Math.min(parent.width - Theme.spacingL * 2, parent.height)
            height: width
            radius: Theme.cornerRadiusXL
            item: root.track
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

    Column {
        id: controls
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.margins: Theme.spacingL
        spacing: Theme.spacingS

        Rectangle {
            width: parent.width
            height: titleZone.implicitHeight + Theme.spacingS * 2
            visible: titleZone.populated
            radius: PlayerMetrics.mediaInnerRadius
            color: ArtColors.chipColor
            border.width: Theme.layerOutlineWidth
            border.color: Theme.outlineMedium

            NowPlayingZone {
                id: titleZone
                x: Theme.spacingS
                y: Theme.spacingS
                width: parent.width - Theme.spacingS * 2
                instances: root.zones.title
                context: "side"
                textLook: "side"
                centered: true
                column: true
                opensView: true
            }
        }

        Rectangle {
            width: parent.width
            height: playback.implicitHeight + Theme.spacingL * 2
            visible: topZone.populated || controlsZone.populated || seekZone.populated || bottomZone.populated
            radius: PlayerMetrics.mediaInnerRadius
            color: ArtColors.chipColor
            border.width: Theme.layerOutlineWidth
            border.color: Theme.outlineMedium

            Column {
                id: playback
                x: Theme.spacingL
                y: Theme.spacingL
                width: parent.width - Theme.spacingL * 2
                spacing: Theme.spacingS

                NowPlayingZone {
                    id: topZone
                    width: parent.width
                    instances: root.zones.top
                    context: "side"
                    look: "row"
                    room: width
                }

                NowPlayingZone {
                    id: controlsZone
                    width: parent.width
                    instances: root.zones.controls
                    context: "side"
                    look: root.m3e ? "m3e" : "transport"
                    fill: !root.m3e
                    room: width
                }

                NowPlayingZone {
                    id: seekZone
                    width: parent.width
                    instances: root.zones.seek
                    context: "side"
                }

                NowPlayingZone {
                    id: bottomZone
                    anchors.horizontalCenter: parent.horizontalCenter
                    instances: root.zones.bottom
                    context: "side"
                    room: parent.width
                }
            }
        }
    }
}
