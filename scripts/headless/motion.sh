# Full view, bar and side panel motion: frame gaps while they move, every frame's positions and mid-motion stills, what
# a click on the bar opens, a track that disappears, reduce motion and the layout modes. GAP_MAX (default 80 ms) is the
# longest gap allowed inside a move; the worst gap of every leg goes to facts.txt. The page text switches to grayscale
# antialiasing after the first open and close of the full view, so a cycle runs before any shot is compared.
failed=0
SECONDS=0
GAP_MAX=${GAP_MAX:-80}
SETTLE=0.8
L="$XDG_STATE_HOME/dplayer/ui.log"
fact() { echo "$*" >>"$W/out/facts.txt"; }
fail() {
    echo "FAIL $*" | tee -a "$W/out/facts.txt"
    failed=1
}
differs() { # a b: fail when the two shots are byte for byte the same
    if cmp -s "$W/out/$1.png" "$W/out/$2.png"; then fail "$3: $1 equals $2"; else fact "ok   $3"; fi
}
same() {
    if cmp -s "$W/out/$1.png" "$W/out/$2.png"; then fact "ok   $3"; else fail "$3: $1 differs from $2"; fi
}
near() { # a fade can leave anti-aliased corners one colour step off
    if [ "$(magick compare -metric AE -fuzz 1% "$W/out/$1.png" "$W/out/$2.png" null: 2>&1 | cut -d' ' -f1)" = 0 ]; then fact "ok   $3"; else fail "$3: $1 differs from $2"; fi
}
focused() { qs ipc -p "$W/tree" call headless focused; }
geometry() {
    hyprctl -j clients | python3 -I -c '
import json, sys
c = next(c for c in json.load(sys.stdin) if c["title"] == "Dux Player")
print(sys.argv[1] % (*c["at"], *c["size"]))' "$1"
}
cap() { grim -g "$(geometry '%d,%d %dx%d')" "$W/out/$1.png"; }
# crop NAME FROM X Y W H: a region of a window shot, relative to the window
crop() { magick "$W/out/$2.png" -crop "$5x$6+$3+$4" +repage -strip "$W/out/$1.png"; }

# gaps NAME: the longest gap between frames of a move, from the first frame where something moved to the last one
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

# Every frame's positions between tracking and tracked NAME land in out/NAME.track, one JSON object per line.
tracking() { ui trackStart; }
tracked() { qs ipc -p "$W/tree" call headless trackStop >"$W/out/$1.track"; }
judge() { # NAME CODE LABEL: CODE sees the samples as s and prints why it failed, or nothing
    local out status
    out=$(python3 -I -c "
import json, sys
s = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
if not s: sys.exit('no samples')
$2" "$W/out/$1.track" 2>&1)
    status=$?
    if [ "$status" != 0 ] || [ -n "$out" ]; then fail "$3 ($1): exit $status $out"; else fact "ok   $3 ($1)"; fi
}
always() { # NAME EXPR LABEL: EXPR over a sample x holds on every frame
    judge "$1" "print(*[x for x in s if not ($2)][:1])" "$3"
}
sometime() { # NAME EXPR LABEL: EXPR over a sample x holds on some frame
    judge "$1" "any($2 for x in s) or print('never in', len(s), 'frames')" "$3"
}
# steady NAME FIELD [MAX]: FIELD moved, and over any 50 ms never faster than MAX px/ms (default RATE_MAX). Above the
# curve's own top speed at the animation duration in use, so only a jump exceeds it.
steady() {
    judge "$1" "
v = [(x['t'], x['$2']) for x in s]
pairs = [(a, next((b for b in v[i:] if b[0] - a[0] >= 50), None)) for i, a in enumerate(v)]
worst = max([abs(b[1] - a[1]) / (b[0] - a[0]) for a, b in pairs if b] or [0])
if len({x[1] for x in v}) < 3: print('did not move')
elif worst > ${3:-$RATE_MAX}: print(f'{worst:.2f} px/ms')
" "$2 moves without a jump"
}
settled() { # NAME FIELD [TIMES]: FIELD changed at most TIMES (1), so what depends on it laid out that often
    judge "$1" "
v = [x['$2'] for x in s]
n = sum(a != b for a, b in zip(v, v[1:]))
n <= ${3:-1} or print(n, 'changes')" "$2 lays out once"
}

# Around 700 tracks, so a relayout has something to chew on
album="$W/music/Test Artist/Test Album"
seq 9 700 | xargs -P 8 -I{} ffmpeg -loglevel error -i "$album/1.flac" -c copy -metadata title="Track {}" -metadata track={} "$album/{}.flac"
library=$(dp ipc libraries.list | python3 -I -c 'import json, sys; d = json.load(sys.stdin); d = d.get("result", d) if isinstance(d, dict) else d; print(d[0]["id"])')
rpc libraries.scan "{\"library\":\"$library\"}" >/dev/null
sleep 8

hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1000})' >/dev/null
sleep 1
look dark bento
go tracks
dp play "$W/music" >/dev/null
sleep 3
expand true
expand false
shot tracks-playing

# 1. gaps, playing, on Tracks
for style in bento m3e; do
    look dark $style
    expand true
    expand false
    for pane in none queue lyrics; do
        expand true
        [ "$pane" != none ] && element "$pane" view
        sleep 0.5
        leg "$style-$pane-close" expand false
        leg "$style-$pane-open" expand true
        expand false
    done
    expand true
    element lyrics view
    expand false
done
look dark bento
leg panel-open element queue bar
leg panel-close element queue bar
ui element dock bar
sleep 1
leg dock-open expand true
leg dock-close expand false
ui element dock bar
sleep 1

# 2. mid-motion stills, paused, slow
dp pause >/dev/null
sleep 1
ui animationDuration 2000
sleep 0.5
cap rest-closed
ui expand true
sleep 0.4
cap mid-open
sleep 4.5
cap end-open
ui expand false
sleep 0.4
cap mid-close
sleep 4.5
cap end-close
differs mid-open end-open "mid-open still moves"
differs mid-open rest-closed "mid-open left the pages"
differs mid-close end-open "mid-close left the view"
differs mid-close end-close "mid-close still moves"
crop pages-rest rest-closed 0 0 1600 860
crop pages-end end-close 0 0 1600 860
near pages-end pages-rest "the pages come back as they were"
ui element queue bar
sleep 0.4
cap mid-panel
sleep 2.5
cap end-panel
differs mid-panel end-panel "mid-panel still moves"
read -r _ _ ww _ < <(geometry '%d %d %d %d')
# the page's left part, narrower than the pane at its end: the same pixels when it took its width once
crop page-mid mid-panel 130 200 400 500
crop page-end end-panel 130 200 400 500
same page-end page-mid "the page keeps its layout while the panel slides"
ui element queue bar
sleep 3
ui element dock bar
sleep 3
# the docked panel stays where it is while the view moves, and the morph starts from its rectangle. Short enough
# for a track to fit in one IPC reply.
ui animationDuration 1000
sleep 0.5
tracking
ui expand true
sleep 2.5
ui expand false
sleep 2.5
tracked morph-side
settled morph-side host
# starts NAME X Y W H: the first frame showing the card has it where the morph from that rectangle to the full size
# is at that frame's progress, whatever the layout direction. Qt advances animations one vsync per frame and the first
# frames after idle can render back to back, so how far along the first frame is says nothing about the source.
starts() {
    judge "$1" "
x = next(x for x in s if x['card'])
r = x['remaining']
d = max(abs(x['cardX'] - ($2) * r), abs(x['cardY'] - ($3) * r), abs(x['cardW'] - x['fullW'] - (($4) - x['fullW']) * r), abs(x['cardH'] - x['fullH'] - (($5) - x['fullH']) * r))
d <= 12 or print(d, 'px off', x)" "the morph starts on its source"
}
starts morph-side "x['hostX']" "x['hostY']" "x['host']" "x['hostH']"
always morph-side "x['view'] == 0 or x['pages'] == 0" "the view never shows over the pages"
judge morph-side "s[-1]['card'] and print('still shown')" "the card is gone at rest"
ui element dock bar
sleep 1.5
tracking
ui expand true
sleep 2.5
ui expand false
sleep 2.5
tracked morph-bar
starts morph-bar "x['barX']" "x['barY']" "x['barW']" "x['barH']"
always morph-bar "x['view'] == 0 or x['pages'] == 0" "the view never shows over the pages"
judge morph-bar "s[-1]['card'] and print('still shown')" "the card is gone at rest"

# turned around halfway, both ways: the view and the card carry on from where they are
tracking
ui expand true
sleep 1.2
ui expand false
sleep 2.5
tracked morph-back
ui expand true
sleep 2.5
tracking
ui expand false
sleep 1.6
ui expand true
sleep 0.3
reopened=$(focused)
sleep 2.5
tracked morph-again
case "$reopened" in NowPlayingView*) fact "ok   a turned around close gives the view its focus" ;; *) fail "turned around close: focus on '$reopened'" ;; esac
for name in morph-back morph-again; do
    steady "$name" cardH 6
    steady "$name" view 0.015
    always "$name" "x['view'] == 0 or x['pages'] == 0" "the view never shows over the pages"
done
ui expand false
sleep 2.5

# 2b. the bar and the side panel, both moving, at a slow enough pace to see a jump
ui animationDuration 6000
RATE_MAX=1.8
together="x['barShows'] and x['bar'] > min(y['bar'] for y in s) + 2 and 0 < x['host'] < 300"
sleep 0.5
tracking
ui element dock bar
sleep 0.3
cap slide-in-mid
sleep 5.5
tracked slide-in
sometime slide-in "$together" "bar and panel move together"
steady slide-in bar 0.7
steady slide-in host
settled slide-in page

tracking
ui element dock bar
sleep 0.3
cap slide-out-mid
sleep 5.5
tracked slide-out
sometime slide-out "$together" "bar and panel move together"
always slide-out "x['host'] == 0 or x['kind'] == 'NowPlayingSide'" "the side panel stays on the way out"
steady slide-out bar 0.7
steady slide-out host
settled slide-out page

# there and back inside one slide
tracking
ui element dock bar
sleep 0.3
ui element dock bar
sleep 0.3
cap slide-back-mid
sleep 5.5
tracked slide-back
sometime slide-back "$together" "bar and panel move together"
always slide-back "x['host'] == 0 or x['kind'] == 'NowPlayingSide'" "the panel stays on the way back"
steady slide-back bar 0.7
steady slide-back host

# with the queue open the host stays and only its content and width change
element queue bar
sleep 5.5
tracking
ui element dock bar
sleep 0.3
cap swap-in-mid
sleep 5.5
tracked swap-in
always swap-in "x['host'] > 300" "the host stays open"
steady swap-in host 0.25
tracking
ui element dock bar
sleep 0.3
cap swap-out-mid
sleep 5.5
tracked swap-out
always swap-out "x['host'] > 300" "the host stays open"
steady swap-out host 0.25
element queue bar
ui animationDuration 250
sleep 1

# 2c. only the cover and the title open the full view
read -r wx wy _ _ < <(geometry '%d %d %d %d')
opens() { # LABEL X Y WANTED: a real click at window coordinates, then whether the view is open
    click "$((wx + $2))" "$((wy + $3))"
    sleep 1
    local open
    open=$(qs ipc -p "$W/tree" call headless viewOpen)
    [ "$open" = "$4" ] && fact "ok   $1: open $open" || fail "$1: open is $open, wanted $4"
    expand false
}
read -r bx by bw bh < <(qs ipc -p "$W/tree" call headless barRect)
read -r sx sy sw sh st < <(qs ipc -p "$W/tree" call headless stackRect)
opens "a click on the bar's empty edge" "$((bx + bw / 2))" "$((by + 3))" false
shot bar-misclick
opens "a click on the bar's cover" "$((bx + bh / 2))" "$((by + bh / 2))" true
opens "a click on the bar's title" "$((sx + 12))" "$((sy + sh / 2))" true
opens "a click beside the bar's title" "$((sx + sw - 4))" "$((sy + sh / 2))" false
ui element dock bar
sleep 1
read -r cx cy cw ch hx < <(qs ipc -p "$W/tree" call headless coverRect)
read -r sx sy sw sh st < <(qs ipc -p "$W/tree" call headless stackRect)
opens "a click on the side panel's cover" "$((cx + cw / 2))" "$((cy + ch / 2))" true
opens "a click on the side panel's title" "$((sx + sw / 2))" "$((sy + sh / 2))" true
opens "a click beside the side panel's title" "$((sx + sw - 4))" "$((sy + sh / 2))" false
opens "a click on empty side panel space" "$((hx + 4))" "$((cy + ch + 8))" false
ui element dock bar
sleep 1

# 2d. a click reaches the bar once the view has faded, while the card is still on its way
ui animationDuration 2000
sleep 0.5
expand true
sleep 4.5
ui expand false
sleep 3
click "$((wx + bx + bh / 2))" "$((wy + by + bh / 2))"
sleep 0.3
open=$(qs ipc -p "$W/tree" call headless viewOpen)
[ "$open" = true ] && fact "ok   the fading view lets the click through" || fail "a click late in the close did not reach the bar"
ui animationDuration 250
sleep 1
expand false

# 2e. reduce motion: the bar, the dock and the queue swap land at once
ui reduceMotion true
sleep 0.5
tracking
ui element dock bar
sleep 1
ui element dock bar
sleep 1
tracked rm-dock
ui element queue bar
sleep 1
tracking
ui element dock bar
sleep 1
ui element dock bar
sleep 1
tracked rm-swap
ui element queue bar
sleep 1
ui reduceMotion false
sleep 0.5
# two flips each, and the first can land before the first frame
for f in bar host; do settled rm-dock "$f" 2; done
settled rm-swap host 2
always rm-swap "x['panelFade'] == 1" "reduce motion: the swap doesn't fade"

# 3. a track that disappears
dp play "$W/music" >/dev/null
sleep 2
expand true
ui expand false
rpc queue.clear '{}' >/dev/null
sleep 1.5
shot cleared-closed
dp play "$W/music" >/dev/null
sleep 2
expand true
rpc queue.clear '{}' >/dev/null
sleep 1.5
shot cleared-open
dp play "$W/music" >/dev/null
sleep 2
expand true
shot play-after-clear
expand false
shot play-after-clear-closed
go home
go tracks

# 4. reduce motion: the end state lands at once
dp pause >/dev/null
ui reduceMotion true
sleep 0.5
ui expand true
sleep 0.15
shot rm-open-early
sleep 1.5
shot rm-open
ui expand false
sleep 0.15
shot rm-close-early
sleep 1.5
shot rm-close
same rm-open-early rm-open "reduce motion: open lands at once"
same rm-close-early rm-close "reduce motion: close lands at once"
ui reduceMotion false
sleep 0.5

# 5. search from the full view lands in the page's field
go tracks
expand true
ui searchEverything
sleep 1
wtype "abc"
sleep 0.5
text=$(qs ipc -p "$W/tree" call headless searchText)
[ "$text" = abc ] && fact "ok   search from the full view takes typing" || fail "search from the full view: typed abc, field holds '$text'"

expand true
ui searchEverything
sleep 1
wtype "def"
sleep 0.5
text=$(qs ipc -p "$W/tree" call headless searchText)
[ "$text" = abcdef ] && fact "ok   search again from the view takes typing" || fail "search again: wanted abcdef, field holds '$text'"

# with the Play on sheet open, whose close hands focus back to the deck button that opened it
expand true
opened=$(qs ipc -p "$W/tree" call headless pressDevice)
sleep 1
ui searchEverything
sleep 1
wtype "ghi"
sleep 0.5
text=$(qs ipc -p "$W/tree" call headless searchText)
[ "$opened" = true ] && [ "$text" = abcdefghi ] && fact "ok   search with the sheet open takes typing" || fail "search with the sheet open: sheet opened '$opened', wanted abcdefghi, field holds '$text'"
go tracks

# 6. layout modes: the pages are the same before and after a trip to the full view
for mode in "dock:element dock bar" "undock:element dock bar" "railLabels:navStyle railLabels" "bar:navStyle bar" "auto:navStyle auto"; do
    name=${mode%%:*}
    ui ${mode#*:}
    sleep 1
    expand true
    expand false
    cap "mode-$name-before"
    expand true
    cap "mode-$name-open"
    expand false
    cap "mode-$name-after"
    same "mode-$name-after" "mode-$name-before" "$name: same layout after the full view"
done

# 7. the compact overlay keeps its focus
hyprctl dispatch 'hl.dsp.window.resize({x=700,y=900})' >/dev/null
sleep 1.5
ui element queue bar
sleep 1.5
shot compact-panel
before=$(focused)
ui expand true
sleep 1
ui expand false
sleep 1
after=$(focused)
if [ -n "$before" ] && [ "$before" = "$after" ]; then fact "ok   compact overlay keeps its focus: $after"; else fail "compact overlay focus: '$before' before, '$after' after"; fi
shot compact-after

# a search from the full view stays in its field while the overlay is open
ui expand true
sleep 1
ui searchEverything
sleep 1
before=$(qs ipc -p "$W/tree" call headless searchText)
wtype "xyz"
sleep 0.5
text=$(qs ipc -p "$W/tree" call headless searchText)
[ "$text" = "${before}xyz" ] && fact "ok   compact search takes typing over the overlay" || fail "compact search: wanted '${before}xyz', field holds '$text', focus on $(focused)"

if grep -qiE 'binding loop' "$L"; then
    fail "binding loop in ui.log"
    grep -iE 'binding loop' "$L" | sort -u | cut -c1-200 | head -5 | tee -a "$W/out/facts.txt"
fi
fact "scenario took $SECONDS s"
[ "$failed" = 0 ]
