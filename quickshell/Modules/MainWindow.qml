pragma ComponentBehavior: Bound

import QtQuick
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
    surfaceColor: Theme.withAlpha(Theme.blend(Theme.surface, ArtColors.surface, body.fullReveal), Theme.floatingWindowTransparency)
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
            sequences: [StandardKey.Back, "Alt+Left"]
            enabled: Router.canGoBack
            onActivated: Router.back()
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

                readonly property bool fullWanted: Router.nowPlayingOpen && PlayerService.current !== null
                // follows the flag and outlives it, so the view is still there to slide away
                property real fullReveal: fullWanted && viewLoader.status === Loader.Ready ? 1 : 0
                // the view's top edge; rounded, or the two sides of the seam land on different pixels
                readonly property int seam: Math.round(height * (1 - fullReveal))

                // in the flag's turn, so a field Router.searchEverything focuses right after keeps the focus
                onFullWantedChanged: {
                    if (fullWanted)
                        (viewLoader.item as NowPlayingView)?.forceActiveFocus();
                    else if (sideHost.covering)
                        (panelLoader.item as SidePanel)?.takeFocus();
                }

                Behavior on fullReveal {
                    enabled: Theme.animationsEnabled
                    NumberAnimation {
                        duration: Theme.expressiveDurations.normal
                        easing.type: Easing.BezierSpline
                        easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
                    }
                }

                // cut at the seam and hidden under the view, so the window's own transparency shows the same way everywhere
                Item {
                    width: parent.width
                    height: body.seam
                    clip: body.fullReveal > 0
                    // back the moment the flag drops: Router.searchEverything focuses a page field in the same turn
                    visible: body.fullReveal < 1 || !body.fullWanted

                    // the pages keep their size while the seam moves, so nothing lays out again
                    Item {
                        width: body.width
                        height: body.height

                        DExpandedRail {
                            id: labelRail
                            visible: window.navMode === "railLabels"
                            anchors.left: parent.left
                            anchors.top: parent.top
                            anchors.bottom: bottomBar.shown ? bottomBar.top : parent.bottom
                            width: window.navMode === "railLabels" ? implicitWidth : 0
                            model: window.navModel
                            currentIndex: Router.navIndex
                            onActivated: index => Router.openEntry(Router.navEntries[index])
                        }

                        DNavigationBar {
                            id: rail
                            visible: window.navMode === "rail"
                            orientation: Qt.Vertical
                            evenlySpaced: false
                            // DNavigationBar turns mirroring off on itself, so a left anchor would stay left in RTL
                            x: I18n.isRtl ? parent.width - width : 0
                            anchors.top: parent.top
                            anchors.bottom: bottomBar.shown ? bottomBar.top : parent.bottom
                            model: window.navModel
                            currentIndex: Router.navIndex
                            onActivated: index => Router.openEntry(Router.navEntries[index])
                        }

                        DNavigationBar {
                            id: topBar
                            visible: !window.railNav
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.top: parent.top
                            anchors.leftMargin: Theme.windowInset
                            anchors.rightMargin: Theme.windowInset
                            height: !window.railNav ? implicitHeight : 0
                            model: window.navModel
                            currentIndex: Router.navIndex
                            onActivated: index => Router.openEntry(Router.navEntries[index])
                        }

                        Item {
                            id: main
                            anchors.left: window.navMode === "railLabels" ? labelRail.right : window.navMode === "rail" ? rail.right : parent.left
                            anchors.right: parent.right
                            anchors.top: topBar.bottom
                            anchors.bottom: bottomBar.shown ? bottomBar.top : parent.bottom
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
                                    readonly property string route: Router.page + "/" + Router.itemId

                                    // the final width, so the page lays out once while the panel slides
                                    anchors.left: parent.left
                                    anchors.top: parent.top
                                    anchors.bottom: parent.bottom
                                    width: Math.max(0, main.width - (sideHost.overlay || !sideHost.shown ? 0 : sideHost.fullWidth + Theme.windowInset))
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
                                property real reveal: shown && panelLoader.status === Loader.Ready ? 1 : 0
                                property Item returnFocus: null

                                width: fullWidth * reveal
                                visible: shown || reveal > 0
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
                                    active: sideHost.shown || sideHost.reveal > 0
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
                            // the page's anchors read this: visible also follows the hidden parent
                            readonly property bool shown: !window.sidePanel && PlayerService.current !== null
                            visible: shown
                            opacity: shown ? 1 : 0
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.bottom: parent.bottom
                            anchors.margins: Theme.windowInset
                            compact: window.compact

                            Behavior on opacity {
                                enabled: Theme.animationsEnabled
                                NumberAnimation {
                                    duration: Theme.shortDuration
                                    easing.type: Theme.standardEasing
                                }
                            }
                        }
                    }
                }

                Loader {
                    id: viewLoader
                    y: body.seam
                    width: parent.width
                    height: parent.height
                    active: body.fullWanted || body.fullReveal > 0
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
            bottomInset: DaemonBridge.connected && bottomBar.shown && !Router.nowPlayingOpen ? bottomBar.height + Theme.windowInset : 0
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
