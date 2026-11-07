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

# the first entry deleted with no history to go back to, by Delete page at 516,785 and the dialog's Delete at 1498,768
ui editFirstView
sleep 2
pointer w 30
sleep 1.5
click 516 785
sleep 1.5
click 1498 768
sleep 1.5
shot deleted-first
expect "deleting the first entry opens the next" "$(route)" "home/"
pointer m 1280 720
sleep 0.5

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

# a real mouse holds the button for a while and moves a little: one action on release either way
held() {
    # the pointer helper returns at once, so wait out the hold too
    wait=$(awk "BEGIN { print $1 / 1000 + 0.8 }")
    go tracks
    go albums
    pointer b "$1" "$2"
    sleep "$wait"
    expect "side button back, $3" "$(route)" "tracks/"
    go tracks
    go albums
    alt Left
    pointer f "$1" "$2"
    sleep "$wait"
    expect "extra button forward, $3" "$(route)" "albums/"
}
held 1000 0 "held 1 s"
held 300 20 "moved 20 px"
pointer m 1280 720
sleep 0.3

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

# a playlist deleted from another page leaves both stacks
playlist() { rpc localPlaylists.create "{\"name\":\"$1\"}" | python3 -I -c 'import json, sys; print(":" + json.load(sys.stdin)["result"]["id"])'; }
behind=$(playlist Behind)
ahead=$(playlist Ahead)
sleep 1
go libraries
ui open playlist "$behind"
sleep 1
go albums
ui deletePlaylist "$behind"
sleep 1
alt Left
expect "back skips a deleted playlist" "$(route)" "libraries/"

ui open playlist "$ahead"
sleep 1
go albums
alt Left
alt Left
ui deletePlaylist "$ahead"
sleep 1
alt Right
expect "forward skips a deleted playlist" "$(route)" "albums/"

# what the deleted page separated is one step: equal neighbours and a top equal to the current route make no dead press
twice=$(playlist Twice)
sleep 1
go tracks
go libraries
ui open playlist "$twice"
sleep 1
go libraries
go albums
ui deletePlaylist "$twice"
sleep 1
alt Left
expect "back to the route around a deleted playlist" "$(route)" "libraries/"
alt Left
expect "back past the route around a deleted playlist" "$(route)" "tracks/"

top=$(playlist Top)
sleep 1
go libraries
go tracks
ui open playlist "$top"
sleep 1
go tracks
ui deletePlaylist "$top"
sleep 1
alt Left
expect "back from the route around a deleted playlist" "$(route)" "libraries/"

ahead2=$(playlist Ahead2)
sleep 1
go history
go tracks
ui open playlist "$ahead2"
sleep 1
go tracks
go albums
alt Left
alt Left
alt Left
alt Left
ui deletePlaylist "$ahead2"
sleep 1
alt Right
expect "forward to the route around a deleted playlist" "$(route)" "tracks/"
alt Right
expect "forward past the route around a deleted playlist" "$(route)" "albums/"

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
