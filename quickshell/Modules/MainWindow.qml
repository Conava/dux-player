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

    // the side panel's and bottom bar's slides
    component PanelMotion: NumberAnimation {
        duration: Theme.expressiveDurations.normal
        easing.type: Easing.BezierSpline
        easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
    }

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

        // A button takes the grab of any mouse button and Qt reads a move under it as a left press, so a right click
        // that moved also clicked. The first move takes the grab away; a MouseArea on these buttons sets preventStealing.
        DragHandler {
            acceptedButtons: Qt.RightButton | Qt.MiddleButton
            target: null
            dragThreshold: 0
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
            enabled: layer !== null && !layer.handlesKey && MenuState.open === null
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
                readonly property real fullTarget: fullWanted && viewLoader.status === Loader.Ready ? 1 : 0
                // follows the flag and outlives it, so the view is still there while the surface shrinks
                property real fullReveal: fullTarget
                // the same move at an even pace, as the curve would crush the fades into a few frames
                property real fullFade: fullTarget

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
                        duration: Theme.expressiveDurations.large
                        easing.type: Easing.BezierSpline
                        easing.bezierCurve: Theme.expressiveCurves.standard
                    }
                }

                Behavior on fullFade {
                    enabled: Theme.animationsEnabled
                    NumberAnimation {
                        duration: Theme.expressiveDurations.large
                    }
                }

                // the now-playing surface growing into the window, behind everything that moves with it
                Rectangle {
                    id: card
                    readonly property Rectangle source: window.sidePanel ? sideHost : bottomBar
                    // x and y are live in either layout direction, where a side panel sits on the left in RTL
                    readonly property point sourcePos: window.sidePanel ? Qt.point(main.x + sideHost.x, main.y + sideHost.y) : Qt.point(bottomBar.x, bottomBar.y)
                    readonly property real remaining: 1 - body.fullReveal

                    x: sourcePos.x * remaining
                    y: sourcePos.y * remaining
                    width: body.width + (source.width - body.width) * remaining
                    height: body.height + (source.height - body.height) * remaining
                    radius: source.radius * remaining
                    // the attached look's bar has no card of its own, and the shape has to be seen to read as growing
                    readonly property color from: source.color.a > 0 ? source.color : ArtColors.cardColor

                    // turns into the window's own surface as it grows and thins out all the way, so it never ends in a jump
                    color: Theme.blend(from, Theme.withAlpha(ArtColors.surface, from.a), body.fullFade)
                    // under the fading source this adds up to one layer of its colour, not two
                    opacity: (pages.opacity < 1 ? (1 - pages.opacity) / (1 - color.a * pages.opacity) : 0) * (1 - body.fullFade * body.fullFade)
                    visible: body.fullFade > 0 && body.fullFade < 1
                }

                // laid out at a fixed size the whole time, so nothing lays out again while the view moves
                Item {
                    id: pages
                    anchors.fill: parent
                    opacity: Math.max(0, 1 - body.fullFade / PlayerMetrics.morphPagesFade)
                    // back the moment the flag drops: Router.searchEverything focuses a page field in the same turn
                    visible: body.fullReveal < 1 || !body.fullWanted

                    DExpandedRail {
                        id: labelRail
                        visible: window.navMode === "railLabels"
                        anchors.left: parent.left
                        anchors.top: parent.top
                        anchors.bottom: bottomBar.top
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
                        anchors.bottom: bottomBar.top
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
                        anchors.bottom: bottomBar.top
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

                                // the final size, so the page lays out once while the panel and bar slide
                                anchors.left: parent.left
                                anchors.top: parent.top
                                width: Math.max(0, main.width - (sideHost.overlay || !sideHost.shown ? 0 : sideHost.fullWidth + Theme.windowInset))
                                height: Math.max(0, pages.height - topBar.height - Theme.windowInset - (bottomBar.shown ? bottomBar.height + Theme.windowInset : 0))
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
                            readonly property Component wanted: window.sidePanel ? sidePanelComponent : Router.panel !== "" ? panelComponent : null
                            // keeps the outgoing panel on screen while the host closes
                            property Component held: null
                            readonly property Component content: wanted ?? held
                            readonly property real fullWidth: overlay ? parent.width - PlayerMetrics.compactPanelPeek : content === sidePanelComponent ? PlayerMetrics.sidePanelWidth : PlayerMetrics.queueWidth
                            readonly property bool shown: wanted !== null
                            // animated instead of the width, so resizing the window doesn't make the panel trail behind
                            property real reveal: shown && panelLoader.status === Loader.Ready ? 1 : 0
                            // animated instead of the width for the same reason
                            property real dockMix: content === sidePanelComponent ? 1 : 0
                            property Item returnFocus: null

                            width: (overlay ? fullWidth : PlayerMetrics.queueWidth + (PlayerMetrics.sidePanelWidth - PlayerMetrics.queueWidth) * dockMix) * reveal
                            visible: shown || reveal > 0
                            radius: UiConfig.attachedPanes ? 0 : PlayerMetrics.paneRadius
                            color: UiConfig.attachedPanes && !overlay ? "transparent" : SettingsMetrics.paneColor
                            clip: true

                            onWantedChanged: {
                                if (wanted)
                                    held = wanted;
                            }

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
                                PanelMotion {}
                            }

                            Behavior on dockMix {
                                enabled: Theme.animationsEnabled
                                PanelMotion {}
                            }

                            // full width from the start: the panel slides in instead of reflowing as it grows
                            Loader {
                                id: panelLoader
                                anchors.left: parent.left
                                anchors.top: parent.top
                                height: sideHost.height
                                width: sideHost.fullWidth
                                active: sideHost.shown || sideHost.reveal > 0
                                sourceComponent: sideHost.content
                                onSourceComponentChanged: {
                                    if (sideHost.reveal > 0 && Theme.animationsEnabled)
                                        panelSwap.restart();
                                }
                                onLoaded: {
                                    if (sideHost.covering)
                                        (item as SidePanel)?.takeFocus();
                                }
                            }

                            NumberAnimation {
                                id: panelSwap
                                target: panelLoader
                                property: "opacity"
                                from: 0
                                to: 1
                                duration: Theme.expressiveDurations.expressiveEffects
                                easing.type: Easing.BezierSpline
                                easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
                            }
                        }
                    }

                    NowPlayingBar {
                        id: bottomBar
                        // the page's height reads this: visible also follows the hidden parent
                        readonly property bool shown: !window.sidePanel && PlayerService.current !== null
                        property real barReveal: shown ? 1 : 0
                        visible: barReveal > 0
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.bottom: parent.bottom
                        anchors.margins: Theme.windowInset
                        // at 0 its top edge is the window's bottom, which is where the page's edge rests without it
                        anchors.bottomMargin: Theme.windowInset - (height + Theme.windowInset) * (1 - barReveal)
                        compact: window.compact

                        Behavior on barReveal {
                            enabled: Theme.animationsEnabled
                            PanelMotion {}
                        }
                    }
                }

                // full size throughout, so it lays out once; it covers the pages' clicks until it has faded
                Loader {
                    id: viewLoader
                    // never ahead of the surface, or the view would show outside it as it shrinks
                    readonly property real entry: Math.max(0, Math.min((body.fullFade - PlayerMetrics.morphViewFade) / (1 - PlayerMetrics.morphViewFade), (body.fullReveal - PlayerMetrics.morphViewCover) / (1 - PlayerMetrics.morphViewCover)))

                    width: parent.width
                    height: parent.height
                    opacity: entry
                    enabled: body.fullWanted || entry > 0
                    active: body.fullWanted || body.fullReveal > 0
                    sourceComponent: NowPlayingView {}
                    // only here is the view in the scene, so Tab starts in it and not at the window's controls
                    onLoaded: (item as NowPlayingView)?.forceActiveFocus()

                    transform: Scale {
                        origin.x: card.sourcePos.x + card.source.width / 2
                        origin.y: card.sourcePos.y + card.source.height / 2
                        xScale: 1 - (1 - PlayerMetrics.pageEnterScale) * Math.pow(1 - viewLoader.entry, 2)
                        yScale: xScale
                    }
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
            bottomInset: DaemonBridge.connected && !Router.nowPlayingOpen ? (bottomBar.height + Theme.windowInset) * bottomBar.barReveal : 0
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
