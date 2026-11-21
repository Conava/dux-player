import QtQuick
import qs.DCommon.Widgets as Up

// The shared DButton plus press feedback, until the shared one has it.
Up.DButton {
    id: root

    DPressFeedback {
        control: root
        rippleColor: root.textColor
    }
}
