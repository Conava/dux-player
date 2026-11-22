pragma ComponentBehavior: Bound

import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// M3 navigation rail that morphs between collapsed (labels under the icons) and expanded (icon and label side by side
// in a full-width pill) with `expansion`, 0 to 1. Same model and signals as DNavigationBar ([{text, icon}],
// currentIndex, activated).
FocusScope {
    id: root

    property var model: []
    property int currentIndex: -1
    // 0 collapsed, 1 expanded; animate it from outside, the rail follows every frame
    property real expansion: 1

    signal activated(int index)

    // room around the destinations so the clipped view doesn't cut their focus ring
    readonly property real ringRoom: Style.focusRingOffset + Style.focusRingWidth
    // the labels beside the icons come in over the second half of an expand and leave in the first third of a collapse;
    // the labels under the icons the other way round
    property bool expanding: true
    property real lastExpansion: 0
    readonly property real sideLabels: expanding ? clamp((expansion - 0.5) * 2) : clamp((expansion - 2 / 3) * 3)
    readonly property real underLabels: expanding ? clamp(1 - expansion * 3) : clamp(1 - expansion * 2)

    function clamp(x) {
        return Math.max(0, Math.min(1, x));
    }
    function mix(from, to) {
        return from + (to - from) * expansion;
    }

    onExpansionChanged: {
        expanding = expansion > lastExpansion;
        lastExpansion = expansion;
    }
    Component.onCompleted: lastExpansion = expansion

    implicitWidth: Style.navigationRailWidth * (1 + expansion)
    activeFocusOnTab: true
    LayoutMirroring.enabled: I18n.isRtl
    LayoutMirroring.childrenInherit: true
    Accessible.role: Accessible.PageTabList

    Keys.onPressed: event => {
        const count = model.length;
        if (count === 0)
            return;
        switch (event.key) {
        case Qt.Key_Return:
        case Qt.Key_Enter:
        case Qt.Key_Space:
            // a one-entry rail with nothing selected has no other way to be activated
            if (event.isAutoRepeat || currentIndex >= 0 || count !== 1)
                return;
            activated(0);
            break;
        case Qt.Key_Down:
            activated((Math.max(0, currentIndex) + 1) % count);
            break;
        case Qt.Key_Up:
            activated((Math.max(0, currentIndex) - 1 + count) % count);
            break;
        case Qt.Key_Home:
            activated(0);
            break;
        case Qt.Key_End:
            activated(count - 1);
            break;
        default:
            return;
        }
        event.accepted = true;
    }

    DFlickable {
        anchors.fill: parent
        contentWidth: width
        contentHeight: column.height + column.y * 2
        fadeEdges: false
        clip: true

        Column {
            id: column
            x: root.mix(0, Style.spacingM)
            y: root.mix(0, root.ringRoom)
            width: parent.width - x * 2
            spacing: root.mix(0, Style.spacingXS)

            Repeater {
                model: root.model

                StyledButton {
                    id: destination
                    required property var modelData
                    required property int index
                    readonly property bool selected: root.currentIndex === index
                    // as DNavigationBar's destinations: the indicator pill with the label under it
                    readonly property real collapsedHeight: Math.max(Style.navigationHeight, Style.navigationVerticalPadding * 2 + Style.navigationIndicatorHeight + Style.spacingXS + under.implicitHeight)

                    width: column.width
                    height: root.mix(collapsedHeight, Style.navigationHeight)
                    radius: Style.fullRadius(width, height)
                    color: "transparent"
                    focusPolicy: Qt.NoFocus
                    Accessible.role: Accessible.PageTab
                    Accessible.name: modelData?.text ?? ""
                    Accessible.selected: selected
                    onClicked: root.activated(index)

                    DPressFeedback {
                        control: destination
                    }

                    // the indicator collapsed, the whole destination expanded
                    Rectangle {
                        id: pill
                        x: root.mix((destination.width - Style.navigationIndicatorWidth) / 2, 0)
                        y: root.mix(Style.navigationVerticalPadding, 0)
                        width: root.mix(Style.navigationIndicatorWidth, destination.width)
                        height: root.mix(Style.navigationIndicatorHeight, destination.height)
                        radius: Style.fullRadius(width, height)
                        color: destination.selected ? Style.selectedContainer : "transparent"

                        StateLayer {
                            control: destination
                            stateColor: destination.selected ? Style.onSelectedContainer : Style.onSurface
                        }

                        // the destinations take no focus; the rail shows where the keyboard is
                        FocusRing {
                            visible: root.activeFocus && (destination.selected || (root.currentIndex < 0 && root.model.length === 1))
                        }

                        DIcon {
                            id: icon
                            readonly property real start: root.mix((pill.width - width) / 2, Style.spacingL)
                            x: I18n.isRtl ? pill.width - width - start : start
                            anchors.verticalCenter: parent.verticalCenter
                            name: destination.modelData?.icon ?? ""
                            size: Style.iconSize
                            filled: destination.selected
                            color: destination.selected ? Style.onSelectedContainer : Style.onSurfaceVariant
                        }

                        // StyledText eases every opacity change, so the fade is set on a plain item around it
                        Item {
                            anchors.left: icon.right
                            anchors.leftMargin: Style.spacingM
                            anchors.verticalCenter: parent.verticalCenter
                            width: Math.max(0, pill.width - Style.iconSize - Style.spacingL * 2 - Style.spacingM)
                            height: side.implicitHeight
                            visible: opacity > 0
                            opacity: root.sideLabels

                            StyledText {
                                id: side
                                width: parent.width
                                text: destination.modelData?.text ?? ""
                                font.pixelSize: Style.fontSizeMedium
                                font.weight: Style.fontWeightMedium
                                color: destination.selected ? Style.onSelectedContainer : Style.onSurfaceVariant
                                elide: Text.ElideRight
                                wrapMode: Text.NoWrap
                            }
                        }
                    }

                    Item {
                        x: (destination.width - width) / 2
                        y: pill.y + pill.height + Style.spacingXS
                        width: Style.navigationRailWidth - Style.spacingXS * 2
                        height: under.implicitHeight
                        visible: opacity > 0
                        opacity: root.underLabels

                        StyledText {
                            id: under
                            width: parent.width
                            text: destination.modelData?.text ?? ""
                            font.pixelSize: Style.fontSizeSmall
                            font.weight: Style.fontWeightMedium
                            horizontalAlignment: Text.AlignHCenter
                            wrapMode: Text.Wrap
                            color: destination.selected ? Style.secondary : Style.onSurfaceVariant
                        }
                    }
                }
            }
        }
    }
}
