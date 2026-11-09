# Pages and popups feel instant: frame gaps while an album page opens and closes and the full view opens and closes,
# a menu and a dialog that open at their settled size, and a rating, a playlist reorder and a settings switch that
# show their new value while the server (or the daemon) hasn't answered yet. Needs SERVER=1.
# GAP_MAX (80 ms) is the longest wait allowed between frames of a move, counted from the action to the first frame;
# SEEN_MAX (400 ms) the longest an optimistic action may take to show; DELAY (800 ms) how long the fake server holds
# every answer. The worst gap of every leg goes to facts.txt.
failed=0
GAP_MAX=${GAP_MAX:-80}
SEEN_MAX=${SEEN_MAX:-400}
DELAY=${DELAY:-800}
SETTLE=1
fact() { echo "$*" >>"$W/out/facts.txt"; }
fail() {
    echo "FAIL $*" | tee -a "$W/out/facts.txt"
    failed=1
}
if [ -z "${SERVER:-}" ]; then
    fail "needs SERVER=1"
    return 1 2>/dev/null || exit 1
fi

geometry() {
    hyprctl -j clients | python3 -I -c '
import json, sys
c = next(c for c in json.load(sys.stdin) if c["title"] == "Dux Player")
print(sys.argv[1] % (*c["at"], *c["size"]))' "$1"
}

# leg NAME ACTION ARG: the frame gaps while ACTION happens (see timed in run.sh), checked against GAP_MAX
leg() {
    local name=$1 worst
    qs ipc -p "$W/tree" call headless timed "$2" "${3:-}" >/dev/null
    sleep 1.5
    qs ipc -p "$W/tree" call headless timedStop >"$W/out/$name.frames"
    worst=$(tr ' ' '\n' <"$W/out/$name.frames" | sort -n | tail -1)
    fact "gap $name: worst ${worst:--} ms of $(wc -w <"$W/out/$name.frames") frames"
    if [ -z "$worst" ]; then
        fail "gap $name: no frames"
    elif [ "$worst" -gt "$GAP_MAX" ]; then
        fail "gap $name: a wait of $worst ms between frames, over $GAP_MAX"
    fi
}

# sizes KIND NAME LABEL: the popups sampled since popupsStart held one size from their first sample on, and a dialog
# never drew scaled down
sizes() {
    qs ipc -p "$W/tree" call headless popupsStop >"$W/out/$2.samples"
    python3 -I - "$W/out/$2.samples" "$1" <<'P' && fact "ok   $3" || fail "$3 (see $2.samples)"
import json, sys
s = [json.loads(l) for l in open(sys.argv[1]) if l.strip() and json.loads(l)["kind"] == sys.argv[2]]
if not s:
    sys.exit("never showed")
size = {(x["w"], x["h"]) for x in s}
if len(size) > 1:
    sys.exit(f"{len(s)} samples, first {s[0]['w']}x{s[0]['h']}, last {s[-1]['w']}x{s[-1]['h']}, {len(size)} sizes")
if any(x.get("scale", 1) < 0.999 for x in s):
    sys.exit("drawn scaled down")
P
}

# seen NAME WANTED CMD...: CMD prints what the screen shows; it must be WANTED within SEEN_MAX ms of the action, which
# the caller has just done, and still be after the reply
seen() {
    local name=$1 wanted=$2 got
    shift 2
    sleep "$(python3 -c "print($SEEN_MAX / 1000)")"
    got=$("$@")
    if [ "$got" = "$wanted" ]; then fact "ok   $name shows '$got' within $SEEN_MAX ms"; else fail "$name shows '$got' after $SEEN_MAX ms, wanted '$wanted'"; fi
}
settles() { # NAME WANTED CMD...: the value is still right once the server has answered
    local name=$1 wanted=$2 got
    shift 2
    sleep "$(python3 -c "print($DELAY * 5 / 1000)")"
    got=$("$@")
    if [ "$got" = "$wanted" ]; then fact "ok   $name stays '$got' after the reply"; else fail "$name is '$got' after the reply, wanted '$wanted'"; fi
}

hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1000})' >/dev/null
sleep 1
look dark bento
dp play "$W/music" >/dev/null
sleep 3
dp pause >/dev/null
go home
album=$(ask albumId "First Light")
[ -n "$album" ] || fail "no album First Light in the catalog"

# 1. frame gaps. The first visit builds everything cold; only the second counts.
leg album-first-open open "album $album"
ui back
sleep 1.5
leg album-open open "album $album"
leg home-back back
leg album-open-again open "album $album"
ui back
sleep 1.5
leg fullview-open expand true
leg fullview-close expand false
shot snappy-gaps

# 2. a menu and a dialog open at their settled size
ui open album "$album"
sleep 1.5
read -r px py < <(ask trackPoint)
read -r wx wy _ _ < <(geometry '%d %d %d %d')
[ -n "$px" ] || fail "no track row on the album page"
ui popupsStart
rclick "$((wx + px))" "$((wy + py))"
sleep 1.5
shot snappy-menu
sizes menu menu "a track menu opens at its settled size"
wtype -k Escape
sleep 1
ui popupsStart
ui showDetails "First Light 1"
sleep 2
shot snappy-dialog
sizes dialog dialog "the details dialog opens at its settled size"
wtype -k Escape
sleep 1

# 3. a rating, a playlist reorder and a settings switch show at once; the server holds its answers for DELAY ms
echo "$DELAY" >"$W/server/delay"
current=$(ask ratingShown)
if [ "$current" = -1 ]; then
    fail "no rating bar on the album page"
else
    stars=$((current % 5 + 1))
    ask rate "$stars" >/dev/null
    seen "rating" "$stars" ask ratingShown
    settles "rating" "$stars" ask ratingShown
fi

pl=$(ask playlistId "Server Mix")
ui loadPlaylist "$pl"
for _ in $(seq 1 20); do
    [ -n "$(ask playlistOrder "$pl")" ] && break
    sleep 0.5
done
IFS=, read -ra order <<<"$(ask playlistOrder "$pl")"
if [ "${#order[@]}" -lt 2 ]; then
    fail "playlist Server Mix didn't load"
else
    moved=$(
        IFS=,
        echo "${order[*]:1},${order[0]}"
    )
    ui moveFirstLast "$pl"
    seen "playlist order" "$moved" ask playlistOrder "$pl"
    settles "playlist order" "$moved" ask playlistOrder "$pl"
fi
rm -f "$W/server/delay"

# the switch is a daemon setting, so the daemon is the one to hold back: stopped, it answers nothing
ui settings general
sleep 2
label="Keep running in the background"
on=$(ask toggleOn "$label")
if [ "$on" = -1 ]; then
    fail "no switch '$label' in the settings"
else
    daemon=$(ls "$XDG_RUNTIME_DIR"/dplayer-*.sock | head -1)
    daemon=${daemon##*dplayer-}
    daemon=${daemon%.sock}
    kill -STOP "$daemon"
    ask press "$label" 0 >/dev/null
    seen "settings switch" "$((1 - on))" ask toggleOn "$label"
    kill -CONT "$daemon"
    settles "settings switch" "$((1 - on))" ask toggleOn "$label"
fi

fact "scenario took $SECONDS s"
[ "$failed" = 0 ]
