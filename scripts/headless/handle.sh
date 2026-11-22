# The full view has no top bar, only a handle that moves the window; the "Top bar" setting brings the
# bar back and removes the handle. With title bars off the header is gone everywhere: the handle sits above every page,
# Search and Settings are navigation entries (a rail shows them as its FAB and at its bottom), and Search in header is
# greyed out. Shots of every state go to out/; fails when a drag sends no move request or an entry is missing.
failed=0
fact() { echo "$*" >>"$W/out/facts.txt"; }
fail() {
    echo "FAIL $*" | tee -a "$W/out/facts.txt"
    failed=1
}
origin() {
    hyprctl -j clients | python3 -I -c '
import json, sys
c = next(c for c in json.load(sys.stdin) if c["title"] == "'"${WT:-Dux Player}"'")
print(c["at"][0], c["at"][1], c["size"][0])'
}

L="$XDG_STATE_HOME/dplayer/ui.log"
# The protocol log shows the move request and the hand cursors (set_shape 16 grab, 17 grabbing); the nested
# compositor may not act on the request from a virtual pointer, so the window's own position isn't evidence.
dp quit >/dev/null 2>&1
sleep 2
WAYLAND_DEBUG=client dp show -c "$W/tree" >>"$W/out/show.log" 2>&1 &
SHOW=$!
sleep 1
wait_for_daemon
sleep 5
hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1000})' >/dev/null
sleep 1
dp play "$W/music" >/dev/null
sleep 2
for style in bento m3e; do
    look dark "$style"
    expand true
    shot "handle-$style"
done

# drag FROM_Y: press at the window's top centre, move down-right in steps, release; prints nothing, sets x1 y1
drag() {
    read -r x0 y0 w0 <<<"$(origin)"
    cx=$((x0 + w0 / 2))
    pointer m "$((cx - 2))" "$((y0 + $1))"
    sleep 0.3
    pointer m "$cx" "$((y0 + $1))"
    sleep 0.3
    from=$(logmark "$L")
    pointer d
    sleep 0.2
    for i in 1 2 3 4 5 6 7 8; do
        pointer m "$((cx + i * 20))" "$((y0 + $1 + i * 10))"
        sleep 0.1
    done
    shot "$2"
    pointer u
    sleep 0.5
    read -r x1 y1 _ <<<"$(origin)"
    logsince "$L" "$from" >"$W/out/$2.wl"
}
# seen LOG PATTERN WHAT
seen() {
    if grep -qE "$2" "$W/out/$1.wl"; then fact "ok   $3"; else fail "$3: no $2 in $1.wl"; fi
}
drag 10 handle-dragging
seen handle-dragging 'xdg_toplevel[@#][0-9]+\.move\(' "the handle asks the compositor to move the window"
seen handle-dragging 'set_shape\([0-9]+, 17\)' "the handle shows the closed hand while dragging"
fact "window position $x0 $y0 -> $x1 $y1"

ui fullViewTopBar true
sleep 1
shot handle-topbar
drag 22 header-dragging
seen header-dragging 'xdg_toplevel[@#][0-9]+\.move\(' "control: the top bar asks the same of the compositor"
fact "window position $x0 $y0 -> $x1 $y1"
ui fullViewTopBar false
sleep 1

# title bars off: no header, the handle on every page, Search and Settings in the navigation
nav() { ask navEntries | tr -d '\n'; }
expect() {
    if [ "$2" = "$3" ]; then fact "ok   $1: $2"; else fail "$1: wanted '$3', got '$2'"; fi
}
expect "title bars on: the stored navigation" "$(nav | grep -c 'search\|settings')" 0
ui titleBars false
sleep 1
expect "title bars off: Search and Settings close the navigation" "$(nav | tr ' ' '\n' | grep -c '^page:search$\|^settings:$')" 2
expect "title bars off: Settings is last" "$(nav | tr ' ' '\n' | tail -1)" "settings:"
expand false
go home
shot titlebars-home

# a rail shows Search as its FAB and Settings at its bottom, below the destinations
read -r wx wy _ <<<"$(origin)"
read -r fx fy fw fh _ <<<"$(ask rect railSearch)"
if [ -n "$fx" ]; then
    click $((wx + fx + fw / 2)) $((wy + fy + fh / 2))
    sleep 1.5
    expect "title bars off: the rail's Search button opens Search" "$(ask route | tr -d '\n')" "search/"
    shot titlebars-rail-search
else
    fail "title bars off: no Search button in the rail"
fi
read -r _ dy _ dh _ <<<"$(ask rect railDestinations)"
read -r sx sy _ sh _ <<<"$(ask rect railSettings)"
if [ -n "$sx" ] && [ -n "$dy" ] && [ "$sy" -ge $((dy + dh)) ] && [ $((sy + sh)) -gt 600 ]; then
    fact "ok   title bars off: Settings is at the rail's bottom ($sy, destinations end at $((dy + dh)))"
else
    fail "title bars off: Settings not at the rail's bottom: '$sy $sh' after destinations '$dy $dh'"
fi
go home
go albums
ui open album "$(ask albumId "Test Album" | tr -d '\n')"
sleep 1.5
shot titlebars-album
drag 10 titlebars-dragging
seen titlebars-dragging 'xdg_toplevel[@#][0-9]+\.move\(' "the handle moves the window above a page"
expand true
shot titlebars-full
drag 10 titlebars-full-dragging
seen titlebars-full-dragging 'xdg_toplevel[@#][0-9]+\.move\(' "the handle moves the window in the full view"
expand false
ui settings pages
sleep 2
hyprctl dispatch 'hl.dsp.window.fullscreen({mode="maximized"})' >/dev/null
sleep 2
pointer w 15
sleep 1.5
shot titlebars-settings-navigation
ui settings general
sleep 2
shot titlebars-settings-general
# the settings window has the same handle, and a close button in its sidebar instead of the header
WT="Dux Player Settings"
read -r cx cy cw ch _ <<<"$(ask rect settingsClose)"
if [ -n "$cx" ]; then
    read -r wx wy _ <<<"$(origin)"
    fact "close button at $cx $cy $cw $ch, window at $wx $wy"
    click $((wx + cx + cw / 2)) $((wy + cy + ch / 2))
    sleep 1
    expect "title bars off: the close button closes the settings window" "$(hyprctl -j clients | grep -c '"title": "Dux Player Settings"')" 0
else
    fail "title bars off: no close button in the settings sidebar"
fi
ui settings general
sleep 2
read -r hx hy hw hh _ <<<"$(ask rect settingsHandle)"
if [ -n "$hx" ] && [ "$hy" -lt 40 ]; then
    fact "ok   title bars off: the settings window has its handle ($hx $hy $hw $hh)"
else
    fail "title bars off: no handle at the top of the settings window: '$hx $hy'"
fi
drag 10 titlebars-settings-dragging
seen titlebars-settings-dragging 'xdg_toplevel[@#][0-9]+\.move\(' "the handle moves the settings window"
WT=
ui titleBars true
sleep 1
expect "title bars on again: the stored navigation is as it was" "$(nav | grep -c 'search\|settings')" 0
shot titlebars-settings-general-on
[ "$failed" = 0 ]
