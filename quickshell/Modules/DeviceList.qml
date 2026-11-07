pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// Where playback goes: one device, or several local outputs at once (the device picker and Bento's Play on sheet).
Column {
    id: root

    property bool several: PlayerService.multiOutput
    // the selected output's row holds the player's volume
    property bool showVolume: false
    // several outputs at once are local outputs only
    readonly property var shown: PlayerService.outputDevices.filter(d => !several || Phrases.isLocalOutput(d))
    readonly property int localCount: PlayerService.outputDevices.filter(d => Phrases.isLocalOutput(d)).length

    // a single output was picked
    signal picked

    function reset() {
        several = PlayerService.multiOutput;
    }

    spacing: Theme.spacingM

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
        // stays enabled while on, so it can be turned off after an output went away
        enabled: SettingsService.canRoute && PlayerService.canMultiOutput && (root.localCount > 1 || root.several)
        text: I18n.tr("Play on multiple outputs", "playback device setting")
        description: {
            if (!SettingsService.canRoute)
                return I18n.tr("Requires mpv.", "why a playback setting is unavailable");
            if (!PlayerService.canMultiOutput)
                return I18n.tr("Requires PipeWire (pipewire, pw-cli and pw-play)", "why several outputs and the equalizer are unavailable");
            if (!enabled)
                return I18n.tr("Requires two or more outputs", "why playing on multiple outputs is unavailable");
            return I18n.tr("Choose the outputs to play on at the same time.", "playback device setting description");
        }
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
                readonly property real lineHeight: note !== "" ? Theme.listItemTwoLineHeight : Theme.listItemHeight
                readonly property bool volumeShown: root.showVolume && selected && !root.several
                width: parent.width
                implicitHeight: lineHeight + (volumeShown ? volume.height + Theme.spacingM : 0)
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
                    root.picked();
                }

                DIcon {
                    id: icon
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.spacingL
                    y: (row.lineHeight - height) / 2
                    name: UiConfig.deviceIcon(row.modelData.kind)
                    size: Theme.iconSize
                    color: row.contentColor
                }

                Column {
                    anchors.left: icon.right
                    anchors.leftMargin: Theme.spacingL
                    anchors.right: check.left
                    anchors.rightMargin: Theme.spacingM
                    y: (row.lineHeight - height) / 2
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
                    y: (row.lineHeight - height) / 2
                    visible: root.several || row.selected
                    name: root.several ? (row.selected ? "check_box" : "check_box_outline_blank") : "check"
                    filled: row.selected
                    size: Theme.iconSize
                    color: row.selected ? Theme.primary : row.supportingContentColor
                }

                MediaVolumeSlider {
                    id: volume
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.margins: Theme.spacingL
                    y: row.lineHeight - Theme.spacingXS
                    visible: row.volumeShown
                }
            }
        }
    }
}
