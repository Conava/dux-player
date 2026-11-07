# Full view and side panel motion: frame gaps while they move, mid-slide stills, a track that disappears, reduce motion
# and the layout modes. GAP_MAX (default 80 ms) is the longest gap allowed inside a slide; the worst gap of every
# leg goes to facts.txt. The page text switches to grayscale antialiasing after the first open and close of the full
# view, so a cycle runs before any shot is compared.
failed=0
SECONDS=0
GAP_MAX=${GAP_MAX:-80}
SETTLE=0.6
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

# gaps NAME: the longest gap between frames inside a slide. Idle time before the first frame at pace and after the
# last one is not a gap, and a leg without a single frame at pace is a failure.
gaps() {
    local worst
    worst=$(python3 -I - "$W/out/$1.frames" <<'P'
import sys
g = [int(x) for x in open(sys.argv[1]).read().split()]
pace = [i for i, x in enumerate(g) if x <= 25]
print(max(g[pace[0]:pace[-1] + 1]) if pace else -1)
P
    )
    fact "gap $1: worst $worst ms of $(wc -w <"$W/out/$1.frames") frames"
    if [ "$worst" -lt 0 ]; then fail "gap $1: no frames"; elif [ "$worst" -gt "$GAP_MAX" ]; then fail "gap $1: $worst ms over $GAP_MAX"; fi
}
leg() { # NAME CMD...: frames around CMD, then the check
    local name=$1
    frames "$@"
    gaps "$name"
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

# 2. mid-slide stills, paused, slow
dp pause >/dev/null
sleep 1
ui animationDuration 2000
sleep 0.5
cap rest-closed
ui expand true
sleep 0.4
cap mid-open
sleep 2.5
cap end-open
ui expand false
sleep 0.4
cap mid-close
sleep 2.5
cap end-close
differs mid-open end-open "mid-open still moves"
differs mid-open rest-closed "mid-open left the pages"
differs mid-close end-open "mid-close left the view"
differs mid-close end-close "mid-close still moves"
crop pages-rest rest-closed 0 0 1600 860
crop pages-end end-close 0 0 1600 860
same pages-end pages-rest "the pages come back as they were"
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
# the docked panel stays where it is while the view slides
cap dock-rest
ui expand true
sleep 3
ui expand false
sleep 0.4
cap dock-mid-close
crop dock-strip-rest dock-rest $((ww - 250)) 130 150 500
crop dock-strip-mid dock-mid-close $((ww - 250)) 130 150 500
same dock-strip-rest dock-strip-mid "the docked panel does not slide in again"
sleep 3
ui element dock bar
ui animationDuration 250
sleep 1

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
