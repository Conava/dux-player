import QtQuick
import qs.DCommon.Widgets as Up

// The shared DIconButton plus press feedback, until the shared one has it.
// Replaces the shared widget only where `import qs.Widgets` comes after the DCommon import.
Up.DIconButton {
    id: root

    DPressFeedback {
        control: root
        rippleColor: root.stateColor
    }
}
