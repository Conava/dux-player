import QtQuick
import qs.Common
import qs.DCommon.Widgets

// A DDialog over its whole window. Declared inside a page, set `host` to that page: the dialog moves to the
// window's content item, out of the page's clip.
DDialog {
    id: root

    property Item host: null

    anchors.fill: parent
    z: Theme.zDialog
    embedded: false
    opened: false

    // Out of the window before the page's context goes: a Loader deletes the page later, and until then the
    // dialog's focus handlers would still run in the dead context ("invalid context" warnings).
    Component.onDestruction: {
        if (host !== null)
            parent = null;
    }

    readonly property EscapeLayer _escapeLayer: EscapeLayer {
        owner: root
        active: root.opened
        handlesKey: true
    }

    readonly property Binding _hostParent: Binding {
        target: root
        property: "parent"
        when: root.host !== null
        value: root.host?.Window.window?.contentItem ?? root.host
    }
}
