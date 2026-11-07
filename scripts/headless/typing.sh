# Typing into a field whose text changes the list around it: every key must land, whatever the list does. Real keys
# through wtype; QtTest's would keep the focus. Fails when a character goes missing.
failed=0
expect() {
    if [ "$2" = "$3" ]; then
        echo "ok   $1: $3" >>"$W/out/facts.txt"
    else
        echo "FAIL $1: wanted '$3', got '$2'" | tee -a "$W/out/facts.txt"
        failed=1
    fi
}
keys() {
    for ((i = 0; i < ${#1}; i++)); do
        wtype "${1:i:1}"
        sleep 0.7
    done
}
stored_rule() {
    python3 -I -c 'import json, sys
d = json.load(open(sys.argv[1]))["layout"]
print(d["lists"]["overrides"][sys.argv[2]]["rules"]["list"][0].get("value", ""))' "$XDG_CONFIG_HOME/dplayer/ui-settings.json" "$1"
}
search_text() { qs ipc -p "$W/tree" call headless searchText; }

# The focus goes when rows are rebuilt with some left, so each case has to pass through that: all to one for tracks
# ("Track 2"), none back to one for the only album. Going straight to no rows proves nothing.
for page in tracks:1467,468 albums:1450,532; do
    name=${page%%:*}
    at=${page##*:}
    go "$name"
    ui pageOptions "$name" '{"rules":{"match":"all","list":[{"field":"title","op":"startsWith"}]}}'
    sleep 1
    ui edit "$name"
    sleep 3
    click "${at%,*}" "${at#*,}"
    sleep 1
    if [ "$name" = tracks ]; then
        want="Track 2xy"
        keys "$want"
    else
        want=Te
        keys x
        wtype -k BackSpace
        sleep 0.7
        keys "$want"
    fi
    sleep 1
    shot "typing-$name"
    wtype -k Escape
    sleep 2
    expect "rule on $name" "$(stored_rule "$name")" "$want"
    sleep 1
    ui pageOptions "$name" '{"rules":{"match":"all","list":[]}}'
    sleep 1
done

go tracks
ui focusSearch
sleep 1
keys "Track 2xy"
sleep 1
shot typing-search
expect "page search" "$(search_text)" "Track 2xy"

[ "$failed" = 0 ]
