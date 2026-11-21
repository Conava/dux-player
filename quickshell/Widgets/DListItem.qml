import QtQuick
import qs.DCommon.Widgets as Up

// The shared DListItem plus press feedback, until the shared one has it.
Up.DListItem {
    id: root

    DPressFeedback {
        control: root
        rippleColor: root.contentColor
    }
}
