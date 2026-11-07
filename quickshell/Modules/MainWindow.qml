pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Templates as T
import Quickshell
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import "../Common/EscapeStack.js" as Escape

DFloatingWindow {
    id: window

    signal closeRequested

    readonly property bool compact: width < PlayerMetrics.compactBreakpoint
    // "rail" | "railLabels" | "bar"; automatic follows M3: a rail on wide windows, a bar on narrow ones
    readonly property string navMode: SettingsData.navStyle === "auto" ? (compact ? "bar" : "rail") : SettingsData.navStyle
    readonly property bool railNav: navMode !== "bar"
    // the header search as a full field; a narrow window keeps the button
    readonly property bool headerField: SettingsData.headerSearch === "field" && !compact
    readonly property bool sidePanel: Router.dockMode === "side" && !compact
    readonly property var navModel: Router.navEntries.map(e => ({
                "text": UiConfig.entryLabel(e),
                "icon": UiConfig.entryIcon(e)
            }))

    Binding {
        target: Router
        property: "headerSearchField"
        value: window.headerField
    }

    title: I18n.tr("Dux Player", "app name")
    minimumSize: Qt.size(PlayerMetrics.windowMinWidth, PlayerMetrics.windowMinHeight)
    implicitWidth: PlayerMetrics.windowWidth
    implicitHeight: screen ? Math.min(PlayerMetrics.windowHeight, screen.height - PlayerMetrics.screenMargin) : PlayerMetrics.windowHeight
    surfaceColor: Theme.withAlpha(Router.nowPlayingOpen ? ArtColors.surface : Theme.surface, Theme.floatingWindowTransparency)
    visible: true
    onClosed: window.closeRequested()

    FocusScope {
        id: focusScope
        anchors.fill: parent
        focus: true

        LayoutMirroring.enabled: I18n.isRtl
        LayoutMirroring.childrenInherit: true

        Component.onCompleted: {
            forceActiveFocus();
            if (UiConfig.nowPlaying.start === "full" && PlayerService.current !== null)
                Router.openNowPlaying("");
        }

        Shortcut {
            sequence: UiConfig.shortcut("searchPage")
            enabled: Router.pageSearch !== null
            onActivated: Router.pageSearch.forceActiveFocus()
        }

        Shortcut {
            sequence: UiConfig.shortcut("searchAll")
            onActivated: Router.searchEverything()
        }

        Shortcut {
            sequences: [StandardKey.Preferences, "Ctrl+,"]
            onActivated: Router.openSettings("")
        }

        Shortcut {
            sequence: "Ctrl+Space"
            onActivated: PlayerService.toggle()
        }

        // a dialog or sheet is open in this window: it owns the keys and the mouse buttons
        readonly property bool dialogOpen: Escape.top(Router.escapeLayers, l => l.handlesKey && l.owner.visible && l.owner.Window.window === focusScope.Window.window) !== null

        // Player keys: a Shortcut fires before the focused item, so `enabled` is what leaves a key to a field or to a control reached by Tab
        readonly property Item keyItem: Window.activeFocusItem
        readonly property bool typing: (keyItem as TextInput) !== null || (keyItem as TextEdit) !== null
        readonly property bool onSlider: keyItem?.Accessible.role === Accessible.Slider
        // Tab gives a control visualFocus and a click doesn't; items that aren't Controls only take focus from Tab
        readonly property bool keyboardHeld: (keyItem as T.Control)?.visualFocus ?? keyItem?.activeFocusOnTab ?? false
        readonly property bool playerKeys: !dialogOpen && !typing
        readonly property bool plainKeys: playerKeys && (onSlider || !keyboardHeld)
        readonly property bool arrowKeys: plainKeys && !onSlider
        // Hyprland leaves the keyboard here under an open dropdown, so every Shortcut would beat its popup. Its trigger holds the focus.
        readonly property bool dropdownOpen: (keyItem?.parent as DDropdown)?.menuOpen ?? false
        Keys.onShortcutOverride: event => event.accepted = dropdownOpen
        readonly property bool canSeek: PlayerService.current !== null && PlayerService.seekable && PlayerService.duration > 0

        Shortcut {
            sequence: "Space"
            enabled: focusScope.plainKeys
            autoRepeat: false
            onActivated: PlayerService.toggle()
        }

        // left is back in right-to-left layouts too, as on the seekbar
        Shortcut {
            sequence: "Left"
            enabled: focusScope.arrowKeys && focusScope.canSeek
            onActivated: PlayerService.seek(PlayerService.livePosition() - 5)
        }

        Shortcut {
            sequence: "Right"
            enabled: focusScope.arrowKeys && focusScope.canSeek
            // the end itself would skip the track, as on the seekbar
            onActivated: PlayerService.seek(Math.max(PlayerService.livePosition(), Math.min(PlayerService.livePosition() + 5, PlayerService.duration * 0.99)))
        }

        Shortcut {
            sequence: "Ctrl+Left"
            enabled: focusScope.playerKeys
            onActivated: PlayerService.previous()
        }

        Shortcut {
            sequence: "Ctrl+Right"
            enabled: focusScope.playerKeys
            onActivated: PlayerService.next()
        }

        // with no track the view would hide the page over nothing
        Shortcut {
            sequence: "F"
            enabled: focusScope.plainKeys && (Router.nowPlayingOpen || PlayerService.current !== null)
            autoRepeat: false
            onActivated: Router.nowPlayingOpen ? Router.nowPlayingOpen = false : Router.openNowPlaying("")
        }

        // the only Escape shortcut: it closes whatever was opened or focused last (EscapeLayer)
        Shortcut {
            readonly property var layer: Escape.top(Router.escapeLayers, l => l.owner.visible && l.owner.Window.window === focusScope.Window.window)
            sequence: "Escape"
            enabled: layer !== null && !layer.handlesKey
            onActivated: layer.escaped()
        }

        readonly property EscapeLayer _viewLayer: EscapeLayer {
            owner: focusScope
            active: Router.nowPlayingOpen
            onEscaped: Router.nowPlayingOpen = false
        }

        Shortcut {
            sequences: [StandardKey.Back]
            enabled: !focusScope.dialogOpen && (Router.canGoBack || Router.nowPlayingOpen)
            onActivated: Router.back()
        }

        Shortcut {
            sequences: [StandardKey.Forward]
            enabled: !focusScope.dialogOpen && Router.canGoForward
            onActivated: Router.forward()
        }

        TapHandler {
            acceptedButtons: Qt.BackButton | Qt.ForwardButton
            enabled: !focusScope.dialogOpen
            onTapped: (point, button) => {
                if (button === Qt.BackButton)
                    Router.back();
                else
                    Router.forward();
            }
        }

        Column {
            anchors.fill: parent
            spacing: 0

            DWindowHeader {
                id: header
                width: parent.width
                z: Theme.zHeader
                controls: windowControls
                title: I18n.tr("Dux Player", "app name")
                // a centered title would collide with the field
                titleAlignment: window.headerField ? Text.AlignLeft : Text.AlignHCenter
                onCloseRequested: window.closeRequested()

                ScopedSearch {
                    id: headerSearch
                    visible: window.headerField
                    pageField: false
                    anchors.verticalCenter: parent.verticalCenter
                    width: PlayerMetrics.headerSearchWidth
                    height: Theme.buttonHeightXS
                    placeholderText: I18n.tr("Search albums, artists and tracks", "search field placeholder")
                    Accessible.name: I18n.tr("Search", "header button")
                    onTextChanged: {
                        if (!inputFocused)
                            return;
                        PlayerService.search(text);
                        if (text === "" || Router.page === "search")
                            return;
                        Router.go("search");
                        // the search page focuses its own field when it loads
                        forceActiveFocus();
                    }

                    // a Binding, not a plain one: Escape's clear assigns the text
                    readonly property Binding _query: Binding {
                        target: headerSearch
                        property: "text"
                        value: PlayerService.searchQuery
                        when: !headerSearch.inputFocused
                    }
                }

                // the app's own actions as one connected group, apart from the window buttons
                DActionGroup {
                    anchors.verticalCenter: parent.verticalCenter

                    DActionGroup.Button {
                        visible: !window.headerField
                        buttonSize: Theme.buttonHeightXS
                        iconSize: Theme.iconSizeSmall
                        iconName: "search"
                        tooltipText: I18n.tr("Search", "header button")
                        Accessible.name: tooltipText
                        onClicked: Router.searchEverything()
                    }

                    DActionGroup.Button {
                        id: overflowButton
                        buttonSize: Theme.buttonHeightXS
                        iconSize: Theme.iconSizeSmall
                        iconName: "more_vert"
                        tooltipText: I18n.tr("More", "track row menu button")
                        Accessible.name: tooltipText
                        onClicked: overflowMenu.open(overflowButton)

                        DMenu {
                            id: overflowMenu
                            model: [
                                {
                                    "id": "settings",
                                    "text": I18n.tr("Settings", "the settings window"),
                                    "icon": "settings"
                                },
                                {
                                    "id": "quit",
                                    "text": I18n.tr("Quit Dux Player", "window menu action that stops playback and the player"),
                                    "icon": "power_settings_new"
                                }
                            ]
                            onTriggered: id => {
                                if (id === "settings")
                                    Router.openSettings("");
                                else
                                    Router.flushUndo(() => DaemonBridge.request("daemon.quit", null, I18n.tr("Couldn't quit the player", "toast when quitting the player fails")));
                            }
                        }
                    }
                }
            }

            Item {
                id: body
                width: parent.width
                height: parent.height - header.height
                visible: DaemonBridge.connected

                DExpandedRail {
                    id: labelRail
                    visible: window.navMode === "railLabels" && !Router.nowPlayingOpen
                    anchors.left: parent.left
                    anchors.top: parent.top
                    anchors.bottom: bottomBar.visible ? bottomBar.top : parent.bottom
                    width: visible ? implicitWidth : 0
                    model: window.navModel
                    currentIndex: Router.navIndex
                    onActivated: index => Router.openEntry(Router.navEntries[index])
                }

                DNavigationBar {
                    id: rail
                    visible: window.navMode === "rail" && !Router.nowPlayingOpen
                    orientation: Qt.Vertical
                    evenlySpaced: false
                    // DNavigationBar turns mirroring off on itself, so a left anchor would stay left in RTL
                    x: I18n.isRtl ? parent.width - width : 0
                    anchors.top: parent.top
                    anchors.bottom: bottomBar.visible ? bottomBar.top : parent.bottom
                    model: window.navModel
                    currentIndex: Router.navIndex
                    onActivated: index => Router.openEntry(Router.navEntries[index])
                }

                DNavigationBar {
                    id: topBar
                    visible: !window.railNav && !Router.nowPlayingOpen
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.top: parent.top
                    anchors.leftMargin: Theme.windowInset
                    anchors.rightMargin: Theme.windowInset
                    height: visible ? implicitHeight : 0
                    model: window.navModel
                    currentIndex: Router.navIndex
                    onActivated: index => Router.openEntry(Router.navEntries[index])
                }

                // hidden under now playing so the window's own transparency shows the same way everywhere
                Item {
                    id: main
                    visible: !Router.nowPlayingOpen
                    anchors.left: window.navMode === "railLabels" ? labelRail.right : window.navMode === "rail" ? rail.right : parent.left
                    anchors.right: parent.right
                    anchors.top: topBar.bottom
                    anchors.bottom: bottomBar.visible ? bottomBar.top : parent.bottom
                    anchors.leftMargin: window.railNav ? 0 : Theme.windowInset
                    anchors.rightMargin: Theme.windowInset
                    anchors.bottomMargin: Theme.windowInset

                    Rectangle {
                        id: contentPane
                        anchors.left: parent.left
                        anchors.right: sideHost.overlay ? parent.right : sideHost.left
                        // grows with the side panel, so the gap doesn't jump as it slides in
                        anchors.rightMargin: sideHost.overlay ? 0 : Theme.windowInset * sideHost.reveal
                        anchors.top: parent.top
                        anchors.bottom: parent.bottom
                        radius: PlayerMetrics.paneRadius
                        color: SettingsMetrics.paneColor
                        clip: true

                        Loader {
                            id: pageLoader
                            readonly property string route: Router.route

                            anchors.fill: parent
                            onRouteChanged: {
                                if (Theme.animationsEnabled)
                                    pageEnter.restart();
                            }
                            source: {
                                switch (Router.page) {
                                case "album":
                                    return "AlbumPage.qml";
                                case "artist":
                                    return "ArtistPage.qml";
                                case "playlist":
                                    return "PlaylistPage.qml";
                                case "libraries":
                                    return "LibrariesPage.qml";
                                case "search":
                                    return "SearchPage.qml";
                                case "history":
                                    return "HistoryPage.qml";
                                case "home":
                                    return "HomePage.qml";
                                }
                                return "BrowsePage.qml";
                            }

                            ParallelAnimation {
                                id: pageEnter
                                NumberAnimation {
                                    target: pageLoader
                                    property: "opacity"
                                    from: 0
                                    to: 1
                                    duration: Theme.expressiveDurations.normal
                                    easing.type: Easing.BezierSpline
                                    easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
                                }
                                NumberAnimation {
                                    target: pageLoader
                                    property: "scale"
                                    from: PlayerMetrics.pageEnterScale
                                    to: 1
                                    duration: Theme.expressiveDurations.normal
                                    easing.type: Easing.BezierSpline
                                    easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
                                }
                            }
                        }
                    }

                    Rectangle {
                        anchors.fill: parent
                        visible: sideHost.overlay && sideHost.reveal > 0
                        radius: PlayerMetrics.paneRadius
                        color: Theme.scrimColor
                        opacity: Theme.scrimAlpha * sideHost.reveal

                        MouseArea {
                            anchors.fill: parent
                            onClicked: Router.panel = ""
                        }
                    }

                    Rectangle {
                        id: sideHost
                        anchors.right: parent.right
                        anchors.top: parent.top
                        anchors.bottom: parent.bottom
                        // a compact window has no room beside the page, so the panel covers it
                        readonly property bool overlay: window.compact
                        readonly property bool covering: overlay && Router.panel !== ""
                        readonly property real fullWidth: overlay ? parent.width - PlayerMetrics.compactPanelPeek : window.sidePanel ? PlayerMetrics.sidePanelWidth : PlayerMetrics.queueWidth
                        readonly property bool shown: window.sidePanel || Router.panel !== ""
                        // animated instead of the width, so resizing the window doesn't make the panel trail behind
                        property real reveal: shown ? 1 : 0
                        property Item returnFocus: null

                        width: fullWidth * reveal
                        visible: reveal > 0
                        radius: UiConfig.attachedPanes ? 0 : PlayerMetrics.paneRadius
                        color: UiConfig.attachedPanes && !overlay ? "transparent" : SettingsMetrics.paneColor
                        clip: true

                        onCoveringChanged: {
                            if (covering) {
                                returnFocus = Window.activeFocusItem;
                                (panelLoader.item as SidePanel)?.takeFocus();
                            } else if (returnFocus) {
                                returnFocus.forceActiveFocus();
                                returnFocus = null;
                            }
                        }

                        readonly property EscapeLayer _escapeLayer: EscapeLayer {
                            owner: sideHost
                            active: sideHost.covering
                            onEscaped: Router.panel = ""
                        }

                        Behavior on reveal {
                            enabled: Theme.animationsEnabled
                            NumberAnimation {
                                duration: Theme.expressiveDurations.normal
                                easing.type: Easing.BezierSpline
                                easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
                            }
                        }

                        // full width from the start: the panel slides in instead of reflowing as it grows
                        Loader {
                            id: panelLoader
                            anchors.left: parent.left
                            anchors.top: parent.top
                            anchors.bottom: parent.bottom
                            width: sideHost.fullWidth
                            active: parent.visible
                            sourceComponent: window.sidePanel ? sidePanelComponent : panelComponent
                            onLoaded: {
                                if (sideHost.covering)
                                    (item as SidePanel)?.takeFocus();
                            }
                        }
                    }
                }

                NowPlayingBar {
                    id: bottomBar
                    visible: !window.sidePanel && PlayerService.current !== null && !Router.nowPlayingOpen
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    anchors.margins: Theme.windowInset
                    compact: window.compact
                }

                Loader {
                    anchors.fill: parent
                    active: Router.nowPlayingOpen && PlayerService.current !== null
                    z: Theme.zSheet
                    sourceComponent: NowPlayingView {}
                    // only here is the view in the scene, so Tab starts in it and not at the window's controls
                    onLoaded: (item as NowPlayingView)?.forceActiveFocus()
                }
            }

            // instead of stale data while the bridge redials; the next link reloads everything
            Item {
                width: parent.width
                height: parent.height - header.height
                visible: !DaemonBridge.connected

                Column {
                    anchors.centerIn: parent
                    width: parent.width - PlayerMetrics.pagePadding * 2
                    spacing: Theme.spacingS

                    DIcon {
                        anchors.horizontalCenter: parent.horizontalCenter
                        name: "link_off"
                        size: Theme.iconSizeLarge
                        color: Theme.surfaceVariantText
                    }

                    StyledText {
                        width: parent.width
                        horizontalAlignment: Text.AlignHCenter
                        text: I18n.tr("Not connected to the player", "main window state when the daemon is unreachable")
                        font.pixelSize: Theme.fontSizeLarge
                        color: Theme.surfaceText
                    }

                    StyledText {
                        width: parent.width
                        horizontalAlignment: Text.AlignHCenter
                        text: I18n.tr("Trying again…", "main window state when the daemon is unreachable")
                        color: Theme.surfaceVariantText
                    }
                }
            }
        }

        AddToPlaylistDialog {}

        DevicePickerDialog {}

        SleepTimerDialog {}

        SpeedDialog {}

        Snackbar {
            z: Theme.zToast
            bottomInset: bottomBar.visible ? bottomBar.height + Theme.windowInset : 0
        }

        TextPromptDialog {
            id: prompt
            z: Theme.zPrompt
        }

        ConfirmDialog {
            id: confirmDialog
            property var callback: null
            z: Theme.zPrompt
            onConfirmed: callback?.()
        }

        DetailsDialog {}

        Connections {
            target: Router
            function onPromptRequested(title, initial, confirmText, callback, label) {
                prompt.ask(title, initial, confirmText, callback, label);
            }
            function onConfirmRequested(title, text, confirmText, callback) {
                confirmDialog.title = title;
                confirmDialog.supportingText = text;
                confirmDialog.confirmText = confirmText;
                confirmDialog.callback = callback;
                confirmDialog.ask();
            }
        }
    }

    Component {
        id: panelComponent
        SidePanel {}
    }

    Component {
        id: sidePanelComponent
        NowPlayingSide {}
    }

    FloatingWindowControls {
        id: windowControls
        targetWindow: window
    }
}
