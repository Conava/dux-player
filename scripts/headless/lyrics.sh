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
lyrics_menu() { # a python expression over the lyricsMenu JSON as d
    python3 -I -c 'import json, sys; d = json.loads(sys.argv[2]); print(eval(sys.argv[1]))' "$1" "$(qs ipc -p "$W/tree" call headless lyricsMenu)"
}
fact() {
    if [ "$2" = "$3" ]; then echo "ok   $1: $2"; else echo "FAIL $1: wanted '$3', got '$2'"; fi >>"$W/out/facts.txt"
}
centre() { lyrics_menu '"%.1f %s" % (d["centre"]["t"], d["centre"]["text"])'; }
menu_click() { # a menu row by id, or the menu button
    local x y w h at
    read -r x y w h <<<"$(geometry '%d %d %d %d')"
    if [ "$1" = button ]; then at=$(lyrics_menu '"%d %d" % tuple(d["button"])'); else at=$(lyrics_menu '"%d %d" % tuple(d["rows"]["'"$1"'"])'); fi
    read -r mx my <<<"$at"
    click $((x + mx)) $((y + my))
    sleep 1
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

# the timing slider keeps the menu open; two steps later move every line by 1 s, so the line before is centred again
slider_at() { # screen point at a ratio of the slider's track
    local x y w h
    read -r x y w h <<<"$(geometry '%d %d %d %d')"
    lyrics_menu '"%d %d" % (round(d["slider"]["start"][0] + (d["slider"]["end"][0] - d["slider"]["start"][0]) * '"$1"') + '"$x"', d["slider"]["start"][1] + '"$y"')'
}
dp seek 33.5 >/dev/null
sleep 1.5
fact "centred line" "$(centre)" "33.0 Verse line at second 33 of the fixture"
menu_click button
cap bento-lyrics-menu
width=$(lyrics_menu 'd["width"]')
fact "no reset button at 0" "$(lyrics_menu 'd["reset"]')" None
wtype -k Home
sleep 0.4
fact "Home reaches the slider" "$(lyrics_menu 'd["highlight"]')" timing
wtype -k Right
sleep 1
fact "Right steps later" "$(lyrics_menu 'd["offset"]')" 0.5
fact "menu width at +0.5" "$(lyrics_menu 'd["width"]')" "$width"
cap bento-lyrics-menu-half
wtype -k Right
sleep 1
fact "menu open after two steps" "$(lyrics_menu 'd["open"]')" True
fact "offset after two steps" "$(lyrics_menu 'd["offset"]')" 1
fact "centred line 1 s later" "$(centre)" "30.0 Headlights on the empty road"
wtype -k Return
sleep 0.5
fact "Enter on the slider keeps the menu" "$(lyrics_menu 'd["open"]')" True
fact "Enter on the slider keeps the timing" "$(lyrics_menu 'd["offset"]')" 1
cap bento-lyrics-menu-nudged
# a real drag from the handle to a quarter, held while the daemon answers, then past the start
read -r px py <<<"$(slider_at 0.55)"
pointer m $((px - 2)) "$py"
sleep 0.1
pointer m "$px" "$py"
sleep 0.3
pointer d
sleep 0.2
read -r qx qy <<<"$(slider_at 0.25)"
for i in 1 2 3 4 5 6 7 8; do
    pointer m $((px + (qx - px) * i / 8)) "$py"
    sleep 0.03
done
sleep 0.8
fact "slider follows the pointer" "$(lyrics_menu 'd["slider"]["value"]')" -10
fact "lyrics follow during the drag" "$(lyrics_menu 'd["offset"]')" -5
read -r sx sy <<<"$(slider_at 0)"
for i in 1 2 3 4 5 6 7 8; do
    pointer m $((qx + (sx - 40 - qx) * i / 8)) "$py"
    sleep 0.03
done
sleep 0.3
pointer u
sleep 1
fact "menu open after a drag" "$(lyrics_menu 'd["open"]')" True
fact "offset after a drag" "$(lyrics_menu 'd["offset"]')" -10
fact "slider after a drag" "$(lyrics_menu 'd["slider"]["value"]')" -20
fact "menu width at -10" "$(lyrics_menu 'd["width"]')" "$width"
fact "centred line 10 s earlier" "$(centre)" "31.0 Verse line at second 41 of the fixture"
cap bento-lyrics-menu-min10
fact "cli clamps" "$(dp lyrics offset -100)" "Lyrics timing -10.0 s"
read -r x y w h <<<"$(geometry '%d %d %d %d')"
read -r mx my <<<"$(lyrics_menu '"%d %d" % tuple(d["reset"])')"
click $((x + mx)) $((y + my))
sleep 1
fact "menu open after reset" "$(lyrics_menu 'd["open"]')" True
fact "offset after reset" "$(lyrics_menu 'd["offset"]')" 0
fact "slider after reset" "$(lyrics_menu 'd["slider"]["value"]')" 0
fact "centred line after reset" "$(centre)" "33.0 Verse line at second 33 of the fixture"
fact "no reset button after reset" "$(lyrics_menu 'd["reset"]')" None
read -r mx my <<<"$(lyrics_menu '"%d %d" % tuple(d["rows"][""])')"
pointer m $((x + mx - 2)) $((y + my))
sleep 0.1
pointer m $((x + mx)) $((y + my))
sleep 0.5
fact "hover moves the highlight off the slider" "$(lyrics_menu 'repr(d["highlight"])')" "''"
# the info icon at the end of a source row shows its status as a tooltip and doesn't pick the source
info_hover() {
    read -r ix iy <<<"$(lyrics_menu '"%d %d" % tuple(d["infos"]["sidecar"]["at"])')"
    pointer m $((x + ix - 2)) $((y + iy))
    sleep 0.1
    pointer m $((x + ix)) $((y + iy))
    sleep 1.5
}
fact "lyrics file status" "$(lyrics_menu 'd["infos"]["sidecar"]["text"]')" "Lyrics file · not changed"
fact "status is the row's description" "$(lyrics_menu 'd["infos"]["sidecar"]["description"]')" "Lyrics file · not changed"
info_hover
cap bento-lyrics-info
click $((x + ix)) $((y + iy))
sleep 0.5
# a click that picked the row would close the menu
fact "a click on the info icon keeps the menu" "$(lyrics_menu 'd["open"]')" True
click $((x + 40)) $((y + h / 2))
sleep 1
fact "menu closed by a click outside" "$(lyrics_menu 'd["open"]')" False
fact "cli later" "$(dp lyrics offset +0.5)" "Lyrics timing +0.5 s"
fact "cli earlier" "$(dp lyrics offset -1)" "Lyrics timing -0.5 s"
fact "cli sets" "$(dp lyrics offset 2)" "Lyrics timing +2.0 s"
fact "cli reset" "$(dp lyrics offset 0)" "Lyrics timing reset"
pointer m $((x + w - 20)) $((y + h / 2))

# sources that answer while the slider is held join below it, and the drag goes on
menu_click button
ui lyricsSources false
sleep 0.5
read -r px py <<<"$(slider_at 0.5)"
read -r qx _ <<<"$(slider_at 0.75)"
pointer m $((px - 2)) "$py"
sleep 0.1
pointer m "$px" "$py"
sleep 0.3
pointer d
sleep 0.2
for i in 1 2 3 4; do
    pointer m $((px + (qx - px) * i / 4)) "$py"
    sleep 0.03
done
sleep 0.5
ui lyricsSources true
sleep 0.8
fact "sources join a held slider" "$(lyrics_menu '"" in d["rows"] and len(d["rows"]) > 2')" True
fact "highlight stays on the held slider" "$(lyrics_menu 'd["highlight"]')" timing
# the sources widen the menu and with it the slider
read -r rx _ <<<"$(slider_at 0.9)"
for i in 1 2 3 4; do
    pointer m $((qx + (rx - qx) * i / 4)) "$py"
    sleep 0.03
done
sleep 0.5
fact "drag goes on after the sources join" "$(lyrics_menu 'd["slider"]["value"]')" 16
pointer u
sleep 1
fact "offset after the held drag" "$(lyrics_menu 'd["offset"]')" 8
# a change from the command line shows in the menu left open after a drag
dp lyrics offset 0 >/dev/null
sleep 1
fact "slider follows the cli after a drag" "$(lyrics_menu 'd["slider"]["value"]')" 0
read -r x y w h <<<"$(geometry '%d %d %d %d')"
click $((x + 40)) $((y + h / 2))
sleep 0.5
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

# a line change moves the list as one: at every instant the rows keep their spacing (the verse lines are all one height)
dp seek 91 >/dev/null
sleep 1
for _ in $(seq 1 80); do
    qs ipc -p "$W/tree" call headless lyricsRows
done >"$W/out/rows.txt"
dp pause >/dev/null
python3 -I - "$W/out/rows.txt" >>"$W/out/facts.txt" <<'P'
import sys
# above the window an instrumental gap spaces the rows differently, and pooled rows linger on top of live ones
samples = [sorted({round(y, 1) for y in map(float, line.split()[1:]) if y > 0}) for line in open(sys.argv[1])]
spread = lambda rows: max(b - a for a, b in zip(rows, rows[1:])) - min(b - a for a, b in zip(rows, rows[1:]))
rest = samples[0]
moving = [s for s in samples if s != rest and s != samples[-1]]
worst = max(map(spread, moving), default=0)
print(f"{'ok  ' if moving and worst <= 2 else 'FAIL'} line change: {len(moving)} of {len(samples)} samples moving, spacing varied by {worst:.1f} px")
P

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
dp seek 33.5 >/dev/null
sleep 1.5
menu_click button
# off the menu, so only the keyboard moves the highlight
read -r x y w h <<<"$(geometry '%d %d %d %d')"
pointer m $((x + 40)) $((y + h / 2))
sleep 0.3
wtype -k Home
wtype -k Right
sleep 1
fact "material: Right steps later" "$(lyrics_menu 'd["offset"]')" 0.5
cap material-lyrics-menu
wtype -k Up
sleep 0.4
fact "Up leaves the slider" "$(lyrics_menu 'd["highlight"] != "timing"')" True
wtype -k Right
sleep 0.5
fact "Right off the slider keeps the timing" "$(lyrics_menu 'd["offset"]')" 0.5
wtype -k Down
wtype -k Left
sleep 1
fact "Left steps earlier" "$(lyrics_menu 'd["offset"]')" 0
wtype -k Left
sleep 1
fact "Left below zero" "$(lyrics_menu 'd["offset"]')" -0.5
cap material-lyrics-menu-earlier
info_hover
cap material-lyrics-info
wtype -k Escape
sleep 0.5
fact "Escape closes the menu" "$(lyrics_menu 'd["open"]')" False
dp lyrics offset 0 >/dev/null
pointer m $((x + w - 20)) $((y + h / 2))

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
! grep -q FAIL "$W/out/facts.txt"
