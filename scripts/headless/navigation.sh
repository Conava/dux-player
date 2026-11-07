# Back and forward from the keyboard and the mouse side buttons, with and without the full view open. Real keys
# through wtype and a virtual pointer. Fails when a route is wrong.
failed=0
expect() {
    if [ "$2" = "$3" ]; then
        echo "ok   $1: $3" >>"$W/out/facts.txt"
    else
        echo "FAIL $1: wanted '$3', got '$2'" | tee -a "$W/out/facts.txt"
        failed=1
    fi
}
route() { ask route | tr -d '\n'; }
view() { ask nowPlaying | tr -d '\n'; }
alt() { wtype -M alt -k "$1" -m alt; sleep 0.8; }

# the pointer in the window gives it the keyboard focus
pointer m 1278 720
sleep 0.2
pointer m 1280 720
sleep 0.5
dp play "$W/music" >/dev/null
sleep 2

go tracks
go albums
alt Left
expect "alt+left" "$(route)" "tracks/"
alt Right
expect "alt+right" "$(route)" "albums/"

pointer b
sleep 0.8
expect "side button back" "$(route)" "tracks/"
pointer f
sleep 0.8
expect "extra button forward" "$(route)" "albums/"

alt Left
go libraries
alt Right
expect "forward after a new page" "$(route)" "libraries/"

go tracks
go albums
expand true
alt Left
expect "alt+left closes the view" "$(view)" "false"
expect "alt+left keeps the route" "$(route)" "albums/"
expand true
pointer b
sleep 0.8
expect "side button closes the view" "$(view)" "false"
expect "side button keeps the route" "$(route)" "albums/"
pointer b
sleep 0.8
expect "side button then moves" "$(route)" "tracks/"

ui entry 0
sleep 1
expand true
alt Left
expect "view closed, no history" "$(view)" "false"

go tracks
alt Left
expand true
alt Right
expect "forward closes the view" "$(view)" "false"
expect "forward from the view moves" "$(route)" "tracks/"

go albums
element sleep bar
sleep 1
before=$(route)
pointer b
sleep 0.8
expect "side button under a dialog" "$(route)" "$before"
alt Left
expect "alt+left under a dialog" "$(route)" "$before"
wtype -k Escape
sleep 1
shot navigation

[ "$failed" = 0 ]
