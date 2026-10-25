pragma ComponentBehavior: Bound

import QtQuick
import qs.DCommon.Common
import qs.DCommon.Widgets

// Five stars. Clicking the current rating clears it; Left/Right change it from the keyboard.
Item {
    id: root

    property int rating: 0
    property int maximum: 5
    property color starColor: Style.primary
    // replaces the per-star tooltips, e.g. why rating does nothing here
    property string note: ""

    signal rated(int stars)

    implicitWidth: stars.implicitWidth
    implicitHeight: stars.implicitHeight
    activeFocusOnTab: true
    LayoutMirroring.enabled: I18n.isRtl
    LayoutMirroring.childrenInherit: true
    Accessible.role: Accessible.Slider
    Accessible.name: I18n.tr("Rating", "Accessible name of a star rating control")
    Accessible.description: I18n.tr("%1 of %2 stars", "Value of a star rating control").arg(rating).arg(maximum)
    Keys.onPressed: event => {
        const forward = I18n.isRtl ? Qt.Key_Left : Qt.Key_Right;
        const back = I18n.isRtl ? Qt.Key_Right : Qt.Key_Left;
        if (event.key === forward && rating < maximum)
            rated(rating + 1);
        else if (event.key === back && rating > 0)
            rated(rating - 1);
        else
            return;
        event.accepted = true;
    }

    Row {
        id: stars

        Repeater {
            model: root.maximum

            DActionButton {
                required property int index
                readonly property string valueText: I18n.tr("%1 of %2 stars", "Value of a star rating control").arg(index + 1).arg(root.maximum)
                buttonSize: Style.buttonHeightXXS
                iconSize: Style.iconSizeSmall
                iconName: "star"
                iconFilled: index < root.rating
                iconColor: index < root.rating ? root.starColor : Style.onSurfaceVariant
                focusPolicy: Qt.NoFocus
                tooltipText: root.note !== "" ? root.note : valueText
                Accessible.name: valueText
                onClicked: root.rated(root.rating === index + 1 ? 0 : index + 1)
            }
        }
    }

    FocusRing {
        radius: Style.fullRadius(width, height)
        visible: root.activeFocus
    }
}
