# Offline rules end to end against the fake server: copies by rule, the General and library cards, delete (after the Undo toast), delete and exclude, Download again, the rule dialog's taken scope, and collapsing library cards.
# Needs SERVER=1.
failed=0
fact() { # fact NAME ACTUAL WANTED
    if [ "$2" = "$3" ]; then
        echo "ok   $1: $2" >>"$W/out/facts.txt"
        return
    fi
    failed=1
    echo "FAIL $1: got '$2', wanted '$3'" | tee -a "$W/out/facts.txt"
}
# jsonq EXPR: evaluates a Python expression on the JSON reply `d` read from stdin
jsonq() { python3 -I -c 'import json,sys
d = json.load(sys.stdin).get("result")
print(eval(sys.argv[1]))' "$1"; }
# until COMMAND...: the command's output is 1 within 60 s
settle() {
    for _ in $(seq 1 120); do
        [ "$("$@")" = 1 ] && return 0
        sleep 0.5
    done
    return 1
}
count() { rpc cache.list "{\"library\":\"$lib\"}" 100000 | jsonq 'len(d)'; }
copies() { [ "$(count)" = "$1" ] && echo 1; }
excluded() { rpc cache.tracks "{\"library\":\"$lib\",\"excluded\":true}" 100000 | jsonq 'd["total"]'; }
# rows of the open sub-page, in order: the nth press of "Delete" is the row at that place in cache.tracks
place() { rpc cache.tracks "{\"library\":\"$lib\",\"limit\":500}" 100000 | jsonq "[i['track']['id'] for i in d['items']].index('$1')"; }

lib=$(rpc libraries.list '{}' 100000 | jsonq '[l["id"] for l in d if l["kind"] == "subsonic"][0]' 2>/dev/null)
if [ -z "$lib" ]; then
    echo "FAIL no server library (run with SERVER=1)" | tee -a "$W/out/facts.txt"
    exit 1
fi

# two favorites and an album, kept by rules; one track kept by hand
for t in tr-1-1-1 tr-2-1-1; do
    rpc marks.setFavorite "{\"library\":\"$lib\",\"kind\":\"track\",\"id\":\"$t\",\"favorite\":true}" >/dev/null
done
rpc cache.addRule '{"kind":"favorites"}' >/dev/null
rpc cache.addRule "{\"library\":\"$lib\",\"kind\":\"album\",\"id\":\"al-1-2\"}" >/dev/null
settle copies 8
fact "copies by rules" "$(count)" 8
rpc cache.keep "{\"tracks\":[\"$lib:tr-1-1-3\"],\"keep\":true}" >/dev/null
settle copies 9
fact "copy kept by hand" "$(count)" 9

ui settings downloads
sleep 2
# the settings window opens small: maximize it so the whole rules list shows
hyprctl dispatch 'hl.dsp.window.fullscreen({mode="maximized"})' >/dev/null
sleep 2
shot downloads-rules
# one server: its card is open, and the album rule sits in it rather than in General
fact "library card open" "$(ask shown "Downloaded tracks")" 1
fact "rule in the library card" "$(ask shown "Album · Second Wind")" 1
fact "Add rule in General and the library card" "$(ask press "Add rule" 0)" 2
sleep 1
shot downloads-add-sheet
fact "Favorites picked in the sheet" "$(ask pick Favorites)" true
sleep 1
shot downloads-add-taken-scope
# Favorites exists for all servers, so adding it again for All servers is blocked
rules() { rpc cache.status '{}' 100000 | jsonq 'len(d["rules"])'; }
fact "rules before Add" "$(rules)" 2
fact "Add found in the rule dialog" "$(ask press Add 0)" 1
sleep 1
fact "Add blocked for the taken scope" "$(rules)" 2
fact "rule dialog still open" "$(ask press Cancel 0)" 1
sleep 1

# from the library card the scope is implied; editing a rule offers it
ask press "Add rule" 1 >/dev/null
sleep 1
shot downloads-library-add-sheet
fact "Recently played picked in the library sheet" "$(ask pick "Recently played")" true
sleep 1
shot downloads-library-add-dialog
fact "no scope choice when adding" "$(ask shown Scope)" 0
fact "library rule added" "$(ask press Add 0)" 1
settle_rules() { [ "$(rules)" = "$1" ] && echo 1; }
settle settle_rules 3
fact "rules after adding from the library card" "$(rpc cache.status '{}' 100000 | jsonq '[r["library"] for r in d["rules"] if r["kind"] == "played"]')" "['$lib']"
sleep 1
fact "Edit of the General rule found" "$(ask press Edit 0)" 2
sleep 1
shot downloads-edit-dialog
fact "scope choice when editing" "$(ask shown Scope)" 1
fact "edit dialog closes" "$(ask press Cancel 0)" 1
sleep 1
# Recently played would keep tracks once something plays; the counts below are about the other rules
rpc cache.removeRule "{\"library\":\"$lib\",\"kind\":\"played\",\"id\":\"\"}" >/dev/null

fact "Downloaded tracks opens its page" "$(ask press "Downloaded tracks" 0)" 1
sleep 2
fact "library page open" "$(ask shown "Clear downloads")" 1
shot downloads-library

# no rule holds it: deleted without a question, once the Undo toast is gone
fact "row of the hand-kept track found" "$(ask press Delete "$(place tr-1-1-3)")" "9"
fact "hand-kept track waits for the Undo toast" "$(count)" 9
settle copies 8
fact "hand-kept track deleted" "$(count)" 8
fact "no exclusion yet" "$(excluded)" 0

# a rule holds it: the dialog asks
fact "row of the favorite found" "$(ask press Delete "$(place tr-1-1-1)")" "8"
sleep 1
shot downloads-delete-dialog
fact "delete dialog drawn" "$(dimmed downloads-library downloads-delete-dialog)" 1
pressed=$(ask press "Delete and exclude" 0)
fact "delete dialog shows its buttons" "$([ "$pressed" = 1 ] && echo 1 || echo "no, got $pressed")" 1
settle copies 7
fact "copies after delete and exclude" "$(count)" 7
fact "excluded tracks" "$(excluded)" 1

# a sync doesn't fetch it again
rpc libraries.scan "{\"id\":\"$lib\"}" >/dev/null
sleep 8
fact "excluded track not fetched by the next sync" "$(count)" 7
fact "still excluded after the sync" "$(excluded)" 1
fact "status counts the exclusion" "$(rpc cache.status '{}' 100000 | jsonq '[l["excluded"] for l in d["libraries"] if l["library"] == "'"$lib"'"][0]')" 1

ui settings downloads
sleep 2
shot downloads-rules-excluded
fact "Excluded tracks row opens its page" "$(ask press "Excluded tracks" 0)" 1
sleep 2
shot downloads-excluded
fact "Download again found" "$(ask press "Download again" 0)" 1
settle copies 8
fact "copy back after Download again" "$(count)" 8
fact "no exclusion left" "$(excluded)" 0

# a second server library: both cards start closed, and a header opens and closes its card
rpc libraries.add "{\"kind\":\"subsonic\",\"name\":\"Server Two\",\"url\":\"http://127.0.0.1:$(cat "$W/server/port")\",\"username\":\"headless\",\"password\":\"headless\",\"auth\":\"token\"}" >/dev/null
ui settings downloads
sleep 4
shot downloads-two-servers
fact "cards closed with two servers" "$(ask shown "Downloaded tracks")" 0
fact "Server Two header found" "$(ask press "Server Two" 0)" 1
sleep 1
fact "header opens its card" "$(ask shown "Downloaded tracks")" 1
shot downloads-two-servers-open
ask press "Server Two" 0 >/dev/null
sleep 1
fact "header closes its card" "$(ask shown "Downloaded tracks")" 0

# a turned-off library with a rule keeps its card, says it is off and takes no new rules
two=$(rpc libraries.list '{}' 100000 | jsonq '[l["id"] for l in d if l["name"] == "Server Two"][0]')
rpc cache.addRule "{\"library\":\"$two\",\"kind\":\"album\",\"id\":\"al-1-2\"}" >/dev/null
rpc libraries.setEnabled "{\"library\":\"$two\",\"enabled\":false}" >/dev/null
sleep 2
ask press "Server Two" 0 >/dev/null
sleep 1
shot downloads-library-off
fact "turned-off card says so" "$(ask shown "Turned off in settings")" 1
fact "no Add rule in the turned-off card" "$(ask shown "Add rule")" 1

# the library's page and its scrobbling rows
ui settings libraries
sleep 2
ask press Server 0 >/dev/null
sleep 2
shot library-page

[ "$failed" = 0 ]
