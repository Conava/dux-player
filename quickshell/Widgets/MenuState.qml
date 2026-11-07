pragma Singleton

import QtQuick
import Quickshell

// The DMenu that is open. Qt tries a window's shortcuts before it hands a key to a popup, so they check this first.
Singleton {
    property Item open: null
}
