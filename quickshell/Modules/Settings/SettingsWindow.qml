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
            "hint": I18n.tr("Background and windows", "settings sidebar tab hint"),
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
    readonly property var searchEntries: SettingsSearch.merge(tabs.map(t => ({
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
            }))), liveEntries)
    readonly property var searchResults: searching ? SettingsSearch.search(searchEntries, searchField.text) : []
    readonly property string unavailableNote: I18n.tr("This setting isn't available in the current setup.", "settings search: the picked setting is hidden by other settings or missing hardware or software")
    property int searchSelected: 0
    property string revealTitle: ""
    property int revealTries: 0
    property real revealLastY: -1
    property SettingsRow highlightedRow: null
    // the picked result's row exists but the current setup hides it
    property bool revealHidden: false
    property string revealReason: ""

    // The search reads each page once per window session from an unseen copy, its sub-views too: the rows built
    // from lists, and which rows the setup hides and why. About is left to the index, as loading it checks the
    // installed programs. A page whose list changes is read again at the next search.
    property var liveEntries: []
    property var probeQueue: []
    property var probeStale: []
    property string probePage: ""
    property bool probed: false

    function startProbe() {
        const pages = probed ? probeStale : tabs.map(t => t.id).filter(id => id !== "about");
        probed = true;
        probeStale = [];
        probeQueue = probeQueue.concat(pages.filter(p => probeQueue.indexOf(p) < 0));
        if (probePage === "")
            probeNext();
    }

    function probeNext() {
        probePage = probeQueue[0] ?? "";
        probeQueue = probeQueue.slice(1);
    }

    function markStale(page) {
        if (probed && probeStale.indexOf(page) < 0)
            probeStale = probeStale.concat([page]);
    }

    function resetProbe() {
        probed = false;
        probeQueue = [];
        probeStale = [];
        probePage = "";
        liveEntries = [];
    }

    function recordProbe(page) {
        const id = probePage;
        const views = [""].concat(page.subViews ?? []);
        let rows = [];
        for (let i = 0; i < views.length; i++) {
            page.openSubView?.(views[i], true);
            rows = rows.concat(probeRows(page, views[i]));
        }
        liveEntries = liveEntries.filter(e => e.page !== id).concat(rows.map(r => Object.assign(r, {
                "page": id,
                "pageLabel": tabLabel(id)
            })));
    }

    // rows and cards with a title; a hidden one takes the reason of its outermost hidden container that has one
    function probeRows(item, view) {
        const rows = [];
        const walk = (it, section, reason) => {
            if (!reason && !it.visible && it.unavailableReason)
                reason = it.unavailableReason;
            const row = it.isSettingsRow === true || it.isSettingsEntry === true;
            const card = it.isSettingsCard === true;
            if ((row || card) && it.title && it.searchable !== false)
                rows.push({
                    "title": it.title,
                    "section": card ? "" : section,
                    "text": it.subtitle ?? "",
                    "keywords": it.keywords ?? "",
                    "hidden": !it.visible,
                    "reason": it.visible ? "" : reason,
                    "subView": view
                });
            if (row)
                return;
            for (let i = 0; i < it.children.length; i++)
                walk(it.children[i], card ? it.title : section, reason);
        };
        walk(item, "", "");
        return rows;
    }

    function pageFile(id) {
        switch (id) {
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
        showPage(entry.page, entry.subView ?? "");
        revealReason = entry.reason ?? "";
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
        if (!target && findOnPage(pageLoader.item, revealTitle, true)) {
            revealTimer.stop();
            revealHidden = true;
            return;
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

    // "" returns a page with sub-views to its main view
    function applySubPage() {
        const page = pageLoader.item;
        page?.openSubView?.(pendingSubPage, false);
    }

    title: I18n.tr("Dux Player Settings", "settings window title")
    minimumSize: Qt.size(SettingsMetrics.windowMinWidth, SettingsMetrics.windowMinHeight)
    implicitWidth: SettingsMetrics.windowWidth
    implicitHeight: screen ? Math.min(SettingsMetrics.windowHeight, screen.height - PlayerMetrics.screenMargin) : SettingsMetrics.windowHeight
    surfaceColor: Theme.withAlpha(Theme.surface, Theme.floatingWindowTransparency)
    visible: false
    onClosed: visible = false
    onVisibleChanged: {
        if (!visible)
            resetProbe();
    }
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
                            if (window.searching)
                                window.startProbe();
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
                                        hint: {
                                            if (modelData.hidden)
                                                return modelData.reason || window.unavailableNote;
                                            if (modelData.isPage)
                                                return tab.hint;
                                            // sub-view rows repeat per library or surface: their section tells them apart
                                            return modelData.subView ? Phrases.joinAll([tab.label, modelData.section ?? ""]) : tab.label;
                                        }
                                        dimmed: modelData.hidden === true
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
                        text: window.revealReason || window.unavailableNote
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
                        source: window.pageFile(window.currentTab.id)
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

        // the search's unseen copy of a page: opacity, not visible, so each row's own visible binding can be read
        Item {
            width: pane.width
            height: 0
            clip: true
            opacity: 0
            enabled: false
            Accessible.ignored: true

            Loader {
                width: parent.width
                height: pane.height
                asynchronous: true
                active: window.probePage !== ""
                source: active ? window.pageFile(window.probePage) : ""
                onLoaded: {
                    window.recordProbe(item);
                    Qt.callLater(window.probeNext);
                }
                onStatusChanged: {
                    if (status === Loader.Error)
                        Qt.callLater(window.probeNext);
                }
            }
        }
    }

    Connections {
        target: PlayerService

        function onLibrariesChanged() {
            window.markStale("libraries");
            window.markStale("scrobble");
        }

        function onOutputDevicesChanged() {
            window.markStale("devices");
        }

        function onLyricsProvidersChanged() {
            window.markStale("lyrics");
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
