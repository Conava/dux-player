#!/usr/bin/env bash
# Runs the real daemon, window and mpv in a nested Hyprland, invisible on the desktop and with no access to the
# developer's session, config or audio. usage: run.sh [scenario.sh]  (default: smoke.sh; see lib.sh for its helpers)
set -u
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
scenario=$(realpath "${1:-$here/smoke.sh}")
# Short on purpose: Hyprland's socket path below it has to fit in 108 bytes.
W=$(mktemp -d /tmp/dph.XXXXXX)
mkdir -p "$W"/{bin,home,out,rt,music,tree,config/qtengine,state,cache,data}
chmod 700 "$W/rt"

# Null audio on top of the private runtime dir, which already keeps mpv away from the real PipeWire.
real_mpv=$(command -v mpv)
printf '#!/bin/sh\nexec %s --ao=null "$@"\n' "$real_mpv" >"$W/bin/mpv"
chmod +x "$W/bin/mpv"

cd "$W/bin" || exit 1
wayland-scanner client-header "$here/vp.xml" vp.h
wayland-scanner private-code "$here/vp.xml" vp.c
gcc -O1 -o ptr "$here/ptr.c" vp.c -I. -lwayland-client || exit 1

# A copy of the UI tree whose shell.qml adds a "headless" IPC target, so scenarios can navigate without clicking.
for f in "$repo"/quickshell/* "$repo"/quickshell/.[a-z]*; do
    [ "$(basename "$f")" = shell.qml ] || ln -s "$f" "$W/tree/"
done
sed -e '0,/^import Quickshell$/s//import Quickshell\nimport Quickshell.Io/' -e '$d' "$repo/quickshell/shell.qml" >"$W/tree/shell.qml"
cat >>"$W/tree/shell.qml" <<'Q'

    IpcHandler {
        target: "headless"
        function go(page: string): void {
            Router.go(page);
        }
        function open(page: string, id: string): void {
            Router.go(page, id);
        }
        function expand(on: bool): void {
            if (on)
                Router.openNowPlaying();
            else
                Router.nowPlayingOpen = false;
        }
        function viewOpen(): bool {
            return Router.nowPlayingOpen;
        }
        function focusSearch(): void {
            Router.pageSearch.forceActiveFocus();
        }
        function searchText(): string {
            return Router.pageSearch?.text ?? "";
        }
        function elementOptions(id: string): void {
            const find = item => {
                if (!item)
                    return null;
                if (item.optionsId !== undefined)
                    return item;
                for (const child of item.children) {
                    const found = find(child);
                    if (found)
                        return found;
                }
                return null;
            };
            find(settingsLoader.item?.contentItem).optionsId = id;
        }
        // every row that shows: in a group, and rounded by where it sits on screen. One line per fault.
        function cornerFaults(): string {
            const faults = [];
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (item.isSettingsRow === true && item.parent?.isSettingsGroup !== true)
                    faults.push("outside a group: " + item.title);
                if (item.isSettingsGroup === true) {
                    const rows = Array.from(item.children).filter(c => c.visible && c.isSettingsRow === true).sort((a, b) => a.y - b.y);
                    rows.forEach((row, i) => {
                        const top = i === 0 ? Theme.groupedListOuterRadius : Theme.groupedListInnerRadius;
                        const bottom = i === rows.length - 1 ? Theme.groupedListOuterRadius : Theme.groupedListInnerRadius;
                        if (row.topRadius !== top || row.bottomRadius !== bottom)
                            faults.push("corners of " + row.title + ": " + row.topRadius + "/" + row.bottomRadius + ", wanted " + top + "/" + bottom);
                    });
                }
                for (const child of item.children)
                    walk(child);
            };
            walk(settingsLoader.item?.contentItem);
            return faults.join("\n");
        }
        // the time, then the middle of every lyrics line as drawn on screen, top to bottom
        function lyricsRows(): string {
            const rows = [];
            const walk = item => {
                if (!item || !item.visible)
                    return;
                if (item.layout?.part)
                    rows.push(item.mapToItem(null, 0, item.height / 2).y);
                for (const child of item.children)
                    walk(child);
            };
            walk(windowLoader.item?.contentItem);
            return Date.now() + " " + rows.sort((a, b) => a - b).join(" ");
        }
        function customize(surface: string): void {
            Router.customize(surface);
        }
        function settings(page: string): void {
            Router.openSettings(page);
        }
        function look(mode: string, style: string): void {
            SettingsData.themeMode = mode;
            UiConfig.setNowPlaying({
                style: style
            });
        }
        function playerLook(look: string): void {
            UiConfig.setNowPlaying({
                playerLook: look
            });
        }
        function setStyle(key: string, style: string): void {
            const patch = {};
            patch[key] = style;
            UiConfig.setNowPlaying(patch);
        }
        function barTitle(width: string, adaptive: bool): void {
            const surface = JSON.parse(JSON.stringify(UiConfig.surface("bar")));
            const title = surface.zones.left.find(i => i.type === "title");
            title.options.width = width;
            title.options.adaptive = adaptive;
            UiConfig.setSurface("bar", surface);
        }
        function paneLook(look: string): void {
            UiConfig.setNowPlaying({
                paneLook: look
            });
        }
        function coverShape(shape: string): void {
            const surface = JSON.parse(JSON.stringify(UiConfig.surface("fullBento")));
            surface.zones.cover[0].options.shape = shape;
            UiConfig.setSurface("fullBento", surface);
        }
        function artColors(on: bool): void {
            SettingsData.artColors = on;
        }
        function element(type: string, context: string): void {
            Router.runElement(type, context);
        }
        function reduceMotion(on: bool): void {
            SettingsData.reduceMotion = on;
        }
        function lyricsDisplay(key: string, value: string): void {
            UiConfig.setLyricsDisplay({
                [key]: value === "true" || (value !== "false" && value)
            });
        }
        function edit(page: string): void {
            Router.editPage(page, "");
        }
        function pageOptions(kind: string, json: string): void {
            UiConfig.setPageOptions(kind, "", Object.assign({}, UiConfig.pageOptions(kind, ""), JSON.parse(json)));
        }
        function navStyle(style: string): void {
            SettingsData.navStyle = style;
        }
        function resetSurface(id: string): void {
            UiConfig.resetSurface(id);
        }
        function surface(id: string, json: string): void {
            UiConfig.setSurface(id, JSON.parse(json));
        }
        function addElement(surface: string, zone: string, json: string): void {
            const zones = JSON.parse(JSON.stringify(UiConfig.surface(surface).zones));
            zones[zone].push(Object.assign({
                hidden: false,
                placement: "auto",
                options: {}
            }, JSON.parse(json)));
            UiConfig.setSurface(surface, {
                zones: zones
            });
        }
    }
}
Q

# Generated music, so no scenario depends on anyone's library.
album="$W/music/Test Artist/Test Album"
mkdir -p "$album"
for i in 1 2 3 4 5 6 7 8; do
    ffmpeg -loglevel error -f lavfi -i "sine=frequency=$((220 * i)):duration=20" \
        -metadata title="Track $i" -metadata artist="Test Artist" -metadata album="Test Album" -metadata track="$i" \
        "$album/$i.flac" || exit 1
done

# Icon lookups only cost what they cost on a real desktop with a real theme.
[ -f "$HOME/.config/qtengine/config.json" ] && cp "$HOME/.config/qtengine/config.json" "$W/config/qtengine/"

env -i LANG=C.UTF-8 USER="$USER" HOME="$W/home" PATH="$W/bin:$repo/core/bin:/usr/bin:/bin" XDG_RUNTIME_DIR="$W/rt" \
    XDG_CONFIG_HOME="$W/config" XDG_STATE_HOME="$W/state" XDG_CACHE_HOME="$W/cache" XDG_DATA_HOME="$W/data" \
    XDG_DATA_DIRS="${XDG_DATA_DIRS:-/usr/share}" QT_QPA_PLATFORMTHEME=qtengine DPLAYER_WINRULES=off DPLAYER_CAST=off DPLAYER_BACKEND="${DPLAYER_BACKEND:-}" \
    DPLAYER_DISABLE_HOT_RELOAD=1 HYPRLAND_NO_SD_NOTIFY=1 HYPRLAND_NO_SD_VARS=1 HYPRLAND_NO_CRASHREPORTER=1 \
    W="$W" LIB="$here/lib.sh" SCENARIO="$scenario" \
    timeout -k 30 "${HEADLESS_TIMEOUT:-180}" dbus-run-session -- bash "$here/session.sh"
status=$?

log="$W/out/ui.log"
errors=$(grep -hE 'TypeError|ReferenceError|Cannot assign|is not a function|\.qml:[0-9]+.*(rror|nable)' "$log" 2>/dev/null | sort -u)
[ -n "$errors" ] && { echo "QML errors:"; echo "$errors" | cut -c1-200 | head -20; status=1; }
echo "output: $W/out"
ls "$W/out"
exit "$status"
