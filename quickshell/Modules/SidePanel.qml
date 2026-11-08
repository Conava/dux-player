pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// The main window's side panel beside the pages: the queue or the lyrics (Router.panel).
Item {
    id: root

    // in the side panel's pane order, which by default follows the bar's buttons; both always offered here, as
    // the bar's buttons open either
    readonly property var views: UiConfig.sidePanes.filter(p => p !== "cover").concat(["lyrics", "queue"]).filter((p, i, all) => all.indexOf(p) === i && (UiConfig.lyricsOn || p !== "lyrics"))
    readonly property var labels: ({
            "queue": {
                "text": I18n.tr("Queue", "the play queue: panel, toggle"),
                "icon": "queue_music"
            },
            "lyrics": {
                "text": I18n.tr("Lyrics", "song lyrics: page, toggle, settings tab"),
                "icon": "lyrics"
            }
        })

    // keeps the closed pane on screen while the panel slides away
    readonly property string routed: Router.panel
    property string shown: routed !== "" ? routed : "queue"

    onRoutedChanged: {
        if (routed !== "")
            shown = routed;
    }

    function takeFocus() {
        switcher.requestFocus(false);
    }

    LayoutMirroring.enabled: I18n.isRtl
    LayoutMirroring.childrenInherit: true

    DButtonGroup {
        id: switcher
        anchors.top: parent.top
        anchors.topMargin: Theme.spacingL
        anchors.horizontalCenter: parent.horizontalCenter
        size: "small"
        checkEnabled: false
        model: root.views.map(v => root.labels[v])
        currentIndex: root.views.indexOf(root.shown)
        onSelectionChanged: (index, selected) => {
            if (selected)
                Router.panel = root.views[index];
        }
    }

    DActionButton {
        anchors.right: parent.right
        anchors.rightMargin: Theme.spacingM
        anchors.verticalCenter: switcher.verticalCenter
        iconName: "close"
        tooltipText: I18n.tr("Close panel", "side panel close button")
        Accessible.name: tooltipText
        onClicked: Router.panel = ""
    }

    Loader {
        id: pane
        // the first source is no switch: the panel has to be whole when it slides in
        property bool settled: false
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: switcher.bottom
        anchors.bottom: parent.bottom
        anchors.topMargin: Theme.spacingS
        sourceComponent: root.shown === "lyrics" ? lyrics : queue
        onSourceComponentChanged: {
            if (settled && Theme.animationsEnabled)
                paneEnter.restart();
        }
        Component.onCompleted: settled = true
    }

    NumberAnimation {
        id: paneEnter
        target: pane
        property: "opacity"
        from: 0
        to: 1
        duration: Theme.expressiveDurations.expressiveEffects
        easing.type: Easing.BezierSpline
        easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
    }

    Component {
        id: queue
        QueueList {}
    }

    Component {
        id: lyrics
        LyricsView {}
    }
}
