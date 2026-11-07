# The window opens on the first navigation entry with its item: a custom page or a pinned playlist, set live and
# after a restart, and the next entry once the first one is removed before anything navigated. Fails on a wrong route.
failed=0
expect() {
    if [ "$2" = "$3" ]; then
        echo "ok   $1: $3" >>"$W/out/facts.txt"
    else
        echo "FAIL $1: wanted '$3', got '$2'" | tee -a "$W/out/facts.txt"
        failed=1
    fi
}
route() { ask route | tr -d '\n'; }
restart() {
    qs kill -p "$W/tree" >/dev/null
    sleep 2
    dp show -c "$W/tree" >/dev/null
    answered=0
    for _ in $(seq 1 40); do
        ask route >/dev/null 2>&1 && answered=1 && break
        sleep 0.5
    done
    if [ "$answered" = 0 ]; then
        echo "FAIL the window did not answer 20 s after the restart" | tee -a "$W/out/facts.txt"
        failed=1
    fi
    sleep 3
}

view=$(ask pinFirst view "" | tr -d '\n')
sleep 1
expect "a custom page made first" "$(route)" "view/$view"
restart
expect "a custom page first at start" "$(route)" "view/$view"
shot view-first

playlist=$(rpc localPlaylists.create '{"name":"First list"}' | python3 -I -c 'import json, sys; print(":" + json.load(sys.stdin)["result"]["id"])')
sleep 1
ui pinFirst playlist "$playlist"
sleep 2
restart
expect "a pinned playlist first at start" "$(route)" "playlist/$playlist"
shot playlist-first

ui unpinFirst
sleep 1
expect "the first entry removed before navigating" "$(route)" "view/$view"
shot removed-first

[ "$failed" = 0 ]
