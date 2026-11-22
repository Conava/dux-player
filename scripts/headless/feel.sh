# Every motion of the app, timed: track changes, pages, menus, a dialog, queue edits, tabs, the rail, the floating
# pill, a shelf, presses, then the idle window and speed 0. Each leg records the frame gaps of its motions (worst gap
# <= GAP_MAX, default 80 ms), a still a third of the way in at a slow speed that must differ from the end, a shot
# 150 ms in with reduce motion that must equal the settled one, its end state, and that the window draws nothing for
# 1.5 s once settled and paused. FEEL_LEGS="queue press" runs only those legs. A failed check whose id is in RED is
# a target not reached yet: it shows as "red" in facts.txt and doesn't fail the run. gaps.txt lists "leg worst frames".
failed=0
SECONDS=0
GAP_MAX=${GAP_MAX:-80}
SETTLE=0.8
L="$XDG_STATE_HOME/dplayer/ui.log"
# checks that measure where the app should get to; each goes once it holds
RED="
 tabs-library moves
"
fact() { echo "$*" >>"$W/out/facts.txt"; }
fail() {
    echo "FAIL $*" | tee -a "$W/out/facts.txt"
    failed=1
}
# check ID OK DETAIL: OK is 1 when the check holds
check() {
    local listed=
    grep -qxF " $1" <<<"$RED" && listed=1
    case "$2:$listed" in
    1:1) fact "ok   $1: $3 (still listed in RED)" ;;
    1:) fact "ok   $1: $3" ;;
    *:1) fact "red  $1: $3" ;;
    *) fail "$1: $3" ;;
    esac
}
truth() { "$@" && echo 1 || echo 0; }
want() { [ -z "${FEEL_LEGS:-}" ] || [[ " $FEEL_LEGS " == *" $1 "* ]]; }
title() { [ "${1:-main}" = settings ] && echo "Dux Player Settings" || echo "Dux Player"; }
cap() { # NAME [WINDOW]: a shot of the main (or settings) window
    local x y w h
    read -r x y w h < <(hyprctl -j clients | python3 -I -c '
import json, sys
c = next(c for c in json.load(sys.stdin) if c["title"] == sys.argv[1])
print(*c["at"], *c["size"])' "$(title "${2:-}")")
    grim -g "$x,$y ${w}x$h" "$W/out/$1.png"
}
# status FIELD: a field of the daemon's player status; title is the track's
status() { rpc player.status '{}' | python3 -I -c 'import json, sys; r = json.load(sys.stdin)["result"]; print((r.get("track") or {}).get("title", "") if sys.argv[1] == "title" else r.get(sys.argv[1], ""))' "$1"; }
park() { pointer m 5 5; }
# hover X Y [WINDOW]: the pointer on a window point, its hover settled
hover() {
    local wx wy
    read -r wx wy < <(winat "$([ "${3:-main}" = settings ] && echo "Dux Player Settings")")
    pointer m "$((wx + $1 - 2))" "$((wy + $2))"
    sleep 0.1
    pointer m "$((wx + $1))" "$((wy + $2))"
    sleep 0.5
}
centre() { # NAME: "X Y" of the middle of the item ask rect NAME finds
    local x y w h
    read -r x y w h _ < <(ask rect "$1")
    [ -n "$x" ] && echo "$((x + w / 2)) $((y + h / 2))"
}
named() { # LABEL [N]: "X Y W H" of the Nth item ask namedAll LABEL finds
    ask namedAll "$1" | sed -n "${2:-1}p"
}
# calm [WINDOW] [MS]: waits until the window draws nothing for MS ms (300), for at most about 9 s
calm() {
    local quiet
    quiet=$(awk "BEGIN { print ${2:-300} / 1000 }")
    for _ in $(seq 1 $((9000 / ${2:-300}))); do
        ui idleStart "${1:-main}"
        sleep "$quiet"
        [ "$(ask idleStop)" = 0 ] && return 0
    done
    return 1
}

# gaps NAME: ends the recording once no frame came for 400 ms, writes NAME.frames and checks the worst gap
gaps() {
    local worst n
    sleep 0.3
    for _ in $(seq 1 60); do
        [ "$(ask sinceFrame)" -ge 400 ] && break
        sleep 0.1
    done
    ask timedStop >"$W/out/$1.frames"
    LAYOUTS=$(ask timedLayouts)
    worst=$(tr ' ' '\n' <"$W/out/$1.frames" | sort -n | tail -1)
    n=$(wc -w <"$W/out/$1.frames")
    echo "$1 ${worst:--} $n" >>"$W/out/gaps.txt"
    if [ -z "$worst" ]; then
        check "gap $1" 0 "no frames"
    else
        check "gap $1" "$(truth [ "$worst" -le "$GAP_MAX" ])" "worst $worst ms of $n frames"
    fi
}
# timed NAME ACTION [ARG]: an action of the headless target's timed, counted from the call
timed() {
    ui timed "$2" "${3:-}"
    gaps "$1"
}
# from NAME WINDOW CMD...: recording starts, then CMD acts (a click, a key); counted from the first frame
from() {
    local name=$1 win=$2
    shift 2
    ui timedFrom "$win"
    "$@"
    gaps "$name"
}
# layouts NAME MAX: the page and its view changed width at most MAX times during the last recording
layouts() {
    local page view
    read -r page view <<<"$LAYOUTS"
    check "$1 lays out once" "$(truth [ "$page" -le "$2" ] && [ "$view" -le "$2" ])" "page width changed $page times, its view $view"
}
# slow NAME DO UNDO [WINDOW]: at animation speed 2000, a shot shortly into DO differs from the settled shot; UNDO runs
# at the normal speed afterwards
slow() {
    ui animationDuration 2000
    sleep 0.3
    eval "$2"
    sleep 0.35
    cap "$1-mid" "${4:-}"
    calm "${4:-}"
    cap "$1-end" "${4:-}"
    if cmp -s "$W/out/$1-mid.png" "$W/out/$1-end.png"; then
        check "$1 moves" 0 "$1-mid.png equals $1-end.png"
    else
        check "$1 moves" 1 "$1-mid.png differs from $1-end.png"
    fi
    ui animationDuration 250
    sleep 0.2
    eval "$3"
    calm "${4:-}"
}
# snap NAME DO UNDO [WINDOW] [SETTING]: with reduce motion (or SETTING, e.g. "animationDuration 0"), a shot 150 ms
# after DO equals the settled one. The compositor's own popup fade is off meanwhile: it isn't the app's motion.
snap() {
    local setting=${5:-reduceMotion true}
    hyprctl eval 'hl.config({ animations = { enabled = false } })' >/dev/null
    ui $setting
    sleep 0.3
    eval "$2"
    sleep 0.15
    cap "$1-early" "${4:-}"
    calm "${4:-}"
    cap "$1-late" "${4:-}"
    if cmp -s "$W/out/$1-early.png" "$W/out/$1-late.png"; then
        check "$1 lands at once" 1 "$1-early.png equals $1-late.png"
    else
        check "$1 lands at once" 0 "$1-early.png differs from $1-late.png"
    fi
    eval "$3"
    calm "${4:-}"
    case $setting in reduceMotion*) ui reduceMotion false ;; *) ui animationDuration 250 ;; esac
    hyprctl eval 'hl.config({ animations = { enabled = true } })' >/dev/null
    sleep 0.3
}
# still NAME [WINDOW]: settled and paused, the window draws nothing for 1.5 s. A single late redraw is measured again:
# only what keeps drawing fails. A focused text field's caret blinks every 500 ms, which doesn't count.
still() {
    park
    calm "${2:-}" 1000
    local n again=0 gaps='' changed
    cap "idle-$1-a" "${2:-}"
    n=$(idleframes 1500 "${2:-main}")
    cap "idle-$1-b" "${2:-}"
    # where the window changed between the two shots: "WxH+X+Y", or "-1-1" for nowhere
    changed=$(magick "$W/out/idle-$1-a.png" "$W/out/idle-$1-b.png" -compose difference -composite -colorspace gray -threshold 1% -trim info:- 2>/dev/null | cut -d' ' -f4)
    if [ "$n" != 0 ]; then
        ui timedFrom "${2:-main}"
        sleep 1.5
        [ "$(ask sinceFrame)" -lt 1500 ] && again=1
        gaps=$(ask timedStop)
        [ -n "$gaps" ] && again=$(($(wc -w <<<"$gaps") + 1))
        if [ -n "$gaps" ] && [ -z "$(tr ' ' '\n' <<<"$gaps" | awk '$1 < 470 || $1 > 530')" ]; then
            check "idle $1" 1 "only a caret blinks ($n frames in 1.5 s)"
            return
        fi
        [ "$again" = 0 ] && fact "note idle $1: $n late frames, then none (changed $changed)"
    fi
    check "idle $1" "$(truth [ "$again" = 0 ])" "$n frames in 1.5 s$([ "$n" != 0 ] && echo ", $again in the next 1.5 s${gaps:+ (gaps $gaps)}, changed $changed")"
}
pausenow() {
    dp pause >/dev/null
    sleep 0.3
}

# Music: twelve one-track albums with their own cover and lyrics, so a skip changes cover, title and tint, and Home's
# shelves have pages to scroll
for i in $(seq -w 1 12); do
    dir="$W/music/Feel Artist/Feel $i"
    mkdir -p "$dir"
    echo "$dir|$i"
done | xargs -P 8 -I{} bash -c 'IFS="|" read -r d i <<<"{}"
magick -size 300x300 "gradient:hsl($((10#$i * 30)),80%,55%)-hsl($((10#$i * 30 + 90)),70%,25%)" "$d/cover.png"
ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -i "$d/cover.png" -map 0 -map 1 -t 60 -c:v copy \
    -disposition:v attached_pic -metadata title="Feel Song $i" -metadata artist="Feel Artist" -metadata album="Feel $i" \
    -metadata genre="Ambient" -metadata track=1 "$d/1.flac"
for s in 0 5 10 15 20 25 30 35 40 45 50 55; do printf "[00:%02d.00]Line at second %d of song %s\n" "$s" "$s" "$i"; done >"$d/1.lrc"'
library=$(dp ipc libraries.list | python3 -I -c 'import json, sys; d = json.load(sys.stdin); d = d.get("result", d) if isinstance(d, dict) else d; print(d[0]["id"])')
rpc libraries.scan "{\"library\":\"$library\"}" >/dev/null
for _ in $(seq 1 60); do
    [ -n "$(ask albumId "Feel 12" | tr -d '\n')" ] && break
    sleep 0.5
done
# plays on earlier days, so Statistics has bars and rings
rpc browse.tracks "{\"library\":\"$library\"}" | python3 -I -c '
import json, sqlite3, sys, time
tracks = json.load(sys.stdin)["result"]
tracks = tracks.get("tracks", tracks) if isinstance(tracks, dict) else tracks
db = sqlite3.connect(sys.argv[1], timeout=10)
now = int(time.time())
for d in range(20):
    for t in tracks[d % len(tracks):][:1 + d % 4]:
        db.execute("INSERT INTO plays VALUES (?, ?, ?, ?, ?)", (t["library"], t["id"], t.get("albumId", ""), now - d * 86400 - 600, json.dumps(t)))
db.commit()' "$XDG_DATA_HOME/dplayer/history.db"
playlist=$(rpc localPlaylists.create "{\"name\":\"Feel Mix\",\"tracks\":$(rpc browse.tracks "{\"library\":\"$library\"}" | python3 -I -c '
import json, sys
t = json.load(sys.stdin)["result"]
t = t.get("tracks", t) if isinstance(t, dict) else t
print(json.dumps([{"library": x["library"], "id": x["id"]} for x in t[:6]]))')}" | python3 -I -c 'import json, sys; print(":" + json.load(sys.stdin)["result"]["id"])')
artist=$(rpc browse.artists "{\"library\":\"$library\"}" | python3 -I -c '
import json, sys
a = json.load(sys.stdin)["result"]
a = a.get("artists", a) if isinstance(a, dict) else a
print(next(x["id"] for x in a if x["name"] == "Feel Artist"))')
album=$(ask albumId "Feel 03")
# the window knows every track once Tracks has shown
go tracks
fact "setup took $SECONDS s"

hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1000})' >/dev/null
sleep 1
look dark bento
ui nowPlayingPatch '{"paneTabs":true}'
go home
dp play "$W/music/Feel Artist" >/dev/null
sleep 2
pausenow
# one open and close first: the page text switches its antialiasing after the first cycle
expand true
expand false

# a queue of the twelve songs, paused on the first: a skip past a queue's end would start similar tracks playing
fresh() {
    dp play "$W/music/Feel Artist" >/dev/null
    sleep 1
    pausenow
}

leg_trackchange() {
    local where
    fresh
    for where in bento m3e bar side floating; do
        case $where in
        bento | m3e)
            look dark $where
            expand true
            ;;
        bar) expand false ;;
        side) ui element dock bar ;;
        floating)
            ui element dock bar
            ui playerLook floating
            ;;
        esac
        sleep 1
        timed "trackchange-$where-next" next
        timed "trackchange-$where-prev" prev
    done
    ui playerLook card
    look dark bento
    expand true
    # three skips within 300 ms end on the third one's cover and title
    local at want
    at=$(status index)
    ui act next ""
    ui act next ""
    ui act next ""
    sleep 0.5
    calm
    want="Feel Song $(printf %02d $((at + 4)))"
    check "trackchange ends on the last skip" "$(truth [ "$(status title)" = "$want" ])" "playing '$(status title)', wanted '$want'"
    check "trackchange shows the last title" "$(python3 -I -c '
import json, sys
t, f = json.loads(sys.argv[1]), json.loads(sys.argv[2])
print(int(sys.argv[3] in t and all(x == 1 for x in f)))' "$(ask props TextSlide text)" "$(ask props TextSlide fade)" "$want")" "titles $(ask props TextSlide text), fades $(ask props TextSlide fade)"
    # the now-playing art hands a new cover over (the others have no handoff); at rest each shows the current source
    local arts
    arts='import json, sys
s, src, h = (json.loads(a) for a in sys.argv[1:4])
now = [(a, b) for a, b, on in zip(s, src, h) if on]'
    check "trackchange shows the last cover" "$(python3 -I -c "$arts
print(int(bool(now) and all(a == b != '' for a, b in now) and len({b for a, b in now}) == 1))" "$(ask props AlbumArt shown)" "$(ask props AlbumArt source)" "$(ask props AlbumArt handoff)")" "full view: $(ask props AlbumArt handoff | tr -cd t | wc -c) arts hand off"
    expand false
    check "trackchange bar art hands off" "$(python3 -I -c "$arts
print(int(bool(now)))" "$(ask props AlbumArt shown)" "$(ask props AlbumArt source)" "$(ask props AlbumArt handoff)")" "bar: $(ask props AlbumArt handoff | tr -cd t | wc -c) arts hand off"
    slow trackchange "ui act next ''" ":"
    snap trackchange-reduce "ui act next ''" ":"
    pausenow
    still trackchange
}

leg_pages() {
    go home
    # the first visit builds everything cold; only warm visits count
    local page
    for page in "album $album" "artist $artist" "playlist $playlist"; do
        ui open $page
        sleep 1.5
        ui back
        sleep 1
    done
    ui go stats
    sleep 1.5
    ui back
    sleep 1
    timed pages-album open "album $album"
    layouts pages-album 1
    timed pages-back back
    timed pages-forward forward
    layouts pages-forward 1
    ui back
    sleep 1
    timed pages-artist open "artist $artist"
    layouts pages-artist 1
    ui back
    sleep 1
    timed pages-playlist open "playlist $playlist"
    layouts pages-playlist 1
    ui back
    sleep 1
    timed pages-stats go stats
    layouts pages-stats 1
    fact "pages: Statistics shows $(ask countObjects statsBar) bars"
    local x y w h
    read -r x y w h < <(named "30 days")
    hover "$((x + w / 2))" "$((y + h / 2))"
    from pages-stats-range main pointer c
    slow pages-stats-range "clicknamed '7 days'" "clicknamed '30 days'"
    ui pref statsDays 7
    go collection
    ui act libraryTab albums
    sleep 1
    timed pages-tab libraryTab artists
    timed pages-tab-back libraryTab albums
    go home
    slow pages "ui open album $album" "ui back"
    snap pages-reduce "ui open album $album" "ui back"
    ui open album "$album"
    still pages
    go home
}

# menuat NAME X Y: opens a menu with a right click at a window point and closes it with Escape, both timed; then once
# more for its size and its rows at rest
menuat() {
    hover "$2" "$3"
    from "$1-open" main pointer r
    from "$1-close" main wtype -k Escape
    sleep 0.5
    ui popupsStart
    pointer r
    sleep 1.2
    ask popupsStop >"$W/out/$1.samples"
    check "$1 opens at its settled size" "$(python3 -I - "$W/out/$1.samples" <<'P'
import json, sys
s = [json.loads(l) for l in open(sys.argv[1]) if l.strip() and json.loads(l)["kind"] == "menu"]
print(int(bool(s) and len({(x["w"], x["h"]) for x in s}) == 1))
P
)" "$(wc -l <"$W/out/$1.samples") samples"
    local rows
    rows=$(ask menuRows 'opacity*')
    check "$1 rows at rest" "$(python3 -I -c 'import json, sys; r = json.loads(sys.argv[1]); print(int(bool(r) and all(abs(x - 1) < 1e-6 for x in r)))' "$rows")" "row opacities $rows"
    wtype -k Escape
    sleep 0.5
}

leg_menu() {
    ui open album "$album"
    sleep 1.5
    local x y
    read -r x y < <(ask trackPoint)
    menuat menu-row "$x" "$y"
    slow menu "pointer r" "wtype -k Escape; sleep 0.5"
    snap menu-reduce "pointer r" "wtype -k Escape; sleep 0.5"
    go home
    read -r x y w h < <(named "Feel 01, Feel Artist")
    menuat menu-tile "$((x + w / 2))" "$((y + h / 3))"
    expand true
    ui act pane lyrics
    sleep 1
    local button
    button=$(ask lyricsMenu | python3 -I -c 'import json, sys; b = json.load(sys.stdin).get("button"); print(*b if b else "")')
    if [ -n "$button" ]; then
        read -r x y <<<"$button"
        hover "$x" "$y"
        from menu-lyrics-open main pointer c
        from menu-lyrics-close main wtype -k Escape
    else
        fact "menu: no lyrics source menu shows"
    fi
    expand false
    still menu
}

leg_dialog() {
    go home
    from dialog-open main ui showDetails "Feel Song 02"
    from dialog-close main wtype -k Escape
    slow dialog "ui showDetails 'Feel Song 02'" "wtype -k Escape"
    snap dialog-reduce "ui showDetails 'Feel Song 02'" "wtype -k Escape"
    still dialog
}

# queueat WHERE: three tracks added after the current one, one removed, one moved, one added and moved at once; then
# every queue row at rest
queueat() {
    local at
    at=$(status index)
    timed "queue-$1-add" queueAdd 3
    timed "queue-$1-remove" queueRemove "$((at + 2))"
    timed "queue-$1-move" queueMove "$((at + 1)) $((at + 3))"
    # a move while an added row still fades in must not leave it half faded
    ui act queueAdd 1
    ui act queueMove "$((at + 2)) $((at + 1))"
    calm
    local opacity scale
    opacity=$(ask props DListItem 'opacity*')
    scale=$(ask props DListItem 'scale*')
    check "queue-$1 rows at rest" "$(python3 -I -c '
import json, sys
o, s = json.loads(sys.argv[1]), json.loads(sys.argv[2])
print(int(bool(o) and all(abs(x - 1) < 1e-6 for x in o + s)))' "$opacity" "$scale")" "$(wc -c <<<"$opacity$scale") bytes of opacities and scales"
}

leg_queue() {
    go home
    ui act pane queue
    sleep 1.5
    queueat side
    slow queue "ui act queueAdd 1" ":"
    snap queue-reduce "ui act queueAdd 1" ":"
    ui act pane ""
    expand true
    ui act pane queue
    sleep 1
    queueat view
    expand false
    still queue
}

leg_tabs() {
    local style
    for style in bento m3e; do
        look dark $style
        expand true
        ui act pane lyrics
        sleep 1
        timed "tabs-$style-queue" pane queue
        timed "tabs-$style-lyrics" pane lyrics
    done
    look dark bento
    sleep 0.5
    slow tabs "ui act pane queue" "ui act pane lyrics"
    snap tabs-reduce "ui act pane queue" "ui act pane lyrics"
    expand false
    ui element dock bar
    sleep 1
    ui act pane lyrics
    sleep 1
    timed tabs-side-queue pane queue
    timed tabs-side-lyrics pane lyrics
    ui element dock bar
    sleep 1
    go collection
    timed tabs-library libraryTab artists
    timed tabs-library-back libraryTab albums
    slow tabs-library "ui act libraryTab artists" "ui act libraryTab albums"
    ui settings general
    sleep 2
    # the search field has the focus when the window opens, and its caret blinking would count as frames
    wtype -k Tab
    calm settings
    local x y
    # the first visit builds the page
    read -r x y < <(ask settingsPoint Appearance 0)
    hover "$x" "$y" settings
    from tabs-settings-appearance-first settings pointer c
    read -r x y < <(ask settingsPoint General 0)
    hover "$x" "$y" settings
    from tabs-settings-general settings pointer c
    read -r x y < <(ask settingsPoint Appearance 0)
    hover "$x" "$y" settings
    from tabs-settings-appearance settings pointer c
    read -r x y < <(ask settingsPoint General 0)
    hover "$x" "$y" settings
    pointer c
    slow tabs-settings "ui settings appearance" "ui settings general" settings
    still tabs-settings settings
    hyprctl dispatch 'hl.dsp.window.close({window="title:Dux Player Settings"})' >/dev/null 2>&1 || ui settings ""
    go home
}

leg_rail() {
    local style x y
    for style in rail auto; do
        ui navStyle $style
        sleep 1
        read -r x y < <(centre railMenu)
        hover "$x" "$y"
        from "rail-$style-labels" main pointer c
        layouts "rail-$style-labels" 1
        from "rail-$style-back" main pointer c
        layouts "rail-$style-back" 1
    done
    ui navStyle rail
    park
    sleep 1
    slow rail "ui act railLabels ''" "ui act railLabels ''"
    snap rail-reduce "ui act railLabels ''" "ui act railLabels ''"
    ui navStyle auto
    still rail
}

leg_floating() {
    ui playerLook floating
    go home
    timed floating-dock dock
    timed floating-undock dock
    timed floating-open expand true
    timed floating-close expand false
    slow floating "ui expand true" "ui expand false"
    snap floating-reduce "ui expand true" "ui expand false"
    still floating
    ui playerLook card
    sleep 1
}

leg_shelf() {
    go home
    local x y before after prev
    read -r x y < <(centre shelfNext)
    before=$(ask carouselX)
    hover "$x" "$y"
    from shelf-next main pointer c
    after=$(ask carouselX)
    prev=$(ask rect shelfPrev | cut -d' ' -f5)
    check "shelf pages on" "$(truth [ "${after%.*}" -gt "${before%.*}" ])" "carousel x $before to $after"
    check "shelf can page back" "$(truth [ "$prev" = true ])" "previous enabled: $prev"
    read -r x y < <(centre shelfPrev)
    hover "$x" "$y"
    from shelf-prev main pointer c
    read -r x y < <(centre shelfNext)
    hover "$x" "$y"
    slow shelf "pointer c" "pointer c"
    snap shelf-reduce "pointer c" "pointer c"
    still shelf
}

# held ID X Y ACTED [WINDOW]: while the pointer holds X Y down the item there is drawn smaller; back to 1 after the
# release, and ACTED (a command) then succeeds once
held() {
    local during after
    hover "$2" "$3" "${5:-main}"
    pointer d
    sleep 0.35
    during=$(ask scaleAt "$2" "$3" "${5:-main}")
    pointer u
    sleep 0.6
    after=$(ask scaleAt "$2" "$3" "${5:-main}")
    check "press $1 shrinks" "$(python3 -I -c 'import sys; print(int(float(sys.argv[1]) < 0.999))' "$during")" "scale $during while held"
    check "press $1 springs back" "$(python3 -I -c 'import sys; print(int(float(sys.argv[1]) > 0.999))' "$after")" "scale $after after release"
    check "press $1 acts" "$(truth eval "$4")" "$4"
    park
    sleep 0.3
}

leg_press() {
    local x y w h
    ui open album "$album"
    sleep 1.5
    read -r x y w h < <(named Play)
    held button "$((x + w / 2))" "$((y + h / 2))" '[ "$(status state)" = playing ]'
    pausenow
    read -r x y < <(ask pagePoint More)
    held icon "$x" "$y" 'ask props .highlightIndex opened | grep -q true'
    wtype -k Escape
    sleep 0.5
    read -r x y < <(ask trackPoint)
    held row "$x" "$y" '[ "$(status title)" = "Feel Song 03" ]'
    pausenow
    go home
    read -r x y w h < <(named "Feel 01, Feel Artist")
    held tile "$((x + w / 2))" "$((y + h / 3))" 'case $(ask route) in album/*) true ;; *) false ;; esac'
    go home
    ui navStyle railLabels
    sleep 1
    read -r x y w h < <(named Albums)
    held rail "$((x + w / 2))" "$((y + h / 2))" '[ "$(ask route)" = albums/ ]'
    ui titleBars false
    sleep 1
    read -r x y < <(centre railSearch)
    held fab "$x" "$y" '[ "$(ask route)" = search/ ]'
    ui titleBars true
    ui navStyle auto
    sleep 1
    ui act prompt Rename
    sleep 1
    read -r x y w h < <(named Cancel)
    held dialog "$((x + w / 2))" "$((y + h / 2))" '[ "$(ask props .overlayOpen overlayOpen | grep -c true)" = 0 ]'
    go stats
    sleep 1
    local before during after
    read -r x y w h < <(named "30 days")
    before=$w
    hover "$((x + w / 2))" "$((y + h / 2))"
    pointer d
    sleep 0.35
    read -r _ _ during _ < <(named "30 days")
    pointer u
    sleep 0.6
    read -r _ _ after _ < <(named "30 days")
    check "press group grows" "$(truth [ "${during:-0}" -gt "$before" ])" "segment $before px at rest, $during held"
    check "press group acts" "$(truth [ "$(ask prefValue statsDays)" = 30 ])" "range $(ask prefValue statsDays)"
    ui pref statsDays 7
    ui settings general
    sleep 2
    local label="Keep running in the background" on
    on=$(ask toggleOn "$label")
    read -r x y < <(ask settingsPoint "$label" 0)
    held settings "$x" "$y" '[ "$(ask toggleOn "$label")" = "$((1 - on))" ]' settings
    ask press "$label" 0 >/dev/null
    hyprctl dispatch 'hl.dsp.window.close({window="title:Dux Player Settings"})' >/dev/null 2>&1
    go home
}

leg_idle() {
    local page
    for page in tracks stats home; do
        go $page
        still "idle-$page"
    done
    ui open album "$album"
    sleep 1.5
    still idle-album
    go home
    for style in bento m3e; do
        look dark $style
        expand true
        for pane in lyrics queue; do
            ui act pane $pane
            sleep 1
            still "idle-$style-$pane"
        done
        expand false
    done
    look dark bento
    ui settings general
    sleep 2
    still idle-settings-general settings
    ui settings appearance
    sleep 1.5
    still idle-settings-appearance settings
    hyprctl dispatch 'hl.dsp.window.close({window="title:Dux Player Settings"})' >/dev/null 2>&1
    ui playerLook floating
    sleep 1
    still idle-floating
    ui playerLook card
    sleep 1
}

leg_speed() {
    fresh
    go home
    snap speed-pages "ui open album $album" "ui back" main "animationDuration 0"
    ui open album "$album"
    sleep 1.5
    local x y
    read -r x y < <(ask trackPoint)
    hover "$x" "$y"
    snap speed-menu "pointer r" "wtype -k Escape; sleep 0.5" main "animationDuration 0"
    snap speed-trackchange "ui act next ''" ":" main "animationDuration 0"
    pausenow
    go home
}

for leg in trackchange pages menu dialog queue tabs rail floating shelf press idle speed; do
    want "$leg" || continue
    start=$SECONDS
    "leg_$leg"
    fact "leg $leg took $((SECONDS - start)) s"
done

if grep -qiE 'binding loop' "$L"; then
    fail "binding loop in ui.log"
    grep -iE 'binding loop' "$L" | sort -u | cut -c1-200 | head -5 | tee -a "$W/out/facts.txt"
fi
fact "scenario took $SECONDS s"
[ "$failed" = 0 ]
