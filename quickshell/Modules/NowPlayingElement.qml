pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Common as DC
import qs.DCommon.Widgets
import qs.Widgets
import "../Common/Format.js" as Format

// One element of a now-playing surface by type (UiConfig.elementLabel lists them), in the look of the shell's
// media chromes. Colors come from ArtColors. Track info elements sit in a NowPlayingZone's text stack.
Loader {
    id: root

    required property string elementId
    property var options: ({})
    // "bar" | "side" | "view": where lyrics and the queue open (Router.runElement)
    property string context: "bar"
    property string size: "s"
    // "plain" | "transport" (Bento's tonal buttons) | "m3e" (the M3E player's transport) | "outlined" (the M3E
    // player's button group) | "row" (Bento's device row: volume as a ring, the device as a chip)
    property string look: "plain"
    // track info: "bar" | "side" | "bento" | "m3e"
    property string textLook: "bar"
    property bool centered: false
    // in a column (a title card): dividers run across, spacers go down
    property bool inColumn: false
    // the transport fills its row, the play button wider (Bento)
    property bool fill: false
    property real weight: 1
    // the spring-animated weight of a filling transport button
    property real liveWeight: weight

    readonly property var track: PlayerService.current
    readonly property bool transportId: ["shuffle", "previous", "playPause", "next", "repeat"].indexOf(elementId) >= 0
    readonly property real buttonSize: size === "m" ? Theme.buttonHeightM : Theme.buttonHeightS
    // the volume is a button showing the level (Bento's ring, M3E's fill); scroll changes it, a click opens the slider
    readonly property Component volumeComponent: UiConfig.nowPlaying.style === "m3e" ? volumeLevel : volumeRing
    readonly property real spacerSize: {
        switch (options.size ?? "m") {
        case "s":
            return Theme.spacingS;
        case "l":
            return Theme.spacingXL;
        }
        return Theme.spacingL;
    }

    function alignedX(itemWidth, total) {
        if (centered)
            return (total - itemWidth) / 2;
        return I18n.isRtl ? total - itemWidth : 0;
    }

    // what the zone lays out by: a row's layout reads these, since the element sits in a wrapper
    readonly property bool fills: fill && (sourceComponent === button || sourceComponent === playPause)
    readonly property bool fillsWidth: elementId === "seekbar" || fills || elementId === "spacer" && options.size === "fill"
    readonly property real preferredWidth: fills ? Theme.minimumTouchTargetSize * liveWeight : -1
    readonly property real minimumWidth: elementId === "seekbar" ? PlayerMetrics.barSeekMinWidth : fills ? Theme.iconSizeMedium + Theme.spacingXS * 2 : -1
    // the zone decides (NowPlayingZone.shows): a cast target only while casting, an album only when there is one
    property bool shown: true
    // the row's width, for the device chip's share of it
    property real rowWidth: 0

    visible: shown

    sourceComponent: {
        switch (elementId) {
        case "cover":
            return cover;
        case "title":
            return title;
        case "artists":
        case "album":
            return subtitle;
        case "libraryBadge":
            return badge;
        case "playingFrom":
            return textLook === "bento" || textLook === "side" ? playingFromChip : playingFromText;
        case "castTarget":
            return castTarget;
        case "spacer":
            return spacer;
        case "divider":
            return divider;
        case "favorite":
            return favorite;
        case "rating":
            return rating;
        case "elapsed":
        case "duration":
            return time;
        case "seekbar":
            return seekbar;
        case "volume":
            return volumeComponent;
        case "device":
            return look === "row" ? deviceChip : device;
        case "playPause":
            return playPause;
        }
        return button;
    }

    // the shell's transport press: the pressed button widens a little on a spring
    component Springy: DIconButton {
        id: springy
        readonly property real targetWeight: root.weight * (pressed ? PlayerMetrics.mediaTransportPressScale : 1)
        // Bento's buttons outside a filled row: squares, play wider
        width: root.look === "transport" && !root.fills ? Math.round(buttonSize * root.weight) : iconSize + horizontalInset * 2
        onTargetWeightChanged: spring.retarget(targetWeight)

        Binding {
            target: root
            property: "liveWeight"
            value: spring.value
            when: root.fills
        }

        DC.SpringMotion {
            id: spring
            readonly property var preset: Theme.springPreset("fast", Theme.expressiveDurations.expressiveFastSpatial)
            enabled: root.fills && springy.visible && !Theme.springMotionDisabled
            stiffness: preset.stiffness
            damping: preset.damping
            Component.onCompleted: snapTo(springy.targetWeight)
        }
    }

    // a small cover outside the full view's cover zone; in the bar and side panel it opens the full view
    Component {
        id: cover
        Item {
            implicitWidth: PlayerMetrics.barArtSize
            implicitHeight: PlayerMetrics.barArtSize

            StyledButton {
                x: root.alignedX(width, parent.width)
                width: PlayerMetrics.barArtSize
                height: width
                radius: Theme.cornerRadiusM
                color: "transparent"
                enabled: root.context !== "view"
                Accessible.name: I18n.tr("Open now playing", "now playing bar button")
                onClicked: Router.openNowPlaying("")

                AlbumArt {
                    anchors.fill: parent
                    item: root.track
                }
            }
        }
    }

    Component {
        id: title
        Item {
            implicitWidth: titleLoader.implicitWidth
            implicitHeight: titleLoader.implicitHeight

            Loader {
                id: titleLoader
                width: parent.width
                sourceComponent: {
                    switch (root.textLook) {
                    case "side":
                        return sideTitle;
                    case "bento":
                        return bentoTitle;
                    case "m3e":
                        return m3eTitle;
                    }
                    return barTitle;
                }
            }

            Component {
                id: barTitle
                DScrollingText {
                    text: root.track?.title ?? ""
                    font.pixelSize: Theme.fontSizeMedium
                    font.weight: Theme.fontWeightMedium
                    color: ArtColors.text
                    horizontalAlignment: root.centered ? Text.AlignHCenter : Text.AlignLeft
                    active: PlayerService.playing
                    fadeEdges: true
                    loop: true
                    pxPerMs: PlayerMetrics.mediaTextScrollSpeed / 1000
                    stepInterval: PlayerMetrics.mediaTextScrollStep
                }
            }

            Component {
                id: sideTitle
                MediaTitle {
                    fullPixelSize: Theme.fontSizeXXLarge
                    minimumPixelSize: Theme.fontSizeXLarge
                    text: root.track?.title ?? ""
                    horizontalAlignment: root.centered ? Text.AlignHCenter : Text.AlignLeft
                    active: PlayerService.playing
                }
            }

            Component {
                id: bentoTitle
                MediaTitle {
                    text: root.track?.title ?? ""
                    horizontalAlignment: root.centered ? Text.AlignHCenter : Text.AlignLeft
                    active: PlayerService.playing && root.visible
                }
            }

            Component {
                id: m3eTitle
                StyledText {
                    text: root.track?.title ?? ""
                    color: ArtColors.text
                    font.pixelSize: Theme.fontSizeXXLarge
                    elide: Text.ElideRight
                    wrapMode: Text.WrapAtWordBoundaryOrAnywhere
                    maximumLineCount: 2
                    horizontalAlignment: root.centered ? Text.AlignHCenter : Text.AlignLeft
                }
            }
        }
    }

    Component {
        id: subtitle
        MediaSubtitle {
            track: root.track
            line: root.elementId
            centered: root.centered
            fontSize: root.textLook === "m3e" && root.elementId === "artists" ? Theme.fontSizeMedium : Theme.fontSizeSmall
        }
    }

    Component {
        id: badge
        Item {
            implicitWidth: libraryBadge.implicitWidth
            implicitHeight: libraryBadge.implicitHeight

            LibraryBadge {
                id: libraryBadge
                x: root.alignedX(width, parent.width)
                libraryId: root.track?.library ?? ""
                maximumWidth: parent.width
            }
        }
    }

    // Bento's "Playing from" chip, which toggles the queue
    Component {
        id: playingFromChip
        Item {
            implicitWidth: chipRow.implicitWidth
            implicitHeight: Theme.buttonHeightXS

            Row {
                id: chipRow
                x: root.alignedX(Math.min(implicitWidth, parent.width), parent.width)
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.spacingS

                StyledText {
                    id: sourceLabel
                    anchors.verticalCenter: parent.verticalCenter
                    width: Math.min(implicitWidth, chipRow.parent.width / 2)
                    text: I18n.tr("Playing from", "now playing: label before where the queue came from")
                    color: ArtColors.textVariant
                    font.pixelSize: Theme.fontSizeSmall
                    wrapMode: Text.NoWrap
                    elide: Text.ElideRight
                }

                DButton {
                    maximumWidth: Math.max(0, chipRow.parent.width - sourceLabel.width - chipRow.spacing)
                    buttonHeight: Theme.buttonHeightXS
                    horizontalPadding: Theme.spacingM
                    text: UiConfig.contextLabel(PlayerService.playContext)
                    backgroundColor: ArtColors.accentContainer
                    textColor: ArtColors.onAccentContainer
                    tooltipText: I18n.tr("Queue", "the play queue: panel, toggle")
                    Accessible.name: I18n.tr("Playing from", "now playing: label before where the queue came from") + " " + text
                    onClicked: Router.runElement("playingFrom", root.context)
                }
            }
        }
    }

    Component {
        id: playingFromText
        StyledText {
            text: UiConfig.contextLabel(PlayerService.playContext)
            color: ArtColors.textVariant
            font.pixelSize: root.textLook === "m3e" ? Theme.fontSizeMedium : Theme.fontSizeSmall
            font.weight: root.textLook === "m3e" ? Theme.fontWeightMedium : Font.Normal
            wrapMode: Text.NoWrap
            elide: Text.ElideRight
            horizontalAlignment: root.centered ? Text.AlignHCenter : Text.AlignLeft
        }
    }

    Component {
        id: castTarget
        StyledText {
            text: I18n.tr("Playing on %1", "playback device button tooltip").arg(PlayerService.castDeviceName)
            font.pixelSize: Theme.fontSizeSmall
            color: ArtColors.accent
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
            horizontalAlignment: root.centered ? Text.AlignHCenter : Text.AlignLeft
        }
    }

    Component {
        id: spacer
        Item {
            implicitWidth: root.inColumn ? 0 : root.spacerSize
            implicitHeight: root.inColumn ? root.spacerSize : 0
        }
    }

    Component {
        id: divider
        Item {
            implicitWidth: root.inColumn ? 0 : Theme.dividerWidth + Theme.spacingS * 2
            implicitHeight: root.inColumn ? Theme.dividerWidth + Theme.spacingS * 2 : Theme.iconSize

            Rectangle {
                anchors.centerIn: parent
                width: root.inColumn ? parent.width : Theme.dividerWidth
                height: root.inColumn ? Theme.dividerWidth : parent.height
                color: Theme.outlineMedium
            }
        }
    }

    Component {
        id: favorite
        FavoriteButton {
            kind: "track"
            item: root.track
            buttonSize: root.buttonSize
            activeColor: ArtColors.accent
            inactiveColor: ArtColors.textVariant
        }
    }

    Component {
        id: rating
        RatingStars {
            kind: "track"
            item: root.track
            starColor: ArtColors.accent
        }
    }

    Component {
        id: time
        NumericText {
            id: timeText
            text: Format.duration(root.elementId === "elapsed" ? PlayerService.position : PlayerService.duration)
            reserveText: Format.duration(PlayerService.duration).replace(/\d/g, "8")
            width: Math.ceil(reservedWidth)
            isMonospace: false
            font.pixelSize: Theme.fontSizeSmall
            font.weight: Theme.fontWeightMedium
            color: ArtColors.textVariant
            horizontalAlignment: root.elementId === "elapsed" ? Text.AlignLeft : Text.AlignRight

            PositionHold {
                active: root.elementId === "elapsed" && timeText.visible && timeText.Window.visibility !== Window.Minimized
            }
        }
    }

    Component {
        id: seekbar
        MediaSeekbar {}
    }

    // Bento's volume: a ring showing the level around a button opening the volume panel
    Component {
        id: volumeRing
        DRingGauge {
            id: ring
            width: ringButton.buttonSize + (strokeWidth + trackGap) * 2
            height: width
            value: PlayerService.muted ? 0 : PlayerService.volume / Math.max(0.01, PlayerService.volumeMax)
            startAngle: 135
            spanAngle: 270
            strokeWidth: Theme.outlineWidthFocused + Theme.dividerWidth
            trackGap: Theme.spacingXXS
            ringColor: ArtColors.accent
            trackColor: ArtColors.accentTrack
            animated: ring.visible

            DActionButton {
                id: ringButton
                anchors.centerIn: parent
                buttonSize: Theme.buttonHeightXS
                radius: Theme.fullRadius(width, height)
                iconName: PlayerService.muted || PlayerService.volume === 0 ? "volume_off" : PlayerService.volume < 0.5 ? "volume_down" : "volume_up"
                iconColor: PlayerService.muted ? ArtColors.textVariant : ArtColors.text
                tooltipText: I18n.tr("Volume", "audio volume: settings section, slider") + ": " + Math.round(PlayerService.volume * 100) + "%"
                Accessible.name: tooltipText
                onClicked: panel.show()

                MouseArea {
                    anchors.fill: parent
                    acceptedButtons: Qt.NoButton
                    onWheel: wheel => {
                        if (wheel.angleDelta.y === 0)
                            return;
                        wheel.accepted = true;
                        PlayerService.setVolume(PlayerService.volume + (wheel.angleDelta.y > 0 ? 0.05 : -0.05));
                    }
                }

                VolumePanel {
                    id: panel
                    anchorItem: ringButton
                }
            }
        }
    }

    // M3E's volume: a tonal button filling up from the bottom with the level
    Component {
        id: volumeLevel
        StyledButton {
            id: levelButton
            readonly property real level: PlayerService.muted ? 0 : PlayerService.volume / Math.max(0.01, PlayerService.volumeMax)
            width: root.buttonSize
            height: width
            radius: Theme.fullRadius(width, height)
            color: "transparent"
            Accessible.name: I18n.tr("Volume", "audio volume: settings section, slider") + ": " + Math.round(PlayerService.volume * 100) + "%"
            onClicked: levelPanel.show()

            Rectangle {
                anchors.fill: parent
                radius: parent.radius
                color: ArtColors.accentSecondaryContainer
            }

            // the level: the same circle, cut off at its height
            Item {
                anchors.bottom: parent.bottom
                width: parent.width
                height: parent.height * levelButton.level
                clip: true

                Behavior on height {
                    enabled: Theme.animationsEnabled
                    NumberAnimation {
                        duration: Theme.shortDuration
                    }
                }

                Rectangle {
                    anchors.bottom: parent.bottom
                    width: levelButton.width
                    height: levelButton.height
                    radius: levelButton.radius
                    color: ArtColors.accentContainer
                }
            }

            StateLayer {
                control: levelButton
                stateColor: ArtColors.onAccentContainer
            }

            DIcon {
                anchors.centerIn: parent
                name: PlayerService.muted || PlayerService.volume === 0 ? "volume_off" : PlayerService.volume < 0.5 ? "volume_down" : "volume_up"
                size: Theme.iconSize
                color: levelButton.level > 0.5 ? ArtColors.onAccentContainer : ArtColors.onAccentSecondaryContainer
            }

            MouseArea {
                anchors.fill: parent
                acceptedButtons: Qt.NoButton
                onWheel: wheel => {
                    if (wheel.angleDelta.y === 0)
                        return;
                    wheel.accepted = true;
                    PlayerService.setVolume(PlayerService.volume + (wheel.angleDelta.y > 0 ? 0.05 : -0.05));
                }
            }

            VolumePanel {
                id: levelPanel
                anchorItem: levelButton
            }
        }
    }

    // Bento's output chip: the device's icon and name
    Component {
        id: deviceChip
        DButton {
            readonly property string deviceName: PlayerService.multiOutput ? I18n.tr("%1 outputs", "playback device chip, %1 is 2 or more").arg(PlayerService.selectedOutputs.length) : PlayerService.castDeviceName || (PlayerService.currentOutput?.name ?? "")
            maximumWidth: Math.round(root.rowWidth * PlayerMetrics.mediaDeviceNameWidthRatio)
            text: deviceName
            iconName: PlayerService.multiOutput ? "speaker_group" : UiConfig.deviceIcon(PlayerService.currentOutput?.kind ?? "default")
            tooltipText: I18n.tr("Playback device", "playback device: settings tab, button, dialog") + ": " + deviceName
            Accessible.name: tooltipText
            buttonHeight: Theme.buttonHeightXS
            backgroundColor: ArtColors.accentSecondaryContainer
            textColor: ArtColors.onAccentSecondaryContainer
            horizontalPadding: Theme.spacingM
            onClicked: Router.devicePickerOpen = true
        }
    }

    Component {
        id: device
        DeviceButton {
            size: root.size
            variant: root.look === "outlined" ? "outlined" : "standard"
            containerColor: ArtColors.accentContainer
            contentColor: ArtColors.onAccentContainer
            iconColor: checked ? ArtColors.onAccentContainer : ArtColors.textVariant
        }
    }

    Component {
        id: playPause
        Springy {
            readonly property bool bento: root.look === "transport"
            size: root.size
            widthMode: "wide"
            variant: "filled"
            round: false
            buttonSize: root.buttonSize
            // the M3E play button turns into a pill while playing
            checkable: true
            checked: PlayerService.playing
            radius: bento ? Theme.buttonRadius(width, height, buttonSize, pressed, false) : (pressed ? Theme.buttonRadius(width, height, buttonSize, true, false) : checked ? roundRadius : squareRadius)
            containerColor: ArtColors.accentContainer
            contentColor: ArtColors.onAccentContainer
            // a stream still being looked up shows the loading indicator instead of a position that stands still
            iconName: PlayerService.loading ? "" : PlayerService.playing ? "pause" : "play_arrow"
            iconFilled: false
            iconSize: root.size === "m" ? Theme.iconSizeLarge : Theme.iconSize
            enabled: PlayerService.current !== null
            Accessible.name: PlayerService.loading ? I18n.tr("Loading", "something still waiting for data: the play button, a list") : PlayerService.playing ? I18n.tr("Pause", "transport button") : I18n.tr("Play", "button that starts playback")
            tooltipText: Accessible.name
            onClicked: PlayerService.toggle()

            DLoadingIndicator {
                anchors.centerIn: parent
                visible: PlayerService.loading
                size: parent.iconSize
                color: parent.contentColor
            }
        }
    }

    Component {
        id: button
        Springy {
            readonly property string id_: root.elementId
            readonly property bool bento: root.look === "transport"
            size: root.size
            round: !bento
            buttonSize: root.buttonSize
            radius: bento ? Theme.buttonRadius(width, height, buttonSize, pressed, false) : (pressed ? Theme.buttonRadius(width, height, buttonSize, true, false) : round !== checked ? roundRadius : squareRadius)
            variant: bento ? "tonal" : root.look === "outlined" ? "outlined" : "standard"
            // M3 toggles: a Bento mode that is on fills with the accent, off stays tonal; M3E's standard buttons
            // fill with the container when on
            containerColor: !bento ? ArtColors.accentContainer : checked ? ArtColors.accent : ArtColors.accentSecondaryContainer
            contentColor: !bento ? ArtColors.onAccentContainer : checked ? ArtColors.onAccent : ArtColors.onAccentSecondaryContainer
            iconColor: filled ? contentColor : ArtColors.textVariant
            iconFilled: false
            iconName: {
                switch (id_) {
                case "repeat":
                    return PlayerService.repeat === "one" ? "repeat_one" : "repeat";
                case "dock":
                    return Router.dockMode === "side" ? "bottom_panel_open" : "right_panel_open";
                }
                return UiConfig.elementIcon(id_);
            }
            checkable: ["shuffle", "repeat", "lyrics", "queue", "sleep", "speed"].indexOf(id_) >= 0
            checked: {
                switch (id_) {
                case "shuffle":
                    return PlayerService.shuffle;
                case "repeat":
                    return PlayerService.repeat !== "off";
                case "lyrics":
                case "queue":
                    return root.context === "bar" ? Router.panel === id_ : root.context === "side" ? Router.sideTab === id_ : Router.viewPane === id_;
                case "sleep":
                    return PlayerService.sleepMode !== "off";
                case "speed":
                    return PlayerService.settings.speed !== 1;
                }
                return false;
            }
            enabled: {
                switch (id_) {
                case "previous":
                case "next":
                case "addToPlaylist":
                case "goToArtist":
                case "details":
                    return root.track !== null;
                case "goToAlbum":
                    return (root.track?.albumId ?? "") !== "";
                }
                return true;
            }
            Accessible.name: {
                switch (id_) {
                case "repeat":
                    return PlayerService.repeat === "all" ? I18n.tr("Repeat all", "transport button state") : PlayerService.repeat === "one" ? I18n.tr("Repeat one", "transport button state") : I18n.tr("Repeat off", "transport button state");
                case "dock":
                    return Router.dockMode === "side" ? I18n.tr("Move to bottom bar", "button moving now playing out of the sidebar") : I18n.tr("Move to sidebar", "button moving now playing from the bottom bar into the sidebar");
                }
                return UiConfig.elementLabel(id_);
            }
            tooltipText: {
                if (id_ === "sleep" && PlayerService.sleepMode === "minutes")
                    return I18n.tr("Sleep in %1", "sleep timer button tooltip, %1 is the remaining time").arg(Format.duration(PlayerService.sleepRemaining));
                if (id_ === "sleep" && PlayerService.sleepMode === "endOfTrack")
                    return I18n.tr("Sleep after this track", "sleep timer button tooltip");
                if (id_ === "speed")
                    return I18n.tr("Playback speed %1×", "speed button tooltip").arg(PlayerService.settings.speed);
                return Accessible.name;
            }
            onClicked: Router.runElement(id_, root.context)
        }
    }
}
