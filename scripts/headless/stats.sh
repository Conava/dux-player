# The Statistics page against real and seeded plays: one real counted play, then copies of its row on earlier days.
# The page's totals match history.stats per range, the 30 days range takes in the older seeds and stops before the one
# 40 days back, the bars row holds a bar per day, the first top artist opens its page, and clearing the history shows
# the empty state. Shots (dark, light, compact) go to out/.
failed=0
fact() { echo "$*" >>"$W/out/facts.txt"; }
fail() {
    echo "FAIL $*" | tee -a "$W/out/facts.txt"
    failed=1
}
expect() { # NAME ACTUAL WANTED
    if [ "$2" = "$3" ]; then fact "ok   $1: $2"; else fail "$1: wanted '$3', got '$2'"; fi
}
plays() { rpc history.stats "{\"days\":$1}" | python3 -I -c 'import json, sys; print(json.load(sys.stdin)["result"]["plays"])'; }
shown() { ask objectText statsPlays | tr -dc '0-9'; }
resize() {
    hyprctl dispatch "hl.dsp.window.resize({x=$1,y=$2})" >/dev/null
    sleep 1.5
}

# a track longer than twice the 30 s a play needs to count, heard past half
dir="$W/music/Stats Artist/Stats Album"
mkdir -p "$dir"
ffmpeg -loglevel error -f lavfi -i "sine=frequency=330:duration=40" -metadata title="Stats Track" -metadata artist="Stats Artist" \
    -metadata album="Stats Album" -metadata genre="Rock" -metadata track=1 "$dir/1.flac" || exit 1
library=$(dp ipc libraries.list | python3 -I -c 'import json, sys; d = json.load(sys.stdin); print(d.get("result", d)[0]["id"] if isinstance(d, dict) else d[0]["id"])')
rpc libraries.scan "{\"library\":\"$library\"}" >/dev/null
for _ in $(seq 1 60); do
    [ -n "$(ask albumId "Stats Album" | tr -d '\n')" ] && break
    sleep 0.5
done
hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
resize 1600 1000
look dark m3e
dp play "$dir" >/dev/null
for _ in $(seq 1 40); do
    [ "$(plays 7)" -ge 1 ] && break
    sleep 1
done
dp pause >/dev/null
expect "one real play counted" "$(plays 7)" 1

# copies of that play 1, 3, 5, 10, 15 and 20 days back, and one 40 days back
python3 -I - "$XDG_DATA_HOME/dplayer/history.db" <<'P'
import sqlite3, sys, time
db = sqlite3.connect(sys.argv[1], timeout=10)
now = int(time.time())
for d in (1, 3, 5, 10, 15, 20, 40):
    db.execute("INSERT INTO plays SELECT library, track_id, album_id, ? , track FROM plays LIMIT 1", (now - d * 86400,))
db.commit()
P
expect "history.stats 7 days" "$(plays 7)" 4
expect "history.stats 30 days" "$(plays 30)" 7

go stats
sleep 1.5
expect "route is the statistics page" "$(ask route)" "stats/"
expect "7 days: the page shows history.stats' plays" "$(shown)" "$(plays 7)"
read -r rx ry rw rh _ < <(ask rect statsRange)
[ -n "$rx" ] || fail "the range selector doesn't show"
shot stats-dark
# the right third of the range selector is its 30 days segment
wclick "$((rx + rw * 5 / 6))" "$((ry + rh / 2))"
sleep 2
expect "30 days: the page shows history.stats' plays" "$(shown)" "$(plays 30)"
expect "30 days: the page stops before the play 40 days back" "$(shown)" 7
expect "30 days: a bar per day" "$(ask countObjects statsBar)" 30
shot stats-30
look light m3e
sleep 1
shot stats-light
resize 700 900
shot stats-compact
resize 1600 1000
look dark m3e

# the first top artist is a link
clickunder statsTopArtists "Stats Artist"
sleep 1.5
case "$(ask route)" in artist/*) fact "ok   a top artist opens its page: $(ask route)" ;; *) fail "a top artist did not open its page: $(ask route)" ;; esac

# a wide card with several genres: the percent stays by its name and the colours differ; then a large history
python3 -I - "$XDG_DATA_HOME/dplayer/history.db" <<'P'
import json, random, sqlite3, sys, time
db = sqlite3.connect(sys.argv[1], timeout=10)
lib, tid, aid, at, base = db.execute("SELECT * FROM plays LIMIT 1").fetchone()
base = json.loads(base)
now = int(time.time())
random.seed(7)
genres = ["Rock"] * 5 + ["Jazz"] * 4 + ["Electronic"] * 3 + ["Folk"] * 2 + ["Classical", "Metal", "Pop"]
rows = []
for i in range(150000):
    t = dict(base, id=f"seed{i % 3000}", title=f"Seed {i % 3000}", artist=f"Seed Artist {i % 120}", album=f"Seed Album {i % 300}",
             albumId=f"seed-album-{i % 300}", genre=random.choice(genres))
    t.pop("artistId", None)
    t.pop("artists", None)
    rows.append((lib, t["id"], t["albumId"], now - random.randint(0, 29 * 86400) - 3600, json.dumps(t)))
db.executemany("INSERT INTO plays VALUES (?,?,?,?,?)", rows)
db.commit()
P
go home
sleep 1
start=$(date +%s.%N)
go stats
for _ in $(seq 1 60); do
    [ "$(shown)" -ge 150000 ] 2>/dev/null && break
    sleep 0.25
done
fact "large history: statistics page shows $(shown) plays after $(awk -v a="$start" -v b="$(date +%s.%N)" 'BEGIN { printf "%.1f", b - a }') s (go settles 1.5 s first)"
sleep 1.5
shot stats-big
ping=$(date +%s%N)
ask rect statsRange >/dev/null
fact "large history: the UI answers in $((($(date +%s%N) - ping) / 1000000)) ms"
resize 2200 1000
sleep 1.5
shot stats-big-wide
look light m3e
sleep 1
shot stats-big-wide-light
look dark m3e
resize 1600 1000

# clearing the history empties the page
go stats
sleep 1
rpc history.clear '{}' >/dev/null
sleep 2
expect "after clearing: the totals are gone" "$(ask rect statsPlays)" ""
expect "after clearing: history.stats is empty" "$(plays 30)" 0
shot stats-empty

ui pref statsDays 7
exit $failed
