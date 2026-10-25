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
    readonly property var starts: ["bar", "side", "full"]
    readonly property var openPanes: ["last", "lyrics", "queue", "cover"]
    readonly property bool twoColumns: overview.width >= SettingsMetrics.compactBreakpoint

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

    // one mark for the start surface and the player style in use
    function badgeOf(id) {
        const np = UiConfig.nowPlaying;
        const marked = id === "bar" || id === "side" ? np.start === id : (id === "fullBento") === (np.style === "bento");
        return marked ? I18n.tr("Default", "now playing surface badge: the start surface, or the player style in use") : "";
    }

    function summaryOf(id) {
        const surface = UiConfig.surface(id);
        const count = Object.keys(surface.zones).filter(z => z !== "panes").reduce((n, z) => n + surface.zones[z].filter(i => !i.hidden).length, 0);
        const elements = count === 1 ? I18n.tr("1 element", "now playing surface summary, singular") : I18n.tr("%1 elements", "now playing surface summary, plural").arg(count);
        return id === "side" && surface.panesFollowBar ? I18n.tr("%1 · follows the bar", "side panel summary, %1 is the element count").arg(elements) : elements;
    }

    function resetAll() {
        for (const id of surfaces)
            UiConfig.resetSurface(id);
        UiConfig.setNowPlaying({
            "start": "bar",
            "style": "bento",
            "openPane": "last"
        });
    }

    SettingsPage {
        id: overview
        visible: root.subPage === ""

        SettingsCard {
            SettingsButtonGroupRow {
                text: I18n.tr("Start with", "now playing setting: where now playing shows at start")
                description: I18n.tr("The view button in the header changes it only temporarily.", "now playing start description")
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
                text: I18n.tr("Player style", "now playing setting")
                model: [I18n.tr("Bento", "now playing player style"), I18n.tr("Material", "now playing player style")]
                currentIndex: UiConfig.playerStyles.indexOf(UiConfig.nowPlaying.style)
                onSelectionChanged: (index, selected) => {
                    if (selected)
                        UiConfig.setNowPlaying({
                            "style": UiConfig.playerStyles[index]
                        });
                }
            }

            SettingsButtonGroupRow {
                text: I18n.tr("Full view opens with", "now playing setting")
                model: [I18n.tr("Last used", "full view opens with: the pane used last"), I18n.tr("Lyrics", "song lyrics: page, toggle, settings tab"), I18n.tr("Queue", "the play queue: panel, toggle"), I18n.tr("Cover only", "full view opens with: no lyrics or queue pane")]
                currentIndex: root.openPanes.indexOf(UiConfig.nowPlaying.openPane)
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
        }

        Grid {
            id: cards
            width: parent.width
            columns: root.twoColumns ? 2 : 1
            spacing: Theme.spacingL

            Repeater {
                model: root.surfaces

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
                subtitle: I18n.tr("Restores where it starts, the style and every surface.", "now playing reset description")
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
        supportingText: I18n.tr("The bar, the side panel and both full views return to their defaults.", "reset now playing confirmation")
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
        BentoPlayer {}
    }

    Component {
        id: m3ePreview
        M3EPlayer {}
    }
}
