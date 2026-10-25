pragma ComponentBehavior: Bound

import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// Shows a key combination as a keycap; click (or Enter/Space) and press keys to record a new one. Escape or
// leaving the field cancels. `captured` carries Qt's portable text, e.g. "Ctrl+Shift+F".
FocusScope {
    id: root

    property string sequence: ""
    property bool recording: false

    signal captured(string sequence)

    // the key's portable name, or "" for a modifier alone or a key without one
    function keyName(event) {
        const k = event.key;
        if (k >= Qt.Key_A && k <= Qt.Key_Z || k >= Qt.Key_0 && k <= Qt.Key_9)
            return String.fromCharCode(k);
        if (k >= Qt.Key_F1 && k <= Qt.Key_F35)
            return "F" + (k - Qt.Key_F1 + 1);
        switch (k) {
        case Qt.Key_Space:
            return "Space";
        case Qt.Key_Return:
        case Qt.Key_Enter:
            return "Return";
        case Qt.Key_Delete:
            return "Del";
        case Qt.Key_Home:
            return "Home";
        case Qt.Key_End:
            return "End";
        case Qt.Key_PageUp:
            return "PgUp";
        case Qt.Key_PageDown:
            return "PgDown";
        case Qt.Key_Left:
            return "Left";
        case Qt.Key_Right:
            return "Right";
        case Qt.Key_Up:
            return "Up";
        case Qt.Key_Down:
            return "Down";
        }
        // punctuation: the character itself, for the keys Qt names by it
        return k > 0x20 && k < 0x7f ? String.fromCharCode(k) : "";
    }

    function capture(event) {
        if (event.key === Qt.Key_Escape && event.modifiers === Qt.NoModifier) {
            recording = false;
            return;
        }
        const key = keyName(event);
        if (key === "")
            return;
        const mods = [];
        if (event.modifiers & Qt.ControlModifier)
            mods.push("Ctrl");
        if (event.modifiers & Qt.AltModifier)
            mods.push("Alt");
        if (event.modifiers & Qt.ShiftModifier)
            mods.push("Shift");
        if (event.modifiers & Qt.MetaModifier)
            mods.push("Meta");
        recording = false;
        captured(mods.concat([key]).join("+"));
    }

    function start() {
        recording = true;
        forceActiveFocus();
    }

    implicitWidth: Math.max(Style.buttonHeightS * 4, content.implicitWidth + Style.spacingM * 2)
    implicitHeight: Style.buttonHeightS
    activeFocusOnTab: true
    Accessible.role: Accessible.Button
    Accessible.name: recording ? hint.text : sequence
    Accessible.onPressAction: start()
    onActiveFocusChanged: {
        if (!activeFocus)
            recording = false;
    }
    Keys.onPressed: event => {
        if (recording) {
            event.accepted = true;
            capture(event);
            return;
        }
        if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter || event.key === Qt.Key_Space) {
            event.accepted = true;
            start();
        }
    }

    Rectangle {
        id: frame
        anchors.fill: parent
        radius: Style.cornerRadiusXS
        color: "transparent"
        border.color: root.recording ? Style.primary : Style.outline
        border.width: root.recording ? Style.outlineWidthFocused : Style.outlineWidth
    }

    MouseArea {
        anchors.fill: parent
        cursorShape: Qt.PointingHandCursor
        onClicked: root.recording ? root.recording = false : root.start()
    }

    Row {
        id: content
        anchors.centerIn: parent
        spacing: Style.spacingS

        DIcon {
            anchors.verticalCenter: parent.verticalCenter
            name: "keyboard"
            size: Style.iconSize
            color: Style.surfaceVariantText
        }

        DKeycap {
            anchors.verticalCenter: parent.verticalCenter
            visible: !root.recording && root.sequence !== ""
            text: root.sequence
        }

        StyledText {
            id: hint
            anchors.verticalCenter: parent.verticalCenter
            visible: root.recording
            text: I18n.tr("Press keys…", "Key combination field while it records")
            font.pixelSize: Style.fontSizeMedium
            color: Style.surfaceVariantText
        }
    }

    FocusRing {
        radius: frame.radius
    }
}
