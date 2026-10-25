pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// Where playback goes: one device, or several local outputs at once.
WindowDialog {
    id: root

    property bool several: PlayerService.multiOutput
    // several outputs at once are local outputs only
    readonly property var shown: PlayerService.outputDevices.filter(d => !several || Phrases.isLocalOutput(d))

    opened: Router.devicePickerOpen
    title: I18n.tr("Playback device", "playback device: settings tab, button, dialog")
    acceptEnabled: false
    onRejected: Router.devicePickerOpen = false
    onOpenedChanged: {
        // cast devices are looked for only while the picker shows
        PlayerService.watchCastDevices(opened);
        if (opened) {
            several = PlayerService.multiOutput;
            forceActiveFocus();
        }
    }

    StyledText {
        width: parent.width
        visible: PlayerService.castStatus.updateNeeded === true
        text: I18n.tr("cast-helper is out of date. Update it to cast.", "playback device picker note")
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.error
        wrapMode: Text.WordWrap
    }

    StyledText {
        width: parent.width
        visible: text !== ""
        text: OutputService.castErrorText
        font.pixelSize: Theme.fontSizeSmall
        color: Theme.error
        wrapMode: Text.WordWrap
    }

    DToggle {
        width: parent.width
        enabled: PlayerService.canMultiOutput
        text: I18n.tr("Play on multiple outputs", "playback device setting")
        description: PlayerService.canMultiOutput ? I18n.tr("Choose the outputs to play on at the same time.", "playback device setting description") : I18n.tr("Requires PipeWire (pipewire, pw-cli and pw-play)", "why several outputs and the equalizer are unavailable")
        checked: root.several
        onToggled: value => {
            root.several = value;
            if (!value && PlayerService.multiOutput)
                PlayerService.selectOutput(PlayerService.selectedOutputs[0]);
        }
    }

    Column {
        width: parent.width
        spacing: Theme.groupedListGap

        Repeater {
            model: root.shown

            DListItem {
                id: row
                required property var modelData
                required property int index
                readonly property bool selected: PlayerService.selectedOutputs.indexOf(modelData.id) >= 0
                readonly property string note: Phrases.deviceNote(modelData)
                width: parent.width
                implicitHeight: note !== "" ? Theme.listItemTwoLineHeight : Theme.listItemHeight
                enabled: modelData.available
                isSelected: selected && !root.several
                firstInGroup: index === 0
                lastInGroup: index === root.shown.length - 1
                Accessible.role: root.several ? Accessible.CheckBox : Accessible.RadioButton
                Accessible.name: modelData.name
                Accessible.checked: selected
                onClicked: {
                    if (root.several) {
                        PlayerService.toggleOutput(modelData.id);
                        return;
                    }
                    PlayerService.selectOutput(modelData.id);
                    Router.devicePickerOpen = false;
                }

                DIcon {
                    id: icon
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.spacingL
                    anchors.verticalCenter: parent.verticalCenter
                    name: UiConfig.deviceIcon(row.modelData.kind)
                    size: Theme.iconSize
                    color: row.contentColor
                }

                Column {
                    anchors.left: icon.right
                    anchors.leftMargin: Theme.spacingL
                    anchors.right: check.left
                    anchors.rightMargin: Theme.spacingM
                    anchors.verticalCenter: parent.verticalCenter
                    spacing: Theme.spacingXXS

                    StyledText {
                        width: parent.width
                        text: row.modelData.name
                        font.pixelSize: Theme.fontSizeMedium
                        color: row.contentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                    }

                    StyledText {
                        width: parent.width
                        visible: row.note !== ""
                        text: row.note
                        font.pixelSize: Theme.fontSizeSmall
                        color: row.supportingContentColor
                        elide: Text.ElideRight
                        wrapMode: Text.NoWrap
                    }
                }

                DIcon {
                    id: check
                    anchors.right: parent.right
                    anchors.rightMargin: Theme.spacingL
                    anchors.verticalCenter: parent.verticalCenter
                    visible: root.several || row.selected
                    name: root.several ? (row.selected ? "check_box" : "check_box_outline_blank") : "check"
                    filled: row.selected
                    size: Theme.iconSize
                    color: row.selected ? Theme.primary : row.supportingContentColor
                }
            }
        }
    }

    actions: [
        DButton {
            visible: root.several
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
