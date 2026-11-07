import QtQuick
import qs.Common

Column {
    id: root

    readonly property bool isSettingsGroup: true
    // found once per layout here, not by every row: rows scanning their siblings made a group of n rows cost
    // n³ while a Repeater filled it (a minute for 500 cached songs). Not a binding on visibleChildren: a Repeater
    // moves its new row into place after adding it at the end, and that move doesn't notify.
    property Item firstRow: null
    property Item lastRow: null

    function edgeRow(first) {
        const kids = visibleChildren;
        let last = null;
        for (let i = 0; i < kids.length; i++) {
            if (kids[i].isSettingsRow !== true)
                continue;
            if (first)
                return kids[i];
            last = kids[i];
        }
        return last;
    }

    width: parent?.width ?? 0
    spacing: Theme.groupedListGap
    onPositioningComplete: {
        firstRow = edgeRow(true);
        lastRow = edgeRow(false);
    }
}
