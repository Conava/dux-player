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

for page in libraries downloads general appearance pages nowplaying nowplaying/bar nowplaying/side nowplaying/fullBento nowplaying/fullM3e playback lyrics scrobble devices integrations about; do
    ui settings "$page"
    audit "$page"
    [ "$page" = downloads ] && shot downloads
done

# a library's own page: the local one always, the server's with SERVER=1
libs=$(rpc libraries.list '{}' 100000 | python3 -I -c 'import json,sys
d = json.load(sys.stdin)
for l in (d.get("result", d) if isinstance(d, dict) else d):
    print(l["kind"], l["id"])')
local=$(echo "$libs" | awk '$1 == "local" {print $2; exit}')
ui settings "libraries/$local"
audit "libraries/<local>"
shot library-local

server=$(rpc libraries.list '{}' 100000 | python3 -I -c 'import json,sys
d = json.load(sys.stdin)
for l in (d.get("result", d) if isinstance(d, dict) else d):
    if l.get("kind") == "subsonic":
        print(l["id"]); break')
if [ -n "$server" ]; then
    # a rule in General and one in the library card, the card open and then closed
    rpc cache.addRule '{"kind":"favorites"}' >/dev/null
    rpc cache.addRule "{\"library\":\"$server\",\"kind\":\"album\",\"id\":\"al-1-2\"}" >/dev/null
    ui settings downloads
    audit "downloads with rules"
    shot downloads-rules
    ask press Server 0 >/dev/null
    audit "downloads, library card closed"
    shot downloads-closed
    ask press Server 0 >/dev/null
    for page in "libraries/$server" "downloads/$server" "downloads/excluded:$server"; do
        ui settings "$page"
        audit "$page"
        shot "$(echo "$page" | tr '/:' '--')"
    done
else
    echo "skip libraries/<server>, downloads/<server> and downloads/excluded:<server> (no server, run with SERVER=1)" >>"$W/out/facts.txt"
fi

[ "$failed" = 0 ]
