pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// Expanded now playing in the user's player style (Router.openNowPlaying picks the starting pane). Lyrics and the
// queue sit in their own pane beside the player when wide, or take the art's place when narrow; never over it.
Rectangle {
    id: root

    readonly property bool m3e: UiConfig.nowPlaying.style === "m3e"
    readonly property bool wide: width >= PlayerMetrics.compactBreakpoint
    // what the second pane shows: "lyrics" | "queue" | ""
    readonly property string pane: Router.viewPane
    readonly property bool beside: wide && pane !== ""
    readonly property bool inPlace: !wide && pane !== ""
    readonly property color paneColor: ArtColors.cardColor

    // the window paints the background, so the view has the same transparency as every other page
    color: "transparent"
    focus: true
    Component.onCompleted: forceActiveFocus()

    // swallows clicks so the pages underneath stay inert
    MouseArea {
        anchors.fill: parent
    }

    DIconButton {
        id: closeButton
        anchors.left: parent.left
        anchors.top: parent.top
        anchors.margins: Theme.spacingL
        iconName: "keyboard_arrow_down"
        variant: "tonal"
        containerColor: ArtColors.accentSecondaryContainer
        contentColor: ArtColors.onAccentSecondaryContainer
        Accessible.name: I18n.tr("Close now playing", "expanded now playing close button")
        tooltipText: Accessible.name
        onClicked: Router.nowPlayingOpen = false
    }

    DIconButton {
        id: customizeButton
        anchors.right: parent.right
        anchors.top: parent.top
        anchors.margins: Theme.spacingL
        iconName: "tune"
        iconColor: ArtColors.textVariant
        Accessible.name: I18n.tr("Customize now playing", "now playing button opening its settings")
        tooltipText: Accessible.name
        onClicked: customizeMenu.open(customizeButton)

        DMenu {
            id: customizeMenu
            model: [
                {
                    "id": "bento",
                    "text": I18n.tr("Bento", "now playing player style"),
                    "checked": !root.m3e
                },
                {
                    "id": "m3e",
                    "text": I18n.tr("Material", "now playing player style"),
                    "checked": root.m3e
                },
                {
                    "divider": true
                },
                {
                    "id": "customize",
                    "text": I18n.tr("Customize this view…", "now playing full view menu"),
                    "icon": "tune"
                }
            ]
            onTriggered: id => {
                if (id === "customize")
                    Router.customize(root.m3e ? "fullM3e" : "fullBento");
                else
                    UiConfig.setNowPlaying({
                        "style": id
                    });
            }
        }
    }

    Loader {
        id: player
        anchors.left: parent.left
        anchors.top: closeButton.bottom
        anchors.bottom: parent.bottom
        anchors.margins: Theme.spacingXL
        anchors.topMargin: Theme.spacingL
        width: (root.beside ? parent.width / 2 : parent.width) - Theme.spacingXL * 2
        sourceComponent: root.m3e ? m3ePlayer : bentoPlayer
    }

    Component {
        id: bentoPlayer
        BentoPlayer {
            inPlace: root.inPlace ? (root.pane === "queue" ? queuePane : lyricsPane) : null
        }
    }

    Component {
        id: m3ePlayer
        M3EPlayer {
            inPlace: root.inPlace ? (root.pane === "queue" ? queuePane : lyricsPane) : null
        }
    }

    Loader {
        anchors.left: player.right
        anchors.right: parent.right
        anchors.top: player.top
        anchors.bottom: parent.bottom
        anchors.margins: Theme.spacingXL
        anchors.topMargin: 0
        active: root.beside
        sourceComponent: root.pane === "queue" ? queuePane : lyricsPane
    }

    Component {
        id: lyricsPane
        LyricsView {
            radius: PlayerMetrics.mediaInnerRadius
            backgroundColor: root.paneColor
            artScheme: ArtColors.nowPlaying
        }
    }

    Component {
        id: queuePane
        Rectangle {
            radius: PlayerMetrics.mediaInnerRadius
            color: root.paneColor

            QueueList {
                anchors.fill: parent
            }
        }
    }
}
