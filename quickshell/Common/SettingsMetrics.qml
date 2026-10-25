pragma Singleton

import QtQuick
import Quickshell
import qs.Common

Singleton {
    readonly property real windowWidth: 960
    readonly property real windowHeight: 760
    readonly property real windowMinWidth: 520
    readonly property real windowMinHeight: 420
    readonly property real sidebarWidth: 260
    readonly property real compactBreakpoint: 640
    readonly property real contentMaxWidth: 920
    readonly property real pagePaddingV: Theme.spacingXL
    readonly property real paneMargin: Theme.windowInset
    readonly property real panePadding: Theme.spacingXL
    readonly property real rowPaddingH: Theme.spacingL + Theme.spacingXS
    readonly property real rowPaddingV: Theme.spacingL
    readonly property real rowContentSpacing: Theme.spacingL
    readonly property real sectionLabelTopGap: Theme.spacingS
    readonly property real sectionLabelBottomGap: Theme.spacingM
    readonly property real navIconSize: Theme.avatarSize
    readonly property real navItemMinHeight: Theme.listItemHeight
    readonly property real buttonGroupCompactThreshold: Theme.spacingXL * 8
    readonly property real fieldWidth: 320
    readonly property real dropdownWidth: fieldWidth / 2
    // M3 linear progress track
    readonly property real usageBarHeight: 4
    readonly property real highlightBlend: 0.2
    readonly property color paneColor: Theme.floatingWindowPaneSurface
    readonly property color sidebarRowColor: Theme.floatingWindowPaneSurface
    readonly property color rowColor: Theme.foregroundColor(Theme.chipSurface, true)
    readonly property color selectedRowColor: Theme.blend(rowColor, Theme.onSurface, Theme.stateLayerDrag)
    readonly property int fadeDuration: Theme.expressiveDurations.expressiveEffects
}
