import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// Lyrics / Queue switch at the top of the full view's second pane. Picking a tab changes the pane but never closes it.
DButtonGroup {
    id: root
    objectName: "paneTabs"

    readonly property var panes: ["lyrics", "queue"]

    size: "small"
    checkEnabled: false
    selectedColor: ArtColors.accent
    selectedContentColor: ArtColors.onAccent
    unselectedColor: ArtColors.accentSecondaryContainer
    unselectedContentColor: ArtColors.onAccentSecondaryContainer
    usePopupTransparency: false
    model: [I18n.tr("Lyrics", "Lyrics"), I18n.tr("Queue", "Queue")]
    currentIndex: panes.indexOf(Router.viewPane)
    onSelectionChanged: (index, selected) => {
        if (!selected || Router.viewPane === panes[index])
            return;
        Router.viewPane = panes[index];
        SettingsData.lastPane = panes[index];
    }
}
