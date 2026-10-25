import QtQuick
import qs.Common
import qs.DCommon.Widgets

// A now-playing surface on the overview: a scaled-down live preview, its name, a badge ("Default": the start
// surface or the style in use), and a summary or its first problem. A click opens its editor.
DListItem {
    id: root

    property string title: ""
    property string badge: ""
    property string summary: ""
    property string warning: ""
    property Component preview: null
    // the size the preview is laid out at before scaling down
    property size previewSize: Qt.size(1, 1)

    readonly property real previewHeight: Theme.listItemHeight * 3.5

    surfaceColor: SettingsMetrics.rowColor
    implicitHeight: content.height + SettingsMetrics.rowPaddingV * 2
    Accessible.name: title
    Accessible.description: warning !== "" ? warning : summary

    Column {
        id: content
        x: SettingsMetrics.rowPaddingH
        y: SettingsMetrics.rowPaddingV
        width: root.width - SettingsMetrics.rowPaddingH * 2
        spacing: Theme.spacingM

        Preview {
            width: parent.width
            height: root.previewHeight
            preview: root.preview
            previewSize: root.previewSize
        }

        Row {
            width: parent.width
            spacing: Theme.spacingS

            Column {
                width: parent.width - chevron.width - parent.spacing
                spacing: Theme.spacingXXS

                Row {
                    width: parent.width
                    spacing: Theme.spacingS

                    StyledText {
                        width: Math.min(implicitWidth, parent.width - (badgeLabel.visible ? badgeLabel.width + parent.spacing : 0))
                        text: root.title
                        font.pixelSize: Theme.fontSizeMedium
                        font.weight: Theme.fontWeightMedium
                        color: root.contentColor
                        elide: Text.ElideRight
                        horizontalAlignment: Text.AlignLeft
                    }

                    DBadge {
                        id: badgeLabel
                        anchors.verticalCenter: parent.verticalCenter
                        visible: root.badge !== ""
                        text: root.badge
                    }
                }

                StyledText {
                    width: parent.width
                    text: root.warning !== "" ? root.warning : root.summary
                    font.pixelSize: Theme.fontSizeSmall
                    color: root.warning !== "" ? Theme.error : root.supportingContentColor
                    wrapMode: Text.WordWrap
                    horizontalAlignment: Text.AlignLeft
                }
            }

            DIcon {
                id: chevron
                anchors.verticalCenter: parent.verticalCenter
                name: I18n.isRtl ? "chevron_left" : "chevron_right"
                size: Theme.iconSize
                color: root.supportingContentColor
            }
        }
    }

    // a live, non-interactive, scaled-down copy of a component laid out at previewSize; the editor uses it too
    component Preview: Rectangle {
        id: area

        property Component preview: null
        property size previewSize: Qt.size(1, 1)

        implicitHeight: width * previewLoader.height / previewLoader.width
        radius: Theme.cornerRadiusS
        color: Theme.surface
        clip: true
        Accessible.ignored: true

        // a clipped zero-size parent keeps the real component out of the pointer's reach
        Item {
            width: 0
            height: 0
            clip: true

            Loader {
                id: previewLoader
                width: area.previewSize.width
                // height 0: the component's own height
                height: area.previewSize.height > 0 ? area.previewSize.height : Math.max(1, (item as Item)?.implicitHeight ?? 1)
                sourceComponent: area.preview
            }
        }

        ShaderEffectSource {
            readonly property real fit: Math.min(area.width / previewLoader.width, area.height / previewLoader.height)
            anchors.centerIn: parent
            width: previewLoader.width * fit
            height: previewLoader.height * fit
            sourceItem: previewLoader
            hideSource: true
            mipmap: true
            smooth: true
        }
    }
}
