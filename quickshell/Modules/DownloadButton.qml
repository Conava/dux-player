import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// Download on a playlist, album or artist page, drawn like the heart: adds the item's offline rule, and asks
// before removing it. Sits in PlayButtons; anchors inside its Flow stop it from laying out, so this centres itself.
Item {
    id: root

    // "playlist" | "album" | "artist"
    property string kind: "playlist"
    property var item: null
    readonly property var keep: SettingsService.keepState(kind, item)
    // why the button is locked; the page shows it under the buttons
    readonly property string blocker: keep.blocker

    visible: keep.shown
    implicitWidth: button.width
    implicitHeight: Theme.buttonHeightM
    width: implicitWidth
    height: implicitHeight

    DActionButton {
        id: button
        anchors.verticalCenter: parent.verticalCenter
        enabled: root.blocker === ""
        iconName: root.keep.kept ? "download_done" : "download"
        iconColor: root.keep.kept ? Theme.primary : Theme.onSurfaceVariant
        tooltipText: root.keep.kept ? I18n.tr("Downloaded", "button state: the item's tracks are kept offline") : I18n.tr("Download", "button that keeps an item's tracks for offline play")
        Accessible.role: Accessible.CheckBox
        Accessible.name: tooltipText
        Accessible.checked: root.keep.kept
        onClicked: {
            if (root.keep.kept)
                removeDialog.ask();
            else
                SettingsService.setKept(root.kind, root.item, true);
        }
    }

    ConfirmDialog {
        id: removeDialog
        host: root
        title: I18n.tr("Remove download?", "remove offline rule confirmation title")
        supportingText: I18n.tr("Deletes the downloaded tracks from this device.", "remove offline rule confirmation")
        confirmText: I18n.tr("Remove", "button that removes an item")
        onConfirmed: SettingsService.setKept(root.kind, root.item, false)
    }
}
