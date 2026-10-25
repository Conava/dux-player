//@ pragma Env QT_WAYLAND_DISABLE_WINDOWDECORATION=1
//@ pragma Env QSG_RENDER_LOOP=threaded
//@ pragma Env QT_QUICK_CONTROLS_STYLE=Material
//@ pragma UseQApplication
//@ pragma AppId io.github.conava.DuxPlayer

pragma ComponentBehavior: Bound

import QtQuick
import Quickshell
import Quickshell.Wayland
import qs.Common
import qs.DCommon.Common as DC
import qs.Modules
import qs.Modules.Settings
import qs.Services

ShellRoot {
    id: root

    // the daemon sets it when it runs the embedded, read-only tree
    readonly property bool enableHotReload: Quickshell.env("DPLAYER_DISABLE_HOT_RELOAD") !== "1"

    Component.onCompleted: {
        Quickshell.watchFiles = enableHotReload;
        DC.Style.theme = Theme;
        DC.Style.settings = StyleSource;
        DC.I18n.backend = I18n;
        DC.Paths.backend = Paths;
        DC.Log.backend = Log;
        DC.Host.files = FilesService;
        DC.Host.ownWindowDecorations = Qt.binding(() => SettingsData.windowTitleBars);
    }

    // Closing the window keeps the daemon playing when "Keep running" is on; the UI process quits either way.
    function closeWindow() {
        SettingsData.flush();
        Router.flushUndo(() => Qt.quit());
    }

    LazyLoader {
        id: windowLoader
        active: true

        MainWindow {
            onCloseRequested: root.closeWindow()
        }
    }

    // a second `dplayer show` raises the open window
    function raiseWindow() {
        const title = (windowLoader.item as MainWindow)?.title ?? "";
        const own = (ToplevelManager.toplevels?.values ?? []).find(t => t.appId === WindowRuleService.appId && t.title === title);
        if (!own)
            return;
        if (own.minimized)
            own.minimized = false;
        own.activate();
    }

    Connections {
        target: PlayerService
        function onShowRequested() {
            root.raiseWindow();
        }
    }

    Connections {
        target: Router
        function onSettingsRequested(page) {
            settingsLoader.active = true;
            (settingsLoader.item as SettingsWindow).open(page);
        }

        function onRaiseRequested() {
            root.raiseWindow();
        }
    }

    LazyLoader {
        id: settingsLoader
        active: false

        // a child of the main window, so tiling compositors open it floating like a dialog
        SettingsWindow {
            parentWindow: windowLoader.item
        }
    }
}
