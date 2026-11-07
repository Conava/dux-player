# A tile's buttons go when the pointer leaves after its menu closed, and Tab reaches them. A menu's popup stays on its
# window in fullscreen with a reserved top area: the compositor's xdg_popup.configure says where it really went.
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
