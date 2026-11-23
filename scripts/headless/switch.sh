# Page switches on a library of a real size: 1000 albums by 700 artists. Clicks every navigation entry on the rail
# with the real pointer, three rounds, and records the frames from the click on. The first round may build a page
# (the entries' pages are built ahead once the window shows its first page); later rounds must only show it. Fails on
# a gap over FIRST_MAX (80 ms) in the first round or WARM_MAX (40 ms) after it. gaps.txt lists "round entry worst".
failed=0
FIRST_MAX=${FIRST_MAX:-80}
WARM_MAX=${WARM_MAX:-40}
fact() { echo "$*" >>"$W/out/facts.txt"; }
fail() {
    echo "FAIL $*" | tee -a "$W/out/facts.txt"
    failed=1
}

ffmpeg -loglevel error -f lavfi -i anullsrc=r=44100:cl=mono -t 1 "$W/seed.flac"
python3 -I - "$W" <<'PY'
import os, shutil, subprocess, sys
w = sys.argv[1]
genres = ["Rock", "Jazz", "Electronic", "Pop", "Classical", "Hip-Hop", "Folk", "Metal"]
for a in range(1000):
    artist = f"Artist {a % 700:03d} {chr(65 + a % 26)}"
    album = f"{chr(65 + (a * 7) % 26)}lbum {a:04d}"
    d = f"{w}/music/big/{artist}/{album}"
    os.makedirs(d, exist_ok=True)
    f = f"{d}/1.flac"
    shutil.copy(f"{w}/seed.flac", f)
    subprocess.run(["metaflac", "--remove-all-tags", f"--set-tag=TITLE=Song {a}", f"--set-tag=ARTIST={artist}", f"--set-tag=ALBUM={album}", "--set-tag=TRACKNUMBER=1", f"--set-tag=GENRE={genres[a % 8]}", f"--set-tag=DATE={1970 + a % 50}", f], check=True)
PY
library=$(dp ipc libraries.list | python3 -I -c 'import json, sys; d = json.load(sys.stdin); print(d.get("result", d)[0]["id"] if isinstance(d, dict) else d[0]["id"])')
rpc libraries.scan "{\"library\":\"$library\"}" >/dev/null
last=$(python3 -I -c 'print(chr(65 + (999 * 7) % 26) + "lbum 0999")')
for _ in $(seq 1 240); do
    [ -n "$(ask albumId "$last" | tr -d '\n')" ] && break
    sleep 0.5
done
[ -n "$(ask albumId "$last" | tr -d '\n')" ] || fail "the scan did not finish in 120 s"

ui navStyle rail
sleep 1.5
pointer m 1278 720
sleep 0.2
pointer m 1280 720
# the entries' pages build ahead after the first page shows
sleep 6

labels=()
for entry in $(ask navEntries); do
    case $entry in
    page:home) labels+=(Home) ;;
    page:playlists) labels+=(Playlists) ;;
    page:favorites) labels+=(Favorites) ;;
    page:tracks) labels+=(Tracks) ;;
    page:albums) labels+=(Albums) ;;
    page:artists) labels+=(Artists) ;;
    esac
done
[ "${#labels[@]}" -ge 5 ] || fail "only ${#labels[@]} known navigation entries: $(ask navEntries)"

for round in 1 2 3; do
    for label in "${labels[@]}"; do
        read -r x y w h < <(ask namedAll "$label" | sed -n 1p)
        [ -n "$x" ] || {
            fail "no rail entry $label"
            continue
        }
        read -r wx wy < <(winat)
        pointer m "$((wx + x + w / 2 - 2))" "$((wy + y + h / 2))"
        sleep 0.1
        pointer m "$((wx + x + w / 2))" "$((wy + y + h / 2))"
        sleep 0.4
        ui timedFrom main
        pointer c
        sleep 1.2
        frames=$(ask timedStop)
        worst=$(tr ' ' '\n' <<<"$frames" | sort -n | tail -1)
        echo "$round $label ${worst:--}" >>"$W/out/gaps.txt"
        max=$([ "$round" = 1 ] && echo "$FIRST_MAX" || echo "$WARM_MAX")
        if [ -z "$worst" ]; then
            fail "round $round $label: no frames"
        elif [ "$worst" -gt "$max" ]; then
            fail "round $round $label: worst gap $worst ms over $max ms ($frames)"
        else
            fact "ok   round $round $label: worst gap $worst ms"
        fi
    done
done
shot end
[ "$failed" = 0 ]
