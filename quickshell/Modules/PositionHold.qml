import QtQuick
import qs.Services

// Holds PlayerService.watchPosition while active, so the position only ticks while something shows it.
QtObject {
    id: root

    property bool active: false
    property bool _held: false

    // no helper function: while the object is torn down QML can no longer resolve its own functions
    onActiveChanged: {
        if (root._held === root.active)
            return;
        root._held = root.active;
        PlayerService.watchPosition(root.active);
    }
    Component.onCompleted: {
        if (root._held === root.active)
            return;
        root._held = root.active;
        PlayerService.watchPosition(root.active);
    }
    Component.onDestruction: {
        if (!root._held)
            return;
        root._held = false;
        PlayerService.watchPosition(false);
    }
}
