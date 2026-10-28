pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets
import qs.Widgets
import "SettingsSearch.js" as SettingsSearch
import "SettingsSearchIndex.js" as SettingsSearchIndex

DFloatingWindow {
    id: window

    property string currentPage: "libraries"
    property bool menuVisible: false
    readonly property bool isCompactMode: width < SettingsMetrics.compactBreakpoint

    readonly property var tabs: [
        {
            "id": "libraries",
            "group": 0,
            "accent": "green",
            "label": I18n.tr("Libraries", "the music libraries: page, settings tab, section"),
            "hint": I18n.tr("Folders and servers", "settings sidebar tab hint"),
            "icon": "library_music"
        },
        {
            "id": "general",
            "group": 1,
            "accent": "purple",
            "label": I18n.tr("General", "settings sidebar tab label"),
            "hint": I18n.tr("Background, windows and shortcuts", "settings sidebar tab hint"),
            "icon": "tune"
        },
        {
            "id": "appearance",
            "group": 1,
            "accent": "purple",
            "label": I18n.tr("Appearance", "settings sidebar tab label"),
            "hint": StyleSource.following ? I18n.tr("Following Dux Shell", "settings sidebar tab hint") : I18n.tr("Theme, shape and motion", "settings sidebar tab hint"),
            "icon": "palette"
        },
        {
            "id": "pages",
            "group": 1,
            "accent": "purple",
            "label": I18n.tr("Pages", "settings sidebar tab label"),
            "hint": I18n.tr("Navigation and page layout", "settings sidebar tab hint"),
            "icon": "dashboard_customize"
        },
        {
            "id": "nowplaying",
            "group": 1,
            "accent": "purple",
            "label": I18n.tr("Now playing", "settings sidebar tab label"),
            "hint": I18n.tr("Layout and controls", "settings sidebar tab hint"),
            "icon": "play_circle"
        },
        {
            "id": "playback",
            "group": 2,
            "accent": "blue",
            "label": I18n.tr("Playback", "settings sidebar tab label"),
            "hint": I18n.tr("Equalizer, crossfade, speed", "settings sidebar tab hint"),
            "icon": "graphic_eq"
        },
        {
            "id": "lyrics",
            "group": 2,
            "accent": "blue",
            "label": I18n.tr("Lyrics", "song lyrics: page, toggle, settings tab"),
            "hint": I18n.tr("Providers and display", "settings sidebar tab hint"),
            "icon": "lyrics"
        },
        {
            "id": "scrobble",
            "group": 2,
            "accent": "blue",
            "label": I18n.tr("Scrobbling", "scrobbling: feature and settings tab"),
            "hint": I18n.tr("ListenBrainz and Last.fm", "settings sidebar tab hint"),
            "icon": "podcasts"
        },
        {
            "id": "devices",
            "group": 2,
            "accent": "blue",
            "label": I18n.tr("Playback device", "playback device: settings tab, button, dialog"),
            "hint": I18n.tr("Outputs, casting and sync", "settings sidebar tab hint"),
            "icon": "speaker"
        },
        {
            "id": "about",
            "group": 3,
            "accent": "pink",
            "label": I18n.tr("About", "settings sidebar tab label"),
            "hint": I18n.tr("Version, license and credits", "settings sidebar tab hint"),
            "icon": "info"
        }
    ]
    readonly property var tabGroups: tabs.reduce((all, t) => {
        if (all.length <= t.group)
            all.push([]);
        all[t.group].push(t);
        return all;
    }, [])
    readonly property var currentTab: tabOf(currentPage)
    // the sub-page to show, e.g. "bar" for "nowplaying/bar" (the bar's editor)
    property string pendingSubPage: ""

    // page: "" (as it was), a tab id, or "<tab>/<sub-page>"
    function open(page) {
        if (page) {
            const slash = page.indexOf("/");
            showPage(slash < 0 ? page : page.slice(0, slash), slash < 0 ? "" : page.slice(slash + 1));
            menuVisible = false;
        }
        visible = true;
        if (!page && !isCompactMode)
            searchField.forceActiveFocus();
    }

    function focusSearch() {
        if (isCompactMode)
            menuVisible = true;
        searchField.forceActiveFocus();
    }

    function showPage(id, sub) {
        pendingSubPage = sub;
        revealHidden = false;
        if (currentPage === id)
            applySubPage();
        currentPage = id;
    }

    // M3 search bar height
    readonly property real searchBarHeight: Theme.buttonHeightM
    // how often a search result's page is checked for its row while the page lays out, and for how long
    readonly property int revealPollInterval: 50
    readonly property int revealMaxTries: 20
    readonly property int searchHighlightDuration: 2500

    readonly property bool searching: searchField.text.trim() !== ""
    readonly property var searchEntries: tabs.map(t => ({
                "page": t.id,
                "title": t.label,
                "text": t.hint,
                "isPage": true
            })).concat(SettingsSearchIndex.entries.map(e => ({
                "page": e.page,
                "pageLabel": tabLabel(e.page),
                "title": translated(e.title),
                "section": e.section ? translated(e.section) : "",
                "text": (e.text ?? []).map(translated).join(" "),
                "keywords": e.keywords ?? ""
            }))).concat(dynamicEntries)
    // rows the pages build from service lists, which the generated index can't see
    readonly property var dynamicEntries: PlayerService.libraries.map(l => dynamicEntry("libraries", "", l.name, PlayerService.kindLabel(l.kind))).concat(PlayerService.lyricsProviders.map(p => dynamicEntry("lyrics", I18n.tr("Providers", "lyrics settings section label"), p.id === "server" ? I18n.tr("Your server", "lyrics provider row for a server library's own lyrics") : p.name, ""))).concat(Object.keys(UiConfig.shortcutDefaults).map(k => dynamicEntry("general", I18n.tr("Keyboard shortcuts", "general settings section label"), UiConfig.shortcutLabel(k), UiConfig.shortcut(k)))).concat(PlayerService.outputDevices.map(d => dynamicEntry("devices", I18n.tr("Outputs", "playback device settings section"), d.name, ""))).concat(offlineEntries)
    // each server library's offline overview: picking one opens that library's
    readonly property var offlineEntries: PlayerService.libraries.filter(l => PlayerService.isRemote(l.id)).map(l => Object.assign(dynamicEntry("libraries", I18n.tr("Offline rules for %1", "library offline overview section, %1 is the library").arg(l.name), I18n.tr("Clear downloads", "offline action for one library"), I18n.tr("Deletes downloaded tracks. Rules are kept.", "offline clear action description")), {
            "cacheLibrary": l.id
        }))
    readonly property var searchResults: searching ? SettingsSearch.search(searchEntries, searchField.text) : []
    property int searchSelected: 0
    property string revealTitle: ""
    property int revealTries: 0
    property real revealLastY: -1
    property SettingsRow highlightedRow: null
    // the picked result's row exists but the current setup hides it
    property bool revealHidden: false

    function dynamicEntry(page, section, title, text) {
        return {
            "page": page,
            "pageLabel": tabLabel(page),
            "title": title,
            "section": section,
            "text": text
        };
    }

    // an index pair: [source, context] for I18n.tr, or [text, ""] for a brand name
    function translated(pair) {
        return pair[1] === "" ? pair[0] : I18n.tr(pair[0], pair[1]);
    }

    function tabOf(id) {
        return tabs.find(t => t.id === id) ?? tabs[0];
    }

    function tabLabel(id) {
        return tabOf(id).label;
    }

    function openSearchResult(entry) {
        showPage(entry.page, "");
        if (entry.cacheLibrary)
            (pageLoader.item as LibrariesSettings)?.openCachePage(entry.cacheLibrary);
        menuVisible = false;
        clearHighlight();
        revealTitle = entry.isPage ? "" : entry.title;
        revealTries = 0;
        revealLastY = -1;
        if (revealTitle !== "")
            revealTimer.restart();
    }

    // the first row, card or section label on the page with this title; with hidden, also one that isn't shown
    function findOnPage(item, title, hidden) {
        if (!item || !(item.visible || hidden))
            return null;
        if (item.title === title || item.text === title)
            return item;
        for (let i = 0; i < item.children.length; i++) {
            const hit = findOnPage(item.children[i], title, hidden);
            if (hit)
                return hit;
        }
        return null;
    }

    function reveal() {
        let target = findOnPage(pageLoader.item, revealTitle, false);
        if (!target) {
            target = findOnPage(pageLoader.item, revealTitle, true);
            if (target && !openSubViewOf(target)) {
                revealTimer.stop();
                revealHidden = true;
                return;
            }
        }
        let flick = null;
        for (let p = target?.parent; p && !flick; p = p.parent)
            flick = p as DFlickable;
        if (!flick) {
            if (++revealTries >= revealMaxTries)
                revealTimer.stop();
            return;
        }
        const y = target.mapToItem(flick.contentItem, 0, 0).y;
        flick.revealRange(y, y + flick.height);
        // the page may still be laying out: done once the target stays put
        if (y !== revealLastY && ++revealTries < revealMaxTries) {
            revealLastY = y;
            return;
        }
        revealTimer.stop();
        highlightedRow = target as SettingsRow;
        if (!highlightedRow)
            return;
        highlightedRow.highlighted = true;
        highlightTimer.restart();
    }

    function clearHighlight() {
        if (highlightedRow)
            highlightedRow.highlighted = false;
        highlightedRow = null;
    }

    // also returns Libraries and Playback device to their main view
    function applySubPage() {
        const nowPlaying = pageLoader.item as NowPlayingSettings;
        if (nowPlaying)
            nowPlaying.subPage = pendingSubPage;
        const libraries = pageLoader.item as LibrariesSettings;
        if (libraries)
            libraries.cachePage = "";
        const devices = pageLoader.item as DevicesSettings;
        if (devices)
            devices.syncPage = false;
    }

    // a hidden row that is on a sub-view: opens the sub-view when it can be opened
    function openSubViewOf(item) {
        const devices = pageLoader.item as DevicesSettings;
        if (devices && devices.syncing.length > 1) {
            devices.syncPage = true;
            if (item.visible)
                return true;
            devices.syncPage = false;
        }
        return false;
    }

    title: I18n.tr("Dux Player Settings", "settings window title")
    minimumSize: Qt.size(SettingsMetrics.windowMinWidth, SettingsMetrics.windowMinHeight)
    implicitWidth: SettingsMetrics.windowWidth
    implicitHeight: screen ? Math.min(SettingsMetrics.windowHeight, screen.height - PlayerMetrics.screenMargin) : SettingsMetrics.windowHeight
    surfaceColor: Theme.withAlpha(Theme.surface, Theme.floatingWindowTransparency)
    visible: false
    onClosed: visible = false
    onIsCompactModeChanged: menuVisible = false

    FocusScope {
        anchors.fill: parent
        focus: true

        LayoutMirroring.enabled: I18n.isRtl
        LayoutMirroring.childrenInherit: true

        Keys.onEscapePressed: window.visible = false

        Shortcut {
            sequence: StandardKey.Find
            onActivated: window.focusSearch()
        }

        Column {
            anchors.fill: parent
            spacing: 0

            DWindowHeader {
                id: header
                width: parent.width
                z: Theme.zHeader
                controls: windowControls
                title: I18n.tr("Settings", "the settings window")
                onCloseRequested: window.visible = false

                DActionButton {
                    visible: window.isCompactMode
                    iconName: "menu"
                    buttonSize: Theme.buttonHeightXXS
                    Accessible.name: I18n.tr("Menu", "settings header button that shows the section list on narrow windows")
                    onClicked: window.menuVisible = !window.menuVisible
                }
            }

            Item {
                width: parent.width
                height: parent.height - header.height
                clip: true

                Item {
                    id: sidebar
                    anchors.left: parent.left
                    anchors.top: parent.top
                    anchors.bottom: parent.bottom
                    width: window.isCompactMode ? parent.width : SettingsMetrics.sidebarWidth
                    visible: window.isCompactMode ? window.menuVisible : true

                    DSearchField {
                        id: searchField
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.top: parent.top
                        anchors.leftMargin: SettingsMetrics.paneMargin
                        anchors.rightMargin: SettingsMetrics.paneMargin
                        height: window.searchBarHeight
                        placeholderText: I18n.tr("Search settings", "settings search field placeholder")
                        keyForwardTargets: [searchKeys]
                        onTextChanged: {
                            window.searchSelected = 0;
                            navFlick.contentY = 0;
                        }

                        Item {
                            id: searchKeys

                            function move(delta) {
                                window.searchSelected = Math.max(0, Math.min(window.searchSelected + delta, window.searchResults.length - 1));
                                const row = resultsRepeater.itemAt(window.searchSelected);
                                if (row) {
                                    const top = row.mapToItem(navFlick.contentItem, 0, 0).y;
                                    navFlick.revealRange(top, top + row.height);
                                }
                            }

                            function pick() {
                                const entry = window.searchResults[window.searchSelected];
                                if (entry)
                                    window.openSearchResult(entry);
                            }

                            Keys.onDownPressed: event => {
                                event.accepted = window.searching;
                                if (event.accepted)
                                    move(1);
                            }
                            Keys.onUpPressed: event => {
                                event.accepted = window.searching;
                                if (event.accepted)
                                    move(-1);
                            }
                            Keys.onReturnPressed: event => {
                                event.accepted = window.searching;
                                if (event.accepted)
                                    pick();
                            }
                            Keys.onEnterPressed: event => {
                                event.accepted = window.searching;
                                if (event.accepted)
                                    pick();
                            }
                            // with nothing typed, Escape goes on and closes the window
                            Keys.onEscapePressed: event => {
                                event.accepted = searchField.text !== "";
                                if (event.accepted)
                                    searchField.text = "";
                            }
                        }
                    }

                    DFlickable {
                        id: navFlick
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.top: searchField.bottom
                        anchors.bottom: parent.bottom
                        anchors.topMargin: SettingsMetrics.navGroupGap
                        contentWidth: width
                        contentHeight: navColumn.height
                        clip: true

                        Column {
                            id: navColumn
                            width: parent.width
                            padding: SettingsMetrics.paneMargin
                            topPadding: 0
                            spacing: SettingsMetrics.navGroupGap

                            Column {
                                width: navColumn.width - navColumn.padding * 2
                                spacing: Theme.groupedListGap
                                visible: window.searching

                                Repeater {
                                    id: resultsRepeater
                                    model: window.searchResults

                                    SettingsSidebarItem {
                                        required property var modelData
                                        required property int index
                                        readonly property var tab: window.tabOf(modelData.page)
                                        width: parent?.width ?? 0
                                        iconName: tab.icon
                                        title: modelData.title
                                        hint: modelData.isPage ? tab.hint : tab.label
                                        accent: tab.accent
                                        active: window.searchSelected === index
                                        isFirstInGroup: index === 0
                                        isLastInGroup: index === window.searchResults.length - 1
                                        onClicked: {
                                            window.searchSelected = index;
                                            window.openSearchResult(modelData);
                                        }
                                    }
                                }

                                StyledText {
                                    width: parent.width
                                    visible: window.searchResults.length === 0
                                    topPadding: Theme.spacingM
                                    text: I18n.tr("Nothing matches.", "settings search empty result")
                                    font.pixelSize: Theme.fontSizeSmall
                                    color: Theme.surfaceVariantText
                                    horizontalAlignment: Text.AlignHCenter
                                }
                            }

                            Repeater {
                                model: window.tabGroups

                                Column {
                                    id: navGroup
                                    required property var modelData
                                    width: navColumn.width - navColumn.padding * 2
                                    spacing: Theme.groupedListGap
                                    visible: !window.searching

                                    Repeater {
                                        model: navGroup.modelData

                                        SettingsSidebarItem {
                                            required property var modelData
                                            required property int index
                                            width: navGroup.width
                                            iconName: modelData.icon
                                            title: modelData.label
                                            hint: modelData.hint
                                            accent: modelData.accent
                                            active: window.currentTab.id === modelData.id
                                            isFirstInGroup: index === 0
                                            isLastInGroup: index === navGroup.modelData.length - 1
                                            onClicked: {
                                                window.showPage(modelData.id, "");
                                                window.menuVisible = false;
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                Item {
                    id: pane
                    anchors.left: window.isCompactMode ? parent.left : sidebar.right
                    anchors.right: parent.right
                    anchors.top: parent.top
                    anchors.bottom: parent.bottom
                    anchors.leftMargin: window.isCompactMode ? SettingsMetrics.paneMargin : 0
                    anchors.rightMargin: SettingsMetrics.paneMargin
                    anchors.bottomMargin: SettingsMetrics.paneMargin
                    visible: !window.isCompactMode || !window.menuVisible

                    Rectangle {
                        anchors.fill: parent
                        radius: PlayerMetrics.paneRadius
                        color: SettingsMetrics.paneColor
                    }

                    StyledText {
                        id: pageHeading
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.top: parent.top
                        anchors.leftMargin: SettingsMetrics.panePadding
                        anchors.rightMargin: SettingsMetrics.panePadding
                        height: PlayerMetrics.pageHeaderHeight
                        verticalAlignment: Text.AlignVCenter
                        text: window.currentTab.label
                        font.pixelSize: Theme.fontSizeXXLarge
                        color: Theme.surfaceText
                        horizontalAlignment: Text.AlignLeft
                    }

                    StyledText {
                        id: hiddenNote
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.top: pageHeading.bottom
                        anchors.leftMargin: SettingsMetrics.panePadding
                        anchors.rightMargin: SettingsMetrics.panePadding
                        visible: window.revealHidden
                        bottomPadding: Theme.spacingS
                        text: I18n.tr("This setting isn't available in the current setup.", "settings search: the picked setting is hidden by other settings or missing hardware or software")
                        font.pixelSize: Theme.fontSizeSmall
                        color: Theme.surfaceVariantText
                        wrapMode: Text.WordWrap
                        horizontalAlignment: Text.AlignLeft
                        Accessible.role: Accessible.AlertMessage
                    }

                    Loader {
                        id: pageLoader
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.top: hiddenNote.visible ? hiddenNote.bottom : pageHeading.bottom
                        anchors.bottom: parent.bottom
                        anchors.leftMargin: SettingsMetrics.panePadding
                        anchors.rightMargin: SettingsMetrics.panePadding
                        source: {
                            switch (window.currentTab.id) {
                            case "playback":
                                return "PlaybackSettings.qml";
                            case "lyrics":
                                return "LyricsSettings.qml";
                            case "scrobble":
                                return "ScrobbleSettings.qml";
                            case "devices":
                                return "DevicesSettings.qml";
                            case "general":
                                return "GeneralSettings.qml";
                            case "nowplaying":
                                return "NowPlayingSettings.qml";
                            case "pages":
                                return "PagesSettings.qml";
                            case "appearance":
                                return "AppearanceSettings.qml";
                            case "about":
                                return "AboutSettings.qml";
                            }
                            return "LibrariesSettings.qml";
                        }
                        onLoaded: window.applySubPage()
                    }

                    // Pages' Edit: the page opens in the main window, which comes to the front
                    Connections {
                        target: pageLoader.item as PagesSettings
                        function onCloseRequested() {
                            window.visible = false;
                        }
                    }
                }
            }
        }

        Snackbar {
            z: Theme.zToast
        }
    }

    FloatingWindowControls {
        id: windowControls
        targetWindow: window
    }

    Timer {
        id: revealTimer
        interval: window.revealPollInterval
        repeat: true
        onTriggered: window.reveal()
    }

    Timer {
        id: highlightTimer
        interval: window.searchHighlightDuration
        onTriggered: window.clearHighlight()
    }
}
