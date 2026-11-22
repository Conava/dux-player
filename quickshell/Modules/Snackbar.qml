import QtQuick
import qs.Common
import qs.DCommon.Widgets
import qs.Widgets

// One line of feedback at the bottom of the window: a failed command, what a mixed selection skipped, or a
// removal with its Undo button.
Rectangle {
    id: root

    // room for what sits at the window's bottom edge
    property real bottomInset: 0
    // the Settings window's snackbar; it shows notices only while that window has the focus
    property bool inSettings: false
    readonly property string notice: Router.notice
    readonly property bool showing: notice !== "" && inSettings === Router.settingsFocused
    // offset below its resting place while hidden; separate from bottomInset, which follows the bar's own motion
    property real rise: showing ? 0 : PlayerMetrics.listRise
    readonly property bool undoable: Router.noticeUndo !== null

    // keeps the text while it fades out
    onNoticeChanged: {
        if (notice !== "")
            label.text = notice;
    }
    onShowingChanged: {
        if (showing)
            label.text = notice;
    }

    Binding {
        target: Router
        property: "settingsFocused"
        value: root.Window.active
        when: root.inSettings
    }

    anchors.horizontalCenter: parent.horizontalCenter
    anchors.bottom: parent.bottom
    anchors.bottomMargin: bottomInset + Theme.spacingL + rise
    width: Math.min(parent.width - Theme.spacingL * 2, label.implicitWidth + Theme.spacingL * 2 + (undoButton.visible ? undoButton.width + Theme.spacingS : 0))
    height: Theme.fieldHeightLarge
    radius: Theme.cornerRadiusXS
    color: Theme.inverseSurface
    opacity: showing ? 1 : 0
    visible: opacity > 0
    Accessible.role: Accessible.AlertMessage
    Accessible.name: label.text

    Behavior on opacity {
        enabled: Theme.animationsEnabled
        NumberAnimation {
            duration: root.showing ? Theme.expressiveDurations.normal : Theme.expressiveDurations.expressiveEffects
            easing.type: Easing.BezierSpline
            easing.bezierCurve: root.showing ? Theme.expressiveCurves.emphasizedDecel : Theme.expressiveCurves.emphasizedAccel
        }
    }

    Behavior on rise {
        enabled: Theme.animationsEnabled
        NumberAnimation {
            duration: root.showing ? Theme.expressiveDurations.normal : Theme.expressiveDurations.expressiveEffects
            easing.type: Easing.BezierSpline
            easing.bezierCurve: root.showing ? Theme.expressiveCurves.emphasizedDecel : Theme.expressiveCurves.emphasizedAccel
        }
    }

    StyledText {
        id: label
        anchors.left: parent.left
        anchors.leftMargin: Theme.spacingL
        anchors.verticalCenter: parent.verticalCenter
        width: Math.min(implicitWidth, root.width - Theme.spacingL * 2 - (undoButton.visible ? undoButton.width + Theme.spacingS : 0))
        font.pixelSize: Theme.fontSizeMedium
        color: Theme.inverseOnSurface
        wrapMode: Text.NoWrap
        elide: Text.ElideRight
    }

    DButton {
        id: undoButton
        anchors.right: parent.right
        anchors.rightMargin: Theme.spacingXS
        anchors.verticalCenter: parent.verticalCenter
        visible: root.undoable
        text: I18n.tr("Undo", "snackbar button that reverts a removal")
        buttonHeight: Theme.buttonHeightXS
        backgroundColor: "transparent"
        textColor: Theme.inversePrimary
        onClicked: Router.undo()
    }
}
