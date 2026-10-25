pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import qs.Services
import qs.Modules
import qs.DCommon.Widgets
import qs.Widgets

DFloatingWindow {
    id: window

    property string currentPage: "libraries"
    property bool menuVisible: false
    readonly property bool isCompactMode: width < SettingsMetrics.compactBreakpoint

    readonly property var tabs: [
        {
            "id": "libraries",
            "accent": "green",
            "label": I18n.tr("Libraries", "the music libraries: page, settings tab, section"),
            "hint": I18n.tr("Folders and servers", "settings sidebar tab hint"),
            "icon": "library_music"
        },
        {
            "id": "playback",
            "accent": "blue",
            "label": I18n.tr("Playback", "settings sidebar tab label"),
            "hint": I18n.tr("Equalizer, crossfade, speed", "settings sidebar tab hint"),
            "icon": "graphic_eq"
        },
        {
            "id": "lyrics",
            "accent": "blue",
            "label": I18n.tr("Lyrics", "song lyrics: page, toggle, settings tab"),
            "hint": I18n.tr("Providers and display", "settings sidebar tab hint"),
            "icon": "lyrics"
        },
        {
            "id": "scrobble",
            "accent": "blue",
            "label": I18n.tr("Scrobbling", "scrobbling: feature and settings tab"),
            "hint": I18n.tr("ListenBrainz and Last.fm", "settings sidebar tab hint"),
            "icon": "podcasts"
        },
        {
            "id": "devices",
            "accent": "blue",
            "label": I18n.tr("Playback device", "playback device: settings tab, button, dialog"),
            "hint": I18n.tr("Outputs, casting and sync", "settings sidebar tab hint"),
            "icon": "speaker"
        },
        {
            "id": "nowplaying",
            "accent": "pink",
            "label": I18n.tr("Now playing", "settings sidebar tab label"),
            "hint": I18n.tr("Layout and controls", "settings sidebar tab hint"),
            "icon": "play_circle"
        },
        {
            "id": "pages",
            "accent": "pink",
            "label": I18n.tr("Pages", "settings sidebar tab label"),
            "hint": I18n.tr("Navigation and page layout", "settings sidebar tab hint"),
            "icon": "dashboard_customize"
        },
        {
            "id": "appearance",
            "accent": "purple",
            "label": I18n.tr("Appearance", "settings sidebar tab label"),
            "hint": StyleSource.following ? I18n.tr("Following Dux Shell", "settings sidebar tab hint") : I18n.tr("Theme, shape and motion", "settings sidebar tab hint"),
            "icon": "palette"
        },
        {
            "id": "general",
            "accent": "purple",
            "label": I18n.tr("General", "settings sidebar tab label"),
            "hint": I18n.tr("Background, windows and shortcuts", "settings sidebar tab hint"),
            "icon": "tune"
        },
        {
            "id": "about",
            "accent": "purple",
            "label": I18n.tr("About", "settings sidebar tab label"),
            "hint": I18n.tr("Version, license and credits", "settings sidebar tab hint"),
            "icon": "info"
        }
    ]
    readonly property var currentTab: tabs.find(t => t.id === currentPage) ?? tabs[0]
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
    }

    function showPage(id, sub) {
        pendingSubPage = sub;
        if (currentPage === id)
            applySubPage();
        currentPage = id;
    }

    function applySubPage() {
        const page = pageLoader.item as NowPlayingSettings;
        if (page)
            page.subPage = pendingSubPage;
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

                DFlickable {
                    id: sidebar
                    anchors.left: parent.left
                    anchors.top: parent.top
                    anchors.bottom: parent.bottom
                    width: window.isCompactMode ? parent.width : SettingsMetrics.sidebarWidth
                    visible: window.isCompactMode ? window.menuVisible : true
                    contentWidth: width
                    contentHeight: navColumn.height
                    clip: true

                    Column {
                        id: navColumn
                        width: parent.width
                        padding: SettingsMetrics.paneMargin
                        topPadding: 0
                        spacing: Theme.groupedListGap

                        Repeater {
                            model: window.tabs

                            SettingsSidebarItem {
                                required property var modelData
                                required property int index
                                width: navColumn.width - navColumn.padding * 2
                                iconName: modelData.icon
                                title: modelData.label
                                hint: modelData.hint
                                accent: modelData.accent
                                active: window.currentTab.id === modelData.id
                                isFirstInGroup: index === 0
                                isLastInGroup: index === window.tabs.length - 1
                                onClicked: {
                                    window.showPage(modelData.id, "");
                                    window.menuVisible = false;
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

                    Loader {
                        id: pageLoader
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.top: pageHeading.bottom
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
}
