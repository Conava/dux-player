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
    readonly property bool sidePanel: Router.dockMode === "side" && !compact
    readonly property var navModel: Router.navEntries.map(e => ({
                "text": UiConfig.entryLabel(e),
                "icon": UiConfig.entryIcon(e)
            }))

    title: I18n.tr("Dux Player", "app name")
    minimumSize: Qt.size(PlayerMetrics.windowMinWidth, PlayerMetrics.windowMinHeight)
    implicitWidth: PlayerMetrics.windowWidth
    implicitHeight: screen ? Math.min(PlayerMetrics.windowHeight, screen.height - PlayerMetrics.screenMargin) : PlayerMetrics.windowHeight
    surfaceColor: Theme.withAlpha(Router.nowPlayingOpen && ArtColors.nowPlaying ? ArtColors.nowPlaying.surface : Theme.surface, Theme.floatingWindowTransparency)
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
                onCloseRequested: window.closeRequested()

                DActionButton {
                    iconName: "search"
                    buttonSize: Theme.buttonHeightXXS
                    tooltipText: I18n.tr("Search", "header button")
                    Accessible.name: tooltipText
                    onClicked: Router.searchEverything()
                }

                DActionButton {
                    id: overflowButton
                    iconName: "more_vert"
                    buttonSize: Theme.buttonHeightXXS
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
                    anchors.left: parent.left
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
                        anchors.right: sideHost.visible ? sideHost.left : parent.right
                        anchors.rightMargin: sideHost.visible ? Theme.windowInset : 0
                        anchors.top: parent.top
                        anchors.bottom: parent.bottom
                        radius: PlayerMetrics.paneRadius
                        color: SettingsMetrics.paneColor
                        clip: true

                        Loader {
                            anchors.fill: parent
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
                        }
                    }

                    Rectangle {
                        id: sideHost
                        anchors.right: parent.right
                        anchors.top: parent.top
                        anchors.bottom: parent.bottom
                        width: window.sidePanel ? PlayerMetrics.sidePanelWidth : PlayerMetrics.queueWidth
                        visible: window.sidePanel || Router.panel !== ""
                        radius: PlayerMetrics.paneRadius
                        color: SettingsMetrics.paneColor
                        clip: true

                        Loader {
                            anchors.fill: parent
                            active: parent.visible
                            sourceComponent: window.sidePanel ? sidePanelComponent : panelComponent
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
