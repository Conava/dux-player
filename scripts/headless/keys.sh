# Player keys: Space, Left, Right, Ctrl+Left, Ctrl+Right and F, with focus on a clicked button, a Tab-focused button, a text field, a dialog, Bento's Play on sheet, the volume panel, a menu and a dropdown.
# Real keys through wtype. Fails when a key goes to the wrong place.
failed=0
expect() {
    if [ "$2" = "$3" ]; then
        echo "ok   $1: $3" >>"$W/out/facts.txt"
    else
        echo "FAIL $1: wanted '$3', got '$2'" | tee -a "$W/out/facts.txt"
        failed=1
    fi
}
status() { dp ipc player.status | python3 -I -c "import json, sys; d = json.load(sys.stdin); d = d.get('result', d); print(eval(sys.argv[1]))" "$1"; }
state() { status 'd["state"]'; }
pos() { status 'round(d["position"])'; }
track() { status 'd["track"]["id"]'; }
view() { ask nowPlaying | tr -d '\n'; }
key() { wtype "$@"; sleep 0.8; }

pointer m 1278 720
sleep 0.2
pointer m 1280 720
sleep 0.5
dp play "$W/music" >/dev/null
sleep 2
go tracks
shot keys-start

# Shuffle all, at 200,185: the click leaves it focused
click 200 185
sleep 1.5
expect "playing after the click" "$(state)" "playing"
key -k space
expect "space after a click pauses" "$(state)" "paused"

key -k Tab
key -k space
expect "space on a Tab-focused button is the button's" "$(state)" "paused"
# a click takes the focus back from the Tab-focused button
click 200 185
sleep 1.5

dp pause >/dev/null
sleep 0.5
p=$(pos)
key -k Right
expect "right seeks forward" "$(pos)" "$((p + 5))"
key -k Left
expect "left seeks back" "$(pos)" "$p"

t=$(track)
key -M ctrl -k Right -m ctrl
sleep 1
[ "$(track)" != "$t" ] && n=changed || n=same
expect "ctrl+right changes the track" "$n" "changed"

# held Right stops short of the end, so the track stays. A track is 20 s long
t=$(track)
for _ in 1 2 3 4 5; do key -k Right; done
expect "right stops short of the end" "$(track)" "$t"

# stopped, the player keeps its current track but refuses seeks, so the arrows send none
dp stop >/dev/null
sleep 1
L="$XDG_STATE_HOME/dplayer/ui.log"
from=$(($(wc -l <"$L") + 1))
key -k Right
key -k Left
expect "arrows send no seek when stopped" "$(tail -n "+$from" "$L" | grep -c 'player.seek')" "0"
dp resume >/dev/null
sleep 1.5
dp pause >/dev/null
sleep 0.5

key -k f
expect "f opens the view" "$(view)" "true"
key -k f
expect "f closes the view" "$(view)" "false"

go tracks
ui focusSearch
sleep 1
t=$(track)
s=$(state)
p=$(pos)
key -k f -k space -k x
key -k Left
key -k y
expect "typing keys stay in the field" "$(ask searchText | tr -d '\n')" "f yx"
key -M ctrl -k Left -m ctrl
expect "ctrl+left in the field" "$(track)" "$t"
expect "state kept in the field" "$(state)" "$s"
expect "position kept in the field" "$(pos)" "$p"
expect "view kept in the field" "$(view)" "false"
key -k Escape
key -k Escape

element sleep bar
sleep 1
s=$(state)
key -k space
key -k f
expect "space under a dialog" "$(state)" "$s"
expect "f under a dialog" "$(view)" "false"
key -k Escape
shot keys

# the seekbar reached by Tab: Space stays the player's, the arrows are the bar's
dp pause >/dev/null
sleep 0.5
for _ in $(seq 1 40); do
    [ "$(ask focusRole | tr -d '\n')" = 51 ] && break
    wtype -M shift -k Tab -m shift
    sleep 0.1
done
expect "Tab reaches the seekbar" "$(ask focusRole | tr -d '\n')" "51"
key -k Right
p=$(pos)
key -k Left
[ "$(pos)" -lt "$p" ] && n=back || n=same
expect "left on the seekbar" "$n" "back"
key -k space
expect "space on the seekbar" "$(state)" "playing"

for s in Ctrl+Left Ctrl+Right Ctrl+Space Alt+Left Alt+Right; do
    expect "$s can't be a search shortcut" "$(ask shortcutFree "$s" | tr -d '\n')" "false"
done

# one key at a time, or a pair like Space and Ctrl+Space would cancel out
covered() {
    expand false
    s=$(state)
    key -k space
    expect "space under $1" "$(state)" "$s"
    key -k f
    expect "f under $1" "$(view)" "false"
    p=$(pos)
    key -k Right
    expect "right under $1" "$(pos)" "$p"
    p=$(pos)
    key -k Left
    expect "left under $1" "$(pos)" "$p"
    t=$(track)
    key -M ctrl -k Right -m ctrl
    expect "ctrl+right under $1" "$(track)" "$t"
    t=$(track)
    key -M ctrl -k Left -m ctrl
    expect "ctrl+left under $1" "$(track)" "$t"
    s=$(state)
    key -M ctrl -k space -m ctrl
    expect "ctrl+space under $1" "$(state)" "$s"
}

# Bento's Play on sheet over the full view. Space goes last: the sheet's focused back button takes it and closes the sheet.
sheet() { ask playOnOpen | tr -d '\n'; }
look dark bento
go tracks
expand true
element device view
sleep 1.5
shot keys-sheet
expect "the sheet opens" "$(sheet)" "true"
dp pause >/dev/null
sleep 0.5
p=$(pos)
key -k Right
expect "right under the sheet" "$(pos)" "$p"
key -k Left
expect "left under the sheet" "$(pos)" "$p"
key -k f
expect "f under the sheet" "$(view)" "true"
wtype -M alt -k Left -m alt
sleep 0.8
expect "alt+left under the sheet" "$(view)" "true"
pointer b
sleep 0.8
expect "side button under the sheet" "$(view)" "true"
s=$(state)
key -k space
expect "space under the sheet" "$(state)" "$s"
element device view
sleep 1
key -k Escape
expect "escape closes the sheet" "$(sheet)" "false"
expect "escape keeps the view" "$(view)" "true"
expand false

# the bar's volume button, at 2445,1352: the slider takes the focus and the page stays put
go albums
go tracks
click 2445 1352
sleep 1
shot keys-volume
wtype -M alt -k Left -m alt
sleep 0.8
expect "alt+left under the volume panel" "$(ask route | tr -d '\n')" "tracks/"
key -k f
expect "f under the volume panel" "$(view)" "false"
key -k Escape

dp pause >/dev/null
go tracks
ui edit tracks
sleep 2
# Sort by, at 2264,333; a click anywhere closes it
click 2264 333
sleep 1
shot keys-dropdown
covered "a dropdown"
key -k Down
key -k Return
sleep 1
shot keys-dropdown-picked
go tracks

# the window's overflow menu, at 2417,49: Settings, Quit
click 2417 49
sleep 1
shot keys-menu
covered "a menu"
key -k Escape
key -k f
expect "escape closes the menu and f works again" "$(view)" "true"
key -k f

click 2417 49
sleep 1
key -k Down
key -k space
sleep 2
expect "space picks the highlighted entry" "$(hyprctl clients -j | python3 -I -c 'import json, sys; print(len(json.load(sys.stdin)))')" "2"

[ "$failed" = 0 ]
