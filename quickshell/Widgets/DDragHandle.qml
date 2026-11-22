import QtQuick
import qs.DCommon.Widgets as Up

// The shared DDragHandle without its tooltip while a drag is running, until the shared one drops it.
// Replaces the shared widget only where `import qs.Widgets` comes after the DCommon import.
Up.DDragHandle {
    id: root

    Binding {
        target: root
        property: "tooltipText"
        value: null
        when: root.dragging
    }
}
