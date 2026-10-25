import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// "Keep on this device" on a playlist, album or artist page: adds or removes the item's offline rule.
Row {
    id: root

    // "playlist" | "album" | "artist"
    property string kind: "playlist"
    property var item: null
    readonly property var keep: SettingsService.keepState(kind, item)

    visible: keep.shown
    spacing: Theme.spacingM

    DToggle {
        id: toggle
        anchors.verticalCenter: parent.verticalCenter
        hideText: true
        text: label.text
        enabled: root.keep.blocker === ""
        checked: root.keep.kept
        onToggled: value => SettingsService.setKept(root.kind, root.item, value)
    }

    Column {
        anchors.verticalCenter: parent.verticalCenter

        StyledText {
            id: label
            text: I18n.tr("Keep on this device", "toggle that caches an item's tracks for offline play")
            font.pixelSize: Theme.fontSizeMedium
            color: toggle.enabled ? Theme.surfaceText : Theme.surfaceVariantText
        }

        StyledText {
            visible: text !== ""
            text: root.keep.blocker
            font.pixelSize: Theme.fontSizeSmall
            color: Theme.surfaceVariantText
        }
    }
}
