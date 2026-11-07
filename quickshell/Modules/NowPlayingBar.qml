pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Layouts
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// The now-playing bar: the user's elements in left, center (the seekbar line, with a line above and below it in use,
// over the transport) and right zones. When the width runs short the track info and the seekbar shrink first, then
// right-zone elements move into its ⋮ menu (Overflow.js). Narrow windows keep the left zone and the core transport.
Rectangle {
    id: root

    property bool compact: false
    // the arrangement to show; Settings passes a draft for its live preview
    property var zones: UiConfig.surface("bar").zones
    readonly property var artScheme: ArtColors.nowPlaying
    readonly property bool attached: UiConfig.attachedLook
    readonly property string transportLook: UiConfig.styleOf("bar") === "m3e" ? "m3e" : "transport"
    // the center column at its smallest: the transport keeps its size, the lines lose elements to their ⋮ first
    readonly property real centerLeast: Math.max(controlsZone.implicitWidth - controlsZone.shrinkable, seekZone.leastWidth, lineAbove.leastWidth, lineBelow.leastWidth)
    // the width the right zone may take: the rest once the other zones are at their smallest
    readonly property real rightRoom: width - Theme.spacingM * 2 - Theme.spacingL * 2 - (leftZone.implicitWidth - leftZone.shrinkable) - centerLeast

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
        preventStealing: true
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
            Layout.maximumWidth: Math.min(implicitWidth, Math.max(Layout.minimumWidth, (root.width - (rightZone.visible ? rightZone.implicitWidth : 0)) * PlayerMetrics.barLeftShare))
            Layout.minimumWidth: implicitWidth - shrinkable
            instances: root.zones.left
            opensView: true
        }

        ColumnLayout {
            id: center
            Layout.fillWidth: true
            Layout.minimumWidth: root.centerLeast
            // not from the lines: they follow this column's width, and it would follow them
            Layout.preferredWidth: Math.max(seekZone.implicitWidth, controlsZone.implicitWidth)
            Layout.alignment: Qt.AlignVCenter
            visible: !root.compact
            spacing: Theme.spacingS

            // each zone states its least width: a column takes the widest minimum of its children, whatever its own size
            NowPlayingLine {
                id: lineAbove
                Layout.fillWidth: spreads
                Layout.preferredWidth: fitWidth
                Layout.minimumWidth: leastWidth
                Layout.alignment: Qt.AlignHCenter
                available: center.width
                instances: root.zones.seekAbove
                centered: true
            }

            NowPlayingZone {
                id: seekZone
                Layout.fillWidth: true
                Layout.minimumWidth: leastWidth
                instances: root.zones.seek
                room: center.width
                keepMoves: true
            }

            NowPlayingLine {
                id: lineBelow
                Layout.fillWidth: spreads
                Layout.preferredWidth: fitWidth
                Layout.minimumWidth: leastWidth
                Layout.alignment: Qt.AlignHCenter
                available: center.width
                instances: root.zones.seekBelow
                centered: true
            }

            NowPlayingZone {
                id: controlsZone
                Layout.alignment: Qt.AlignHCenter
                Layout.minimumWidth: implicitWidth - shrinkable
                instances: root.zones.controls
                size: "m"
                look: root.transportLook
                transportOnly: true
            }
        }

        // no room for the seek line: the progress stays as a line above the core transport
        ColumnLayout {
            Layout.fillWidth: true
            Layout.alignment: Qt.AlignVCenter
            visible: root.compact
            spacing: Theme.spacingS

            MediaSeekbar {
                Layout.fillWidth: true
                interactive: false
                flat: root.transportLook === "m3e"
                expressive: !flat
                thin: true
            }

            NowPlayingZone {
                Layout.alignment: Qt.AlignHCenter
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
            keepMoves: root.rightRoom < rightZone.naturalWidth(rightZone.size, true)
            visible: !root.compact && rightZone.populated
            Layout.maximumWidth: Math.max(0, root.rightRoom)
            Layout.minimumWidth: Math.min(implicitWidth - shrinkable, Layout.maximumWidth)
            instances: root.zones.right
            room: Math.max(0, root.rightRoom)
        }
    }
}
