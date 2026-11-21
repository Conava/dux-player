pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Templates as T
import Quickshell
import qs.Common
import qs.Services
import qs.DCommon.Common as DC
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
    readonly property string navMode: SettingsData.navStyle !== "auto" ? SettingsData.navStyle : compact ? "bar" : SettingsData.autoRailLabels ? "railLabels" : "rail"
    readonly property bool railNav: navMode !== "bar"
    readonly property bool railLabels: navMode === "railLabels"
    // Search and Settings are navigation entries while the title bars are off, and the header is gone
    readonly property bool titleBarsOff: !StyleSource.windowTitleBars
    // the header search as a full field, in place of the title on a narrow window
    readonly property bool headerField: SettingsData.headerSearch === "field" && !titleBarsOff
    // a rail then shows Search as its FAB and Settings at its bottom; the bar keeps both as entries
    readonly property bool railChrome: titleBarsOff && railNav
    // a Search entry the user turned on stays beside the FAB
    readonly property bool searchPinned: UiConfig.navigation.some(e => e.kind === "page" && e.target === "search" && !e.hidden)
    readonly property var railEntries: railChrome ? Router.navEntries.filter(e => e.kind !== "settings" && (searchPinned || e.kind !== "page" || e.target !== "search")) : Router.navEntries
    readonly property int railIndex: railEntries.indexOf(Router.navEntries[Router.navIndex])
    readonly property var railModel: railChrome ? entriesModel(railEntries) : navModel
    readonly property var settingsModel: entriesModel(Router.navEntries.filter(e => e.kind === "settings"))
    // the full view has no top bar unless asked for; a handle moves the window instead
    readonly property bool headerHidden: !SettingsData.fullViewTopBar
    // KWin draws its own title bar when ours are off, so it has nothing to be moved by
    readonly property bool handleShown: titleBarsOff && DC.Compositor.desktop !== "kde"
    // what the header's place at the top holds: the header, the handle's strip, or just the inset
    readonly property real topHeight: !titleBarsOff ? header.height : handleShown ? Theme.windowInset * 2 + Theme.bottomSheetHandleHeight : Theme.windowInset
    readonly property bool sidePanel: Router.dockMode === "side" && !compact
    // the bottom bar is the floating pill over the content pane, which then reaches the window's bottom
    readonly property bool floatingBar: UiConfig.floatingLook
    readonly property var navModel: entriesModel(Router.navEntries)

    function entriesModel(entries) {
        return entries.map(e => ({
                    "text": UiConfig.entryLabel(e),
                    "icon": UiConfig.entryIcon(e)
                }));
    }

    // Automatic keeps its bar on narrow windows, so there the button only remembers the wide rail's labels
    function toggleRailLabels() {
        if (SettingsData.navStyle === "auto")
            SettingsData.autoRailLabels = !SettingsData.autoRailLabels;
        else
            SettingsData.navStyle = railLabels ? "rail" : "railLabels";
    }

    Binding {
        target: Router
        property: "headerSearchField"
        value: window.headerField
    }

    // the page ends scroll clear of the pill, following its slide
    Binding {
        target: Router
        property: "pageEndInset"
        value: window.floatingBar ? (PlayerMetrics.floatingHeight + PlayerMetrics.floatingMargin) * bottomBar.barReveal : 0
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

        // a dialog, sheet or volume panel is open in this window: it owns the keys and the mouse buttons
        readonly property bool dialogOpen: Router.playOnOpen || Escape.top(Router.escapeLayers, l => (l.handlesKey || (l.owner as VolumePanel) !== null) && l.owner.visible && l.owner.Window.window === focusScope.Window.window) !== null

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
        readonly property bool canSeek: PlayerService.active && PlayerService.seekable && PlayerService.duration > 0

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
            sequences: ["Alt+Left", "Back"]
            enabled: !focusScope.dialogOpen && (Router.canGoBack || Router.nowPlayingOpen)
            onActivated: Router.back()
        }

        Shortcut {
            sequences: ["Alt+Right", "Forward"]
            enabled: !focusScope.dialogOpen && Router.canGoForward
            onActivated: Router.forward()
        }

        TapHandler {
            // a thumb press is held and shoves the mouse; act on release like browsers do
            acceptedButtons: Qt.BackButton | Qt.ForwardButton
            gesturePolicy: TapHandler.ReleaseWithinBounds
            longPressThreshold: 0
            enabled: !focusScope.dialogOpen
            onTapped: (point, button) => {
                if (button === Qt.BackButton)
                    Router.back();
                else
                    Router.forward();
            }
        }

        // over the header rather than in its row, so the width it takes from the header's buttons can't loop through
        // itself: beside the title on a wide window, in the title's place on a narrow one
        ScopedSearch {
            id: headerSearch
            readonly property real start: window.compact ? header.edgeInset : header.width - header.buttonsReserve + header.titleGap
            visible: window.headerField
            opacity: header.opacity
            enabled: header.enabled
            pageField: false
            z: Theme.zHeader
            x: I18n.isRtl ? header.width - start - width : start
            y: (header.height - height) / 2
            width: window.compact ? Math.max(0, header.width - header.buttonsReserve - header.edgeInset) : PlayerMetrics.headerSearchWidth
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

        Column {
            anchors.fill: parent
            spacing: 0

            DWindowHeader {
                id: header
                width: parent.width
                z: Theme.zHeader
                visible: !window.titleBarsOff
                // gone with the pages; disabled, not hidden, so the column keeps its slot
                opacity: window.headerHidden ? Math.max(0, 1 - body.fullFade / PlayerMetrics.morphPagesFade) : 1
                enabled: opacity > 0
                controls: windowControls
                title: window.headerField && window.compact ? "" : I18n.tr("Dux Player", "app name")
                // a centered title would collide with the field
                titleAlignment: window.headerField ? Text.AlignLeft : Text.AlignHCenter
                onCloseRequested: window.closeRequested()

                // the field's place in the row while it sits beside the title
                Item {
                    width: PlayerMetrics.headerSearchWidth
                    height: Theme.buttonHeightXS
                    visible: window.headerField && !window.compact
                }

                // the app's own actions as one connected group, apart from the window buttons
                DActionGroup {
                    anchors.verticalCenter: parent.verticalCenter

                    DActionGroup.Button {
                        visible: SettingsData.headerSearch === "button"
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
                id: topSpace
                width: parent.width
                height: window.titleBarsOff ? window.topHeight : 0
                visible: window.titleBarsOff
            }

            Item {
                id: body
                width: parent.width
                height: parent.height - window.topHeight
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

                    // the menu button and, with title bars off, the Search FAB; centred in the rail, at the labels' start beside them
                    Column {
                        id: railTop
                        readonly property real labelStart: Theme.spacingM + Theme.spacingL
                        visible: window.railNav
                        x: I18n.isRtl ? parent.width - width : 0
                        width: window.railLabels ? labelRail.implicitWidth : rail.implicitWidth
                        height: window.railNav ? implicitHeight + Theme.spacingM : 0
                        spacing: Theme.spacingS

                        DIconButton {
                            id: railMenu
                            objectName: "railMenu"
                            readonly property real start: window.railLabels ? railTop.labelStart - (width - iconSize) / 2 : (railTop.width - width) / 2
                            x: I18n.isRtl ? railTop.width - start - width : start
                            iconName: window.railLabels ? "menu_open" : "menu"
                            rotation: I18n.isRtl ? 180 : 0
                            tooltipText: window.railLabels ? I18n.tr("Hide labels", "rail menu button") : I18n.tr("Show labels", "rail menu button")
                            Accessible.role: Accessible.Button
                            Accessible.name: I18n.tr("Show labels", "rail menu button")
                            Accessible.checkable: true
                            Accessible.checked: window.railLabels
                            onClicked: window.toggleRailLabels()
                        }

                        DFab {
                            objectName: "railSearch"
                            readonly property real start: window.railLabels ? Theme.spacingM : (railTop.width - width) / 2
                            visible: window.railChrome
                            x: I18n.isRtl ? railTop.width - start - width : start
                            iconName: "search"
                            text: window.railLabels ? I18n.tr("Search", "header button") : ""
                            tooltipText: window.railLabels ? null : I18n.tr("Search", "header button")
                            tooltipSide: I18n.isRtl ? "left" : "right"
                            onClicked: Router.searchEverything()
                        }
                    }

                    DExpandedRail {
                        id: labelRail
                        objectName: "railDestinations"
                        visible: window.railLabels
                        anchors.left: parent.left
                        anchors.top: railTop.bottom
                        anchors.bottom: labelSettings.visible ? labelSettings.top : window.floatingBar ? parent.bottom : bottomBar.top
                        width: window.railLabels ? implicitWidth : 0
                        model: window.railModel
                        currentIndex: window.railIndex
                        onActivated: index => Router.openEntry(window.railEntries[index])
                    }

                    // Settings apart from the pages; it opens its own window, so it never shows as selected
                    DExpandedRail {
                        id: labelSettings
                        objectName: "railSettings"
                        visible: window.railLabels && window.railChrome
                        anchors.left: parent.left
                        anchors.bottom: window.floatingBar ? parent.bottom : bottomBar.top
                        anchors.bottomMargin: Theme.windowInset
                        width: implicitWidth
                        height: Theme.navigationHeight + ringRoom * 2
                        model: window.settingsModel
                        onActivated: Router.openSettings("")
                    }

                    DNavigationBar {
                        id: rail
                        objectName: "railDestinations"
                        visible: window.navMode === "rail"
                        orientation: Qt.Vertical
                        evenlySpaced: false
                        // DNavigationBar turns mirroring off on itself, so a left anchor would stay left in RTL
                        x: I18n.isRtl ? parent.width - width : 0
                        anchors.top: railTop.bottom
                        // from the top, scrolling when they don't fit
                        height: Math.max(0, Math.min(implicitHeight, (railSettings.visible ? railSettings.y : window.floatingBar ? parent.height : bottomBar.y) - y))
                        model: window.railModel
                        currentIndex: window.railIndex
                        onActivated: index => Router.openEntry(window.railEntries[index])
                    }

                    // a one-entry DNavigationBar can't be activated from the keyboard while nothing is selected
                    DIconButton {
                        id: railSettings
                        objectName: "railSettings"
                        visible: window.navMode === "rail" && window.railChrome
                        x: rail.x + (rail.width - width) / 2
                        anchors.bottom: window.floatingBar ? parent.bottom : bottomBar.top
                        anchors.bottomMargin: Theme.windowInset
                        iconName: "settings"
                        tooltipText: I18n.tr("Settings", "the settings window")
                        tooltipSide: I18n.isRtl ? "left" : "right"
                        onClicked: Router.openSettings("")
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
                        anchors.bottom: window.floatingBar ? parent.bottom : bottomBar.top
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

                            // the page loaders stay synchronous: a DTextField in an incubating page runs its change handlers before its motions exist
                            readonly property bool homeWanted: Router.page === "home"
                            // Home is built once and kept, hidden, so going back to it is instant
                            property bool homeBuilt: false
                            // Home shows until the page that replaces it is built, then fades out
                            property bool homeShown: false
                            // the loader whose page is entering, and the route it entered for
                            property Item entering: null
                            property string enteredRoute: ""
                            // the final size, so the page lays out once while the panel and bar slide
                            readonly property real pageWidth: Math.max(0, main.width - (sideHost.overlay || !sideHost.shown ? 0 : sideHost.fullWidth + Theme.windowInset))
                            readonly property real pageHeight: Math.max(0, pages.height - topBar.height - Theme.windowInset - (bottomBar.shown && !window.floatingBar ? bottomBar.height + Theme.windowInset : 0))

                            onHomeWantedChanged: {
                                if (!homeWanted)
                                    return;
                                homeBuilt = true;
                                homeShown = true;
                                homeExit.stop();
                            }
                            Component.onCompleted: {
                                homeBuilt = homeWanted;
                                homeShown = homeWanted;
                            }
                            // starts the shown page's enter once it is built; the page left (Home, which stays) fades out
                            function settle() {
                                const loader = homeWanted ? homeLoader : pageLoader;
                                if (loader.status !== Loader.Ready || enteredRoute === Router.route)
                                    return;
                                const first = enteredRoute === "";
                                enteredRoute = Router.route;
                                if (first || !Theme.animationsEnabled) {
                                    if (loader === pageLoader)
                                        homeShown = false;
                                    return;
                                }
                                if (entering && entering !== loader) {
                                    entering.opacity = 1;
                                    entering.scale = 1;
                                }
                                entering = loader;
                                pageEnter.restart();
                                if (loader !== pageLoader)
                                    return;
                                // Router sets the page and then its id, so a visit can settle twice; a restart would hide Home at once
                                if (homeShown && !homeExit.running)
                                    homeExit.start();
                            }

                            Loader {
                                id: homeLoader
                                objectName: "keptHome"
                                active: contentPane.homeBuilt
                                visible: contentPane.homeShown
                                enabled: contentPane.homeWanted
                                anchors.left: parent.left
                                anchors.top: parent.top
                                width: contentPane.pageWidth
                                height: contentPane.pageHeight
                                source: "HomePage.qml"
                                onStatusChanged: contentPane.settle()
                            }

                            Loader {
                                id: pageLoader
                                readonly property string route: Router.route
                                // a turn later, so the source and status have followed the route
                                onRouteChanged: Qt.callLater(contentPane.settle)
                                active: !contentPane.homeWanted
                                anchors.left: parent.left
                                anchors.top: parent.top
                                width: contentPane.pageWidth
                                height: contentPane.pageHeight
                                onStatusChanged: contentPane.settle()
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
                                    case "stats":
                                        return "StatisticsPage.qml";
                                    }
                                    return "BrowsePage.qml";
                                }
                            }

                            ParallelAnimation {
                                id: pageEnter
                                NumberAnimation {
                                    target: contentPane.entering
                                    property: "opacity"
                                    from: 0
                                    to: 1
                                    duration: Theme.expressiveDurations.normal
                                    easing.type: Easing.BezierSpline
                                    easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
                                }
                                NumberAnimation {
                                    target: contentPane.entering
                                    property: "scale"
                                    from: PlayerMetrics.pageEnterScale
                                    to: 1
                                    duration: Theme.expressiveDurations.normal
                                    easing.type: Easing.BezierSpline
                                    easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
                                }
                            }

                            NumberAnimation {
                                id: homeExit
                                target: homeLoader
                                property: "opacity"
                                from: 1
                                to: 0
                                duration: Theme.expressiveDurations.expressiveEffects
                                easing.type: Easing.BezierSpline
                                easing.bezierCurve: Theme.expressiveCurves.expressiveEffects
                                // the hidden page starts over at full opacity
                                onStopped: {
                                    homeLoader.opacity = 1;
                                    if (!contentPane.homeWanted)
                                        contentPane.homeShown = false;
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
                            // covering the page, it ends above the pill
                            anchors.bottomMargin: overlay ? Router.pageEndInset : 0
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
                        // the space under the bar; the pill floats that far over the content pane's bottom
                        readonly property real gap: floating ? Theme.windowInset + PlayerMetrics.floatingMargin : Theme.windowInset
                        // how far the shown bar reaches up from the window's bottom
                        readonly property real reach: height + gap
                        // the content pane's centre at its final width, so the pill doesn't trail a sliding panel;
                        // the pane sits at the end of main in right-to-left layouts
                        readonly property real paneCentre: main.x + (I18n.isRtl ? main.width - contentPane.pageWidth / 2 : contentPane.pageWidth / 2)
                        visible: barReveal > 0
                        floating: window.floatingBar
                        x: floating ? paneCentre - width / 2 : Theme.windowInset
                        // a compact window gives the pill its whole width
                        width: floating ? Math.min(window.compact ? Number.POSITIVE_INFINITY : PlayerMetrics.floatingMaxWidth, contentPane.pageWidth - PlayerMetrics.floatingMargin * 2) : parent.width - Theme.windowInset * 2
                        anchors.bottom: parent.bottom
                        // at 0 its top edge is the window's bottom, which is where the page's edge rests without it
                        anchors.bottomMargin: gap - reach * (1 - barReveal)
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

                    // without the top bar the view reaches to the window's top, at this size all the time
                    readonly property real reach: window.headerHidden || window.titleBarsOff ? window.topHeight : 0

                    y: -reach
                    width: parent.width
                    height: parent.height + reach
                    opacity: entry
                    enabled: body.fullWanted || entry > 0
                    asynchronous: true
                    active: body.fullWanted || body.fullReveal > 0
                    sourceComponent: NowPlayingView {}
                    // only here is the view in the scene, so Tab starts in it and not at the window's controls
                    onLoaded: (item as NowPlayingView)?.forceActiveFocus()

                    transform: Scale {
                        origin.x: card.sourcePos.x + card.source.width / 2
                        origin.y: card.sourcePos.y + card.source.height / 2 + viewLoader.reach
                        xScale: 1 - (1 - PlayerMetrics.pageEnterScale) * Math.pow(1 - viewLoader.entry, 2)
                        yScale: xScale
                    }
                }
            }

            // instead of stale data while the bridge redials; the next link reloads everything
            Item {
                width: parent.width
                height: parent.height - window.topHeight
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

        DSheetHandle {
            anchors.horizontalCenter: parent.horizontalCenter
            anchors.top: parent.top
            height: window.titleBarsOff ? window.topHeight : implicitHeight
            z: Theme.zHeader
            opacity: window.titleBarsOff ? (window.handleShown ? 1 : 0) : window.headerHidden ? viewLoader.entry : 0
            visible: opacity > 0
            enabled: window.titleBarsOff || body.fullWanted
            Accessible.name: I18n.tr("Move window", "drag handle of the full view")
            onDragStarted: windowControls.tryStartMove()
        }

        AddToPlaylistDialog {}

        DevicePickerDialog {}

        SleepTimerDialog {}

        SpeedDialog {}

        Snackbar {
            z: Theme.zToast
            bottomInset: DaemonBridge.connected && !Router.nowPlayingOpen ? bottomBar.reach * bottomBar.barReveal : 0
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
