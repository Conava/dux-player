# Walks the settings pages and fails on a row outside a group or with the wrong corners for its place on screen.
failed=0
audit() {
    sleep 2
    faults=$(qs ipc -p "$W/tree" call headless cornerFaults)
    if [ -z "$faults" ]; then
        echo "ok   $1" >>"$W/out/facts.txt"
        return
    fi
    failed=1
    echo "FAIL $1" | tee -a "$W/out/facts.txt"
    echo "$faults" | sed 's/^/     /' | tee -a "$W/out/facts.txt"
}

dp play "$W/music" >/dev/null 2>&1
sleep 2

# First, on a page built for the first time: its rows then arrive in the order that swapped the corners
ui customize fullBento
sleep 2
ui elementOptions title
audit "element options"
shot element-options
ui elementOptions ""

for page in libraries general appearance pages nowplaying nowplaying/bar nowplaying/side nowplaying/fullBento nowplaying/fullM3e playback lyrics scrobble devices about; do
    ui settings "$page"
    audit "$page"
done

[ "$failed" = 0 ]
