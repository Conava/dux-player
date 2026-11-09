# A library's page: its local and server rows, a rename through the Name dialog, a scrobbling switch, the Server dialog and Downloads opened with that library's card open.
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
# until COMMAND...: the command's output is 1 within 30 s
settle() {
    for _ in $(seq 1 60); do
        [ "$("$@")" = 1 ] && return 0
        sleep 0.5
    done
    return 1
}

lib=$(rpc libraries.list '{}' 100000 | jsonq '[l["id"] for l in d if l["kind"] == "subsonic"][0]' 2>/dev/null)
if [ -z "$lib" ]; then
    echo "FAIL no server library (run with SERVER=1)" | tee -a "$W/out/facts.txt"
    exit 1
fi
local=$(rpc libraries.list '{}' 100000 | jsonq '[l["id"] for l in d if l["kind"] == "local"][0]')

ui settings "libraries/$local"
sleep 2
shot library-local
# shown and press count the settings search's unseen copy of the page too: only presence is checked
fact "local page shows its folders" "$(ask shown "Add folder")" 1

# a second server, so Downloads starts with every card closed
rpc libraries.add "{\"kind\":\"subsonic\",\"name\":\"Server Two\",\"url\":\"http://127.0.0.1:$(cat "$W/server/port")\",\"username\":\"headless\",\"password\":\"headless\",\"auth\":\"token\"}" >/dev/null
sleep 2

ui settings libraries
sleep 2
fact "list row opens the page" "$(ask press Server 0)" 1
sleep 2
shot library-server

fact "Name row found" "$(ask press Name 0)" 1
sleep 1
shot library-name-dialog
wtype Renamed
sleep 0.5
wtype -k Return
named() { [ "$(rpc libraries.list '{}' 100000 | jsonq "[l['name'] for l in d if l['id'] == '$lib'][0]")" = Renamed ] && echo 1; }
settle named
fact "renamed through the Name dialog" "$(rpc libraries.list '{}' 100000 | jsonq "[l['name'] for l in d if l['id'] == '$lib'][0]")" Renamed

off() { rpc scrobble.status '{}' 100000 | jsonq "int('$lib' in (d.get('noServerLibraries') or []))"; }
fact "plays reported to the server" "$(off)" 0
sleep 1
fact "scrobble switch found" "$([ "$(ask press "Report plays to Renamed" 0)" -ge 1 ] && echo 1)" 1
settle off
fact "plays no longer reported to the server" "$(off)" 1
shot library-scrobble-off

fact "Server row found" "$(ask press Server 0)" 1
sleep 1
shot library-server-dialog
fact "Server dialog has Test connection" "$(ask shown "Test connection")" 1
ask press Cancel 0 >/dev/null
sleep 1

# the sidebar tab comes first
fact "Downloads row found" "$([ "$(ask press Downloads 1)" -ge 2 ] && echo 1)" 1
sleep 3
shot downloads-from-library
fact "its card is open" "$(ask shown "Downloaded tracks")" 1

[ "$failed" = 0 ]
