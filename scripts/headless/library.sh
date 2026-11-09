# A library's page: its local and server rows, a rename through the Name dialog, a scrobbling switch, the Server dialog, Downloads opened with that library's card open, and leaving the page by the header's Back, Alt+Left, Backspace and the mouse back button.
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

# leaving a sub-page: real keys and pointer, so the settings window needs the pointer in it for the keyboard
sub() { ask settingsSub | tr -d '\n'; }
ui settings libraries
sleep 2
at=$(hyprctl -j clients | python3 -I -c 'import json,sys
c = [c for c in json.load(sys.stdin) if c["title"] == "Dux Player Settings"][0]
print(c["at"][0], c["at"][1])')
# settingsclick LABEL N: clicks the nth item of the settings window labelled LABEL
settingsclick() {
    set -- $(ask settingsPoint "$1" "$2") $at
    click $(($1 + $3)) $(($2 + $4))
}
fact "no Back on a page's main view" "$(ask shown Back)" 0
settingsclick Renamed 0
sleep 1.5
shot library-header-back
fact "a row opens its page" "$(sub)" Renamed
fact "header shows Back" "$(ask shown Back)" 1
wtype -k BackSpace
sleep 1
fact "Backspace goes back" "$(sub)" ""
fact "the row gets the focus back" "$(ask settingsFocus | tr -d '\n')" Renamed
shot library-back-done
settingsclick Renamed 0
sleep 1.5
wtype -M alt -k Left -m alt
sleep 1
fact "Alt+Left goes back" "$(sub)" ""
settingsclick Renamed 0
sleep 1.5
pointer b
sleep 1
fact "mouse back button goes back" "$(sub)" ""
ask press Renamed 0 >/dev/null
sleep 1.5
fact "header Back found" "$(ask press Back 0)" 1
sleep 1
fact "header Back goes back" "$(sub)" ""
# a text field keeps its Backspace; leaving the page from it gives the focus to the row that opened the page
settingsclick Test 0
sleep 1.5
fact "Enter a path found" "$(ask press "Enter a path" 0)" 1
sleep 0.5
wtype x
wtype -k BackSpace
wtype -k BackSpace
sleep 1
fact "Backspace in a field stays" "$(sub)" Test
wtype -M alt -k Left -m alt
sleep 1
fact "Alt+Left from a field goes back" "$(sub)" ""
fact "the opening row gets the focus back" "$(ask settingsFocus | tr -d '\n')" Test

[ "$failed" = 0 ]
