import QtQuick
import qs.Common
import qs.DCommon.Widgets

Item {
    id: root

    property string title: ""
    property string subtitle: ""
    property bool showBack: Router.canGoBack
    // the page's own search field; "" for none. searchEdited reports what the user types.
    property string searchPlaceholder: ""
    property string searchText: ""
    // Edit page: the page owns `editing` and sets it from editRequested and done
    property bool editable: false
    property bool editing: false

    signal searchEdited(string text)
    signal editRequested
    signal resetRequested
    signal done
    default property alias actions: pageActions.data

    width: parent?.width ?? 0
    implicitHeight: Math.max(PlayerMetrics.pageHeaderHeight, textColumn.implicitHeight + Theme.spacingM * 2)
    LayoutMirroring.enabled: I18n.isRtl
    LayoutMirroring.childrenInherit: true

    readonly property EscapeLayer _escapeLayer: EscapeLayer {
        owner: root
        active: root.editing
        onEscaped: root.done()
    }

    DIconButton {
        id: backButton
        anchors.left: parent.left
        anchors.verticalCenter: parent.verticalCenter
        visible: root.showBack
        iconName: "arrow_back"
        rotation: I18n.isRtl ? 180 : 0
        Accessible.name: I18n.tr("Back", "page header back button")
        tooltipText: Accessible.name
        onClicked: Router.back()
    }

    Column {
        id: textColumn
        anchors.left: backButton.visible ? backButton.right : parent.left
        anchors.leftMargin: backButton.visible ? Theme.spacingS : 0
        anchors.right: actionRow.left
        anchors.rightMargin: Theme.spacingM
        anchors.verticalCenter: parent.verticalCenter
        spacing: Theme.spacingXXS

        StyledText {
            width: parent.width
            text: root.title
            font.pixelSize: Theme.fontSizeXXLarge
            color: Theme.surfaceText
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
            horizontalAlignment: Text.AlignLeft
        }

        StyledText {
            width: parent.width
            text: root.subtitle
            visible: text !== ""
            font.pixelSize: Theme.fontSizeMedium
            color: Theme.surfaceVariantText
            elide: Text.ElideRight
            wrapMode: Text.NoWrap
            horizontalAlignment: Text.AlignLeft
        }
    }

    Row {
        id: actionRow
        anchors.right: parent.right
        anchors.verticalCenter: parent.verticalCenter
        spacing: Theme.spacingS

        ScopedSearch {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.searchPlaceholder !== "" && !root.editing
            placeholderText: root.searchPlaceholder
            text: root.searchText
            onTextChanged: root.searchEdited(text)
        }

        Row {
            id: pageActions
            anchors.verticalCenter: parent.verticalCenter
            visible: !root.editing
            spacing: Theme.spacingS
        }

        DButton {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.editable && !root.editing
            text: I18n.tr("Edit page", "page header button that opens the page's edit mode")
            iconName: "edit"
            backgroundColor: Theme.secondaryContainer
            textColor: Theme.onSecondaryContainer
            onClicked: root.editRequested()
        }

        DButton {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.editing
            text: I18n.tr("Reset", "button that restores the defaults")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: root.resetRequested()
        }

        DButton {
            anchors.verticalCenter: parent.verticalCenter
            visible: root.editing
            text: I18n.tr("Done", "edit mode finish button")
            iconName: "check"
            onClicked: root.done()
        }
    }
}
