pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets

// Home shelf: one row of genre chips, the ones with the most albums first, each opening its genre.
Column {
    id: root

    required property string title

    readonly property var genres: PlayerService.query("genres", {
        "sort": ""
    }).slice().sort((a, b) => b.albumCount - a.albumCount || a.name.localeCompare(b.name))
    readonly property bool hasContent: genres.length > 0
    // about three chips, so an arrow press moves the row a few at a time
    readonly property real chipStep: Theme.buttonHeightXS * 4

    width: parent?.width ?? 0
    spacing: Theme.spacingS

    ShelfHeader {
        title: root.title
        seeAll: ["genres", ""]
        carousel: chipList
        tileStep: root.chipStep
    }

    // room around the chips for the focus ring
    DHorizontalList {
        id: chipList
        x: -Theme.spacingS
        width: parent.width + Theme.spacingS * 2
        height: Theme.buttonHeightXS + Theme.spacingXS * 2
        leftMargin: Theme.spacingS
        rightMargin: Theme.spacingS
        spacing: Theme.spacingS
        model: root.genres

        delegate: Item {
            id: cell
            required property var modelData
            height: chipList.height
            width: chip.width

            StyledButton {
                id: chip
                anchors.verticalCenter: parent.verticalCenter
                width: label.implicitWidth + Theme.spacingL * 2
                height: Theme.buttonHeightXS
                radius: Theme.cornerRadiusS
                color: "transparent"
                Accessible.role: Accessible.Button
                Accessible.name: cell.modelData.name
                onClicked: Router.go("genre", cell.modelData.id)

                StyledText {
                    id: label
                    anchors.centerIn: parent
                    text: cell.modelData.name
                    font.pixelSize: Theme.fontSizeMedium
                    font.weight: Theme.fontWeightMedium
                    color: Theme.surfaceVariantText
                }

                Rectangle {
                    anchors.fill: parent
                    radius: chip.radius
                    color: "transparent"
                    border.width: Theme.outlineWidth
                    border.color: Theme.outline
                }

                StateLayer {
                    control: chip
                    stateColor: Theme.onSurface
                }

                DPressFeedback {
                    control: chip
                }
            }
        }
    }
}
