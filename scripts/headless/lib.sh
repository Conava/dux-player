# Helpers for scenarios. The window sits on a 2560x1440 output.
# DPLAYER_BACKEND=builtin make headless also starts a private sound server with the sinks headless_a, _b and _c.
# SERVER=1 make headless also adds the (Open)Subsonic library "Server" from a fake server (fakesubsonic.py) with two
#   artists, three albums, 21 tracks and the playlist "Server Mix"; its password sits in a fresh keyring on the private
#   bus. The session waits for its catalog sync. Needs gnome-keyring-daemon. Writing MS to "$W/server/delay" makes it
#   answer every call after MS milliseconds; deleting the file stops that.
#   shot NAME        screenshot to out/NAME.png
#   go PAGE          show a page: home, libraries, history, search, ...
#   expand true|false  open or close the full now playing view
#   look MODE STYLE  theme and player style: look dark m3e
#   element TYPE CONTEXT  run a player element: element queue view
#   ui NAME ARGS     any function of the headless IPC target in run.sh: ui open album ID, ui settings PAGE, ui playerLook attached,
#                    ui pageOptions tracks '{"lines":["format"]}', ui addElement fullBento title '{"type":"text"}',
#                    ui nowPlayingPatch '{"paneTabs":false}' sets any Now playing key, ui pageEndInset 80 pads the page ends
#   dp ARGS          the dplayer CLI against this sandbox: dp play "$W/music", dp pause
#   rpc METHOD JSON  one daemon IPC call; prints the whole reply
#   pointer m X Y | c | r | d | u | D | U | s MS | w NOTCHES   through a real virtual pointer: move, click, right click,
#                    left press and release, right press and release, pause, wheel. Lines of one write keep their
#                    timing: pointer "$(printf 'D\ns 30\nU')"
#   pointer b | f [MS [DX]]   hold the side button (back) or the extra button (forward) for MS, moving DX halfway
#   ask NAME ARGS    like ui, but prints the reply: ask route
#   pref NAME JSON   set an app setting (SettingsData[NAME]) from JSON: ui pref albumLayout '"sideBySide"'
#   rect OBJECTNAME  ask rect NAME prints "X Y W H enabled" of the first visible item with that objectName, "" when none;
#                    ask rect and ask namedAll print points relative to the window's top-left corner, which click does not know:
#                    add winat's origin, or use wclick, clickrect and clicknamed
#   winat [TITLE]    "X Y": the top-left corner of the main window (or the window with that title)
#   wclick X Y [TITLE]  click at a point given relative to a window
#   clickrect NAME   click the middle of the item ask rect NAME finds; fails when none shows
#   clicknamed LABEL [N]  click the middle of the Nth (default 1st) item ask namedAll LABEL finds (accessible name or text); fails when none shows
#   clickunder NAME LABEL  click the item labelled LABEL inside the item with objectName NAME (a tab of the Library tabs)
#   other ask helpers: navLit (the selected navigation entry), tint (the page tint), playingTiles, carouselX, topItem
#                    (the tile at the top of a page's list), heroRect (an album's cover and facts), dialTiles (Speed dial),
#                    prefValue NAME (an app setting as JSON)
#   themeColor NAME  ask themeColor primary prints Theme.primary as #rrggbb
#   objectText NAME, countObjects NAME   the text of the first visible item with that objectName, and how many there are
#   swatches         ask swatches prints "X Y W H chosen" per Base color swatch of the settings window (window coordinates)
#   settingsFocusSearch, settingsResults   focus the settings search field (then wtype the query); "page title" per result
#   click X Y        move and click, in output coordinates
#   frames NAME CMD  run CMD and write the ms between the main window's frames while its moving parts move to out/NAME.frames
#   rclick X Y       move and right click
#   ask timed ACTION ARG, then ask timedStop: the ms to the first frame and between frames while ACTION runs; besides
#                    go, open, back and expand: forward, next, prev, queueAdd N, queueRemove I, queueMove "FROM TO",
#                    pane lyrics|queue, libraryTab NAME, railLabels, dock, prompt TITLE (run.sh lists them)
#   ui timedFrom main|settings  records like timed without acting: the scenario clicks or types, then ask timedStop;
#                    ask sinceFrame (ms since its last frame) and ask timedLayouts ("page view": width changes) go with it
#   ui act ACTION ARG  one of timed's newer actions, untimed: ui act next "", ui act pane queue
#   idleframes MS [settings]  the frames the main (or settings) window draws in the next MS ms; 0 when settled
#   ask props TYPE PROP  JSON array of PROP over the visible items (both windows and open menus) whose type name starts
#                    with TYPE, or with .NAME that have a property NAME; PROP* multiplies it along the parents:
#                    ask props TextSlide text, ask props .entry 'opacity*'
#   ask menuRows PROP  the same over the rows of the open menus: ask menuRows 'opacity*'
#   ask scaleAt X Y main|settings  the smallest scale an item at that window point is drawn with
#   dimmed A B       1 when the settings window's left edge is darker in out/B.png than in out/A.png: a dialog opened
#                    between the shots really draws its scrim, rather than only reporting itself open
dp() { dplayer-dev "$@"; }
# The UI runtime through the dev build, like the window launcher starts it; a function, so the real qs stays on PATH for the fallback. Warn level keeps the runtime's "Loaded" line out of every call's stderr.
qs() { DPLAYER_LOG_LEVEL=warn dplayer-dev qs "$@"; }

wait_for_daemon() {
    for _ in $(seq 1 60); do
        ls "$XDG_RUNTIME_DIR"/dplayer*.sock >/dev/null 2>&1 && return 0
        sleep 0.5
    done
    return 1
}

rpc() {
    python3 -I - "$1" "$2" <<'P'
import glob, json, os, socket, sys
s = socket.socket(socket.AF_UNIX)
s.connect(sorted(glob.glob(os.environ["XDG_RUNTIME_DIR"] + "/dplayer*.sock"))[-1])
f = s.makefile("rwb")
f.readline()
f.write(json.dumps({"id": 1, "method": sys.argv[1], "params": json.loads(sys.argv[2])}).encode() + b"\n")
f.flush()
print(f.readline().decode().strip())
P
}

ask() { qs ipc -p "$W/tree" call headless "$@"; }
ui() { qs ipc -p "$W/tree" call headless "$@" >/dev/null; }
go() { ui go "$1"; sleep "${SETTLE:-1.5}"; }
expand() { ui expand "$1"; sleep "${SETTLE:-1.5}"; }
look() { ui look "$1" "$2"; sleep "${SETTLE:-1.5}"; }
element() { ui element "$1" "$2"; }
shot() { grim "$W/out/$1.png"; }
# frames NAME CMD...: runs CMD, then records for 1.5 s the ms between frames, from the first to the last one in which the main window's moving parts moved, in out/NAME.frames
frames() {
    local name=$1
    shift
    qs ipc -p "$W/tree" call headless framesStart >/dev/null
    "$@"
    sleep 1.5
    qs ipc -p "$W/tree" call headless framesStop >"$W/out/$name.frames"
}

idleframes() {
    ui idleStart "${2:-main}"
    sleep "$(awk "BEGIN { print $1 / 1000 }")"
    ask idleStop
}

# winat [TITLE]: "X Y", the top-left corner of the window with that title (default the main window)
winat() {
    hyprctl -j clients | python3 -I -c 'import json, sys
c = next(c for c in json.load(sys.stdin) if c["title"] == sys.argv[1])
print(*c["at"])' "${1:-Dux Player}"
}

pointer() {
    if [ ! -p "$W/ptr" ]; then
        mkfifo "$W/ptr"
        ptr <"$W/ptr" 2>"$W/out/ptr.log" &
        exec 9>"$W/ptr"
    fi
    echo "$*" >&9
}
click() { pointer m "$(($1 - 2))" "$2"; sleep 0.1; pointer m "$1" "$2"; sleep 0.3; pointer "${3:-c}"; }
rclick() { click "$1" "$2" r; }
# wclick X Y [TITLE]: click at a point relative to the window, the way ask rect, ask namedAll and ask settingsPoint report one
wclick() {
    local wx wy
    read -r wx wy < <(winat "${3:-}")
    click "$((wx + $1))" "$((wy + $2))"
}
clickrect() {
    local x y w h enabled
    read -r x y w h enabled < <(ask rect "$1")
    [ -n "$x" ] && wclick "$((x + w / 2))" "$((y + h / 2))"
}
clicknamed() {
    local x y w h
    read -r x y w h < <(ask namedAll "$1" | sed -n "${2:-1}p")
    [ -n "$x" ] && wclick "$((x + w / 2))" "$((y + h / 2))"
}
clickunder() {
    local x y
    read -r x y < <(ask namedUnder "$1" "$2")
    [ -n "$x" ] && wclick "$x" "$y"
}

dimmed() {
    hyprctl -j clients | python3 -I -c '
import json, sys
from PIL import Image
c = [c for c in json.load(sys.stdin) if c["title"] == "Dux Player Settings"][0]
(x, y), (w, h) = c["at"], c["size"]
box = (x + 8, y + h * 3 // 10, x + min(150, w // 6), y + h * 9 // 10)
mean = lambda f: sum(Image.open(f).convert("L").crop(box).getdata()) / ((box[2] - box[0]) * (box[3] - box[1]))
print(1 if mean(sys.argv[2]) < 0.75 * mean(sys.argv[1]) else 0)' "$W/out/$1.png" "$W/out/$2.png"
}
