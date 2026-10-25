pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

SettingsPage {
    id: root

    readonly property var settings: PlayerService.settings
    readonly property var replayGainModes: ["off", "track", "album"]
    readonly property var continueModes: ["off", "random", "similar"]
    readonly property var speeds: PlayerMetrics.playbackSpeeds
    readonly property string ffmpegNote: I18n.tr("Requires ffmpeg to measure the silence.", "why the skip silence settings are unavailable")
    readonly property string eqNote: {
        switch (PlayerService.equalizerState) {
        case "casting":
            return I18n.tr("The equalizer doesn't apply while casting", "equalizer state");
        case "unavailable":
            return PlayerService.devicesError !== "" ? I18n.tr("The equalizer couldn't start: %1", "equalizer state, %1 is the daemon's reason").arg(PlayerService.devicesError) : I18n.tr("Requires PipeWire (pipewire, pw-cli and pw-play)", "why several outputs and the equalizer are unavailable");
        }
        return "";
    }

    function presetLabel(id) {
        switch (id) {
        case "flat":
            return I18n.tr("Flat", "equalizer preset");
        case "bass":
            return I18n.tr("Bass boost", "equalizer preset");
        case "treble":
            return I18n.tr("Treble boost", "equalizer preset");
        case "vocal":
            return I18n.tr("Vocal", "equalizer preset");
        case "rock":
            return I18n.tr("Rock", "equalizer preset");
        case "jazz":
            return I18n.tr("Jazz", "equalizer preset");
        case "classical":
            return I18n.tr("Classical", "equalizer preset");
        case "electronic":
            return I18n.tr("Electronic", "equalizer preset");
        case "pop":
            return I18n.tr("Pop", "equalizer preset");
        }
        return id;
    }

    SettingsCard {
        title: I18n.tr("Transitions", "playback settings section label")

        SettingsToggleRow {
            text: I18n.tr("Gapless playback", "playback setting")
            checked: root.settings.gapless
            onToggled: value => PlayerService.setSetting("gapless", value)
        }

        SettingsToggleRow {
            text: I18n.tr("Crossfade", "playback setting")
            checked: root.settings.crossfade.enabled
            onToggled: value => PlayerService.setSetting("crossfade", {
                    "enabled": value
                })
        }

        SettingsSliderRow {
            text: I18n.tr("Crossfade duration", "playback setting")
            enabled: root.settings.crossfade.enabled
            minimum: 1
            maximum: 12
            unit: I18n.tr("s", "unit suffix for seconds")
            value: root.settings.crossfade.seconds
            onSliderDragFinished: finalValue => PlayerService.setSetting("crossfade", {
                    "seconds": finalValue
                })
        }

        SettingsToggleRow {
            text: I18n.tr("Skip silence at the start", "playback setting")
            description: SettingsService.ffmpeg ? "" : root.ffmpegNote
            enabled: SettingsService.ffmpeg
            checked: root.settings.skipSilenceStart
            onToggled: value => PlayerService.setSetting("skipSilenceStart", value)
        }

        SettingsToggleRow {
            text: I18n.tr("Skip silence at the end", "playback setting")
            description: SettingsService.ffmpeg ? "" : root.ffmpegNote
            enabled: SettingsService.ffmpeg
            checked: root.settings.skipSilenceEnd
            onToggled: value => PlayerService.setSetting("skipSilenceEnd", value)
        }

        // also keeps silence between an album's tracks, so it applies with crossfade off
        SettingsToggleRow {
            text: I18n.tr("Keep albums gapless", "playback setting")
            description: I18n.tr("No crossfade or skipped silence between tracks of one album.", "playback setting description")
            checked: root.settings.crossfade.albumGapless
            onToggled: value => PlayerService.setSetting("crossfade", {
                    "albumGapless": value
                })
        }
    }

    SettingsCard {
        title: I18n.tr("Volume", "audio volume: settings section, slider")

        SettingsButtonGroupRow {
            text: I18n.tr("Normalize volume", "playback setting: ReplayGain mode")
            description: I18n.tr("Uses the ReplayGain values in the files.", "playback setting description")
            model: [I18n.tr("Off", "option that turns something off"), I18n.tr("Track", "replaygain mode"), I18n.tr("Album", "an album: page label, list column, ReplayGain mode")]
            currentIndex: root.replayGainModes.indexOf(root.settings.replayGain)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    PlayerService.setSetting("replayGain", root.replayGainModes[index]);
            }
        }

        SettingsButtonGroupRow {
            text: I18n.tr("Volume controls", "playback setting: what the volume slider changes")
            description: SettingsService.systemVolume ? I18n.tr("System volume changes the volume of the default output.", "playback setting description") : I18n.tr("System volume requires wpctl (WirePlumber).", "why the system volume option is unavailable")
            enabled: SettingsService.systemVolume || root.settings.volumeTarget === "system"
            model: [I18n.tr("App volume", "volume controls option"), I18n.tr("System volume", "volume controls option")]
            currentIndex: root.settings.volumeTarget === "system" ? 1 : 0
            onSelectionChanged: (index, selected) => {
                if (selected)
                    PlayerService.setSetting("volumeTarget", index === 1 ? "system" : "app");
            }
        }

        SettingsToggleRow {
            text: I18n.tr("Allow volume above 100%", "playback setting")
            description: I18n.tr("Up to 130%. Loud tracks may distort.", "playback setting description")
            checked: root.settings.allowVolumeBoost
            onToggled: value => PlayerService.setSetting("allowVolumeBoost", value)
        }
    }

    SettingsCard {
        title: I18n.tr("Equalizer", "playback settings section label")

        SettingsToggleRow {
            text: I18n.tr("Use the equalizer", "equalizer switch")
            description: root.eqNote
            descriptionColor: PlayerService.equalizerState === "unavailable" ? Theme.error : Theme.surfaceVariantText
            checked: root.settings.equalizer.enabled
            onToggled: value => PlayerService.setSetting("equalizer", {
                    "enabled": value
                })
        }

        SettingsRow {
            id: presetRow
            title: I18n.tr("Preset", "equalizer setting")
            enabled: root.settings.equalizer.enabled

            DDropdown {
                id: presetDropdown
                enabled: presetRow.enabled
                dropdownWidth: SettingsMetrics.dropdownWidth
                options: PlayerService.eqPresetIds.map(id => root.presetLabel(id)).concat([I18n.tr("Custom", "option for your own values")])
                optionValues: PlayerService.eqPresetIds.concat(["custom"])
                onValueChanged: value => {
                    if (value !== "custom")
                        PlayerService.setEqualizerPreset(value);
                }
            }

            Binding {
                target: presetDropdown
                property: "currentValue"
                value: root.settings.equalizer.preset
            }
        }

        // one row per band; DCommon has no vertical slider for the usual fader look
        SettingsRow {
            enabled: root.settings.equalizer.enabled
            title: I18n.tr("Bands", "equalizer setting")
            subtitle: I18n.tr("Changing a band switches the preset to Custom.", "equalizer setting description")

            body: Column {
                width: parent.width
                spacing: Theme.spacingXS

                Repeater {
                    model: PlayerService.eqBands

                    Row {
                        id: band
                        required property int modelData
                        required property int index
                        readonly property string label: modelData >= 1000 ? I18n.tr("%1 kHz", "equalizer band").arg(modelData / 1000) : I18n.tr("%1 Hz", "equalizer band").arg(modelData)
                        width: parent.width
                        spacing: Theme.spacingM

                        StyledText {
                            anchors.verticalCenter: parent.verticalCenter
                            width: PlayerMetrics.eqBandLabelWidth
                            text: band.label
                            font.pixelSize: Theme.fontSizeSmall
                            color: Theme.surfaceVariantText
                        }

                        DSlider {
                            id: bandSlider
                            anchors.verticalCenter: parent.verticalCenter
                            width: parent.width - PlayerMetrics.eqBandLabelWidth - parent.spacing
                            size: "s"
                            minimum: -12
                            maximum: 12
                            unit: I18n.tr(" dB", "unit suffix for decibels")
                            enabled: root.settings.equalizer.enabled
                            wheelEnabled: false
                            Accessible.name: band.label
                            // the daemon applies changes live, so the band is heard while dragging
                            onSliderValueChanged: newValue => PlayerService.setEqualizerBand(band.index, newValue)

                            Binding on value {
                                value: root.settings.equalizer.bands[band.index]
                                when: !bandSlider.isDragging
                                restoreMode: Binding.RestoreNone
                            }
                        }
                    }
                }
            }
        }
    }

    SettingsCard {
        title: I18n.tr("Speed and queue", "playback settings section label")

        SettingsButtonGroupRow {
            text: I18n.tr("Playback speed", "now playing button")
            model: root.speeds.map(s => I18n.tr("%1×", "playback speed option").arg(s))
            currentIndex: root.speeds.indexOf(root.settings.speed)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    PlayerService.setSetting("speed", root.speeds[index]);
            }
        }

        SettingsButtonGroupRow {
            text: I18n.tr("When the queue ends", "playback setting")
            description: I18n.tr("Similar picks tracks by the same artist or genre.", "playback setting description")
            model: [I18n.tr("Stop", "auto-continue option"), I18n.tr("Random", "auto-continue option"), I18n.tr("Similar", "auto-continue option")]
            currentIndex: root.continueModes.indexOf(root.settings.autoContinue)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    PlayerService.setSetting("autoContinue", root.continueModes[index]);
            }
        }

        SettingsSliderRow {
            text: I18n.tr("Previous restarts the track after", "playback setting")
            description: I18n.tr("Before this point, Previous goes to the previous track.", "playback setting description")
            minimum: 0
            maximum: 30
            unit: I18n.tr("s", "unit suffix for seconds")
            value: root.settings.previousThreshold
            onSliderDragFinished: finalValue => PlayerService.setSetting("previousThreshold", finalValue)
        }
    }
}
