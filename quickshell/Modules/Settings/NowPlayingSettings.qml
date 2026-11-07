pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Modules
import qs.DCommon.Widgets

// Now playing: where it starts, the style, what the full view opens with, and a card per surface that opens
// its editor.
Item {
    id: root

    // a surface id: its editor is open (SettingsWindow sets it for "nowplaying/<surface>")
    property string subPage: ""

    readonly property var surfaces: ["bar", "side", "fullBento", "fullM3e"]
    // one card for the full view: the layout of its current style
    readonly property var cards: ["bar", "side", UiConfig.nowPlaying.style === "m3e" ? "fullM3e" : "fullBento"]
    readonly property var styleNames: [I18n.tr("Bento", "now playing player style"), I18n.tr("Material", "now playing player style")]
    // sub-views the settings search opens
    readonly property var subViews: surfaces
    readonly property var starts: ["bar", "side", "full"]
    // the Lyrics option is left out while lyrics are off
    readonly property var openPanes: UiConfig.lyricsOn ? ["last", "lyrics", "queue", "cover"] : ["last", "queue", "cover"]
    readonly property var playerLooks: ["card", "attached"]
    readonly property var paneLooks: ["player", "card", "attached"]
    readonly property bool twoColumns: overview.width >= SettingsMetrics.overviewTwoColumnWidth

    function surfaceLabel(id) {
        switch (id) {
        case "side":
            return I18n.tr("Side panel", "now playing placement option");
        case "fullBento":
            return I18n.tr("Full view · Bento", "now playing surface: the full view in the Bento style");
        case "fullM3e":
            return I18n.tr("Full view · Material", "now playing surface: the full view in the Material style");
        }
        return I18n.tr("Bottom bar", "now playing placement option");
    }

    function previewOf(id) {
        switch (id) {
        case "side":
            return sidePreview;
        case "fullBento":
            return bentoPreview;
        case "fullM3e":
            return m3ePreview;
        }
        return barPreview;
    }

    // the size a surface's preview is laid out at; height 0 = the component's own
    function previewSizeOf(id) {
        switch (id) {
        case "bar":
            return Qt.size(PlayerMetrics.compactBreakpoint, 0);
        case "side":
            return Qt.size(PlayerMetrics.sidePanelWidth, PlayerMetrics.sidePanelWidth * 1.4);
        }
        return Qt.size(PlayerMetrics.compactBreakpoint, PlayerMetrics.compactBreakpoint * 0.6);
    }

    function badgeOf(id) {
        const np = UiConfig.nowPlaying;
        const marked = np.start === (id === "bar" || id === "side" ? id : "full");
        return marked ? I18n.tr("Default", "now playing surface badge: the start surface, or the player style in use") : "";
    }

    function summaryOf(id) {
        const surface = UiConfig.surface(id);
        const count = Object.keys(surface.zones).filter(z => z !== "panes").reduce((n, z) => n + surface.zones[z].filter(i => !i.hidden).length, 0);
        const elements = count === 1 ? I18n.tr("1 element", "now playing surface summary, singular") : I18n.tr("%1 elements", "now playing surface summary, plural").arg(count);
        return id === "side" && surface.panesFollowBar ? I18n.tr("%1 · follows the bar", "side panel summary, %1 is the element count").arg(elements) : elements;
    }

    function openSubView(id) {
        subPage = id;
    }

    function resetAll() {
        for (const id of surfaces)
            UiConfig.resetSurface(id);
        UiConfig.setNowPlaying({
            "start": "bar",
            "style": "bento",
            "openPane": "last",
            "barStyle": "bento",
            "sideStyle": "bento",
            "playerLook": "card",
            "paneLook": "player"
        });
    }

    SettingsPage {
        id: overview
        visible: root.subPage === ""

        SettingsCard {
            SettingsButtonGroupRow {
                text: I18n.tr("Start with", "now playing setting: where now playing shows at start")
                description: I18n.tr("Switching with the header button is temporary.", "now playing start description")
                model: [I18n.tr("Bottom bar", "now playing placement option"), I18n.tr("Side panel", "now playing placement option"), I18n.tr("Full view", "now playing start option")]
                currentIndex: root.starts.indexOf(UiConfig.nowPlaying.start)
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        UiConfig.setNowPlaying({
                            "start": root.starts[index]
                        });
                }
            }

            SettingsButtonGroupRow {
                text: I18n.tr("Player look", "now playing setting")
                keywords: "bar bottom side panel lyrics queue pane flat background"
                model: [I18n.tr("Card", "player look option"), I18n.tr("Attached", "player look option")]
                currentIndex: root.playerLooks.indexOf(UiConfig.nowPlaying.playerLook)
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        UiConfig.setNowPlaying({
                            "playerLook": root.playerLooks[index]
                        });
                }
            }

            SettingsButtonGroupRow {
                text: I18n.tr("Lyrics and queue look", "now playing setting")
                keywords: "side panel pane flat background"
                model: [I18n.tr("Same as player", "lyrics and queue look option"), I18n.tr("Card", "player look option"), I18n.tr("Attached", "player look option")]
                currentIndex: root.paneLooks.indexOf(UiConfig.nowPlaying.paneLook)
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        UiConfig.setNowPlaying({
                            "paneLook": root.paneLooks[index]
                        });
                }
            }

            SettingsButtonGroupRow {
                text: I18n.tr("Full view opens with", "now playing setting")
                model: root.openPanes.map(p => ({
                            "last": I18n.tr("Last used", "full view opens with: the pane used last"),
                            "lyrics": I18n.tr("Lyrics", "song lyrics: page, toggle, settings tab"),
                            "queue": I18n.tr("Queue", "the play queue: panel, toggle"),
                            "cover": I18n.tr("Cover only", "full view opens with: no lyrics or queue pane")
                        })[p])
                // lyrics off: the view opens with the cover, whatever is saved
                currentIndex: root.openPanes.indexOf(UiConfig.nowPlaying.openPane === "lyrics" && !UiConfig.lyricsOn ? "cover" : UiConfig.nowPlaying.openPane)
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        UiConfig.setNowPlaying({
                            "openPane": root.openPanes[index]
                        });
                }
            }
        }

        SectionLabel {
            text: I18n.tr("Surfaces", "now playing settings section: bar, side panel and full views")
            keywords: "bar side panel full view layout controls"
        }

        SettingsCard {
            SettingsButtonGroupRow {
                text: I18n.tr("Bottom bar", "now playing placement option")
                keywords: "bento material style"
                model: root.styleNames
                currentIndex: UiConfig.playerStyles.indexOf(UiConfig.nowPlaying.barStyle)
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        UiConfig.setNowPlaying({
                            "barStyle": UiConfig.playerStyles[index]
                        });
                }
            }

            SettingsButtonGroupRow {
                text: I18n.tr("Side panel", "now playing placement option")
                keywords: "bento material style"
                model: root.styleNames
                currentIndex: UiConfig.playerStyles.indexOf(UiConfig.nowPlaying.sideStyle)
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        UiConfig.setNowPlaying({
                            "sideStyle": UiConfig.playerStyles[index]
                        });
                }
            }

            SettingsButtonGroupRow {
                text: I18n.tr("Full view", "now playing placement option")
                keywords: "bento material style"
                model: root.styleNames
                currentIndex: UiConfig.playerStyles.indexOf(UiConfig.nowPlaying.style)
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        UiConfig.setNowPlaying({
                            "style": UiConfig.playerStyles[index]
                        });
                }
            }
        }

        Grid {
            id: cards
            width: parent.width
            columns: root.twoColumns ? 2 : 1
            spacing: Theme.spacingL

            Repeater {
                model: root.cards

                SurfaceCard {
                    required property string modelData
                    width: (cards.width - cards.spacing * (cards.columns - 1)) / cards.columns
                    title: root.surfaceLabel(modelData)
                    badge: root.badgeOf(modelData)
                    summary: root.summaryOf(modelData)
                    warning: UiConfig.problems(modelData)[0] ?? ""
                    preview: root.previewOf(modelData)
                    previewSize: root.previewSizeOf(modelData)
                    onClicked: root.subPage = modelData
                }
            }
        }

        SettingsCard {
            SettingsRow {
                title: I18n.tr("Reset all now playing", "now playing settings action")
                keywords: "defaults"
                subtitle: I18n.tr("Resets start view, style and surfaces.", "now playing reset description")
                iconName: "restart_alt"
                clickable: true
                onClicked: resetDialog.ask()
            }
        }
    }

    ConfirmDialog {
        id: resetDialog
        host: root
        title: I18n.tr("Reset now playing?", "reset now playing confirmation title")
        supportingText: I18n.tr("Resets the bar, side panel and both full views.", "reset now playing confirmation")
        confirmText: I18n.tr("Reset", "button that restores the defaults")
        onConfirmed: root.resetAll()
    }

    Loader {
        anchors.fill: parent
        active: root.surfaces.indexOf(root.subPage) >= 0
        sourceComponent: SurfaceEditor {
            surfaceId: root.subPage
            title: root.surfaceLabel(root.subPage)
            preview: root.previewOf(root.subPage)
            previewSize: root.previewSizeOf(root.subPage)
            onBack: root.subPage = ""
        }
    }

    Component {
        id: barPreview
        NowPlayingBar {}
    }

    Component {
        id: sidePreview
        NowPlayingSide {}
    }

    Component {
        id: bentoPreview
        BentoPlayer {
            preview: true
        }
    }

    Component {
        id: m3ePreview
        M3EPlayer {
            preview: true
        }
    }
}
