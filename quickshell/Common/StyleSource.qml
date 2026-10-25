pragma Singleton
pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import qs.Common
import "CustomTheme.js" as CustomTheme
import "Scheme.js" as Scheme

// The one place that resolves every style value; Theme and DCommon (as DC.Style.settings) read only from here.
// "Follow Dux Shell" takes every group from the shell. Otherwise each group (colors, mode, font, transparency,
// radius, motion, ripple, effects) comes from the shell or from the app's own settings, per group.
Singleton {
    id: root

    readonly property ShellStyleSource shell: ShellStyleSource {}

    readonly property var groups: ["colors", "mode", "font", "transparency", "radius", "motion", "ripple", "effects"]
    readonly property bool detected: shell.found
    readonly property bool following: detected && SettingsData.followShell
    // per group: true when its values come from the shell
    readonly property var fromShell: {
        const result = {};
        for (const group of groups)
            result[group] = detected && (following || SettingsData.styleGroups[group] === "shell");
        return result;
    }

    // Colors: the shell's dms-colors.json when the group follows the shell, or as the fallback while the app has
    // no colors of its own picked.
    readonly property bool colorsFromShell: fromShell.colors || (detected && colorScheme === "")
    // "" (unset) | "stock" | "seed" | "custom"
    readonly property string colorScheme: SettingsData.colorScheme
    readonly property string presetTheme: SettingsData.presetTheme
    readonly property var customTheme: SettingsData.customTheme

    // The app's own palette for a mode, on top of the stock preset's semantic colors.
    function ownColors(preset, isLight) {
        switch (colorScheme) {
        case "seed":
            return Object.assign({}, preset, Scheme.fromSeed(SettingsData.seedColor, isLight));
        case "custom":
            return Object.assign({}, preset, CustomTheme.palette(customTheme, isLight));
        }
        return preset;
    }

    readonly property string themeMode: fromShell.mode ? (shell.isLightMode ? "light" : "dark") : SettingsData.themeMode

    readonly property string fontSource: fromShell.font ? "auto" : (SettingsData.fontFamily !== "" ? "custom" : "auto")
    readonly property string fontFamily: fromShell.font ? "" : SettingsData.fontFamily
    readonly property string monoFontFamily: fromShell.font ? "" : SettingsData.monoFontFamily
    readonly property string displayFontFamily: fromShell.font ? "" : SettingsData.displayFontFamily
    readonly property int fontWeight: fromShell.font ? shell.value("fontWeight", 400) : SettingsData.fontWeight
    readonly property real fontScale: fromShell.font ? shell.value("fontScale", 1) : SettingsData.fontScale

    // the shell syncs window transparency to its popup value unless floatingWindowSyncGlobal is off
    readonly property bool shellWindowSync: shell.value("floatingWindowSyncGlobal", true)
    readonly property real popupTransparency: fromShell.transparency ? shell.value("popupTransparency", 1) : SettingsData.windowTransparency
    readonly property real floatingWindowTransparency: fromShell.transparency ? (shellWindowSync ? popupTransparency : shell.value("floatingWindowTransparency", 1)) : SettingsData.windowTransparency
    readonly property real foregroundLayerTransparency: fromShell.transparency ? shell.value("foregroundLayerTransparency", 1) : 1
    readonly property bool foregroundLayers: fromShell.transparency ? shell.value("blurForegroundLayers", true) : true
    readonly property real floatingWindowForegroundTransparency: fromShell.transparency && !shellWindowSync ? shell.value("floatingWindowForegroundTransparency", 1) : foregroundLayerTransparency
    readonly property bool floatingWindowForegroundLayers: fromShell.transparency && !shellWindowSync ? shell.value("floatingWindowForegroundLayers", true) : foregroundLayers

    readonly property real radiusStrength: fromShell.radius ? shell.value("radiusStrength", 50) : SettingsData.radiusStrength
    readonly property real fixedRadius: fromShell.radius && shell.value("radiusMode", "scale") === "fixed" ? shell.value("fixedRadius", 12) : -1
    readonly property string density: SettingsData.density

    readonly property int animationDuration: fromShell.motion ? shell.value("animationDuration", 250) : SettingsData.animationDuration
    readonly property bool reduceMotion: fromShell.motion ? shell.value("reduceMotion", false) : SettingsData.reduceMotion
    readonly property int springBounce: fromShell.motion ? shell.value("springBounce", 1) : SettingsData.springBounce

    readonly property bool enableRippleEffects: fromShell.ripple ? shell.value("enableRippleEffects", true) : SettingsData.enableRippleEffects
    readonly property bool focusRingEnabled: fromShell.ripple ? shell.value("focusRingEnabled", true) : SettingsData.focusRingEnabled
    readonly property real focusRingWidth: fromShell.ripple ? shell.value("focusRingWidth", 1.5) : SettingsData.focusRingWidth
    readonly property string focusRingColor: fromShell.ripple ? shell.value("focusRingColor", "primary") : SettingsData.focusRingColor

    readonly property bool elevationEnabled: fromShell.effects ? shell.value("m3ElevationEnabled", true) : SettingsData.elevationEnabled
    readonly property bool blurEnabled: fromShell.effects ? shell.value("blurEnabled", false) : SettingsData.blurEnabled
    readonly property real blurLayerOutlineOpacity: fromShell.effects ? shell.value("blurLayerOutlineOpacity", 0) : 0

    // freedesktop icon theme for app icons; the player shows none yet
    readonly property string iconTheme: {
        if (!following)
            return "";
        const light = themeMode === "light" && shell.value("iconThemePerMode", false);
        const name = shell.value(light ? "iconThemeLight" : "iconThemeDark", "System Default");
        return name === "System Default" ? "" : name;
    }

    // What DCommon reads through DC.Style.settings
    readonly property bool popoutElevationEnabled: elevationEnabled
    readonly property bool scrollbarsEnabled: following ? shell.value("scrollbarsEnabled", true) : true
    readonly property bool showExtensions: true
    readonly property int textRenderType: fromShell.font ? shell.value("textRenderType", 0) : 0
    readonly property int textRenderQuality: fromShell.font ? shell.value("textRenderQuality", 0) : 0
    readonly property bool blurBorderEnabled: fromShell.effects && shell.value("blurBorderEnabled", false)
    readonly property real blurBorderOpacity: fromShell.effects ? shell.value("blurBorderOpacity", 0.35) : 0.35
    readonly property string blurBorderColor: fromShell.effects ? shell.value("blurBorderColor", "outline") : "outline"
    readonly property string blurBorderCustomColor: fromShell.effects ? shell.value("blurBorderCustomColor", "#ffffff") : "#ffffff"
    readonly property bool powerActionConfirm: true
    readonly property real powerActionHoldDuration: 0.5
    readonly property var powerMenuActions: []
    readonly property string powerMenuDefaultAction: "logout"
    readonly property bool powerMenuGridLayout: false

    function setGroupSource(group, source) {
        const all = Object.assign({}, SettingsData.styleGroups);
        all[group] = source;
        SettingsData.styleGroups = all;
    }
}
