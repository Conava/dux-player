import QtQuick
import qs.Common
import qs.DCommon.Widgets

// Leaves a settings sub-page.
DButton {
    text: I18n.tr("Back", "page header back button")
    iconName: I18n.isRtl ? "arrow_forward" : "arrow_back"
    backgroundColor: "transparent"
    textColor: Theme.primary
}
