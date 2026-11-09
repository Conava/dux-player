pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets

// Where playback goes, playing on several outputs at once, and their sync.
SettingsPage {
    id: root

    property bool syncPage: false
    // sub-views the settings search opens: the Sync page, when it can be opened
    readonly property var subViews: syncing.length > 1 ? ["sync"] : []
    readonly property string syncReason: I18n.tr("Select at least two outputs", "why the sync page is disabled")
    readonly property var locals: PlayerService.outputDevices.filter(d => Phrases.isLocalOutput(d))
    readonly property var syncing: PlayerService.selectedOutputs.map(id => PlayerService.output(id)).filter(d => d && Phrases.isLocalOutput(d))

    readonly property string subTitle: syncPage ? I18n.tr("Sync", "playback device sync page") : ""

    function openSubView(id) {
        syncPage = id === "sync";
    }

    function goBack() {
        syncPage = false;
    }

    SettingsCard {
        visible: !root.syncPage
        title: I18n.tr("Outputs", "playback device settings section")
        keywords: "speaker headphones audio cast"

        Repeater {
            model: PlayerService.outputDevices.filter(d => !PlayerService.multiOutput || !Phrases.isLocalOutput(d))

            SettingsRow {
                id: deviceRow
                required property var modelData
                readonly property bool selected: PlayerService.selectedOutputs.indexOf(modelData.id) >= 0
                title: modelData.name
                subtitle: Phrases.deviceNote(modelData)
                iconName: UiConfig.deviceIcon(modelData.kind)
                enabled: modelData.available
                clickable: true
                Accessible.role: Accessible.RadioButton
                Accessible.checked: selected
                onClicked: PlayerService.selectOutput(modelData.id)

                DIcon {
                    anchors.verticalCenter: parent.verticalCenter
                    visible: deviceRow.selected
                    name: "check"
                    size: Theme.iconSize
                    color: Theme.primary
                }

                // the playing output carries the volume
                body: Loader {
                    width: parent.width
                    active: deviceRow.selected && PlayerService.selectedOutputs[0] === deviceRow.modelData.id
                    visible: active

                    sourceComponent: Row {
                        id: volumeRow
                        readonly property bool muted: PlayerService.muted || PlayerService.volume === 0
                        spacing: Theme.spacingS

                        DActionButton {
                            buttonSize: Theme.iconButtonSize
                            iconName: volumeRow.muted ? "volume_off" : PlayerService.volume < 0.5 ? "volume_down" : "volume_up"
                            iconSize: Theme.iconSizeMedium
                            iconColor: Theme.surfaceVariantText
                            anchors.verticalCenter: parent.verticalCenter
                            Accessible.name: I18n.tr("Mute", "volume button")
                            Accessible.role: Accessible.CheckBox
                            Accessible.checked: volumeRow.muted
                            onClicked: PlayerService.setMuted(!PlayerService.muted)
                        }

                        DSlider {
                            id: volumeSlider
                            size: "s"
                            width: parent.width - (Theme.iconButtonSize + parent.spacing) - volumeText.width - parent.spacing
                            anchors.verticalCenter: parent.verticalCenter
                            maximum: Math.max(1, Math.round(PlayerService.volumeMax * 100))
                            unit: "%"
                            wheelEnabled: false
                            showValue: false
                            Accessible.name: I18n.tr("Volume", "audio volume: settings section, slider") + ": " + deviceRow.title
                            onSliderValueChanged: newValue => PlayerService.setVolume(newValue / 100)

                            Binding on value {
                                value: volumeRow.muted ? 0 : Math.round(PlayerService.volume * 100)
                                when: !volumeSlider.isDragging
                                restoreMode: Binding.RestoreNone
                            }
                        }

                        NumericText {
                            id: volumeText
                            width: Math.ceil(reservedWidth)
                            anchors.verticalCenter: parent.verticalCenter
                            isMonospace: false
                            text: volumeSlider.formatValue(volumeSlider.value)
                            reserveText: volumeSlider.formatValue(volumeSlider.maximum)
                            font.pixelSize: Theme.fontSizeSmall
                            color: Theme.surfaceVariantText
                            horizontalAlignment: Text.AlignRight
                        }
                    }
                }
            }
        }
    }

    SettingsCard {
        visible: !root.syncPage
        title: I18n.tr("When outputs change", "playback device settings section")

        SettingsToggleRow {
            text: I18n.tr("Pause when the output disconnects", "playback device setting")
            keywords: "bluetooth headphones unplug"
            description: I18n.tr("When off, playback moves to the system default output.", "playback device setting description")
            checked: OutputService.pauseOnDisconnect
            onToggled: value => OutputService.setPauseOnDisconnect(value)
        }

        SettingsToggleRow {
            text: I18n.tr("Use the system default output at startup", "playback device setting")
            checked: PlayerService.resetOutputOnStart
            onToggled: value => PlayerService.setResetOutputOnStart(value)
        }
    }

    // with several outputs, each one's disconnect can pause playback or leave the others playing
    SettingsCard {
        visible: !root.syncPage && root.syncing.length > 1
        unavailableReason: root.syncReason
        title: I18n.tr("When an output disconnects", "playback device settings section")

        Repeater {
            model: root.syncing

            SettingsButtonGroupRow {
                required property var modelData
                text: modelData.name
                model: [I18n.tr("Pause", "transport button"), I18n.tr("Keep playing", "on-disconnect option: the other outputs keep playing")]
                currentIndex: modelData.onDisconnect === "continue" ? 1 : 0
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        OutputService.setOnDisconnect(modelData.id, index === 1 ? "continue" : "pause");
                }
            }
        }
    }

    SettingsCard {
        visible: !root.syncPage
        title: I18n.tr("Multiple outputs", "playback device settings section")
        keywords: "pipewire several"

        Repeater {
            model: root.locals

            SettingsRow {
                id: localRow
                required property var modelData
                readonly property bool selected: PlayerService.selectedOutputs.indexOf(modelData.id) >= 0
                title: modelData.name
                subtitle: !modelData.available ? I18n.tr("Not connected", "playback device state") : !SettingsService.canRoute ? I18n.tr("Requires mpv.", "why a playback setting is unavailable") : PlayerService.canMultiOutput ? "" : I18n.tr("Requires PipeWire (pipewire, pw-cli and pw-play)", "why several outputs and the equalizer are unavailable")
                iconName: UiConfig.deviceIcon(modelData.kind)
                enabled: modelData.available && SettingsService.canRoute && PlayerService.canMultiOutput
                clickable: true
                Accessible.role: Accessible.CheckBox
                Accessible.checked: selected
                onClicked: PlayerService.toggleOutput(modelData.id)

                DIcon {
                    anchors.verticalCenter: parent.verticalCenter
                    name: localRow.selected ? "check_box" : "check_box_outline_blank"
                    filled: localRow.selected
                    size: Theme.iconSize
                    color: localRow.selected ? Theme.primary : Theme.onSurfaceVariant
                }
            }
        }

        SettingsRow {
            title: I18n.tr("Sync", "playback device sync page")
            keywords: "delay latency bluetooth"
            subtitle: root.syncing.length > 1 ? I18n.tr("Adjust the delay of each output", "playback device sync row") : I18n.tr("Select at least two outputs", "why the sync page is disabled")
            iconName: "tune"
            showChevron: true
            clickable: true
            enabled: root.syncing.length > 1
            onClicked: root.syncPage = true
        }
    }

    // the Sync sub-page
    SettingsPageIntro {
        visible: root.syncPage
        text: I18n.tr("Bluetooth outputs play later than wired ones. Delay the others until the test tone is in sync.", "playback device sync intro")
    }

    SettingsCard {
        visible: root.syncPage
        unavailableReason: root.syncReason
        title: I18n.tr("Delay", "playback device sync section")

        Repeater {
            model: root.syncing

            SettingsSliderRow {
                required property var modelData
                text: modelData.name
                minimum: 0
                maximum: PlayerMetrics.outputDelayMax
                step: 10
                unit: I18n.tr(" ms", "unit suffix for milliseconds with a space")
                value: PlayerService.outputDelays[modelData.id] ?? 0
                onSliderDragFinished: finalValue => PlayerService.setOutputDelay(modelData.id, finalValue)
            }
        }

        SettingsRow {
            title: I18n.tr("Test tone", "playback device sync action")
            subtitle: PlayerService.multiOutput ? I18n.tr("Plays a short click on every output.", "playback device sync action description") : I18n.tr("Available when multiple outputs are on", "why the test tone is disabled")

            DButton {
                anchors.verticalCenter: parent.verticalCenter
                text: PlayerService.testTonePlaying ? I18n.tr("Playing…", "test tone state") : I18n.tr("Play", "button that starts playback")
                // the other label's width too, so the button doesn't resize when it switches
                reserveText: PlayerService.testTonePlaying ? I18n.tr("Play", "button that starts playback") : I18n.tr("Playing…", "test tone state")
                iconName: "graphic_eq"
                backgroundColor: Theme.secondaryContainer
                textColor: Theme.onSecondaryContainer
                enabled: !PlayerService.testTonePlaying && PlayerService.multiOutput
                onClicked: PlayerService.playTestTone()
            }
        }
    }
}
