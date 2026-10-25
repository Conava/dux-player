import QtQuick
import qs.Common
import qs.DCommon.Widgets

// A text link: underlined while hovered or focused, Enter or Space follows it. Without `linked` it is plain text.
StyledText {
    id: root

    property bool linked: true

    signal activated

    elide: Text.ElideRight
    wrapMode: Text.NoWrap
    font.underline: linked && (mouse.containsMouse || activeFocus)
    activeFocusOnTab: linked
    Accessible.role: linked ? Accessible.Link : Accessible.StaticText
    Accessible.name: text
    Accessible.onPressAction: root.activated()
    Keys.onPressed: event => {
        if (!linked || (event.key !== Qt.Key_Return && event.key !== Qt.Key_Enter && event.key !== Qt.Key_Space))
            return;
        activated();
        event.accepted = true;
    }

    MouseArea {
        id: mouse
        anchors.fill: parent
        enabled: root.linked
        hoverEnabled: true
        cursorShape: Qt.PointingHandCursor
        onClicked: root.activated()
    }

    FocusRing {
        radius: Theme.cornerRadiusXS
        visible: root.activeFocus
    }
}
