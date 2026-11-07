# Hovers a text field, a column gap and a button, and fails when the window doesn't ask the compositor for their
# cursors. Screenshots don't show the pointer, so this reads the Wayland requests: set_shape with 1 default, 4 pointer,
# 9 text, 30 col_resize.
failed=0
L="$XDG_STATE_HOME/dplayer/ui.log"
expect() {
    if tail -n "+$2" "$L" | grep -qE "set_shape\([0-9]+, $3\)"; then
        echo "ok   $1: set_shape $3" >>"$W/out/facts.txt"
    else
        echo "FAIL $1: no set_shape $3" | tee -a "$W/out/facts.txt"
        failed=1
    fi
}
blank() {
    pointer m 1400 1000
    sleep 0.5
    from=$(($(wc -l <"$L") + 1))
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

cp "$L" "$W/out/wl.log"
[ "$failed" = 0 ]
