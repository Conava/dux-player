import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.Widgets
import "../../Common/Format.js" as Format

// The activity card as Discord's dark theme draws it, filled from the templates.
Rectangle {
    id: root

    property bool playing: false
    property real position: 0
    property string detailsText: ""
    property string stateText: ""
    property string largeText: ""
    // https cover the daemon sends, "" for the logo
    property string image: ""

    // Discord's own palette and sizes: the card mirrors Discord, not the theme
    readonly property color discordBackground: "#232428"
    readonly property color discordImageBackground: "#313338"
    readonly property color discordPrimaryText: "#f2f3f5"
    readonly property color discordSecondaryText: "#b5bac1"
    readonly property real discordRadius: 8
    readonly property real discordImageSize: 64
    readonly property real discordTextWidth: 240
    readonly property string logo: Paths.resolveIconPath(WindowRuleService.appId)

    implicitWidth: Math.min(parent?.width ?? 0, Theme.spacingL * 2 + discordImageSize + Theme.spacingM + discordTextWidth)
    implicitHeight: content.implicitHeight + Theme.spacingL * 2
    radius: discordRadius
    color: discordBackground
    Accessible.role: Accessible.Grouping
    Accessible.name: [heading.text, root.detailsText, root.stateText].filter(t => t !== "").join(", ")

    Column {
        id: content
        anchors.fill: parent
        anchors.margins: Theme.spacingL
        spacing: Theme.spacingS

        StyledText {
            id: heading
            width: parent.width
            text: I18n.tr("Listening to %1", "discord card preview heading, %1 is the app name").arg("Dux Player")
            font.pixelSize: Theme.fontSizeSmall
            font.weight: Theme.fontWeightBold
            color: root.discordPrimaryText
            elide: Text.ElideRight
        }

        Row {
            width: parent.width
            spacing: Theme.spacingM

            Rectangle {
                width: root.discordImageSize
                height: root.discordImageSize
                radius: root.discordRadius
                color: root.discordImageBackground
                clip: true

                Image {
                    id: art
                    anchors.fill: parent
                    source: root.image !== "" ? root.image : root.logo
                    sourceSize: Qt.size(root.discordImageSize * 2, root.discordImageSize * 2)
                    fillMode: Image.PreserveAspectCrop
                    asynchronous: true
                }

                DIcon {
                    anchors.centerIn: parent
                    visible: art.status !== Image.Ready
                    name: "music_note"
                    size: Theme.iconSize
                    color: root.discordSecondaryText
                }

                // hover only: no tint, no click, arrow cursor
                StateLayer {
                    hovered: false
                    enableRipple: false
                    acceptedButtons: Qt.NoButton
                    cursorShape: Qt.ArrowCursor
                    tooltipText: root.largeText !== "" ? root.largeText : null
                }
            }

            Column {
                width: parent.width - root.discordImageSize - parent.spacing
                anchors.verticalCenter: parent.verticalCenter
                spacing: Theme.spacingXXS

                StyledText {
                    width: parent.width
                    visible: text !== ""
                    text: root.detailsText
                    font.pixelSize: Theme.fontSizeMedium
                    font.weight: Theme.fontWeightMedium
                    color: root.discordPrimaryText
                    elide: Text.ElideRight
                }

                StyledText {
                    width: parent.width
                    visible: text !== ""
                    text: root.stateText
                    font.pixelSize: Theme.fontSizeSmall
                    color: root.discordSecondaryText
                    elide: Text.ElideRight
                }

                StyledText {
                    width: parent.width
                    text: I18n.tr("%1 elapsed", "discord card preview: time since the track started").arg(Format.duration(root.playing ? root.position : 0))
                    font.pixelSize: Theme.fontSizeSmall
                    color: root.discordSecondaryText
                    elide: Text.ElideRight
                }
            }
        }
    }
}
