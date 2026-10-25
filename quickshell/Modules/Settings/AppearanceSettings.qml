pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Io
import qs.Common
import qs.Services
import qs.DCommon.Widgets
import qs.DCommon.FileBrowser
import qs.DCommon.Common as DC
import "../../Common/CustomTheme.js" as CustomTheme
import "../../Common/StockTheme.js" as StockTheme

// Follow Dux Shell, or set each style group on its own (each can still take the shell's value).
SettingsPage {
    id: root

    readonly property var themeModes: ["auto", "light", "dark"]
    readonly property var colorSchemes: ["stock", "seed", "custom"]
    readonly property var seedChoices: ["red", "orange", "yellow", "green", "teal", "blue", "purple", "pink"]
    readonly property var seedNames: [I18n.tr("Red", "color palette name"), I18n.tr("Orange", "color palette name"), I18n.tr("Yellow", "color palette name"), I18n.tr("Green", "color palette name"), I18n.tr("Teal", "color palette name"), I18n.tr("Blue", "color palette name"), I18n.tr("Purple", "color palette name"), I18n.tr("Pink", "color palette name")]
    property string themeStatus: ""
    property bool themeStatusError: false

    function groupTitle(group) {
        switch (group) {
        case "colors":
            return I18n.tr("Colors", "appearance setting for the color source");
        case "mode":
            return I18n.tr("Light and dark", "appearance style group");
        case "font":
            return I18n.tr("Font", "appearance style group");
        case "transparency":
            return I18n.tr("Transparency", "appearance style group");
        case "radius":
            return I18n.tr("Corners", "appearance style group");
        case "motion":
            return I18n.tr("Motion", "appearance settings section label");
        case "ripple":
            return I18n.tr("Ripple and focus ring", "appearance style group");
        case "effects":
            return I18n.tr("Blur and elevation", "appearance style group");
        }
        return group;
    }

    function importTheme(path) {
        importFile.path = "";
        importFile.path = path;
    }

    function exportTheme(path) {
        const name = SettingsData.customTheme?.name || I18n.tr("Dux Player theme", "default name of an exported theme");
        exportFile.path = path;
        exportFile.setText(CustomTheme.serialize(name, StyleSource.ownColors(StockTheme.getPreset(SettingsData.presetTheme, false), false), StyleSource.ownColors(StockTheme.getPreset(SettingsData.presetTheme, true), true)));
        themeStatusError = false;
        themeStatus = I18n.tr("Exported to %1", "custom theme export status").arg(path);
    }

    // One card per style group: the source in its header, its own controls only while it is "Custom".
    component Group: SettingsCard {
        id: group

        property string key: ""
        readonly property bool shellOwned: StyleSource.fromShell[key] === true
        default property alias controls: body.data

        title: root.groupTitle(key)
        visible: !StyleSource.following
        headerActions: DButtonGroup {
            visible: StyleSource.detected
            size: "small"
            model: [I18n.tr("Dux Shell", "appearance settings section label"), I18n.tr("Custom", "option for your own values")]
            currentIndex: group.shellOwned ? 0 : 1
            onSelectionChanged: (index, selected) => {
                if (selected)
                    StyleSource.setGroupSource(group.key, index === 0 ? "shell" : "custom");
            }
        }

        content: [
            SettingsRow {
                visible: group.shellOwned
                title: I18n.tr("From Dux Shell", "appearance group following the shell")
                subtitle: I18n.tr("Set in Dux Shell settings. Choose Custom to override.", "appearance group following the shell")
                iconName: "link"
            },
            Column {
                id: body
                visible: !group.shellOwned
                width: parent?.width ?? 0
                spacing: Theme.groupedListGap
            }
        ]
    }

    SettingsCard {
        visible: StyleSource.detected
        title: I18n.tr("Dux Shell", "appearance settings section label")

        SettingsToggleRow {
            text: I18n.tr("Follow Dux Shell", "appearance setting")
            description: I18n.tr("Uses all style settings from Dux Shell. Turn off to set each group separately.", "appearance setting description")
            checked: SettingsData.followShell
            onToggled: value => SettingsData.followShell = value
        }

        SettingsRow {
            visible: StyleSource.following
            iconName: "link"
            title: I18n.tr("All style settings come from Dux Shell.", "appearance note while following the shell")
        }
    }

    Group {
        key: "colors"

        SettingsButtonGroupRow {
            text: I18n.tr("Palette", "color source option")
            model: [I18n.tr("Preset", "color scheme option"), I18n.tr("From a color", "color scheme option"), I18n.tr("Custom theme", "color scheme option")]
            description: SettingsData.colorScheme === "" && StyleSource.detected ? I18n.tr("Uses Dux Shell colors until you choose one.", "appearance palette fallback note") : ""
            currentIndex: SettingsData.colorScheme === "" && !StyleSource.detected ? 0 : root.colorSchemes.indexOf(SettingsData.colorScheme)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    SettingsData.colorScheme = root.colorSchemes[index];
            }
        }

        SettingsRow {
            id: presetRow
            visible: SettingsData.colorScheme === "stock" || (SettingsData.colorScheme === "" && !StyleSource.detected)
            title: I18n.tr("Preset", "appearance setting")

            DDropdown {
                id: presetDropdown
                dropdownWidth: SettingsMetrics.dropdownWidth
                options: Theme.presetNames().map(n => Theme.presetLabel(n))
                optionValues: Theme.presetNames()
                onValueChanged: value => {
                    SettingsData.colorScheme = "stock";
                    SettingsData.presetTheme = value;
                }
            }

            Binding {
                target: presetDropdown
                property: "currentValue"
                value: SettingsData.presetTheme
            }
        }

        SettingsRow {
            visible: SettingsData.colorScheme === "seed"
            title: I18n.tr("Base color", "appearance setting")
            subtitle: I18n.tr("Light and dark palettes are generated from this color.", "appearance setting description")

            body: Column {
                width: parent.width
                spacing: Theme.spacingS

                Flow {
                    width: parent.width
                    spacing: Theme.spacingS

                    Repeater {
                        model: root.seedChoices

                        StyledButton {
                            id: seedSwatch
                            required property string modelData
                            required property int index
                            readonly property bool chosen: Qt.colorEqual(Qt.color(SettingsData.seedColor), swatch)
                            readonly property color swatch: Theme.accent(modelData)?.container ?? Theme.primary
                            width: Theme.buttonHeightXS
                            height: width
                            radius: Theme.fullRadius(width, height)
                            color: swatch
                            border.width: chosen ? Theme.outlineWidthFocused : 0
                            border.color: Theme.onSurface
                            Accessible.role: Accessible.RadioButton
                            Accessible.name: root.seedNames[index]
                            Accessible.checked: chosen
                            onClicked: SettingsData.seedColor = swatch.toString()

                            StateLayer {
                                control: seedSwatch
                                stateColor: Theme.onSurface
                            }

                            FocusRing {
                                visible: seedSwatch.visualFocus
                            }
                        }
                    }
                }

                DTextField {
                    width: Math.min(parent.width, SettingsMetrics.fieldWidth)
                    outlined: true
                    labelText: I18n.tr("Hex color", "appearance seed color field")
                    text: SettingsData.seedColor
                    leftIconName: "palette"
                    onAccepted: {
                        if (/^#[0-9a-fA-F]{6}$/.test(text.trim()))
                            SettingsData.seedColor = text.trim();
                    }
                }
            }
        }

        SettingsRow {
            visible: SettingsData.colorScheme === "custom"
            title: SettingsData.customTheme ? I18n.tr("Theme: %1", "loaded custom theme name").arg(SettingsData.customTheme.name || I18n.tr("Unnamed", "custom theme without a name")) : I18n.tr("No custom theme loaded", "custom theme state")
            subtitle: I18n.tr("A Dux Shell theme file with light and dark palettes. Export the current colors to use as a starting point.", "custom theme description")

            body: Column {
                width: parent.width
                spacing: Theme.spacingS

                Row {
                    spacing: Theme.spacingS

                    DButton {
                        text: I18n.tr("Import…", "button that opens a file to import")
                        iconName: "file_open"
                        backgroundColor: Theme.secondaryContainer
                        textColor: Theme.onSecondaryContainer
                        onClicked: importPicker.open()
                    }

                    DButton {
                        text: I18n.tr("Export…", "button that saves a file to export")
                        iconName: "save"
                        backgroundColor: "transparent"
                        textColor: Theme.primary
                        onClicked: exportPicker.open()
                    }
                }

                StyledText {
                    width: parent.width
                    visible: text !== ""
                    text: root.themeStatus
                    font.pixelSize: Theme.fontSizeSmall
                    color: root.themeStatusError ? Theme.error : Theme.surfaceVariantText
                    wrapMode: Text.WordWrap
                }
            }
        }
    }

    Group {
        key: "mode"

        SettingsButtonGroupRow {
            text: I18n.tr("Color mode", "appearance setting for light or dark")
            description: I18n.tr("Auto follows the desktop.", "appearance setting description")
            model: [I18n.tr("System", "theme mode"), I18n.tr("Light", "theme mode"), I18n.tr("Dark", "theme mode")]
            currentIndex: root.themeModes.indexOf(SettingsData.themeMode)
            onSelectionChanged: (index, selected) => {
                if (selected)
                    SettingsData.themeMode = root.themeModes[index];
            }
        }
    }

    Group {
        key: "font"

        SettingsRow {
            id: fontRow
            title: I18n.tr("Family", "font setting")

            DDropdown {
                id: fontDropdown
                dropdownWidth: SettingsMetrics.dropdownWidth
                enableFuzzySearch: true
                options: [I18n.tr("Default", "font family dropdown option for the bundled font")]
                optionValues: ["default"]
                onValueChanged: value => SettingsData.fontFamily = value === "default" ? "" : value
                Component.onCompleted: Qt.callLater(() => {
                    const families = DC.Fonts.selectableFamilies(Theme.defaultFontFamily);
                    options = [I18n.tr("Default", "font family dropdown option for the bundled font")].concat(families);
                    optionValues = ["default"].concat(families);
                })
            }

            Binding {
                target: fontDropdown
                property: "currentValue"
                value: SettingsData.fontFamily || "default"
            }
        }

        SettingsSliderRow {
            text: I18n.tr("Text size", "appearance setting")
            minimum: 80
            maximum: 140
            step: 5
            unit: "%"
            value: Math.round(SettingsData.fontScale * 100)
            onSliderDragFinished: finalValue => SettingsData.fontScale = finalValue / 100
        }

        // numbers like the shell's own weight slider: weight names would share terms with the theme and size options
        SettingsSliderRow {
            text: I18n.tr("Weight", "font setting")
            minimum: Font.Thin
            maximum: Font.Black
            step: 100
            showStops: true
            value: SettingsData.fontWeight
            onSliderDragFinished: finalValue => SettingsData.fontWeight = finalValue
        }
    }

    Group {
        key: "transparency"

        SettingsSliderRow {
            text: I18n.tr("Window opacity", "appearance setting")
            minimum: 0
            maximum: 100
            step: 5
            unit: "%"
            value: Math.round(SettingsData.windowTransparency * 100)
            onSliderDragFinished: finalValue => SettingsData.windowTransparency = finalValue / 100
        }
    }

    Group {
        key: "radius"

        SettingsSliderRow {
            text: I18n.tr("Corner roundness", "appearance setting")
            minimum: 0
            maximum: 100
            value: SettingsData.radiusStrength
            onSliderDragFinished: finalValue => SettingsData.radiusStrength = finalValue
        }
    }

    Group {
        key: "motion"

        SettingsSliderRow {
            text: I18n.tr("Animation duration", "appearance setting")
            enabled: !SettingsData.reduceMotion
            minimumLabel: I18n.tr("Off", "option that turns something off")
            minimum: 0
            maximum: 600
            step: 50
            unit: I18n.tr(" ms", "unit suffix for milliseconds with a space")
            value: SettingsData.animationDuration
            onSliderDragFinished: finalValue => SettingsData.animationDuration = finalValue
        }

        SettingsToggleRow {
            text: I18n.tr("Reduce motion", "appearance setting")
            checked: SettingsData.reduceMotion
            onToggled: value => SettingsData.reduceMotion = value
        }
    }

    Group {
        key: "ripple"

        SettingsToggleRow {
            text: I18n.tr("Ripple effects", "appearance setting")
            checked: SettingsData.enableRippleEffects
            onToggled: value => SettingsData.enableRippleEffects = value
        }

        SettingsToggleRow {
            text: I18n.tr("Focus ring", "appearance setting")
            description: I18n.tr("Highlights the focused control when using the keyboard.", "appearance setting description")
            checked: SettingsData.focusRingEnabled
            onToggled: value => SettingsData.focusRingEnabled = value
        }
    }

    Group {
        key: "effects"

        SettingsToggleRow {
            text: I18n.tr("Elevation shadows", "appearance setting")
            checked: SettingsData.elevationEnabled
            onToggled: value => SettingsData.elevationEnabled = value
        }

        SettingsToggleRow {
            text: I18n.tr("Blur behind menus", "appearance setting")
            description: I18n.tr("Requires compositor blur and a transparent window.", "appearance setting description")
            checked: SettingsData.blurEnabled
            onToggled: value => SettingsData.blurEnabled = value
        }
    }

    SettingsCard {
        title: I18n.tr("Cover colors", "appearance settings section label")

        SettingsToggleRow {
            text: I18n.tr("Color now playing from the cover", "appearance setting")
            description: I18n.tr("Applies to now playing and the now-playing bar.", "appearance setting description")
            checked: SettingsData.artColors
            onToggled: value => SettingsData.artColors = value
        }

        SettingsToggleRow {
            text: I18n.tr("Color the whole app", "appearance setting: album art colors everywhere")
            enabled: SettingsData.artColors
            checked: SettingsData.artColorsApp
            onToggled: value => SettingsData.artColorsApp = value
        }
    }

    function importProblem(code) {
        switch (code) {
        case "empty":
            return I18n.tr("The file is empty", "why a theme file can't be imported");
        case "tooLarge":
            return I18n.tr("The file is too large", "why a theme file can't be imported");
        case "noColors":
            return I18n.tr("The file has no colors", "why a theme file can't be imported");
        }
        return I18n.tr("Not a theme file", "why a theme file can't be imported");
    }

    FileView {
        id: importFile
        printErrors: false
        blockLoading: false
        onLoaded: {
            const result = CustomTheme.parse(text());
            if (result.error) {
                root.themeStatusError = true;
                root.themeStatus = I18n.tr("Couldn't import the theme: %1", "custom theme import failed, %1 is the reason").arg(root.importProblem(result.error));
                return;
            }
            SettingsData.customTheme = result.theme;
            SettingsData.colorScheme = "custom";
            root.themeStatusError = false;
            root.themeStatus = I18n.tr("Imported %1", "custom theme import status").arg(result.theme.name || path);
        }
        onLoadFailed: {
            root.themeStatusError = true;
            root.themeStatus = I18n.tr("Couldn't read the theme file", "custom theme import failed");
        }
    }

    FileView {
        id: exportFile
        printErrors: false
        atomicWrites: true
    }

    FileBrowserModal {
        id: importPicker
        parentModal: root.QsWindow.window
        browserTitle: I18n.tr("Import theme", "file picker title")
        bucket: "theme"
        filters: ["*.json"]
        onFileSelected: path => root.importTheme(path)
    }

    FileBrowserModal {
        id: exportPicker
        parentModal: root.QsWindow.window
        browserTitle: I18n.tr("Export theme", "file picker title")
        bucket: "theme"
        saveMode: true
        filters: ["*.json"]
        defaultFileName: "dux-player-theme.json"
        onFileSelected: path => root.exportTheme(path)
    }
}
