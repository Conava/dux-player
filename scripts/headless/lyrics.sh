# Synced lyrics: word fill, line-timed lines, instrumental breaks, follow, reduce motion, in both styles.
# The first line starts after a long intro and a blank LRC line opens a gap at 0:21.
music=$W/music
dir="$music/Lyric Test/Fixture"
mkdir -p "$dir"
magick -size 600x600 -seed 7 "plasma:#6366f1-#0f172a" -blur 0x24 "$dir/cover.png"
n=0
for title in "Synced Track" "Unsynced Track" "Duet Track"; do
    n=$((n + 1))
    ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -i "$dir/cover.png" -map 0 -map 1 -t 120 -c:v copy \
        -disposition:v attached_pic -metadata title="$title" -metadata artist="Lyric Test" -metadata album="Fixture" \
        -metadata track=$n "$dir/$n.flac"
done
stamp() { python3 -I -c 'import sys; t = float(sys.argv[1]); print(sys.argv[2] % (t // 60, t % 60), end="")' "$1" "$2"; }
words() { # start seconds, seconds per word, words...
    local at=$1 step=$2 line
    line=$(stamp "$1" '[%02d:%05.2f]')
    shift 2
    for word in "$@"; do
        line+="$(stamp "$at" '<%02d:%05.2f>')$word "
        at=$(python3 -I -c 'import sys; print(float(sys.argv[1]) + float(sys.argv[2]))' "$at" "$step")
    done
    echo "${line% }"
}
{
    words 7 0.6 Midnight drive, we never stop
    words 11 0.5 Counting stars between the lines
    echo '[00:15.00]Radio hums an old refrain'
    words 18 0.6 שלום עולם כאן שרים לילה
    echo '[00:21.00]'
    words 29 0.5 Headlights on the empty road
    for i in $(seq 33 4 110); do
        printf '[%02d:%02d.00]Verse line at second %d of the fixture\n' $((i / 60)) $((i % 60)) "$i"
    done
} >"$dir/1.lrc"
printf 'Plain text without timestamps\nA second line that is long enough to wrap in a narrow pane\nA third line\n' >"$dir/2.lrc"
cat >"$dir/3.ttml" <<'T'
<tt xmlns="http://www.w3.org/ns/ttml" xmlns:ttm="http://www.w3.org/ns/ttml#metadata"><head><metadata>
<ttm:agent xml:id="a" type="person"><ttm:name type="full">First singer</ttm:name></ttm:agent>
<ttm:agent xml:id="b" type="person"><ttm:name type="full">Second singer</ttm:name></ttm:agent>
</metadata></head><body><div>
<p begin="1" end="5" ttm:agent="a"><span begin="1" end="2.5">First </span><span begin="2.5" end="4">singer </span><span begin="4" end="5">leads</span><span ttm:role="x-bg"><span begin="2" end="5">(echo in the back)</span></span></p>
<p begin="5" end="9" ttm:agent="b"><span begin="5" end="7">Second </span><span begin="7" end="9">answers</span></p>
<p begin="9" end="13" ttm:agent="a"><span begin="9" end="13">First again</span></p>
</div></body></tt>
T

json() { python3 -I -c "import json, sys; d = json.load(sys.stdin); d = d.get('result', d) if isinstance(d, dict) else d; print(eval(sys.argv[1]))" "$1"; }
library=$(dp ipc libraries.list | json 'd[0]["id"]')
rpc libraries.scan "{\"library\":\"$library\"}" >/dev/null
sleep 5

hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1000})' >/dev/null
geometry() {
    hyprctl -j clients | python3 -I -c '
import json, sys
c = next(c for c in json.load(sys.stdin) if c["title"] == "Dux Player")
print(sys.argv[1] % (*c["at"], *c["size"]))' "$1"
}
cap() {
    sleep "${SETTLE:-1.5}"
    grim -g "$(geometry '%d,%d %dx%d')" "$W/out/$1.png"
}
# the player paused at a time, so what a shot shows doesn't depend on timing
at() {
    dp seek "$1" >/dev/null
    cap "$2"
}

dp play "$dir" >/dev/null
sleep 2
dp pause >/dev/null
look dark bento
expand true
element lyrics view
sleep 2

at 4 bento-intro-ring
at 8.1 bento-word-fill
at 16 bento-line-timed
at 19 bento-rtl-fill
at 25 bento-gap-ring

look light bento
at 8.1 bento-light-fill
look dark bento

# scrolling away floats the follow button; clicking it comes back
at 12 bento-before-scroll
read -r x y w h <<<"$(geometry '%d %d %d %d')"
pointer m $((x + w / 4 - 2)) $((y + h / 2))
sleep 0.1
pointer m $((x + w / 4)) $((y + h / 2))
sleep 0.3
pointer w 3
cap bento-follow-button
click $((x + 410)) $((y + 900))
cap bento-followed-again
pointer m $((x + w - 20)) $((y + h / 2))

# playing: the line change and a far seek, a few frames apart
dp seek 28.2 >/dev/null
dp toggle >/dev/null
SETTLE=0 cap bento-playing-a
sleep 0.5
SETTLE=0 cap bento-playing-b
sleep 0.3
SETTLE=0 cap bento-playing-c
dp seek 90 >/dev/null
sleep 0.15
SETTLE=0 cap bento-far-seek-mid
cap bento-far-seek-end
dp pause >/dev/null

ui reduceMotion true
at 8.1 reduced-word-fill
at 25 reduced-gap-ring
dp toggle >/dev/null
sleep 2
dp pause >/dev/null
cap reduced-after-playing
ui reduceMotion false

look dark m3e
at 8.1 material-word-fill
at 25 material-gap-ring

ui lyricsDisplay alignment start
at 9 material-start-aligned
at 25 material-start-gap
ui lyricsDisplay alignment center
ui lyricsDisplay wordSync false
at 9 material-no-word-sync
ui lyricsDisplay wordSync true

expand false
look dark bento
element lyrics bar
at 9 side-panel
read -r x y w h <<<"$(geometry '%d %d %d %d')"
pointer m $((x + w - 150)) $((y + h / 2))
sleep 0.1
pointer m $((x + w - 148)) $((y + h / 2))
sleep 0.3
pointer w 2
cap side-panel-follow

dp next >/dev/null
sleep 2
dp pause >/dev/null
cap side-panel-unsynced

dp next >/dev/null
sleep 2
dp pause >/dev/null
expand true
at 3 bento-duet
ui lyricsDisplay backgroundVocals false
at 3.1 bento-duet-no-background
