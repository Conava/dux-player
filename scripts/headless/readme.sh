# The README's screenshots, from a made-up library: make headless SCENARIO=scripts/headless/readme.sh
# Everything shown is generated here: artists, titles, lyrics and cover art.
docs=$(dirname "$LIB")/../../docs/screenshots
music=$W/music

album() { # artist, album, seed, colour-colour, genre, year, tracks...
    local artist=$1 name=$2 seed=$3 colours=$4 genre=$5 year=$6 n=0 dir="$music/$1/$2"
    shift 6
    mkdir -p "$dir"
    magick -size 800x800 -seed "$seed" "plasma:$colours" -blur 0x24 -swirl $((90 + seed % 7 * 20)) -modulate 100,135 "$dir/cover.png"
    for title in "$@"; do
        n=$((n + 1))
        ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -i "$dir/cover.png" -map 0 -map 1 -t $((150 + (n * 37 + seed * 11) % 140)) \
            -c:v copy -disposition:v attached_pic -metadata title="$title" -metadata artist="$artist" -metadata album="$name" \
            -metadata track="$n" -metadata date="$year" -metadata genre="$genre" "$dir/$n.flac"
    done
}

rm -rf "$music/Test Artist"
album "Sundial Club" "Harbor Lights" 11 "#f97316-#7c2d12" Indie 2024 "Harbor Lights" "Salt on the Dial" "Long Way to Noon" "Ferry at Dusk" "Tin Lanterns" "Last Boat Home"
album "Paper Orbit" "Slow Satellites" 23 "#6366f1-#0f172a" Electronic 2022 "Slow Satellites" "Fold Along the Line" "Perigee" "Quiet Telemetry" "Ground Station" "Return Window"
album "Low Tide Radio" "Static Garden" 37 "#10b981-#064e3b" Ambient 2021 "Static Garden" "Fern Frequency" "Moss Signal" "Open Band" "Greenhouse Hum"
album "Glass Meadow Trio" "Thin Air" 41 "#ec4899-#4c0519" Jazz 2023 "Thin Air" "Second Bloom" "Porcelain" "Soft Landing" "Afterglow"
album "Sundial Club" "Noon Songs" 53 "#eab308-#713f12" Indie 2020 "Noon Songs" "Bright Corners" "Shade Line" "Sun Clock"
album "Velvet Hours" "Neon Lullabies" 61 "#d946ef-#3b0764" Synthpop 2023 "Neon Lullabies" "Night Bus Home" "Violet Static" "Rooftop Weather" "Slow Strobe"
album "Marrow & Moss" "Field Recordings" 72 "#84cc16-#1a2e05" Folk 2019 "Gravel Road" "Kettle Song" "Hedgerow" "Barn Light" "Wool Coat"
album "Tidal Atlas" "Cold Currents" 85 "#06b6d4-#083344" Electronic 2024 "Cold Currents" "Kelp Forest" "Pressure Drop" "Sonar Bloom" "Deep Shelf"
album "Ember Choir" "Ash and Honey" 97 "#ef4444-#450a0a" Rock 2018 "Ash and Honey" "Smoke Signals" "Ten Thousand Matches" "Slow Burn" "Cinder Hymn"
album "Quiet Parade" "Small Hours" 104 "#a78bfa-#1e1b4b" Pop 2022 "Small Hours" "Paper Crowns" "Elevator Waltz" "Postcards" "Lights Out"
album "Pale Cartographer" "Unmapped" 118 "#fb923c-#431407" Alternative 2021 "Unmapped" "North by Accident" "Compass Rose" "Dead Reckoning" "Uncharted Sunday"
album "Juniper Wire" "Copper Lines" 126 "#14b8a6-#042f2e" Soul 2023 "Copper Lines" "Wire Fence" "Sunday Voltage" "Green Light" "Warm Static"

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
    grim -g "$(window "${2:-main}" '%d,%d %dx%d')" "${3:-$docs}/$1.png"
}

hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1000})' >/dev/null

tracks=$(dp ipc browse.tracks library="$library" | json 'json.dumps([{"library": t["library"], "id": t["id"]} for t in (d["tracks"] if isinstance(d, dict) else d)])')
pick() { echo "$tracks" | json "json.dumps(d[$1])"; }
rpc localPlaylists.create "{\"name\":\"Evening drive\",\"tracks\":$(pick '0:9')}" >/dev/null
rpc localPlaylists.create "{\"name\":\"Deep focus\",\"tracks\":$(pick '6:17')}" >/dev/null
rpc localPlaylists.create "{\"name\":\"Sunday morning\",\"tracks\":$(pick '12:20')}" >/dev/null

ui artColors false

# marks: kind, index into the track list, then the method and its value
mark() { rpc "$3" "{\"library\":\"$library\",\"kind\":\"$1\",\"id\":\"$(echo "$tracks" | json "d[$2]['id']")\",\"$4\":$5}" >/dev/null; }
for i in 1 4 7 9 13 16 19 22 26 31 38 44 51; do mark track "$i" marks.setFavorite favorite true; done
for i in 0 2 5 8 10 12 15 18 21 25 28 33 40 47 55; do mark track "$i" marks.setRating rating $((3 + i % 3)); done

# play counts and the history: each track is played past the half
for i in 40 33 27 20 14 8 3 50 11; do
    rpc player.play "{\"tracks\":[$(pick "$i")]}" >/dev/null
    sleep 1.5
    dp seek 120 >/dev/null
    sleep 1
    dp next >/dev/null
done

dp play "$music/Sundial Club/Harbor Lights" >/dev/null
sleep 2
dp seek 30 >/dev/null

# 1. Home, Bento, dark, with the default shelves
look dark bento
ui navStyle railLabels
ui surface home '{"zones":{"shelves":[{"id":"continue","type":"continue","hidden":false,"placement":"auto","options":{"layout":"carousel","size":"m"}},{"id":"added","type":"recentlyAdded","hidden":false,"placement":"auto","options":{"layout":"carousel","size":"m"}},{"id":"favorites","type":"favoriteTracks","hidden":false,"placement":"auto","options":{"layout":"list","size":"s"}}]}}'
go home
cap home

# 2. Home, Material, light, with other shelves and the navigation along the bottom
look light m3e
ui navStyle bar
ui surface home '{"zones":{"shelves":[{"id":"added","type":"recentlyAdded","hidden":false,"placement":"auto","options":{"layout":"grid","size":"m"}},{"id":"mostplayed","type":"mostPlayed","hidden":false,"placement":"auto","options":{"layout":"list","size":"m"}},{"id":"playlists","type":"playlists","hidden":false,"placement":"auto","options":{"layout":"carousel","size":"m"}}]}}'
go home
cap home-custom
ui resetSurface home
ui navStyle auto

# 3. Bottom bar and side panel in both styles, side by side
tmp=$W/out/bars
mkdir -p "$tmp"
look dark bento
ui setStyle start bar
go home
cap bar-bento main "$tmp"
ui setStyle start side
cap side-bento main "$tmp"
look dark m3e
cap side-material main "$tmp"
ui setStyle start bar
cap bar-material main "$tmp"
ui setStyle start bar
montage -background '#1d1b24' -fill '#e6e1ec' -font Adwaita-Sans -pointsize 30 -tile 2x2 -geometry 1000x625+20+20 \
    -label 'Bento, bottom bar' "$tmp/bar-bento.png" -label 'Material, bottom bar' "$tmp/bar-material.png" \
    -label 'Bento, side panel' "$tmp/side-bento.png" -label 'Material, side panel' "$tmp/side-material.png" "$docs/bars.png"

# 4. The full views and the lyrics
look dark bento
expand true
cap now-playing
element lyrics view
cap lyrics
element lyrics view
sleep 1
look dark m3e
cap now-playing-material
expand false

# 5. Customized: a tracks page with custom columns and its editor, and a full view with other elements
look dark bento
go tracks
ui pageOptions tracks '{"lines":["artist","album","rating","custom:c1"],"lineOptions":{"title":{"size":0.4},"artist":{"size":3},"album":{"size":3},"custom:c1":{"size":2,"title":"Style","template":"%genre%"}}}'
sleep 1
cap tracks
ui edit tracks
sleep 1
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1400})' >/dev/null
cap page-editor
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1000})' >/dev/null
go home
ui setStyle playerLook attached
rpc marks.setRating "{\"library\":\"$library\",\"kind\":\"track\",\"id\":\"$(dp ipc browse.tracks library="$library" | json "[t['id'] for t in (d['tracks'] if isinstance(d, dict) else d) if t['title'] == 'Harbor Lights'][0]")\",\"rating\":4}" >/dev/null
ui addElement fullBento bottom '{"type":"rating","id":"rating"}'
ui addElement fullBento title '{"type":"text","id":"text","options":{"template":"%genre% \u00b7 %year%"}}'
ui coverShape circle
expand true
cap now-playing-custom

# 6. The sources and outputs it works with, as plain text on generic icons (no third-party logos)
cat >"$W/out/sources.svg" <<'S'
<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="560" viewBox="0 0 1600 560" font-family="Adwaita Sans, Noto Sans, sans-serif">
<rect width="1600" height="560" rx="44" fill="#1d1b24"/>
<g fill="#b8b2c4" font-size="26" font-weight="600" letter-spacing="3"><text x="60" y="84">LIBRARIES</text><text x="1200" y="84">OUTPUT</text></g>
<line x1="1170" y1="120" x2="1170" y2="500" stroke="#3a3645" stroke-width="3"/>
<g fill="#2b2833"><rect x="60" y="116" width="340" height="384" rx="36"/><rect x="430" y="116" width="340" height="384" rx="36"/><rect x="800" y="116" width="340" height="384" rx="36"/><rect x="1200" y="116" width="340" height="384" rx="36"/></g>
<g><circle cx="230" cy="226" r="62" fill="#f97316"/><circle cx="600" cy="226" r="62" fill="#6366f1"/><circle cx="970" cy="226" r="62" fill="#10b981"/><circle cx="1370" cy="226" r="62" fill="#ec4899"/></g>
<g fill="none" stroke="#fff" stroke-width="6" stroke-linecap="round" stroke-linejoin="round">
<path transform="translate(230 226)" d="M-32,-22 h22 l10,12 h34 v40 h-66 z"/>
<g transform="translate(600 226)"><rect x="-32" y="-32" width="64" height="28" rx="8"/><rect x="-32" y="4" width="64" height="28" rx="8"/></g>
<g transform="translate(970 226)"><rect x="-34" y="-28" width="68" height="56" rx="10"/><path d="M-8,-12 L14,0 L-8,12 Z"/></g>
<g transform="translate(1370 226)"><path d="M-34,-4 v-20 a6,6 0 0 1 6,-6 h56 a6,6 0 0 1 6,6 v48 a6,6 0 0 1 -6,6 h-18"/><path d="M-34,12 a18,18 0 0 1 18,18 M-34,-2 a32,32 0 0 1 32,32"/></g>
</g>
<g fill="#fff" stroke="none"><circle cx="587" cy="238" r="4"/><circle cx="587" cy="210" r="4"/></g>
<g fill="#e6e1ec" font-size="36" font-weight="700" text-anchor="middle">
<text x="230" y="352">Local folders</text><text x="600" y="352">(Open)Subsonic</text><text x="970" y="352">Jellyfin</text><text x="1370" y="352">Chromecast</text></g>
<g fill="#b8b2c4" font-size="25" text-anchor="middle">
<text x="230" y="402">Music on disk with</text><text x="230" y="436">tags, art and lyric files</text>
<text x="600" y="402">Servers such as</text><text x="600" y="436">Navidrome</text>
<text x="970" y="402">Music libraries,</text><text x="970" y="436">with Quick Connect</text>
<text x="1370" y="402">Play on Cast devices</text><text x="1370" y="436">on the network</text></g>
</svg>
S
rsvg-convert "$W/out/sources.svg" -o "$docs/sources.png"
