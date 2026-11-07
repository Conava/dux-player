pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// The now-playing bar: the user's elements in left, center (the seekbar line over the transport) and right zones.
// When the width runs short the track info and the seekbar shrink first, then right-zone elements move into its
// ⋮ menu (Overflow.js). Narrow windows keep the left zone and the core transport.
Rectangle {
    id: root

    property bool compact: false
    // the arrangement to show; Settings passes a draft for its live preview
    property var zones: UiConfig.surface("bar").zones
    readonly property var artScheme: ArtColors.nowPlaying
    readonly property bool attached: UiConfig.attachedLook
    readonly property string transportLook: UiConfig.styleOf("bar") === "m3e" ? "m3e" : "transport"
    readonly property bool hasSeekbar: zones.seek.some(i => i.type === "seekbar" && !i.hidden)
    // the width the right zone may take: the rest once the other zones are at their smallest
    readonly property real rightRoom: width - Theme.spacingM * 2 - Theme.spacingL * 2 - (leftZone.implicitWidth - leftZone.shrinkable) - Math.max(controlsZone.implicitWidth, hasSeekbar ? PlayerMetrics.barSeekMinWidth : 0)

    implicitHeight: Math.max(PlayerMetrics.barHeight, layout.implicitHeight + Theme.spacingS * 2)
    // attached: no card, the bar sits on the window surface like the navigation rail
    radius: attached ? 0 : PlayerMetrics.paneRadius
    color: attached ? "transparent" : artScheme?.surfaceContainer ?? SettingsMetrics.paneColor
    LayoutMirroring.enabled: I18n.isRtl
    LayoutMirroring.childrenInherit: true

    Behavior on color {
        enabled: Theme.animationsEnabled
        ColorAnimation {
            duration: Theme.expressiveDurations.expressiveEffects
        }
    }

    // an empty stretch of the bar opens the full view; right-click offers the bar's own settings
    MouseArea {
        id: background
        anchors.fill: parent
        acceptedButtons: Qt.LeftButton | Qt.RightButton
        onClicked: mouse => {
            if (mouse.button === Qt.RightButton)
                barMenu.open(background, mouse.x, mouse.y);
            else
                Router.openNowPlaying("");
        }
    }

    // the same menu from the keyboard while a control in here has focus (rows take the Menu key first)
    Keys.onPressed: event => {
        if (event.key !== Qt.Key_Menu && !(event.key === Qt.Key_F10 && (event.modifiers & Qt.ShiftModifier)))
            return;
        barMenu.open(root.Window.activeFocusItem ?? root);
        event.accepted = true;
    }

    DMenu {
        id: barMenu
        model: [
            {
                "id": "customize",
                "text": I18n.tr("Customize bar…", "now playing bar context menu"),
                "icon": "tune"
            },
            {
                "id": "dock",
                "text": I18n.tr("Move to sidebar", "button moving now playing from the bottom bar into the sidebar"),
                "icon": "right_panel_open"
            }
        ]
        onTriggered: id => {
            if (id === "customize")
                Router.customize("bar");
            else
                Router.toggleDock();
        }
    }

    RowLayout {
        id: layout
        anchors.fill: parent
        anchors.leftMargin: Theme.spacingM
        anchors.rightMargin: Theme.spacingM
        spacing: Theme.spacingL

        // a non-filling item keeps its preferred width in a layout, so the zone fills up to that and no further
        NowPlayingZone {
            id: leftZone
            Layout.fillWidth: true
            Layout.maximumWidth: implicitWidth
            Layout.minimumWidth: implicitWidth - shrinkable
            instances: root.zones.left
            opensView: true
        }

        ColumnLayout {
            Layout.fillWidth: true
            Layout.minimumWidth: controlsZone.implicitWidth
            Layout.alignment: Qt.AlignVCenter
            visible: !root.compact
            spacing: Theme.spacingS

            NowPlayingZone {
                Layout.fillWidth: true
                instances: root.zones.seek
            }

            NowPlayingZone {
                id: controlsZone
                Layout.alignment: Qt.AlignHCenter
                instances: root.zones.controls
                size: "m"
                look: root.transportLook
                transportOnly: true
            }
        }

        Item {
            Layout.fillWidth: true
            visible: root.compact
        }

        // no room for the seek line: the progress stays as a line above the core transport
        ColumnLayout {
            Layout.fillWidth: false
            Layout.alignment: Qt.AlignVCenter
            visible: root.compact
            spacing: Theme.spacingS

            MediaSeekbar {
                Layout.fillWidth: true
                interactive: false
                flat: root.transportLook === "m3e"
                thin: true
            }

            NowPlayingZone {
                instances: ["previous", "playPause", "next"].map(type => ({
                            "id": type.toLowerCase(),
                            "type": type,
                            "hidden": false,
                            "placement": "keep",
                            "options": {}
                        }))
                size: "m"
                look: root.transportLook
            }
        }

        NowPlayingZone {
            id: rightZone
            Layout.fillWidth: false
            visible: !root.compact && rightZone.populated
            Layout.maximumWidth: Math.max(0, root.rightRoom)
            Layout.minimumWidth: Math.min(implicitWidth - shrinkable, Layout.maximumWidth)
            instances: root.zones.right
            room: Math.max(0, root.rightRoom)
        }
    }
}
