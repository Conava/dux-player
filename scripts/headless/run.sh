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
        function artColors(on: bool): void {
            SettingsData.artColors = on;
        }
        function element(type: string, context: string): void {
            Router.runElement(type, context);
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
    XDG_DATA_DIRS="${XDG_DATA_DIRS:-/usr/share}" QT_QPA_PLATFORMTHEME=qtengine DPLAYER_WINRULES=off DPLAYER_BACKEND="${DPLAYER_BACKEND:-}" \
    DPLAYER_DISABLE_HOT_RELOAD=1 HYPRLAND_NO_SD_NOTIFY=1 HYPRLAND_NO_SD_VARS=1 HYPRLAND_NO_CRASHREPORTER=1 \
    W="$W" LIB="$here/lib.sh" SCENARIO="$scenario" \
    timeout -k 5 "${HEADLESS_TIMEOUT:-180}" dbus-run-session -- bash "$here/session.sh"
status=$?

log="$W/out/ui.log"
errors=$(grep -hE 'TypeError|ReferenceError|Cannot assign|is not a function|\.qml:[0-9]+.*(rror|nable)' "$log" 2>/dev/null | sort -u)
[ -n "$errors" ] && { echo "QML errors:"; echo "$errors" | cut -c1-200 | head -20; status=1; }
echo "output: $W/out"
ls "$W/out"
exit "$status"
