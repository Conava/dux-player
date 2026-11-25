# Hovers a text field, a column gap and a button, and fails when the window doesn't ask the compositor for their
# cursors. Screenshots don't show the pointer, so this reads the Wayland requests: set_shape with 1 default, 4 pointer,
# 9 text, 30 col_resize. Then the corner and centre of the side panel's cover and the floating ring: the hand only inside.
failed=0
L="$XDG_STATE_HOME/dplayer/ui.log"
expect() {
    if logsince "$L" "$2" | grep -qE "set_shape\([0-9]+, $3\)"; then
        echo "ok   $1: set_shape $3" >>"$W/out/facts.txt"
    else
        echo "FAIL $1: no set_shape $3" | tee -a "$W/out/facts.txt"
        failed=1
    fi
}
blank() {
    pointer m 1400 1000
    sleep 0.5
    from=$(logmark "$L")
}
hover() {
    pointer m "$(($1 - 2))" "$2"
    sleep 0.2
    pointer m "$1" "$2"
    sleep 0.5
}
# Where the gap is depends on the column widths, so cross the whole stretch it can be in
cross_gap() {
    for x in $(seq 900 4 1200); do
        pointer m "$x" 232
        sleep 0.05
    done
    sleep 0.3
}
check_all() {
    blank
    hover 2253 116
    expect "$1 text field" "$from" 9
    blank
    cross_gap
    expect "$1 column gap" "$from" 30
    blank
    hover 200 185
    expect "$1 button" "$from" 4
}

# The daemon passes its environment on to the window, and only this run needs the protocol log
dp quit >/dev/null 2>&1
sleep 2
WAYLAND_DEBUG=client dp show -c "$W/tree" >>"$W/out/show.log" 2>&1 &
SHOW=$!
sleep 1
wait_for_daemon
sleep 5

go tracks
sleep 2
check_all "unfocused"

# A focused search field puts a press catcher over the whole window, which must leave the cursors alone
click 2253 116
sleep 1
check_all "focused"

nohand() { # NAME FROM
    if logsince "$L" "$2" | grep -qE "set_shape\([0-9]+, 4\)"; then
        echo "FAIL $1: set_shape 4" | tee -a "$W/out/facts.txt"
        failed=1
    else
        echo "ok   $1: no set_shape 4" >>"$W/out/facts.txt"
    fi
}
round() { # NAME X Y W H, the item's box in window coordinates
    if [ -z "$5" ]; then
        echo "FAIL $1: not shown" | tee -a "$W/out/facts.txt"
        failed=1
        return
    fi
    local wx wy
    read -r wx wy < <(winat)
    blank
    hover "$((wx + $2 + $4 - 3))" "$((wy + $3 + 3))"
    nohand "$1 corner" "$from"
    blank
    hover "$((wx + $2 + $4 / 2))" "$((wy + $3 + $5 / 2))"
    expect "$1 centre" "$from" 4
}
wtype -k Escape
dp play "$W/music" >/dev/null
ui coverShape side rounded
sleep 2
ui element dock bar
sleep 2
read -r cx cy cw ch _ < <(ask coverRect)
round "side panel cover" "$cx" "$cy" "$cw" "$ch"
ui element dock bar
ui playerLook floating
sleep 2
read -r rx ry rw rh _ < <(ask rect floatingRing)
round "floating ring" "$rx" "$ry" "$rw" "$rh"

cp "$L" "$W/out/wl.log"
[ "$failed" = 0 ]
