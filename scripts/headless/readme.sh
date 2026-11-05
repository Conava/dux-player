# The README's screenshots, from a made-up library: make headless SCENARIO=scripts/headless/readme.sh
# Everything shown is generated here: artists, titles, lyrics and cover art.
docs=$(dirname "$LIB")/../../docs/screenshots
music=$W/music

album() { # artist, album, seed, colour-colour, tracks...
    local artist=$1 name=$2 seed=$3 colours=$4 n=0 dir="$music/$1/$2"
    shift 4
    mkdir -p "$dir"
    magick -size 800x800 -seed "$seed" "plasma:$colours" -blur 0x24 -swirl 140 -modulate 100,135 "$dir/cover.png"
    for title in "$@"; do
        n=$((n + 1))
        ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -i "$dir/cover.png" -map 0 -map 1 -t $((170 + n * 17)) \
            -c:v copy -disposition:v attached_pic -metadata title="$title" -metadata artist="$artist" -metadata album="$name" \
            -metadata track="$n" -metadata date=2024 -metadata genre=Electronic "$dir/$n.flac"
    done
}

rm -rf "$music/Test Artist"
album "Sundial Club" "Harbor Lights" 11 "#f97316-#7c2d12" "Harbor Lights" "Salt on the Dial" "Long Way to Noon" "Ferry at Dusk" "Tin Lanterns" "Last Boat Home"
album "Paper Orbit" "Slow Satellites" 23 "#6366f1-#0f172a" "Slow Satellites" "Fold Along the Line" "Perigee" "Quiet Telemetry" "Ground Station" "Return Window"
album "Low Tide Radio" "Static Garden" 37 "#10b981-#064e3b" "Static Garden" "Fern Frequency" "Moss Signal" "Open Band" "Greenhouse Hum"
album "Glass Meadow Trio" "Thin Air" 41 "#ec4899-#4c0519" "Thin Air" "Second Bloom" "Porcelain" "Soft Landing" "Afterglow"
album "Sundial Club" "Noon Songs" 53 "#eab308-#713f12" "Noon Songs" "Bright Corners" "Shade Line" "Sun Clock"

cat >"$music/Sundial Club/Harbor Lights/1.lrc" <<'L'
[00:00.00]The tide keeps time along the pier
[00:08.00]A row of lamps comes on one by one
[00:16.00]Somebody hums a song I almost know
[00:24.00]And the harbor lights are leading me home
[00:32.00]Salt in the wind, a slow bell ringing
[00:40.00]Every boat a small and steady star
[00:48.00]I count them till the water turns to silver
[00:56.00]And the harbor lights are never far
[01:04.00]Hold the line and let the evening in
[01:12.00]There is room for one more on the shore
L
json() { python3 -I -c "import json, sys; d = json.load(sys.stdin); d = d.get('result', d) if isinstance(d, dict) else d; print(eval(sys.argv[1]))" "$1"; }
library=$(dp ipc libraries.list | json 'd[0]["id"]')
rpc libraries.scan "{\"library\":\"$library\"}" >/dev/null
sleep 6

# one window's own pixels: the main one, or the Settings window with "settings"
window() {
    hyprctl -j clients | python3 -I -c '
import json, sys
main = sys.argv[1] != "settings"
c = next(c for c in json.load(sys.stdin) if (c["title"] == "Dux Player") == main)
print(sys.argv[2] % (*c["at"], *c["size"]))' "$1" "$2"
}
cap() {
    sleep "${SETTLE:-1.5}"
    grim -g "$(window "${2:-main}" '%d,%d %dx%d')" "$docs/$1.png"
}

hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1000})' >/dev/null

tracks=$(dp ipc browse.tracks library="$library" | json 'json.dumps([{"library": t["library"], "id": t["id"]} for t in (d["tracks"] if isinstance(d, dict) else d)])')
pick() { echo "$tracks" | json "json.dumps(d[$1])"; }
rpc localPlaylists.create "{\"name\":\"Evening drive\",\"tracks\":$(pick '0:9')}" >/dev/null
rpc localPlaylists.create "{\"name\":\"Deep focus\",\"tracks\":$(pick '6:17')}" >/dev/null
rpc localPlaylists.create "{\"name\":\"Sunday morning\",\"tracks\":$(pick '12:20')}" >/dev/null

dp play "$music/Sundial Club/Harbor Lights" >/dev/null
sleep 2
dp seek 30 >/dev/null
look dark bento
ui artColors true
go home
cap home
look light bento
cap light
look dark bento
ui element dock bar
cap side-panel
ui element dock bar
expand true
cap now-playing
element lyrics view
cap lyrics
element lyrics view
sleep 1
look dark m3e
cap now-playing-material
element queue view
cap material-queue
element queue view
expand false
look dark bento
go albums
cap albums
ui open album "$library:$(dp ipc browse.albums library="$library" | json '(d["albums"] if isinstance(d, dict) else d)[0]["id"]')"
cap album
go playlists
cap playlists
ui element addToPlaylist view
cap playlist-dialog
wtype -k Escape
sleep 1
ui settings nowplaying
sleep 2
cap settings settings
click $(window settings '%d %d %d %d' | awk '{ print $1 + 130, $2 + 84 }')
sleep 0.5
wtype lyrics
cap settings-search settings
