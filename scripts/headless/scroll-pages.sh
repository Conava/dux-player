# Scroll restore on Home and on a track list in an album page: back puts the offset back, a new visit or another album
# starts at the top. Fails when an offset is wrong.
failed=0
fact() {
    echo "$1" >>"$W/out/facts.txt"
}
fail() {
    echo "FAIL $1" | tee -a "$W/out/facts.txt"
    failed=1
}
near() {
    if awk "BEGIN { d = $2 - $3; exit !(d < 2 && d > -2) }"; then fact "ok   $1: $2"; else fail "$1: wanted $3, got $2"; fi
}
below() {
    if awk "BEGIN { exit !($2 + 20 < $3) }"; then fact "ok   $1: $2 under $3"; else fail "$1: wanted $2 well under $3"; fi
}
offset() { ask offset | tr -d '\n'; }
alt() { wtype -M alt -k "$1" -m alt; sleep 1; }
wheel() { pointer w "$1"; sleep 1; }
silence() { ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -t 1 -metadata title="$1" -metadata artist="$2" -metadata album="$3" -metadata track="$4" "$5"; }

for i in $(seq -w 1 30); do
    mkdir -p "$W/music/Artist $i/Album $i"
    silence "Song $i" "Artist $i" "Album $i" 1 "$W/music/Artist $i/Album $i/1.flac"
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
    [ -n "$(ask albumId "Long Album 2" | tr -d '\n')" ] && scanned=1 && break
    sleep 0.5
done
if [ "$scanned" = 0 ]; then
    echo "FAIL the scan did not finish in 30 s" | tee -a "$W/out/facts.txt"
    failed=1
fi

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

[ "$failed" = 0 ]
