import QtQuick
import QtQuick.Templates as T
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

    // what had the focus before, with its reason, so a clicked opener gets no focus ring back
    property Item _focusBack: null
    property int _focusBackReason: Qt.OtherFocusReason

    onOpenedChanged: {
        if (opened) {
            _focusBack = Window.activeFocusItem;
            _focusBackReason = (_focusBack as T.Control)?.focusReason ?? Qt.OtherFocusReason;
            // later: taking focus inside a binding update of opened loops through DDialog's focus tracking
            Qt.callLater(() => {
                if (opened && !activeFocus)
                    forceActiveFocus();
            });
            return;
        }
        // not when the focus already moved on, e.g. to a dialog opened from this one
        const returns = activeFocus || Window.activeFocusItem === null || Window.activeFocusItem === Window.contentItem;
        if (returns && _focusBack?.visible)
            _focusBack.forceActiveFocus(_focusBackReason);
        _focusBack = null;
    }

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

    // Out of the tree until the host has a window: a dialog that waits inside a hidden host which has no window yet
    // (a sub-page while its page loads) is never drawn after it moves to the window, though it is visible and opened.
    readonly property Binding _hostParent: Binding {
        target: root
        property: "parent"
        when: root.host !== null
        value: root.host?.Window.window?.contentItem ?? null
    }
}
