# A tile's buttons go when the pointer leaves after its menu closed, and Tab reaches them. A menu's popup stays on its
# window in fullscreen with a reserved top area: the compositor's xdg_popup.configure says where it really went.
# A right click that moves a little only opens the menu, and real keys drive an open menu, however it was opened.
failed=0
L="$XDG_STATE_HOME/dplayer/ui.log"
art="150,180 176x176"
expect() {
    if [ "$2" = "$3" ]; then
        echo "ok   $1: $3" >>"$W/out/facts.txt"
    else
        echo "FAIL $1: wanted '$3', got '$2'" | tee -a "$W/out/facts.txt"
        failed=1
    fi
}
idle() { cmp -s "$W/out/art-idle.png" "$1" && echo idle || echo shown; }
status() { rpc player.status '{}' | python3 -I -c 'import json, sys; print(json.load(sys.stdin)["result"][sys.argv[1]])' "$1"; }
key() {
    wtype "$@"
    sleep 0.4
}
# The rows of a menu opened at 900,400; a row looks different from its first shot once the keyboard is on it.
rows="415 455 495 535 592 632 672"
row_shot() { grim -g "910,$1 170x24" "$2"; }
lit() {
    local n=0 y
    for y in $rows; do
        n=$((n + 1))
        row_shot "$y" "$W/out/row.png"
        cmp -s "$W/out/row.png" "$W/out/row-$n.png" || {
            echo "$n"
            return
        }
    done
    echo none
}
menu_keys() {
    local n=0 y
    rclick 900 400
    sleep 1
    for y in $rows; do
        n=$((n + 1))
        row_shot "$y" "$W/out/row-$n.png"
    done
}

# The daemon passes its environment on to the window, and only this run needs the protocol log
dp quit >/dev/null 2>&1
sleep 2
WAYLAND_DEBUG=client dp show -c "$W/tree" >>"$W/out/show.log" 2>&1 &
SHOW=$!
sleep 1
wait_for_daemon
sleep 5

go albums
pointer m 1400 1000
sleep 1
grim -g "$art" "$W/out/art-idle.png"

click 300 205
sleep 1
click 1400 1000
sleep 1.5
grim -g "$art" "$W/out/art-after.png"
expect "tile after its menu" "$(idle "$W/out/art-after.png")" idle

# The tile, its play button and its ⋮ each take a Tab, and all three show the buttons. The count relies on the first
# tile being among the first 14 stops of the Albums page.
go tracks
go albums
shown=0
for i in $(seq 1 14); do
    wtype -k Tab
    sleep 0.5
    grim -g "$art" "$W/out/art-tab.png"
    [ "$(idle "$W/out/art-tab.png")" = shown ] && shown=$((shown + 1))
done
expect "Tab stops showing the buttons" "$shown" 3

go tracks
pointer m 898 397
sleep 0.2
pointer m 900 397
sleep 1
under="880,440 230x270"
grim -g "$under" "$W/out/under.png"
for i in 1 2 3 4 5; do
    pointer "$(printf 'D\ns 40\nm 903 399\ns 40\nU')"
    sleep 0.6
    grim -g "$under" "$W/out/under-menu.png"
    cmp -s "$W/out/under.png" "$W/out/under-menu.png" && expect "moved right click $i opens the menu" closed open
    click 1400 1000
    sleep 0.5
    pointer m 900 397
    sleep 0.3
done
expect "playback after moved right clicks" "$(status state)" stopped

menu_keys
for step in Down:1 Down:2 Up:1 Tab:2 End:7 Home:1 Up:7 Down:1; do
    key -k "${step%:*}"
    expect "menu row after ${step%:*}" "$(lit)" "${step#*:}"
done
key -k Escape
sleep 1
grim -g "$under" "$W/out/under-menu.png"
cmp -s "$W/out/under.png" "$W/out/under-menu.png" && closed=yes || closed=no
expect "Escape closes the menu" "$closed" yes
rclick 900 400
sleep 1
key -k Down
key -k Return
expect "queue after Return on Play next" "$(status length)" 1
rclick 900 400
sleep 1
key -k Down
key -k Down
key -k space
expect "queue after Space on Add to queue" "$(status length)" 2
rpc queue.clear '{}' >/dev/null

# The Menu key on a focused row and on a tile: the same keys, and the look is the same again once it closes
go tracks
pointer m 1400 1000
sleep 0.5
first="300,262 300x40"
grim -g "$first" "$W/out/first-idle.png"
for i in $(seq 1 14); do
    key -k Tab
    grim -g "$first" "$W/out/first.png"
    cmp -s "$W/out/first-idle.png" "$W/out/first.png" || break
done
key -k Menu
sleep 0.6
key -k Down
key -k Return
expect "queue after the Menu key on a row" "$(status length)" 1
grim -g "$first" "$W/out/first-after.png"
cmp -s "$W/out/first.png" "$W/out/first-after.png" && same=yes || same=no
expect "row keeps its keyboard focus" "$same" yes
rpc queue.clear '{}' >/dev/null

go tracks
go albums
for i in $(seq 1 14); do
    key -k Tab
    grim -g "$art" "$W/out/art-tab.png"
    [ "$(idle "$W/out/art-tab.png")" = shown ] && break
done
tile="130,160 240x270"
grim -g "$tile" "$W/out/tile-focus.png"
key -k Menu
sleep 0.6
shot tile-menu
key -k Escape
sleep 0.6
grim -g "$tile" "$W/out/tile-after.png"
cmp -s "$W/out/tile-focus.png" "$W/out/tile-after.png" && same=yes || same=no
expect "tile looks the same after its menu" "$same" yes

# Escape takes the menu, not the full view under it
go tracks
dp play "$W/music" >/dev/null 2>&1
sleep 2
expand true
click 2509 111
sleep 1
shot full-menu
key -k Escape
sleep 0.5
expect "full view after Escape in its menu" "$(qs ipc -p "$W/tree" call headless viewOpen)" true
expand false
rpc queue.clear '{}' >/dev/null
go albums

cat >"$W/bar.qml" <<'Q'
import Quickshell
PanelWindow {
    anchors { top: true; left: true; right: true }
    implicitHeight: 48
    exclusiveZone: 48
}
Q
qs -p "$W/bar.qml" >"$W/out/bar.log" 2>&1 &
bar=$!
sleep 3
hyprctl eval 'hl.dispatch(hl.dsp.window.fullscreen({mode="fullscreen"}))' >/dev/null
sleep 2.5
from=$(($(wc -l <"$L") + 1))
rclick 150 230
sleep 1
shot fullscreen-menu
expect "popup in fullscreen" "$(tail -n +"$from" "$L" | grep -o 'xdg_popup#[0-9]*\.configure([0-9-]*, [0-9-]*' | tail -1 | sed 's/.*(//')" "0, 0"
kill "$bar"

[ "$failed" = 0 ]
