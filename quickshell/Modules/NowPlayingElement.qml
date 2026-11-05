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
    // "plain" | "transport" (Bento's tonal buttons) | "m3e" (Material's standard buttons) | "row" (Bento's device
    // row: volume as a ring, the device as a chip) | "group" (one connected button group, Bento's option row)
    property string look: "plain"
    // in the group look: where this button sits in its run, "start" | "middle" | "end" | "only" (left to right)
    property string groupPosition: "only"
    // in the group look: the width of a wide icon button, which every member takes
    property real groupWidth: buttonSize
    // overrides size's button height (Bento's transport fills the deck's height)
    property real buttonHeight: 0
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
    readonly property real sizeHeight: size === "m" ? Theme.buttonHeightM : Theme.buttonHeightS
    readonly property real buttonSize: buttonHeight > 0 ? buttonHeight : sizeHeight
    // icons grow with a button made taller than its size
    readonly property real iconScale: buttonSize / sizeHeight
    readonly property bool grouped: look === "group"
    readonly property bool groupStart: groupPosition === "start" || groupPosition === "only"
    readonly property bool groupEnd: groupPosition === "end" || groupPosition === "only"
    // the volume is a button showing the level (Bento's ring, M3E's fill); scroll changes it, a click opens the slider
    readonly property Component volumeComponent: UiConfig.nowPlaying.style === "m3e" || grouped ? volumeLevel : volumeRing
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

    // a group member's corner: the run's outer ends fully round, the rest as the button's own radius
    function corner(item, outer) {
        return grouped && outer ? Theme.fullRadius(item.width, item.height) : item.radius;
    }

    function groupRadius(item, pressed, checked) {
        return pressed ? Theme.cornerRadiusXS : checked ? Theme.fullRadius(item.width, item.height) : Theme.cornerRadiusS;
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
        // Bento's buttons outside a filled row: round, play wider
        width: root.grouped ? root.groupWidth : root.look === "transport" && !root.fills ? Math.round(buttonSize * root.weight) : iconSize + horizontalInset * 2
        topLeftRadius: root.corner(springy, root.groupStart)
        bottomLeftRadius: root.corner(springy, root.groupStart)
        topRightRadius: root.corner(springy, root.groupEnd)
        bottomRightRadius: root.corner(springy, root.groupEnd)
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
            id: titleBox

            // the track's heart sits right after the text, wherever that ends
            readonly property bool hasHeart: root.options.favorite === true
            readonly property real heartSpace: hasHeart ? heart.width + Theme.spacingXS : 0
            // one of the title components below, which share no type
            readonly property var titleText: titleLoader.item
            readonly property real textWidth: titleText?.fullTextWidth ?? titleText?.implicitTextWidth ?? 0

            implicitWidth: titleLoader.implicitWidth + heartSpace
            implicitHeight: titleLoader.implicitHeight

            FavoriteButton {
                id: heart
                x: titleLoader.x + titleLoader.width + Theme.spacingXS
                anchors.verticalCenter: titleLoader.verticalCenter
                visible: titleBox.hasHeart
                kind: "track"
                item: root.track
                buttonSize: root.textLook === "bar" ? Theme.buttonHeightXS : Theme.buttonHeightS
                activeColor: ArtColors.accent
                inactiveColor: ArtColors.textVariant
            }

            Loader {
                id: titleLoader
                x: root.centered && titleBox.hasHeart ? Math.round((parent.width - width - titleBox.heartSpace) / 2) : 0
                width: titleBox.hasHeart ? Math.min(parent.width - titleBox.heartSpace, titleBox.textWidth) : parent.width
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
                    expressive: root.options.face !== "display"
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
                    expressive: root.options.face !== "display"
                    text: root.track?.title ?? ""
                    horizontalAlignment: root.centered ? Text.AlignHCenter : Text.AlignLeft
                    active: PlayerService.playing && root.visible
                }
            }

            Component {
                id: m3eTitle
                StyledText {
                    id: m3eText
                    // on one line: a wrapped text's own width depends on the room it is given
                    readonly property real fullTextWidth: m3eSize.advanceWidth
                    text: root.track?.title ?? ""
                    color: ArtColors.text
                    font.pixelSize: root.context === "side" ? Theme.fontSizeXLarge : Theme.fontSizeXXLarge
                    elide: Text.ElideRight
                    wrapMode: Text.WrapAtWordBoundaryOrAnywhere
                    maximumLineCount: 2
                    horizontalAlignment: root.centered ? Text.AlignHCenter : Text.AlignLeft

                    TextMetrics {
                        id: m3eSize
                        text: m3eText.text
                        font: m3eText.font
                    }
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
                spacing: Theme.spacingM

                StyledText {
                    id: sourceLabel
                    anchors.verticalCenter: parent.verticalCenter
                    width: Math.min(implicitWidth, chipRow.parent.width / 2)
                    text: I18n.tr("Playing from", "now playing: label before where the queue came from")
                    color: ArtColors.textVariant
                    font.pixelSize: Theme.fontSizeMedium
                    wrapMode: Text.NoWrap
                    elide: Text.ElideRight
                }

                DButton {
                    maximumWidth: Math.max(0, chipRow.parent.width - sourceLabel.width - chipRow.spacing)
                    buttonHeight: Theme.buttonHeightXS
                    horizontalPadding: Theme.spacingL
                    text: UiConfig.contextLabel(PlayerService.playContext)
                    backgroundColor: ArtColors.accentSecondaryContainer
                    textColor: ArtColors.onAccentSecondaryContainer
                    tooltipText: I18n.tr("Queue", "the play queue: panel, toggle")
                    Accessible.name: I18n.tr("Playing from", "now playing: label before where the queue came from") + " " + text
                    onClicked: Router.runElement("playingFrom", root.context)
                }
            }
        }
    }

    // Material's overline, which opens the queue like Bento's chip
    Component {
        id: playingFromText
        LinkText {
            linked: root.textLook === "m3e"
            text: UiConfig.contextLabel(PlayerService.playContext)
            Accessible.name: linked ? I18n.tr("Playing from", "now playing: label before where the queue came from") + " " + text : text
            onActivated: Router.runElement("playingFrom", root.context)
            color: ArtColors.textVariant
            font.pixelSize: root.textLook === "m3e" ? Theme.fontSizeMedium : Theme.fontSizeSmall
            font.weight: root.textLook === "m3e" ? Theme.fontWeightMedium : Font.Normal
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
            id: favoriteButton
            kind: "track"
            item: root.track
            buttonSize: root.buttonSize
            width: root.grouped ? root.groupWidth : buttonSize
            radius: root.grouped ? root.groupRadius(favoriteButton, pressed, favoriteButton.favorite) : Theme.buttonRadius(width, height, buttonSize, pressed, circular)
            topLeftRadius: root.corner(favoriteButton, root.groupStart)
            bottomLeftRadius: root.corner(favoriteButton, root.groupStart)
            topRightRadius: root.corner(favoriteButton, root.groupEnd)
            bottomRightRadius: root.corner(favoriteButton, root.groupEnd)
            backgroundColor: !root.grouped ? "transparent" : favoriteButton.favorite ? ArtColors.accent : ArtColors.accentContainer
            activeColor: root.grouped ? ArtColors.onAccent : ArtColors.accent
            inactiveColor: root.grouped ? ArtColors.onAccentContainer : ArtColors.textVariant
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
        MediaSeekbar {
            flat: UiConfig.nowPlaying.style === "m3e"
            thin: root.context === "bar"
        }
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

    // Material's volume: a standard button filling up from the bottom with the level (tonal in a group)
    Component {
        id: volumeLevel
        StyledButton {
            id: levelButton
            readonly property real level: PlayerService.muted ? 0 : PlayerService.volume / Math.max(0.01, PlayerService.volumeMax)
            width: root.grouped ? root.groupWidth : root.buttonSize
            height: root.buttonSize
            radius: root.grouped ? root.groupRadius(levelButton, pressed, false) : Theme.fullRadius(width, height)
            topLeftRadius: root.corner(levelButton, root.groupStart)
            bottomLeftRadius: root.corner(levelButton, root.groupStart)
            topRightRadius: root.corner(levelButton, root.groupEnd)
            bottomRightRadius: root.corner(levelButton, root.groupEnd)
            color: "transparent"
            Accessible.name: I18n.tr("Volume", "audio volume: settings section, slider") + ": " + Math.round(PlayerService.volume * 100) + "%"
            onClicked: levelPanel.show()

            Rectangle {
                anchors.fill: parent
                radius: parent.radius
                topLeftRadius: parent.topLeftRadius
                bottomLeftRadius: parent.bottomLeftRadius
                topRightRadius: parent.topRightRadius
                bottomRightRadius: parent.bottomRightRadius
                color: root.grouped ? ArtColors.accentSecondaryContainer : "transparent"
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
                    topLeftRadius: levelButton.topLeftRadius
                    bottomLeftRadius: levelButton.bottomLeftRadius
                    topRightRadius: levelButton.topRightRadius
                    bottomRightRadius: levelButton.bottomRightRadius
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
                color: levelButton.level > 0.5 ? ArtColors.onAccentContainer : root.grouped ? ArtColors.onAccentSecondaryContainer : ArtColors.textVariant
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
            id: deviceButton
            context: root.context
            size: root.size
            buttonSize: root.buttonSize
            widthMode: root.grouped ? "wide" : "default"
            width: root.grouped ? root.groupWidth : iconSize + horizontalInset * 2
            variant: root.grouped ? "tonal" : "standard"
            radius: root.grouped ? root.groupRadius(deviceButton, pressed, checked) : pressed ? Theme.buttonRadius(width, height, buttonSize, true, false) : round !== checked ? roundRadius : squareRadius
            topLeftRadius: root.corner(deviceButton, root.groupStart)
            bottomLeftRadius: root.corner(deviceButton, root.groupStart)
            topRightRadius: root.corner(deviceButton, root.groupEnd)
            bottomRightRadius: root.corner(deviceButton, root.groupEnd)
            containerColor: root.grouped && checked ? ArtColors.accent : ArtColors.accentContainer
            contentColor: root.grouped && checked ? ArtColors.onAccent : ArtColors.onAccentContainer
            iconColor: filled ? contentColor : ArtColors.textVariant
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
            radius: bento ? Theme.buttonRadius(width, height, buttonSize, pressed, true) : (pressed ? Theme.buttonRadius(width, height, buttonSize, true, false) : checked ? roundRadius : squareRadius)
            // the one filled button of the transport, in every style
            containerColor: ArtColors.accent
            contentColor: ArtColors.onAccent
            // a stream still being looked up shows the loading indicator instead of a position that stands still
            iconName: PlayerService.loading ? "" : PlayerService.playing ? "pause" : "play_arrow"
            iconFilled: true
            iconSize: Math.round((root.size === "m" ? Theme.iconSizeLarge : Theme.iconSize) * root.iconScale)
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
            id: elementButton
            readonly property string id_: root.elementId
            readonly property bool bento: root.look === "transport"
            readonly property bool skip: id_ === "previous" || id_ === "next"
            size: root.size
            round: !bento
            buttonSize: root.buttonSize
            iconSize: Math.round(Theme.iconSize * root.iconScale)
            widthMode: root.grouped ? "wide" : "default"
            radius: root.grouped ? root.groupRadius(elementButton, pressed, checked) : bento ? Theme.buttonRadius(width, height, buttonSize, pressed, true) : (pressed ? Theme.buttonRadius(width, height, buttonSize, true, false) : round !== checked ? roundRadius : squareRadius)
            variant: bento || root.grouped ? "tonal" : "standard"
            // M3 toggles: a Bento mode that is on fills with the accent, off stays tonal; M3E's standard buttons
            // fill with the container when on. Previous and next take the accent container beside the filled play.
            containerColor: root.grouped ? (checked ? ArtColors.accent : ArtColors.accentContainer) : !bento || skip ? ArtColors.accentContainer : checked ? ArtColors.accent : ArtColors.accentSecondaryContainer
            contentColor: root.grouped ? (checked ? ArtColors.onAccent : ArtColors.onAccentContainer) : !bento || skip ? ArtColors.onAccentContainer : checked ? ArtColors.onAccent : ArtColors.onAccentSecondaryContainer
            iconColor: filled ? contentColor : ArtColors.textVariant
            iconFilled: bento && root.transportId || root.grouped && checked
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
                    return Phrases.repeatState(PlayerService.repeat);
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
