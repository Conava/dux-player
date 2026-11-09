#!/usr/bin/env bash
# Runs headless scenarios, several at a time. usage: sweep.sh all | affected [base]
# all: every scenario in the table below, JOBS at a time (default 4).
# affected: the scenarios whose row matches a file changed since BASE (default: the merge-base with origin/master),
# counting uncommitted and untracked files.
# DRY=1 only prints the pick.
# GAP_MAX, SEEN_MAX, DELAY and SETTLE reach the scenarios; the frame gap limits fail on a machine that is busy with other work.
# Each run is its own session: private runtime dir, D-Bus, compositors, helper build and output. The only thing
# runs share is the dplayer-dev binary, which the make target builds once before the first starts.
set -u
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)

# scenario | environment | ERE over changed paths: the code it covers (AGENTS.md says what each one checks); slowest first
TABLE='
motion||^quickshell/Modules/(MainWindow|NowPlaying[A-Za-z]*|SidePanel|M3EPlayer|BentoPlayer|PlayerArt|MediaSeekbar|SeekBlock|QueueList)\.qml$
snappy|SERVER=1|^quickshell/Modules/(MainWindow|NowPlaying[A-Za-z]*|[A-Za-z]*Page|[A-Za-z]*Dialog|MediaMenu|MediaTile|MediaRow)\.qml$|^quickshell/Services/(Playlist|Settings|Catalog)Service\.qml$
scroll||^quickshell/Modules/([A-Za-z]*Page|PageScroll|TrackList)\.qml$|^quickshell/Common/WheelInput\.js$
navigation||^quickshell/Modules/(Router|MainWindow)\.qml$
keys||^quickshell/Modules/(MainWindow|EscapeLayer|[A-Za-z]*Dialog|[A-Za-z]*Sheet|SearchBox|ScopedSearch)\.qml$|^quickshell/Common/EscapeStack\.js$|^quickshell/Widgets/
downloads|SERVER=1|^quickshell/Modules/Settings/(DownloadsSettings|DownloadedTracks|OfflineRuleDialog|OfflineItemPicker)\.qml$|^quickshell/Modules/KeepOfflineRow\.qml$|^core/internal/cache/
library|SERVER=1|^quickshell/Modules/Settings/(LibrariesSettings|LibraryPage|LibraryDialog|LibraryServerDialog|ServerSignIn)\.qml$
builtin|DPLAYER_BACKEND=builtin|^core/internal/player/|^quickshell/Services/OutputService\.qml$|^quickshell/Modules/(Snackbar|DeviceButton|DeviceList|DevicePickerDialog)\.qml$|^quickshell/Modules/Settings/DevicesSettings\.qml$
typing||^quickshell/Modules/(PageHeader|SearchBox|ScopedSearch|TrackList|BrowsePage|SearchPage)\.qml$|^quickshell/Common/RowModel\.qml$
corners||^quickshell/Modules/Settings/|^quickshell/scripts/settings_index\.py$
cursors||^quickshell/Modules/(MainWindow|Snackbar|SelectionBar|EscapeLayer)\.qml$
columns||^quickshell/Modules/TrackList\.qml$|^quickshell/Common/(ColumnDrag|Layouts)\.js$
home||^quickshell/Modules/(HomePage|Shelf)\.qml$
menus||^quickshell/Modules/(MediaTile|MediaMenu|MediaRow)\.qml$|^quickshell/DCommon/Widgets/DMenu\.qml$
startup||^quickshell/Modules/Router\.qml$|^quickshell/Modules/Settings/PagesSettings\.qml$|^quickshell/Common/UiConfig\.qml$
'
# these measure frame gaps and click timing, which a loaded machine fails: they run first, one at a time
ALONE=" motion snappy navigation "
# a change here runs every scenario
HARNESS='^scripts/headless/(run\.sh|session\.sh|lib\.sh|audio\.sh|sweep\.sh|fakesubsonic\.py|ptr\.c|vp\.xml)$'
# UI code no row names runs one scenario that loads the whole window and fails on QML errors
FALLBACK_RE='^quickshell/(Modules|Widgets|Common|Services|Shaders)/|^quickshell/shell\.qml$'
FALLBACK=home

rows=()
while IFS= read -r line; do [ -n "$line" ] && rows+=("$line"); done <<<"$TABLE"
names=()
for r in "${rows[@]}"; do names+=("${r%%|*}"); done

declare -A why=()
picked=()
add() { # scenario, file
    [ -n "${why[$1]:-}" ] || picked+=("$1")
    why[$1]+="${why[$1]:+, }$2"
}

pick_affected() {
    local base=${1:-$(git -C "$repo" merge-base HEAD origin/master 2>/dev/null || echo HEAD)} f r s env res hit uncovered=
    local files
    files=$( (git -C "$repo" diff --name-only "$base"; git -C "$repo" ls-files -o --exclude-standard) | sort -u)
    while IFS= read -r f; do
        [ -n "$f" ] || continue
        hit=
        if [[ $f =~ $HARNESS ]]; then
            for s in "${names[@]}"; do add "$s" "$f"; done
            continue
        fi
        for r in "${rows[@]}"; do
            IFS='|' read -r s env res <<<"$r"
            if [[ $f == "scripts/headless/$s.sh" ]] || [[ $f =~ $res ]]; then add "$s" "$f"; hit=1; fi
        done
        [ -n "$hit" ] && continue
        if [[ $f =~ $FALLBACK_RE ]]; then add "$FALLBACK" "$f"; continue; fi
        case $f in *.md | docs/* | LICENSE* | quickshell/translations/* | quickshell/tests/* | */*_test.go | Makefile | scripts/headless/*) ;; *) uncovered+="$f " ;; esac
    done <<<"$files"
    [ -n "$uncovered" ] && echo "no scenario covers: $uncovered"
    if [ ${#picked[@]} = 0 ]; then echo "no scenario to run"; exit 0; fi
    echo "scenarios picked (changes since ${base:0:10}):"
    for s in "${picked[@]}"; do echo "  $s: $(cut -d, -f1-3 <<<"${why[$s]}")$([ "$(tr -cd , <<<"${why[$s]}" | wc -c)" -ge 3 ] && echo ', ...')"; done
}

case "${1:-}" in
all) picked=("${names[@]}") ;;
affected) pick_affected "${2:-}" ;;
*) echo "usage: sweep.sh all | affected [base]"; exit 2 ;;
esac

[ -n "${DRY:-}" ] && exit 0
jobs=${JOBS:-4}
results=$(mktemp -d /tmp/dphs.XXXXXX)
runone() { # scenario
    local s=$1 env row start
    for row in "${rows[@]}"; do [ "${row%%|*}" = "$s" ] && env=$(cut -d'|' -f2 <<<"$row"); done
    start=$SECONDS
    env ${env:-} "$here/run.sh" "$here/$s.sh" >"$results/$s.log" 2>&1
    local rc=$?
    printf '%s %s %s\n' "$rc" "$((SECONDS - start))" "$(sed -n 's/^output: //p' "$results/$s.log")" >"$results/$s.res"
}
for s in "${picked[@]}"; do
    case "$ALONE" in *" $s "*) runone "$s" ;; esac
done
for s in "${picked[@]}"; do
    case "$ALONE" in *" $s "*) continue ;; esac
    while [ "$(jobs -rp | wc -l)" -ge "$jobs" ]; do wait -n; done
    runone "$s" &
done
wait

fail=0
printf '\n%-12s %-6s %5s  %s\n' scenario result secs output
for s in "${picked[@]}"; do
    read -r rc secs out <"$results/$s.res"
    res=ok
    [ "$rc" = 0 ] || { res=FAIL; fail=1; }
    printf '%-12s %-6s %5s  %s\n' "$s" "$res" "$secs" "${out:-(see $results/$s.log)}"
done
echo "logs: $results"
exit $fail
