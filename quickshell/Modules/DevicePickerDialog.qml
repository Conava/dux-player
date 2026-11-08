pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// Where playback goes: one device, or several local outputs at once.
WindowDialog {
    id: root

    opened: Router.devicePickerOpen
    title: I18n.tr("Playback device", "playback device: settings tab, button, dialog")
    acceptEnabled: false
    onRejected: Router.devicePickerOpen = false
    onOpenedChanged: {
        // cast devices are looked for only while the picker shows
        PlayerService.watchCastDevices(opened);
        if (opened)
            list.reset();
    }

    DeviceList {
        id: list
        width: parent.width
        spacing: root.contentSpacing
        onPicked: Router.devicePickerOpen = false
    }

    actions: [
        DButton {
            visible: list.several
            text: I18n.tr("Sync…", "playback device action opening the delay settings")
            iconName: "tune"
            backgroundColor: Theme.secondaryContainer
            textColor: Theme.onSecondaryContainer
            onClicked: {
                Router.devicePickerOpen = false;
                Router.openSettings("devices");
            }
        },
        DButton {
            text: I18n.tr("Close", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: Router.devicePickerOpen = false
        }
    ]
}
