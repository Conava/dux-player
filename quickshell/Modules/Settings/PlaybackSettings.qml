pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Modules
import qs.Widgets

SettingsPage {
    id: root

    readonly property var settings: PlayerService.settings
    readonly property var replayGainModes: ["off", "track", "album"]
    readonly property var continueModes: ["off", "random", "similar"]
    readonly property var playNextOrders: ["added", "newest"]
    readonly property var shuffleModes: ["tracks", "albums"]
    readonly property var historyOptionMonths: [1, 3, 6, 12, 60, 0]

    // 0 is Forever; any other stored value shows the closest listed number of months
    function nearestHistoryMonths(months) {
        if (months <= 0)
            return 0;
        let best = historyOptionMonths[0];
        for (const m of historyOptionMonths) {
            if (m > 0 && Math.abs(m - months) < Math.abs(best - months))
                best = m;
        }
        return best;
    }

    // skip changes the daemon hasn't answered yet, so quick ticks in a sheet build on each other
    property var skipGenres: []
    property var skipLibraries: []
    // chosen values the catalog no longer lists (a library that is off or gone) stay in the sheets, so they can be unticked
    readonly property var genreItems: {
        const names = PlayerService.genres.map(g => g.name);
        for (const g of skipGenres) {
            if (names.indexOf(g) < 0)
                names.push(g);
        }
        return names.sort((a, b) => a.localeCompare(b)).map(name => ({
                    "label": name,
                    "value": name
                }));
    }
    readonly property var libraryItems: {
        const items = CatalogService.libraries.map(l => ({
                    "label": l.name,
                    "value": l.id
                }));
        for (const id of skipLibraries) {
            if (!items.some(i => i.value === id))
                items.push({
                    "label": PlayerService.libraryName(id) || id,
                    "value": id
                });
        }
        return items;
    }
    readonly property var speeds: PlayerMetrics.playbackSpeeds
    readonly property string ffmpegNote: I18n.tr("Requires ffmpeg to measure the silence.", "why the skip silence settings are unavailable")
    readonly property string mpvNote: I18n.tr("Requires mpv.", "why a playback setting is unavailable")
    readonly property bool eqUsable: SettingsService.canRoute && settings.equalizer.enabled
    readonly property string eqNote: {
        switch (PlayerService.equalizerState) {
        case "casting":
            return I18n.tr("The equalizer doesn't apply while casting", "equalizer state");
        case "unavailable":
            return PlayerService.devicesError !== "" ? I18n.tr("The equalizer couldn't start: %1", "equalizer state, %1 is the daemon's reason").arg(PlayerService.devicesError) : I18n.tr("Requires PipeWire (pipewire, pw-cli and pw-play)", "why several outputs and the equalizer are unavailable");
        }
        return "";
    }

    function syncSkipRules() {
        skipGenres = settings.skipRules?.genres ?? [];
        skipLibraries = settings.skipRules?.libraries ?? [];
    }

    onSettingsChanged: syncSkipRules()
    Component.onCompleted: syncSkipRules()

    function setSkipRule(patch) {
        if (patch.genres)
            skipGenres = patch.genres;
        if (patch.libraries)
            skipLibraries = patch.libraries;
        PlayerService.setSetting("skipRules", Object.assign({}, settings.skipRules, {
            "genres": skipGenres,
            "libraries": skipLibraries
        }, patch));
    }

    function toggleSkipLibrary(id) {
        setSkipRule({
            "libraries": skipLibraries.indexOf(id) >= 0 ? skipLibraries.filter(l => l !== id) : skipLibraries.concat([id])
        });
    }

    function toggleSkipGenre(genre) {
        setSkipRule({
            "genres": skipGenres.indexOf(genre) >= 0 ? skipGenres.filter(g => g !== genre) : skipGenres.concat([genre])
        });
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

        // without mpv the rows show what plays (off, 1×) and keep the saved choice for when mpv is back
        SettingsToggleRow {
            text: I18n.tr("Crossfade", "playback setting")
            keywords: "fade"
            description: SettingsService.canCrossfade ? "" : root.mpvNote
            enabled: SettingsService.canCrossfade
            checked: SettingsService.canCrossfade && root.settings.crossfade.enabled
            onToggled: value => PlayerService.setSetting("crossfade", {
                    "enabled": value
                })
        }

        SettingsSliderRow {
            text: I18n.tr("Crossfade duration", "playback setting")
            keywords: "fade"
            description: SettingsService.canCrossfade ? "" : root.mpvNote
            enabled: SettingsService.canCrossfade && root.settings.crossfade.enabled
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
            keywords: "trim"
            description: SettingsService.ffmpeg ? "" : root.ffmpegNote
            enabled: SettingsService.ffmpeg
            checked: root.settings.skipSilenceStart
            onToggled: value => PlayerService.setSetting("skipSilenceStart", value)
        }

        SettingsToggleRow {
            text: I18n.tr("Skip silence at the end", "playback setting")
            keywords: "trim"
            description: SettingsService.ffmpeg ? "" : root.ffmpegNote
            enabled: SettingsService.ffmpeg
            checked: root.settings.skipSilenceEnd
            onToggled: value => PlayerService.setSetting("skipSilenceEnd", value)
        }

        // also keeps silence between an album's tracks, so it applies with crossfade off
        SettingsToggleRow {
            text: I18n.tr("Keep albums gapless", "playback setting")
            keywords: "crossfade"
            description: I18n.tr("No crossfade or silence skipping within an album.", "playback setting description")
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
            keywords: "replaygain loudness"
            description: I18n.tr("Uses ReplayGain tags.", "playback setting description")
            model: [I18n.tr("Off", "option that turns something off"), I18n.tr("Track", "replaygain mode"), I18n.tr("Album", "an album: page label, list column, ReplayGain mode")]
            currentIndex: root.replayGainModes.indexOf(root.settings.replayGain)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    PlayerService.setSetting("replayGain", root.replayGainModes[index]);
            }
        }

        SettingsButtonGroupRow {
            text: I18n.tr("Volume controls", "playback setting: what the volume slider changes")
            keywords: "system"
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
            keywords: "boost amplify"
            description: I18n.tr("Up to 130%. Loud tracks may distort.", "playback setting description")
            checked: root.settings.allowVolumeBoost
            onToggled: value => PlayerService.setSetting("allowVolumeBoost", value)
        }
    }

    SettingsCard {
        title: I18n.tr("Equalizer", "playback settings section label")

        SettingsToggleRow {
            text: I18n.tr("Use the equalizer", "equalizer switch")
            keywords: "eq"
            description: SettingsService.canRoute ? root.eqNote : root.mpvNote
            enabled: SettingsService.canRoute
            descriptionColor: PlayerService.equalizerState === "unavailable" ? Theme.error : Theme.surfaceVariantText
            checked: root.eqUsable
            onToggled: value => PlayerService.setSetting("equalizer", {
                    "enabled": value
                })
        }

        SettingsRow {
            id: presetRow
            title: I18n.tr("Preset", "equalizer setting")
            keywords: "eq"
            enabled: root.eqUsable

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
            enabled: root.eqUsable
            title: I18n.tr("Bands", "equalizer setting")
            keywords: "eq bass treble"
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
                            width: parent.width - PlayerMetrics.eqBandLabelWidth - gain.width - parent.spacing * 2
                            size: "s"
                            minimum: -12
                            maximum: 12
                            unit: I18n.tr(" dB", "unit suffix for decibels")
                            enabled: root.eqUsable
                            wheelEnabled: false
                            showValue: false
                            Accessible.name: band.label
                            // the daemon applies changes live, so the band is heard while dragging
                            onSliderValueChanged: newValue => PlayerService.setEqualizerBand(band.index, newValue)

                            Binding on value {
                                value: root.settings.equalizer.bands[band.index]
                                when: !bandSlider.isDragging
                                restoreMode: Binding.RestoreNone
                            }
                        }

                        NumericText {
                            id: gain
                            anchors.verticalCenter: parent.verticalCenter
                            width: Math.ceil(reservedWidth)
                            isMonospace: false
                            text: (bandSlider.value > 0 ? "+" : "") + bandSlider.formatValue(bandSlider.value)
                            reserveText: "+" + bandSlider.formatValue(bandSlider.maximum)
                            font.pixelSize: Theme.fontSizeSmall
                            color: Theme.surfaceVariantText
                            opacity: bandSlider.enabled ? 1 : Theme.disabledOpacity
                            horizontalAlignment: Text.AlignRight
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
            keywords: "tempo"
            description: SettingsService.canChangeSpeed ? "" : root.mpvNote
            enabled: SettingsService.canChangeSpeed
            model: root.speeds.map(s => I18n.tr("%1×", "playback speed option").arg(s))
            currentIndex: root.speeds.indexOf(PlayerService.effectiveSpeed)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    PlayerService.setSetting("speed", root.speeds[index]);
            }
        }

        SettingsButtonGroupRow {
            text: I18n.tr("When the queue ends", "playback setting")
            keywords: "autoplay repeat stop"
            description: I18n.tr("Similar picks tracks by the same artist or genre.", "playback setting description")
            model: [I18n.tr("Stop", "auto-continue option"), I18n.tr("Random", "auto-continue option"), I18n.tr("Similar", "auto-continue option")]
            currentIndex: root.continueModes.indexOf(root.settings.autoContinue)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    PlayerService.setSetting("autoContinue", root.continueModes[index]);
            }
        }

        SettingsButtonGroupRow {
            text: I18n.tr("Play next order", "playback setting")
            keywords: "queue add newest"
            model: [I18n.tr("In the order you add them", "play next order option"), I18n.tr("Newest first", "play next order option")]
            currentIndex: root.playNextOrders.indexOf(root.settings.playNextOrder)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    PlayerService.setSetting("playNextOrder", root.playNextOrders[index]);
            }
        }

        SettingsButtonGroupRow {
            text: I18n.tr("Shuffle by", "playback setting")
            keywords: "random album"
            description: I18n.tr("Albums mixes the order of whole albums instead of single tracks.", "playback setting description")
            model: [I18n.tr("Tracks", "navigation destination"), I18n.tr("Albums", "navigation destination")]
            currentIndex: root.shuffleModes.indexOf(root.settings.shuffleMode)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    PlayerService.setSetting("shuffleMode", root.shuffleModes[index]);
            }
        }

        SettingsSliderRow {
            text: I18n.tr("Previous restarts the track after", "playback setting")
            keywords: "back rewind"
            description: I18n.tr("Before this point, Previous goes to the previous track.", "playback setting description")
            minimum: 0
            maximum: 30
            unit: I18n.tr("s", "unit suffix for seconds")
            value: root.settings.previousThreshold
            onSliderDragFinished: finalValue => PlayerService.setSetting("previousThreshold", finalValue)
        }
    }

    SettingsCard {
        title: I18n.tr("Automatic picks", "playback settings section label")

        SettingsRow {
            title: I18n.tr("Skip tracks rated below", "playback setting")
            keywords: "stars rating exclude"

            DRatingBar {
                anchors.verticalCenter: parent?.verticalCenter
                rating: root.settings.skipRules?.minRating ?? 0
                onRated: stars => root.setSkipRule({
                        "minRating": stars
                    })
            }
        }

        SettingsRow {
            title: I18n.tr("Skip genres", "playback setting")
            subtitle: root.skipGenres.join(", ")
            keywords: "exclude"
            clickable: true
            showChevron: true
            onClicked: genreSheet.opened = true
        }

        SettingsRow {
            visible: CatalogService.libraries.length > 1 || root.skipLibraries.length > 0
            title: I18n.tr("Skip libraries", "playback setting")
            subtitle: root.libraryItems.filter(i => root.skipLibraries.indexOf(i.value) >= 0).map(i => i.label).join(", ")
            keywords: "exclude"
            clickable: true
            showChevron: true
            onClicked: librarySheet.opened = true
        }

        SettingsSliderRow {
            text: I18n.tr("Skip tracks shorter than", "playback setting")
            keywords: "exclude length interlude"
            minimum: 0
            maximum: 300
            unit: I18n.tr("s", "unit suffix for seconds")
            value: root.settings.skipRules?.shorterThan ?? 0
            onSliderDragFinished: finalValue => root.setSkipRule({
                    "shorterThan": finalValue
                })
        }
    }

    SettingsPageIntro {
        text: I18n.tr("Applies to Shuffle all and to playback after the queue ends.", "playback settings section description")
    }

    SettingsCard {
        title: I18n.tr("History", "navigation destination")

        SettingsRow {
            title: I18n.tr("Keep play history", "playback setting")
            keywords: "history plays forget delete"

            DDropdown {
                id: historyDropdown
                dropdownWidth: SettingsMetrics.dropdownWidth
                options: [I18n.tr("1 month", "play history duration"), I18n.tr("3 months", "play history duration"), I18n.tr("6 months", "play history duration"), I18n.tr("1 year", "play history duration"), I18n.tr("5 years", "play history duration"), I18n.tr("Forever", "play history duration")]
                // DDropdown's currentValue is a string
                optionValues: root.historyOptionMonths.map(m => String(m))
                onValueChanged: value => SettingsService.setSetting("historyMonths", parseInt(value))
            }

            Binding {
                target: historyDropdown
                property: "currentValue"
                value: String(root.nearestHistoryMonths(SettingsService.settings.historyMonths))
            }
        }
    }

    PickerSheet {
        id: genreSheet
        // over the page, not in its scrolling column
        parent: root.parent
        title: I18n.tr("Skip genres", "playback setting")
        sections: [
            {
                "title": I18n.tr("Genres", "navigation destination"),
                "items": root.genreItems
            }
        ]
        multiple: true
        chosen: root.skipGenres
        onPicked: value => root.toggleSkipGenre(value)
    }

    PickerSheet {
        id: librarySheet
        parent: root.parent
        title: I18n.tr("Skip libraries", "playback setting")
        sections: [
            {
                "title": I18n.tr("Libraries", "navigation destination"),
                "items": root.libraryItems
            }
        ]
        multiple: true
        chosen: root.skipLibraries
        onPicked: value => root.toggleSkipLibrary(value)
    }
}
