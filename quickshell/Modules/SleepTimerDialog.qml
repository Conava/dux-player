pragma ComponentBehavior: Bound

import QtQuick
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import "../Common/Format.js" as Format

WindowDialog {
    id: root

    readonly property var choices: [15, 30, 45, 60, 90].map(m => ({
                "label": I18n.tr("%1 minutes", "sleep timer option, %1 is 15 or more").arg(m),
                "minutes": m
            })).concat([
        {
            "label": I18n.tr("End of track", "sleep timer option"),
            "minutes": 0
        }
    ])

    opened: Router.sleepPickerOpen
    title: I18n.tr("Sleep timer", "now playing button")
    supportingText: {
        switch (PlayerService.sleepMode) {
        case "minutes":
            return I18n.tr("Pausing in %1.", "sleep timer status").arg(Format.duration(PlayerService.sleepRemaining));
        case "endOfTrack":
            return I18n.tr("Pausing after this track.", "sleep timer status");
        }
        return I18n.tr("Pauses playback after a set time.", "sleep timer status when off");
    }
    acceptEnabled: false
    onRejected: Router.sleepPickerOpen = false
    onOpenedChanged: {
        if (opened)
            forceActiveFocus();
    }

    Column {
        width: parent.width
        spacing: Theme.groupedListGap

        Repeater {
            model: root.choices

            DListItem {
                required property var modelData
                required property int index
                width: parent.width
                firstInGroup: index === 0
                lastInGroup: index === root.choices.length - 1
                Accessible.name: modelData.label
                onClicked: {
                    if (modelData.minutes > 0)
                        PlayerService.setSleepTimer(modelData.minutes);
                    else
                        PlayerService.setSleepAtEndOfTrack();
                    Router.sleepPickerOpen = false;
                }

                StyledText {
                    anchors.left: parent.left
                    anchors.leftMargin: Theme.spacingL
                    anchors.verticalCenter: parent.verticalCenter
                    text: parent.modelData.label
                    font.pixelSize: Theme.fontSizeMedium
                    color: parent.contentColor
                }
            }
        }
    }

    actions: [
        DButton {
            visible: PlayerService.sleepMode !== "off"
            text: I18n.tr("Turn off", "sleep timer action")
            backgroundColor: Theme.secondaryContainer
            textColor: Theme.onSecondaryContainer
            onClicked: {
                PlayerService.cancelSleepTimer();
                Router.sleepPickerOpen = false;
            }
        },
        DButton {
            text: I18n.tr("Close", "dialog dismiss button")
            backgroundColor: "transparent"
            textColor: Theme.primary
            onClicked: Router.sleepPickerOpen = false
        }
    ]
}
