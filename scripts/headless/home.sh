# Home shelves: Random albums takes in albums a scan adds, and crossing the compact width or changing an unrelated
# layout setting leaves the shelves as they are (Random albums keeps its order). Fails when either breaks.
failed=0
fact() {
    echo "$1" >>"$W/out/facts.txt"
}
fail() {
    echo "FAIL $1" | tee -a "$W/out/facts.txt"
    failed=1
}
expect() {
    if [ "$2" = "$3" ]; then fact "ok   $1: $2"; else fail "$1: wanted $3, got $2"; fi
}
silence() { ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -t 1 -metadata title="$1" -metadata artist="$2" -metadata album="$3" -metadata track=1 "$4"; }
shelf() { ask shelves | grep "^$1 " | head -1; }
resize() {
    hyprctl dispatch "hl.dsp.window.resize({x=$1,y=$2})" >/dev/null
    sleep 2
}

ui headerSearch field
hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
resize 1600 1000
go home
expect "one album before the scan" "$(shelf randomAlbums | cut -d' ' -f3)" 1

for i in $(seq -w 1 40); do
    mkdir -p "$W/music/Artist $i/Album $i"
    silence "Song $i" "Artist $i" "Album $i" "$W/music/Artist $i/Album $i/1.flac"
done
library=$(dp ipc libraries.list | python3 -I -c 'import json, sys; d = json.load(sys.stdin); print(d.get("result", d)[0]["id"] if isinstance(d, dict) else d[0]["id"])')
rpc libraries.scan "{\"library\":\"$library\"}" >/dev/null
scanned=0
for _ in $(seq 1 60); do
    [ -n "$(ask albumId "Album 40" | tr -d '\n')" ] && scanned=1 && break
    sleep 0.5
done
[ "$scanned" = 0 ] && fail "the scan did not finish in 30 s"
sleep 3
expect "recently added after the scan" "$(shelf recentlyAdded | cut -d' ' -f3)" 41
expect "random albums after the scan" "$(shelf randomAlbums | cut -d' ' -f3)" 41
shot scanned

random=$(shelf randomAlbums)
state=$(shelf search | cut -d' ' -f2)
expect "wide: home hides its search field" "${state:-hidden}" hidden
resize 700 900
expect "compact: home shows its search field" "$(shelf search | cut -d' ' -f2)" shown
expect "compact: random albums kept its order" "$(shelf randomAlbums)" "$random"
shot compact
resize 1600 1000
expect "wide again: random albums kept its order" "$(shelf randomAlbums)" "$random"
ui pageOptions tracks '{"lines":["format"]}'
sleep 1
expect "another page's options: random albums kept its order" "$(shelf randomAlbums)" "$random"

# a reload that keeps the album count (one removed, one added) still shows the new album
gone=$(ask albumId "Album 01" | tr -d '\n')
rm -rf "$W/music/Artist 01"
mkdir -p "$W/music/Artist 99/Album 99"
silence "Song 99" "Artist 99" "Album 99" "$W/music/Artist 99/Album 99/1.flac"
rpc libraries.scan "{\"library\":\"$library\"}" >/dev/null
for _ in $(seq 1 60); do
    [ -n "$(ask albumId "Album 99" | tr -d '\n')" ] && break
    sleep 0.5
done
sleep 3
new=$(ask albumId "Album 99" | tr -d '\n')
expect "random albums count after swapping an album" "$(shelf randomAlbums | cut -d' ' -f3)" 41
case "$(shelf randomAlbums)" in *"$new"*) fact "ok   random albums has the new album" ;; *) fail "random albums lacks the new album" ;; esac
case "$(shelf randomAlbums)" in *"$gone"*) fail "random albums still has the removed album" ;; *) fact "ok   random albums dropped the removed album" ;; esac

[ "$failed" = 0 ]
