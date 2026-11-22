import QtQuick
import qs.DCommon.Widgets as Up

// The shared DListItem plus press feedback, until the shared one has it.
Up.DListItem {
    id: root

    // set while a drag owns the press
    property bool pressCancelled: false

    DPressFeedback {
        control: root
        rippleColor: root.contentColor
        cancelled: root.pressCancelled
    }
}
