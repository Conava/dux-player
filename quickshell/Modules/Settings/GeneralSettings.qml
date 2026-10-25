import QtQuick
import Quickshell
import Quickshell.Io
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.DCommon.FileBrowser
import qs.Modules
import qs.Widgets

SettingsPage {
    id: root

    readonly property var removeModes: ["undo", "confirm", "none"]
    readonly property var keyProviders: (SettingsService.artistInfo.providers ?? []).filter(p => p.needsKey)
    readonly property string onlineServices: (SettingsService.artistInfo.providers ?? []).map(p => p.name).join(", ")
    property bool keysShown: false
    property string layoutStatus: ""
    property bool layoutStatusError: false
    // the file read for an import, waiting for the confirm dialog
    property string pendingLayout: ""

    function setStatus(text, error) {
        layoutStatus = text;
        layoutStatusError = error;
    }

    function exportLayout(path) {
        exportFile.path = path;
        exportFile.setText(UiConfig.exportLayout());
        setStatus(I18n.tr("Exported to %1", "custom theme export status").arg(path), false);
    }

    SettingsCard {
        title: I18n.tr("Background", "general settings section label")

        SettingsToggleRow {
            text: I18n.tr("Keep running in the background", "general setting")
            description: I18n.tr("Playback continues after the window is closed.", "general setting description")
            checked: PlayerService.settings.keepRunning
            onToggled: value => PlayerService.setSetting("keepRunning", value)
        }

        SettingsToggleRow {
            text: I18n.tr("Show tray icon", "general setting")
            description: SettingsService.trayKnown ? I18n.tr("Control playback from the system tray.", "general setting description") : PlayerService.unavailableText
            enabled: SettingsService.trayKnown
            checked: SettingsService.settings.showTrayIcon
            onToggled: value => SettingsService.setSetting("showTrayIcon", value)
        }
    }

    SettingsCard {
        title: I18n.tr("Artists", "navigation destination")

        SettingsToggleRow {
            text: I18n.tr("Look up artist info online", "general setting")
            description: SettingsService.artistInfoKnown ? I18n.tr("Fetches missing artist images and biographies. Artist names are sent to %1.", "general setting description, %1 lists online services").arg(root.onlineServices) : PlayerService.unavailableText
            enabled: SettingsService.artistInfoKnown
            checked: SettingsService.settings.artistInfoOnline
            onToggled: value => SettingsService.setSetting("artistInfoOnline", value)
        }

        SettingsRow {
            visible: root.keyProviders.length > 0
            enabled: SettingsService.settings.artistInfoOnline
            title: I18n.tr("API keys", "general setting: keys for online artist info services")
            subtitle: I18n.tr("Required by some services.", "general setting description")
            clickable: true
            Accessible.checkable: true
            Accessible.checked: root.keysShown
            onClicked: root.keysShown = !root.keysShown

            DIcon {
                anchors.verticalCenter: parent.verticalCenter
                name: root.keysShown ? "expand_less" : "expand_more"
                size: Theme.iconSize
                color: Theme.surfaceVariantText
            }
        }

        Repeater {
            model: root.keysShown ? root.keyProviders : []

            SettingsFieldRow {
                id: keyRow
                required property var modelData
                width: parent?.width ?? 0
                enabled: SettingsService.settings.artistInfoOnline
                title: modelData.name
                subtitle: modelData.hasKey ? I18n.tr("Key stored in the system keyring", "artist info API key state") : I18n.tr("No key set", "artist info API key state")
                field.echoMode: TextInput.Password
                field.showPasswordToggle: true
                field.placeholderText: I18n.tr("API key", "API key field")
                onAccepted: {
                    if (field.text.trim() === "")
                        return;
                    SettingsService.setArtistInfoKey(modelData.id, field.text.trim());
                    field.text = "";
                }

                DActionButton {
                    visible: keyRow.modelData.hasKey
                    anchors.verticalCenter: parent?.verticalCenter
                    iconName: "delete"
                    tooltipText: I18n.tr("Remove key", "artist info API key action")
                    Accessible.name: tooltipText
                    onClicked: SettingsService.setArtistInfoKey(keyRow.modelData.id, "")
                }
            }
        }
    }

    SettingsCard {
        title: I18n.tr("Windows", "general settings section label")

        SettingsToggleRow {
            visible: WindowRuleService.viaShell
            text: I18n.tr("Floating windows", "general setting")
            description: I18n.tr("Adds a window rule through Dux Shell.", "general setting description")
            checked: WindowRuleService.active
            onToggled: value => WindowRuleService.setFloating(value)
        }

        SettingsRow {
            visible: WindowRuleService.supported && !WindowRuleService.viaShell
            title: I18n.tr("Floating windows", "general setting")
            subtitle: I18n.tr("Add this rule to your %1 config to open windows floating:", "open floating snippet intro, %1 is the compositor").arg(WindowRuleService.compositor)

            body: Column {
                width: parent.width
                spacing: Theme.spacingS

                Rectangle {
                    width: parent.width
                    height: snippet.implicitHeight + Theme.spacingM * 2
                    radius: Theme.cornerRadiusS
                    color: Theme.surfaceContainerHighest

                    StyledText {
                        id: snippet
                        x: Theme.spacingM
                        y: Theme.spacingM
                        width: parent.width - Theme.spacingM * 2
                        isMonospace: true
                        text: WindowRuleService.snippet()
                        font.pixelSize: Theme.fontSizeSmall
                        color: Theme.onSurface
                        wrapMode: Text.WrapAnywhere
                    }
                }

                DButton {
                    text: I18n.tr("Copy", "copy a config snippet")
                    iconName: "content_copy"
                    backgroundColor: Theme.secondaryContainer
                    textColor: Theme.onSecondaryContainer
                    onClicked: Quickshell.clipboardText = WindowRuleService.snippet()
                }
            }
        }

        SettingsRow {
            visible: !WindowRuleService.supported
            title: I18n.tr("Floating windows", "general setting")
            subtitle: I18n.tr("Set a floating rule for the app id %1 in your compositor.", "open floating note for compositors without known rules").arg(WindowRuleService.appId)
        }
    }

    SettingsCard {
        title: I18n.tr("Customization", "general settings section label")

        SettingsRow {
            title: I18n.tr("Layout", "general setting: every customization as one file")
            subtitle: I18n.tr("Now playing, pages, rows and menus, home and navigation, as one file.", "general setting description")

            body: Column {
                width: parent.width
                spacing: Theme.spacingS

                Row {
                    spacing: Theme.spacingS

                    DButton {
                        text: I18n.tr("Export…", "button that saves a file to export")
                        iconName: "save"
                        backgroundColor: Theme.secondaryContainer
                        textColor: Theme.onSecondaryContainer
                        onClicked: exportPicker.open()
                    }

                    DButton {
                        text: I18n.tr("Import…", "button that opens a file to import")
                        iconName: "file_open"
                        backgroundColor: "transparent"
                        textColor: Theme.primary
                        onClicked: importPicker.open()
                    }
                }

                StyledText {
                    width: parent.width
                    visible: text !== ""
                    text: root.layoutStatus
                    font.pixelSize: Theme.fontSizeSmall
                    color: root.layoutStatusError ? Theme.error : Theme.surfaceVariantText
                    wrapMode: Text.WordWrap
                }
            }
        }

        SettingsRow {
            title: I18n.tr("Reset all customization", "general setting")
            subtitle: I18n.tr("Saved views are kept.", "general setting description")

            DButton {
                anchors.verticalCenter: parent?.verticalCenter
                text: I18n.tr("Reset", "button that restores the defaults")
                backgroundColor: "transparent"
                textColor: Theme.error
                onClicked: resetAllDialog.ask()
            }
        }
    }

    SettingsCard {
        title: I18n.tr("Removing", "general settings section label")

        SettingsButtonGroupRow {
            text: I18n.tr("When removing", "general setting: what happens when removing a track, playlist, shelf or entry")
            description: I18n.tr("Applies to tracks, playlists, shelves and navigation entries.", "general setting description")
            model: [I18n.tr("Offer undo", "removal mode option"), I18n.tr("Ask first", "removal mode option"), I18n.tr("None", "removal mode option")]
            currentIndex: root.removeModes.indexOf(UiConfig.removeMode)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    UiConfig.setUndoToast({
                        "mode": root.removeModes[index]
                    });
            }
        }

        SettingsSliderRow {
            text: I18n.tr("Undo time", "general setting")
            enabled: UiConfig.removeMode === "undo"
            minimum: 3
            maximum: 15
            unit: I18n.tr("s", "unit suffix for seconds")
            value: UiConfig.undoSeconds
            onSliderDragFinished: finalValue => UiConfig.setUndoToast({
                    "seconds": finalValue
                })
        }
    }

    SettingsCard {
        title: I18n.tr("Keyboard shortcuts", "general settings section label")

        Repeater {
            model: ["searchPage", "searchAll"]

            SettingsRow {
                id: shortcutRow
                required property string modelData
                // why the last combination was refused; cleared by the next try
                property string problem: ""
                title: UiConfig.shortcutLabel(modelData)
                subtitle: problem
                subtitleColor: Theme.error

                DKeySequenceField {
                    anchors.verticalCenter: parent.verticalCenter
                    sequence: UiConfig.shortcut(shortcutRow.modelData)
                    Accessible.description: shortcutRow.title
                    onCaptured: sequence => shortcutRow.problem = UiConfig.setShortcut(shortcutRow.modelData, sequence)
                }

                DActionButton {
                    anchors.verticalCenter: parent.verticalCenter
                    iconName: "restart_alt"
                    visible: UiConfig.shortcut(shortcutRow.modelData) !== UiConfig.shortcutDefaults[shortcutRow.modelData]
                    tooltipText: I18n.tr("Reset to %1", "keyboard shortcut reset button, %1 is the default combination").arg(UiConfig.shortcutDefaults[shortcutRow.modelData])
                    Accessible.name: tooltipText
                    onClicked: shortcutRow.problem = UiConfig.setShortcut(shortcutRow.modelData, UiConfig.shortcutDefaults[shortcutRow.modelData])
                }
            }
        }
    }

    FileView {
        id: importFile
        printErrors: false
        blockLoading: false
        onLoaded: {
            root.pendingLayout = text();
            importDialog.ask();
        }
        onLoadFailed: root.setStatus(I18n.tr("Couldn't read the layout file", "layout import failed"), true)
    }

    FileView {
        id: exportFile
        printErrors: false
        atomicWrites: true
    }

    FileBrowserModal {
        id: importPicker
        parentModal: root.QsWindow.window
        browserTitle: I18n.tr("Import layout", "file picker title")
        bucket: "layout"
        filters: ["*.json"]
        onFileSelected: path => {
            importFile.path = "";
            importFile.path = path;
        }
    }

    FileBrowserModal {
        id: exportPicker
        parentModal: root.QsWindow.window
        browserTitle: I18n.tr("Export layout", "file picker title")
        bucket: "layout"
        saveMode: true
        filters: ["*.json"]
        defaultFileName: "dux-player-layout.json"
        onFileSelected: path => root.exportLayout(path)
    }

    ConfirmDialog {
        id: importDialog
        host: root
        title: I18n.tr("Replace your layout?", "layout import confirmation title")
        supportingText: I18n.tr("Every customization is replaced by the one in the file.", "layout import confirmation")
        confirmText: I18n.tr("Replace", "layout import confirm button")
        onConfirmed: {
            const problem = UiConfig.importLayout(root.pendingLayout);
            root.pendingLayout = "";
            if (problem === "")
                root.setStatus(I18n.tr("Layout imported", "layout import status"), false);
            else
                root.setStatus(I18n.tr("Couldn't import the layout: %1", "layout import failed, %1 is the reason").arg(problem), true);
        }
        onRejected: root.pendingLayout = ""
    }

    ConfirmDialog {
        id: resetAllDialog
        host: root
        title: I18n.tr("Reset all customization?", "reset all customization confirmation title")
        supportingText: I18n.tr("Now playing, pages, rows and menus, home and navigation go back to the defaults. Saved views are kept.", "reset all customization confirmation")
        confirmText: I18n.tr("Reset", "button that restores the defaults")
        onConfirmed: {
            UiConfig.resetAll();
            root.setStatus("", false);
        }
    }
}
