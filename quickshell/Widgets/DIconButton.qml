import QtQuick
import qs.DCommon.Widgets as Up

// The shared DIconButton plus press feedback, until the shared one has it.
Up.DIconButton {
    id: root

    DPressFeedback {
        control: root
        rippleColor: root.stateColor
    }
}
