import QtQuick
import qs.Common
import qs.DCommon.Widgets

// A row that opens a web page in the browser.
SettingsRow {
    property string url: ""

    clickable: url !== ""
    onClicked: Qt.openUrlExternally(url)

    DIcon {
        anchors.verticalCenter: parent.verticalCenter
        name: "open_in_new"
        size: Theme.iconSize
        color: Theme.surfaceVariantText
    }
}
