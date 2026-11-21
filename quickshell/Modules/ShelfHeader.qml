import QtQuick
import QtQuick.Window
import qs.Common
import qs.DCommon.Widgets
import qs.Widgets

// A shelf's title row: the title (a link to the full page when seeAll is set), the carousel's page arrows
// and Shuffle. Bleeds into the page padding like the tiles below it.
Item {
    id: root

    property string title
    // [page, id]; null makes the title plain text
    property var seeAll: null
    property bool shuffleShown: false
    signal shuffleRequested

    // the row the arrows page; null shows no arrows
    property Flickable carousel: null
    // distance between two tiles, so a page keeps the next tile in view
    property real tileStep: 0

    readonly property real maxX: carousel ? Math.max(carousel.originX, carousel.contentWidth - carousel.width + carousel.originX) : 0
    readonly property bool arrowsShown: carousel !== null && carousel.contentWidth > carousel.width && (Window.width ?? 0) >= PlayerMetrics.compactBreakpoint
    readonly property real titleHeight: Math.max(Theme.buttonHeightXS, titleText.implicitHeight + Theme.spacingXS)

    function page(direction) {
        const target = Math.max(carousel.originX, Math.min(maxX, carousel.contentX + direction * Math.max(tileStep, carousel.width - tileStep)));
        slide.stop();
        slide.to = target;
        slide.start();
    }

    x: -Theme.spacingS
    width: (parent?.width ?? 0) + Theme.spacingS * 2
    height: Math.max(titleHeight, shuffle.visible ? shuffle.height : 0)

    NumberAnimation {
        id: slide
        target: root.carousel
        property: "contentX"
        duration: Theme.expressiveDurations.expressiveDefaultSpatial
        easing.type: Easing.BezierSpline
        easing.bezierCurve: Theme.expressiveCurves.emphasizedDecel
    }

    StyledButton {
        id: titleLink
        visible: root.seeAll !== null
        anchors.left: parent.left
        anchors.verticalCenter: parent.verticalCenter
        width: titleRow.implicitWidth + Theme.spacingS * 2
        height: root.titleHeight
        radius: Theme.fullRadius(width, height)
        color: "transparent"
        Accessible.role: Accessible.Link
        Accessible.name: I18n.tr("%1, see all", "home shelf title link").arg(root.title)
        onClicked: Router.go(root.seeAll[0], root.seeAll[1])

        StateLayer {
            control: titleLink
            stateColor: Theme.onSurface
        }

        Row {
            id: titleRow
            anchors.centerIn: parent
            spacing: Theme.spacingXXS

            StyledText {
                id: titleText
                anchors.verticalCenter: parent.verticalCenter
                text: root.title
                font.pixelSize: PlayerMetrics.shelfTitleSize
                font.weight: Theme.fontWeightBold
                color: Theme.surfaceText
            }

            DIcon {
                anchors.verticalCenter: parent.verticalCenter
                name: "chevron_right"
                rotation: I18n.isRtl ? 180 : 0
                size: Theme.iconSize
                color: Theme.surfaceText
            }
        }
    }

    StyledText {
        visible: root.seeAll === null
        anchors.left: parent.left
        anchors.leftMargin: Theme.spacingS
        anchors.verticalCenter: parent.verticalCenter
        text: root.title
        font.pixelSize: PlayerMetrics.shelfTitleSize
        font.weight: Theme.fontWeightBold
        color: Theme.surfaceText
    }

    Row {
        anchors.right: parent.right
        anchors.rightMargin: Theme.spacingS
        anchors.verticalCenter: parent.verticalCenter
        spacing: Theme.spacingXS

        DIconButton {
            objectName: "shelfPrev"
            anchors.verticalCenter: parent.verticalCenter
            visible: root.arrowsShown
            buttonSize: Theme.buttonHeightXS
            widthMode: "narrow"
            iconName: "chevron_left"
            variant: "tonal"
            enabled: root.carousel !== null && root.carousel.contentX > root.carousel.originX + 1
            tooltipText: I18n.tr("Previous", "home shelf arrow")
            Accessible.name: tooltipText
            onClicked: root.page(-1)
        }

        DIconButton {
            objectName: "shelfNext"
            anchors.verticalCenter: parent.verticalCenter
            visible: root.arrowsShown
            buttonSize: Theme.buttonHeightXS
            widthMode: "narrow"
            iconName: "chevron_right"
            variant: "tonal"
            enabled: root.carousel !== null && root.carousel.contentX < root.maxX - 1
            tooltipText: I18n.tr("Next", "home shelf arrow")
            Accessible.name: tooltipText
            onClicked: root.page(1)
        }

        DIconButton {
            id: shuffle
            anchors.verticalCenter: parent.verticalCenter
            visible: root.shuffleShown
            iconName: "shuffle"
            variant: "tonal"
            tooltipText: I18n.tr("Shuffle", "button or mode that plays in random order")
            Accessible.name: tooltipText
            onClicked: root.shuffleRequested()
        }
    }
}
