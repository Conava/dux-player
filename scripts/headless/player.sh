# The player's looks and options end to end: the floating pill (size, place, page ends, ring, morph), the seekbar across
# the bar, the labelled play button, the queue header's switch, the pane tabs, the blurred background and blurred lyric
# lines. GAP_MAX (default 80 ms) is the longest frame gap allowed while the full view morphs; the worst gaps go to
# facts.txt. Screenshots land in the output folder.
failed=0
GAP_MAX=${GAP_MAX:-80}
SETTLE=0.8
fact() { echo "$*" >>"$W/out/facts.txt"; }
fail() {
    echo "FAIL $*" | tee -a "$W/out/facts.txt"
    failed=1
}
expect() { # NAME ACTUAL WANTED
    if [ "$2" = "$3" ]; then fact "ok   $1: $2"; else fail "$1: wanted '$3', got '$2'"; fi
}
holds() { # NAME CONDITION-RESULT(1|0) DETAIL
    if [ "$2" = 1 ]; then fact "ok   $1 ($3)"; else fail "$1 ($3)"; fi
}
gaps() {
    local worst
    worst=$(tr ' ' '\n' <"$W/out/$1.frames" | sort -n | tail -1)
    fact "gap $1: worst ${worst:--} ms of $(wc -w <"$W/out/$1.frames") frames"
    if [ -z "$worst" ]; then fail "gap $1: no frames"; elif [ "$worst" -gt "$GAP_MAX" ]; then fail "gap $1: $worst ms over $GAP_MAX"; fi
}
leg() { # NAME CMD...: frames around CMD, then the check
    local name=$1
    frames "$@"
    gaps "$name"
}
# status FIELD: a field of the daemon's player status
status() { rpc player.status '{}' | python3 -I -c 'import json, sys; print(json.load(sys.stdin)["result"][sys.argv[1]])' "$1"; }
# rectw NAME: "X Y W H" of an item, "" when none shows
rectw() { ask rect "$1" | cut -d' ' -f1-4; }
# num NAME INDEX: a field of the item's rect
num() { rectw "$1" | cut -d' ' -f"$2"; }
# crop NAME FROM X Y W H
crop() { magick "$W/out/$2.png" -crop "$5x$6+$3+$4" +repage -strip "$W/out/$1.png"; }
# cap NAME: a shot of the main window
cap() {
    local x y w h
    read -r x y w h < <(hyprctl -j clients | python3 -I -c '
import json, sys
c = next(c for c in json.load(sys.stdin) if c["title"] == "Dux Player")
print(*c["at"], *c["size"])')
    grim -g "$x,$y ${w}x$h" "$W/out/$1.png"
}
wheel() { pointer w "$1"; sleep 1; }
# openpane PANE: the full view open with that pane
openpane() {
    ui expand true
    sleep "$SETTLE"
    [ -z "$(rectw "$([ "$1" = queue ] && echo queueClear || echo paneTabs)")" ] && ui element "$1" view
    sleep "$SETTLE"
}

# Generated music: a lyrics fixture, and many small albums so Albums scrolls. Zulu Album sorts last by title.
dir="$W/music/Lyric Test/Fixture"
mkdir -p "$dir"
magick -size 600x600 -seed 7 "plasma:#6366f1-#0f172a" -blur 0x24 "$dir/cover.png"
ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -i "$dir/cover.png" -map 0 -map 1 -t 120 -c:v copy \
    -disposition:v attached_pic -metadata title="Synced Track" -metadata artist="Lyric Test" -metadata album="Fixture" \
    -metadata track=1 "$dir/1.flac"
{
    for i in $(seq 2 4 110); do
        printf '[%02d:%02d.00]Verse line at second %d of the fixture\n' $((i / 60)) $((i % 60)) "$i"
    done
} >"$dir/1.lrc"
for i in $(seq -w 1 30) Zulu; do
    mkdir -p "$W/music/Filler $i/Album $i"
    echo "$W/music/Filler $i/Album $i/1.flac|Song $i|Filler $i|$([ "$i" = Zulu ] && echo "Zulu Album" || echo "Album $i")"
done | xargs -P 8 -I{} bash -c 'IFS="|" read -r f t a b <<<"$1"; ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -t 30 -metadata title="$t" -metadata artist="$a" -metadata album="$b" -metadata track=1 "$f"' _ {}
rpc libraries.scan "{\"library\":\"$(dp ipc libraries.list | python3 -I -c 'import json, sys; d = json.load(sys.stdin); d = d.get("result", d) if isinstance(d, dict) else d; print(d[0]["id"])')\"}" >/dev/null
for _ in $(seq 1 60); do
    [ -n "$(ask albumId "Album 30" | tr -d '\n')" ] && break
    sleep 0.5
done
sleep 2

hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1000})' >/dev/null
sleep 1
ui look dark bento
ui pageOptions albums '{"sort":"title"}'
go home
dp play "$W/music/Lyric Test" >/dev/null
sleep 2
# one open and close first: the page text switches its antialiasing after the first cycle
expand true
expand false

# 1. Floating: size and place, the page's end above the pill, the ring opens the full view, the ring fills
ui playerLook floating
sleep 1.5
go home
read -r bx by bw bh < <(ask barRect)
expect "floating: pill height is the token" "$bh" 64
holds "floating: pill is at most 640 wide" "$([ "$bw" -le 640 ] && echo 1)" "$bw"
read -r px py pw ph _ < <(ask rect kept:home/)
holds "floating: pill is centred on the content pane" "$([ -n "$px" ] && [ "$(((bx + bw / 2) - (px + pw / 2)))" -ge -3 ] && [ "$(((bx + bw / 2) - (px + pw / 2)))" -le 3 ] && echo 1)" "pill centre $((bx + bw / 2)), pane centre $((px + pw / 2))"
cap floating-home
# the pill opens the lyrics and queue panels and has the heart
holds "floating: the pill has a heart" "$([ -n "$(ask namedAll "Favorite" | awk -v y="$by" '$2 >= y')" ] && echo 1)" "$(ask namedAll Favorite | head -3 | tr '\n' ';')"
read -r qx qy qw qh < <(ask namedAll Queue | awk -v y="$by" '$2 >= y' | head -1)
[ -n "$qx" ] && wclick "$((${qx%.*} + ${qw%.*} / 2))" "$((${qy%.*} + ${qh%.*} / 2))"
sleep 1
expect "floating: the pill's Queue opens the queue panel" "$(ask panel)" queue
# the pill re-centres on the narrower page
read -r qx qy qw qh < <(ask namedAll Queue | awk -v y="$by" '$2 >= y' | tail -1)
[ -n "$qx" ] && wclick "$((${qx%.*} + ${qw%.*} / 2))" "$((${qy%.*} + ${qh%.*} / 2))"
sleep 1
expect "floating: a second click closes it" "$(ask panel)" ""
# quick clicks on Next: Qt takes every second one as a double-click
read -r nx ny nw nh < <(ask namedAll Next | awk -v y="$by" '$2 >= y' | head -1)
dp play "$W/music" >/dev/null
sleep 1
before=$(status index)
if [ -n "$nx" ]; then
    read -r wx wy < <(winat)
    pointer m "$((wx + ${nx%.*} + ${nw%.*} / 2 - 2))" "$((wy + ${ny%.*} + ${nh%.*} / 2))"
    sleep 0.1
    pointer m "$((wx + ${nx%.*} + ${nw%.*} / 2))" "$((wy + ${ny%.*} + ${nh%.*} / 2))"
    sleep 0.3
    for _ in 1 2 3 4; do
        pointer c
        sleep 0.12
    done
    sleep 1.5
fi
expect "floating: four quick clicks on Next skip four tracks" "$(($(status index) - before))" 4
dp play "$W/music/Lyric Test" >/dev/null
sleep 1
go albums
read -r wx wy < <(winat)
pointer m "$((wx + 898))" "$((wy + 500))"
sleep 0.2
pointer m "$((wx + 900))" "$((wy + 500))"
sleep 0.5
wheel 20
wheel 20
read -r _ ty _ th < <(ask namedAll "Zulu Album, Filler Zulu" | tail -1)
read -r bx by bw bh < <(ask barRect)
holds "floating: the last album ends above the pill" "$([ -n "$ty" ] && [ "$((ty + th))" -le "$by" ] && echo 1)" "tile bottom $((ty + th)), pill top $by"
cap floating-albums-end
go home
read -r rx ry rw rh _ < <(ask rect floatingRing)
holds "floating: the ring shows" "$([ -n "$rx" ] && echo 1)" "$rx $ry $rw $rh"
cap f-a
crop ring-a f-a "$rx" "$ry" "$rw" "$rh"
sleep 2
cap f-b
crop ring-b f-b "$rx" "$ry" "$rw" "$rh"
if cmp -s "$W/out/ring-a.png" "$W/out/ring-b.png"; then fail "floating: the ring did not change in two seconds"; else fact "ok   floating: the ring fills while playing"; fi
leg floating-open clickrect floatingRing
expect "floating: the ring opens the full view" "$(ask viewOpen)" true
cap floating-full
leg floating-close ui expand false
read -r wx wy < <(winat)
click "$((wx + rx + rw - 3))" "$((wy + ry + 3))"
sleep 1
expect "floating: a click in the ring's corner, outside the circle" "$(ask viewOpen)" false
ui expand false
ui playerLook card
sleep 1.5

# 2. Seekbar across: the zone spans the bar and a click in it seeks
dp pause >/dev/null
rpc player.seek '{"position":0}' >/dev/null
elem() { printf '{"type":"%s","hidden":false,"placement":"auto","options":{}}' "$1"; }
ui surface bar "{\"zones\":{\"seek\":[],\"across\":[$(elem elapsed),$(elem seekbar),$(elem duration)]}}"
sleep 1.5
read -r ax ay aw ah _ < <(ask rect barAcross)
read -r bx by bw bh < <(ask barRect)
holds "across: the line spans the bar's width" "$([ -n "$ax" ] && [ "$aw" -ge $((bw - 80)) ] && echo 1)" "line $aw, bar $bw"
cap across
wclick "$((ax + aw * 3 / 4))" "$((ay + ah / 2))"
sleep 1
fraction=$(python3 -I -c 'import sys; print(1 if 0.6 <= float(sys.argv[1]) / float(sys.argv[2]) <= 0.85 else 0)' "$(status position)" "$(status duration)")
holds "across: a click at 75% seeks" "$fraction" "position $(status position) of $(status duration)"
ui resetSurface bar

# 3. Labelled play: its width holds across play and pause
ui surface bar "{\"zones\":{\"controls\":[$(elem previous),{\"type\":\"playPause\",\"hidden\":false,\"placement\":\"auto\",\"options\":{\"label\":true}},$(elem next)]}}"
sleep 1.5
dp pause >/dev/null
sleep 1
paused=$(num playPause 3)
cap label-paused
dp resume >/dev/null
sleep 1
playing=$(num playPause 3)
cap label-playing
holds "label: the play button has a width" "$([ -n "$paused" ] && [ "$paused" -gt 0 ] && echo 1)" "$paused"
expect "label: width is the same playing and paused" "$playing" "$paused"
ui resetSurface bar

# 4. Queue header: the switch turns continuing on and off, Clear empties the queue but the current track, header off drops the switch
rpc queue.continue '{"mode":"off"}' >/dev/null
ui nowPlayingPatch '{"queueHeader":true,"paneTabs":true}'
openpane queue
cap queue-header
expect "queue: continuing starts off" "$(status continuing)" False
clickrect continueSwitch
sleep 1
expect "queue: the switch turns continuing on" "$(status continuing)" True
clickrect continueSwitch
sleep 1
expect "queue: the switch turns it off again" "$(status continuing)" False
holds "queue: the header shows Clear" "$([ -n "$(rectw queueClear)" ] && echo 1)" "queueClear"
ui nowPlayingPatch '{"queueHeader":false}'
sleep 1
expect "queue: header off drops the switch" "$(rectw continueSwitch)" ""
cap queue-header-off
ui nowPlayingPatch '{"queueHeader":true}'
sleep 1
clickrect queueClear
sleep 1
expect "queue: Clear keeps only the current track" "$(status length)" 1
ui expand false
dp play "$W/music" >/dev/null
sleep 1

# 5. Tabs and background: the Queue tab, Last used, blur, tabs off
openpane lyrics
cap tabs-lyrics
clickunder paneTabs Queue
sleep 1
holds "tabs: the Queue segment shows the queue" "$([ -n "$(rectw queueClear)" ] && echo 1)" "queueClear"
cap tabs-queue
ui expand false
sleep 1
ui expand true
sleep 1.5
holds "tabs: Last used reopens the queue" "$([ -n "$(rectw queueClear)" ] && echo 1)" "queueClear"
ui expand false
# the generated filler tracks have no cover, and the background shows only with one
dp play "$W/music/Lyric Test" >/dev/null
ui nowPlayingPatch '{"background":"blur"}'
sleep 2
leg blur-open ui expand true
holds "background: the blurred cover is behind the full view" "$([ -n "$(rectw fullBackground)" ] && echo 1)" "fullBackground"
cap background-blur
leg blur-close ui expand false
ui nowPlayingPatch '{"background":"plain"}'
ui nowPlayingPatch '{"paneTabs":false}'
sleep 1
ui expand true
sleep 1.5
expect "tabs: off drops the tabs" "$(rectw paneTabs)" ""
cap tabs-off
ui expand false
ui nowPlayingPatch '{"paneTabs":true}'

# 6. Lyric blur: lines blur around the current one, and a track change with reduce motion still draws
dp play "$W/music/Lyric Test" >/dev/null
sleep 1
rpc player.seek '{"position":12}' >/dev/null
ui lyricsDisplay blurLines true
ui nowPlayingPatch '{"openPane":"lyrics"}'
ui expand true
sleep 2
cap lyrics-blur
rows=$(ask lyricsRows | wc -w)
holds "lyrics: lines are drawn" "$([ "$rows" -gt 3 ] && echo 1)" "$((rows - 1)) rows"
ui reduceMotion true
sleep 0.5
rpc player.next '{}' >/dev/null
sleep 2
cap lyrics-blur-reduced
holds "lyrics: the view survives a track change with reduced motion" "$([ "$(ask viewOpen)" = true ] && echo 1)" "viewOpen"
ui reduceMotion false
ui lyricsDisplay blurLines false
ui nowPlayingPatch '{"openPane":"last"}'
ui expand false
# 7. Full view blur background: the lyric text against the blurred cover, dark and light, dark and bright cover; and the
# pane tabs sit the same distance from the pane's edge in the lyrics and the queue
contrast() { # contrast SHOT: WCAG ratio of the strongest text in the right half of the window to the background around it
    python3 -I -c '
import sys
from PIL import Image
im = Image.open(sys.argv[1]).convert("RGB")
w, h = im.size
im = im.crop((w // 2, h // 6, w - 8, h - h // 6))
lin = lambda c: ((c / 255 / 12.92) if c / 255 <= 0.03928 else (((c / 255) + 0.055) / 1.055) ** 2.4)
lum = lambda p: 0.2126 * lin(p[0]) + 0.7152 * lin(p[1]) + 0.0722 * lin(p[2])
px = list(im.getdata())
ls = sorted(lum(p) for p in px)
bg = ls[len(ls) // 2]
far = sorted(ls, key=lambda v: -abs(v - bg))[: max(1, len(ls) // 300)]
fg = sum(far) / len(far)
a, b = max(fg, bg), min(fg, bg)
print(f"{(a + 0.05) / (b + 0.05):.2f}")' "$W/out/$1.png"
}
for tone_name in dark:"#0a0a14-#1c1c34" bright:"#fff7b8-#ffe45c"; do
    n=${tone_name%%:*}
    d="$W/music/Cover $n/Cover $n"
    mkdir -p "$d"
    magick -size 600x600 -seed 5 "plasma:${tone_name#*:}" -blur 0x24 "$d/cover.png"
    ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -i "$d/cover.png" -map 0 -map 1 -t 120 -c:v copy \
        -disposition:v attached_pic -metadata title="Cover $n Track" -metadata artist="Cover $n" -metadata album="Cover $n" \
        -metadata track=1 "$d/1.flac"
    cp "$dir/1.lrc" "$d/1.lrc"
done
ui nowPlayingPatch '{"background":"blur","paneTabs":true,"openPane":"lyrics"}'
for mode in dark light; do
    for n in dark bright; do
        ui look "$mode" bento
        dp play "$W/music/Cover $n" >/dev/null
        sleep 1.5
        rpc player.seek '{"position":12}' >/dev/null
        dp pause >/dev/null
        ui expand true
        sleep 2.5
        cap "contrast-$mode-$n-lyrics"
        lyr_x=$(num paneTabs 1)
        ui element queue view
        sleep 1.5
        cap "contrast-$mode-$n-queue"
        fact "contrast $mode mode, $n cover: lyrics $(contrast "contrast-$mode-$n-lyrics"):1, queue $(contrast "contrast-$mode-$n-queue"):1; pane tabs x lyrics $lyr_x queue $(num paneTabs 1)"
        expect "pane tabs: the same inset in the lyrics and the queue ($mode, $n)" "$(num paneTabs 1)" "$lyr_x"
        ui element lyrics view
        ui expand false
        sleep 1
    done
done
ui nowPlayingPatch '{"background":"plain","openPane":"last"}'
ui look dark bento

exit $failed
