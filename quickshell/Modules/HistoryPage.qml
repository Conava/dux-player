pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets

// Every play, newest first, grouped by day.
Item {
    id: root

    readonly property var entries: {
        const today = new Date();
        today.setHours(0, 0, 0, 0);
        const dayMs = 86400000;
        return PlayerService.history.map(h => {
            const t = PlayerService.track(h);
            if (!t)
                return null;
            const at = new Date(h.playedAt * 1000);
            const day = new Date(at);
            day.setHours(0, 0, 0, 0);
            const ago = Math.round((today - day) / dayMs);
            const label = ago === 0 ? I18n.tr("Today", "history day header") : ago === 1 ? I18n.tr("Yesterday", "history day header") : at.toLocaleDateString(Qt.locale(), Locale.LongFormat);
            return Object.assign({}, t, {
                "day": label
            });
        }).filter(t => t !== null);
    }

    readonly property var options: UiConfig.pageOptions("history", "")
    readonly property bool editing: Router.editing

    Component.onCompleted: PlayerService.loadHistory(false)

    TrackList {
        anchors.fill: parent
        anchors.leftMargin: PlayerMetrics.pagePadding
        anchors.rightMargin: PlayerMetrics.pagePadding
        tracks: root.entries
        context: ({
                "type": "history"
            })
        lines: root.options.lines
        sectionProperty: "day"
        onEndReached: PlayerService.loadHistory(true)
        header: Component {
            Column {
                width: parent?.width ?? 0
                spacing: Theme.spacingS
                bottomPadding: root.editing ? Theme.spacingM : 0

                PageHeader {
                    title: I18n.tr("History", "navigation destination")
                    subtitle: PlayerService.historyEnd ? Phrases.plays(root.entries.length) : ""
                    editable: true
                    editing: root.editing
                    onEditRequested: Router.editing = true
                    onDone: Router.editing = false
                    onResetRequested: editor.reset()

                    DButton {
                        anchors.verticalCenter: parent.verticalCenter
                        visible: root.entries.length > 0
                        text: I18n.tr("Clear history", "history page action")
                        backgroundColor: "transparent"
                        textColor: Theme.primary
                        onClicked: clearDialog.opened = true
                    }
                }

                PageEditor {
                    id: editor
                    visible: root.editing
                    sheetHost: root
                    kind: "history"
                    type: "tracks"
                    itemType: "track"
                }
            }
        }
    }

    ConfirmDialog {
        id: clearDialog
        host: root
        title: I18n.tr("Clear history?", "clear history confirmation title")
        supportingText: I18n.tr("Play counts are reset. Favorites and ratings are kept.", "clear history confirmation")
        confirmText: I18n.tr("Clear", "button that empties a list")
        onConfirmed: PlayerService.clearHistory()
    }
}
