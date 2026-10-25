import QtQuick
import qs.Services

// Holds PlayerService.watchPosition while active, so the position only ticks while something shows it.
QtObject {
    property bool active: false
    property bool _held: false

    onActiveChanged: _hold(active)
    Component.onCompleted: _hold(active)
    Component.onDestruction: _hold(false)

    function _hold(on) {
        if (_held === on)
            return;
        _held = on;
        PlayerService.watchPosition(on);
    }
}
