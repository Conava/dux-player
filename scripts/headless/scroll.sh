# Back and forward put a page's scroll offset back, a new visit starts at the top, and a late layout change doesn't
# pull the view back once the user has scrolled; the same on Home and on an album's track list. Fails when an offset is wrong.
failed=0
fact() {
    echo "$1" >>"$W/out/facts.txt"
}
fail() {
    echo "FAIL $1" | tee -a "$W/out/facts.txt"
    failed=1
}
# name, the offset, what it should be: within 2 px
near() {
    if awk "BEGIN { d = $2 - $3; exit !(d < 2 && d > -2) }"; then fact "ok   $1: $2"; else fail "$1: wanted $3, got $2"; fi
}
# name, a, b: a is at least 20 px below b
below() {
    if awk "BEGIN { exit !($2 + 20 < $3) }"; then fact "ok   $1: $2 under $3"; else fail "$1: wanted $2 well under $3"; fi
}
offset() { ask offset | tr -d '\n'; }
alt() { wtype -M alt -k "$1" -m alt; sleep 1; }
wheel() { pointer w "$1"; sleep 1; }
silence() { ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -t 1 -metadata title="$1" -metadata artist="$2" -metadata album="$3" -metadata track="$4" "$5"; }

for i in $(seq -w 1 40); do
    dir="$W/music/Artist $i/Album $i"
    mkdir -p "$dir"
    for n in 1 2; do
        ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -t 1 -metadata title="Song $i $n" -metadata artist="Artist $i" \
            -metadata album="Album $i" -metadata track="$n" "$dir/$n.flac"
    done
done
for a in "Long Album" "Long Album 2"; do
    mkdir -p "$W/music/Long Artist/$a"
    for n in $(seq -w 1 40); do
        silence "$a $n" "Long Artist" "$a" "$n" "$W/music/Long Artist/$a/$n.flac"
    done
done
library=$(dp ipc libraries.list | python3 -I -c 'import json, sys; d = json.load(sys.stdin); print(d.get("result", d)[0]["id"] if isinstance(d, dict) else d[0]["id"])')
rpc libraries.scan "{\"library\":\"$library\"}" >/dev/null
scanned=0
for _ in $(seq 1 60); do
    [ -n "$(ask albumId "Long Album 2" | tr -d '\n')" ] && [ -n "$(ask albumId "Album 40" | tr -d '\n')" ] && scanned=1 && break
    sleep 0.5
done
if [ "$scanned" = 0 ]; then
    echo "FAIL the scan did not finish in 30 s" | tee -a "$W/out/facts.txt"
    failed=1
fi

# the pointer in the window gives it the keyboard focus, and the wheel goes to what is under it
pointer m 1278 720
sleep 0.2
pointer m 1280 720
sleep 0.5

# three large album grids make Home taller than the window
grid() { echo "{\"id\":\"$1\",\"type\":\"$2\",\"hidden\":false,\"placement\":\"auto\",\"options\":{\"layout\":\"grid\",\"size\":\"l\"}}"; }
ui surface home "{\"zones\":{\"shelves\":[$(grid a recentlyAdded),$(grid b randomAlbums),$(grid c randomAlbums)]}}"
go home
wheel 6
h=$(offset)
below "home scrolled" 0 "$h"
go libraries
alt Left
near "home, back" "$(offset)" "$h"
shot home
go libraries
go home
near "home, new visit starts at the top" "$(offset)" 0

long=$(ask albumId "Long Album" | tr -d '\n')
other=$(ask albumId "Long Album 2" | tr -d '\n')
ui open album "$long"
sleep 2
wheel 6
l=$(offset)
below "album tracks scrolled" 0 "$l"
go libraries
alt Left
near "album tracks, back" "$(offset)" "$l"
shot album
ui open album "$other"
sleep 1.5
near "another album starts at the top" "$(offset)" 0
ui open album "$long"
sleep 1.5
near "the album again, new visit starts at the top" "$(offset)" 0


ui pageOptions albums '{"layout":"list"}'
go albums
wheel 10
a=$(offset)
below "albums scrolled" 0 "$a"
go libraries
alt Left
near "back to another file" "$(offset)" "$a"
shot back

go tracks
near "new page starts at the top" "$(offset)" 0
wheel 12
t=$(offset)
below "tracks scrolled" 0 "$t"
alt Left
near "back on the same page file" "$(offset)" "$a"
alt Right
near "forward" "$(offset)" "$t"

# the shrinking header would put an offset still held right back on t
wheel -2
ui pageOptions tracks '{"shuffleButton":false}'
sleep 1
below "a scrolled view keeps its place" "$(offset)" "$t"
ui pageOptions tracks '{"shuffleButton":true}'
sleep 1

alt Left
go artists
near "same view, new route starts at the top" "$(offset)" 0
alt Left
near "same view, back" "$(offset)" "$a"

ui entry 0
sleep 1
go tracks
near "after a nav entry" "$(offset)" 0

# one notch from a restored offset lands on the top bound without a stop between the bounds: the held offset has to go
header() {
    ui pageOptions tracks '{"shuffleButton":false}'
    sleep 1
    ui pageOptions tracks '{"shuffleButton":true}'
    sleep 1
}
wheel 1
n=$(offset)
below "one notch" 0 "$n"
go libraries
alt Left
near "one notch, back" "$(offset)" "$n"
wheel -1
near "wheel to the top" "$(offset)" 0
header
near "wheel to the top stays" "$(offset)" 0

# the same with the scrollbar, in the gutter right of the rows
wheel 1
go libraries
alt Left
near "scrollbar, back" "$(offset)" "$n"
pointer m 2511 120
sleep 0.5
pointer d
sleep 0.1
pointer m 2511 20
sleep 0.2
pointer u
sleep 1
near "scrollbar to the top" "$(offset)" 0
header
near "scrollbar to the top stays" "$(offset)" 0
pointer m 1280 720
sleep 0.5

# typing in the page's filter starts over from where the view is
go tracks
go albums
wheel 10
f=$(offset)
go libraries
alt Left
near "before the filter" "$(offset)" "$f"
ui focusSearch
sleep 1
wtype "Album 01"
sleep 1
wtype -k BackSpace -k BackSpace -k BackSpace -k BackSpace -k BackSpace -k BackSpace -k BackSpace -k BackSpace
sleep 1
below "a filter drops the held offset" "$(offset)" "$f"
wtype -k Escape
sleep 0.3
wtype -k Escape
sleep 0.5

# search scrolls a plain Flickable, as home does
go search
wtype "Song"
sleep 2
wheel 3
s=$(offset)
below "search scrolled" 0 "$s"
go libraries
alt Left
near "search, back" "$(offset)" "$s"
wtype -k Escape
sleep 0.3

go albums
wheel 200
end=$(offset)
below "albums at the end" "$a" "$end"
go libraries
ui pageOptions albums '{"rules":{"match":"all","list":[{"field":"title","op":"startsWith","value":"Album 1"}]}}'
sleep 1
alt Left
below "clamped to a shorter list" "$(offset)" "$end"
shot clamped
# rows that come after the user has left: the offset still waiting for them is what back restores
go libraries
ui pageOptions albums '{"rules":{"match":"all","list":[]}}'
sleep 1
alt Left
near "left before the rows came" "$(offset)" "$end"
ui pageOptions albums '{"rules":{"match":"all","list":[]},"layout":"grid","tileSize":"l"}'
sleep 1

wheel 5
g=$(offset)
below "grid scrolled" 0 "$g"
go libraries
alt Left
near "grid, back" "$(offset)" "$g"
shot grid

[ "$failed" = 0 ]
