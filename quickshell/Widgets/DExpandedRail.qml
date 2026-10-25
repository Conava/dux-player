pragma ComponentBehavior: Bound

import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// M3 expanded navigation rail: icon and label side by side in a full-width pill. Same model and signals as
// DNavigationBar ([{text, icon}], currentIndex, activated).
FocusScope {
    id: root

    property var model: []
    property int currentIndex: -1

    signal activated(int index)

    implicitWidth: Style.navigationRailWidth * 2
    activeFocusOnTab: true
    LayoutMirroring.enabled: I18n.isRtl
    LayoutMirroring.childrenInherit: true
    Accessible.role: Accessible.PageTabList

    Keys.onPressed: event => {
        const count = model.length;
        if (count === 0)
            return;
        switch (event.key) {
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
        contentHeight: column.height
        fadeEdges: false
        clip: true

        Column {
            id: column
            x: Style.spacingM
            width: parent.width - Style.spacingM * 2
            spacing: Style.spacingXS

            Repeater {
                model: root.model

                StyledButton {
                    id: destination
                    required property var modelData
                    required property int index
                    readonly property bool selected: root.currentIndex === index

                    width: column.width
                    height: Style.navigationHeight
                    radius: Style.fullRadius(width, height)
                    color: selected ? Style.selectedContainer : "transparent"
                    focusPolicy: Qt.NoFocus
                    Accessible.role: Accessible.PageTab
                    Accessible.name: modelData?.text ?? ""
                    Accessible.selected: selected
                    onClicked: root.activated(index)

                    StateLayer {
                        control: destination
                        stateColor: destination.selected ? Style.onSelectedContainer : Style.onSurface
                    }

                    // the destinations take no focus; the rail shows where the keyboard is
                    FocusRing {
                        visible: root.activeFocus && destination.selected
                    }

                    Row {
                        anchors.left: parent.left
                        anchors.leftMargin: Style.spacingL
                        anchors.verticalCenter: parent.verticalCenter
                        spacing: Style.spacingM

                        DIcon {
                            anchors.verticalCenter: parent.verticalCenter
                            name: destination.modelData?.icon ?? ""
                            size: Style.iconSize
                            filled: destination.selected
                            color: destination.selected ? Style.onSelectedContainer : Style.onSurfaceVariant
                        }

                        StyledText {
                            anchors.verticalCenter: parent.verticalCenter
                            width: destination.width - Style.iconSize - Style.spacingL * 2 - Style.spacingM
                            text: destination.modelData?.text ?? ""
                            font.pixelSize: Style.fontSizeMedium
                            font.weight: Style.fontWeightMedium
                            color: destination.selected ? Style.onSelectedContainer : Style.onSurfaceVariant
                            elide: Text.ElideRight
                            wrapMode: Text.NoWrap
                        }
                    }
                }
            }
        }
    }
}
