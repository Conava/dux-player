import QtQuick

// Something Escape closes: a search field, a selection, edit mode, the full view, a dialog. Active layers stack
// in Router in the order they turned on (raise() puts one back on top); the window's one Escape shortcut closes
// the topmost whose owner shows in that window.
QtObject {
    id: layer

    required property Item owner
    property bool active: false
    // the owner takes Escape itself (a dialog or sheet): the window's shortcut stays off while it is on top
    property bool handlesKey: false

    signal escaped

    function raise() {
        if (active)
            Router.raiseEscape(layer);
    }

    onActiveChanged: active ? Router.raiseEscape(layer) : Router.dropEscape(layer)
    Component.onDestruction: Router.dropEscape(layer)
}
