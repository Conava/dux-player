import QtQuick
import qs.DCommon.Widgets as Up

// The shared DFab plus press feedback, until the shared one has it.
Up.DFab {
    id: root

    DPressFeedback {
        control: root
        rippleColor: root.contentColor
    }
}
