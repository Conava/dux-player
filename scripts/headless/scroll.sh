# Back and forward put a page's scroll offset back, a new visit starts at the top, and a late layout change doesn't
# pull the view back once the user has scrolled. Fails when an offset is wrong.
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

for i in $(seq -w 1 40); do
    dir="$W/music/Artist $i/Album $i"
    mkdir -p "$dir"
    for n in 1 2; do
        ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -t 1 -metadata title="Song $i $n" -metadata artist="Artist $i" \
            -metadata album="Album $i" -metadata track="$n" "$dir/$n.flac"
    done
done
library=$(dp ipc libraries.list | python3 -I -c 'import json, sys; d = json.load(sys.stdin); print(d.get("result", d)[0]["id"] if isinstance(d, dict) else d[0]["id"])')
rpc libraries.scan "{\"library\":\"$library\"}" >/dev/null
sleep 6

# the pointer in the window gives it the keyboard focus, and the wheel goes to what is under it
pointer m 1278 720
sleep 0.2
pointer m 1280 720
sleep 0.5

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
