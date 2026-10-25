pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Services

Singleton {
    // Desktops (including DMS) signal light mode as "default" (0), so only prefer-dark counts as dark.
    readonly property bool systemPrefersLight: PlayerService.colorSchemeAvailable && PlayerService.colorScheme !== 1
}
