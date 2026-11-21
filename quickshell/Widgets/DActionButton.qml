import QtQuick
import qs.DCommon.Widgets as Up

// The shared DActionButton plus press feedback, until the shared one has it.
Up.DActionButton {
    id: root

    DPressFeedback {
        control: root
        rippleColor: root.stateColor
    }
}
