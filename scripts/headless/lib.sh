# Helpers for scenarios. The window sits on a 2560x1440 output.
# DPLAYER_BACKEND=builtin make headless also starts a private sound server with the sinks headless_a, _b and _c.
#   shot NAME        screenshot to out/NAME.png
#   go PAGE          show a page: home, libraries, history, search, ...
#   expand true|false  open or close the full now playing view
#   look MODE STYLE  theme and player style: look dark m3e
#   element TYPE CONTEXT  run a player element: element queue view
#   ui NAME ARGS     any function of the headless IPC target in run.sh: ui open album ID, ui settings PAGE, ui playerLook attached,
#                    ui pageOptions tracks '{"lines":["format"]}', ui addElement fullBento title '{"type":"text"}'
#   dp ARGS          the dplayer CLI against this sandbox: dp play "$W/music", dp pause
#   rpc METHOD JSON  one daemon IPC call; prints the reply
#   pointer m X Y | c | d | u | w NOTCHES   move, click, press, release, wheel, through a real virtual pointer
#   click X Y        move and click
#   frames NAME CMD  run CMD and write the ms between UI frames to out/NAME.frames
dp() { dplayer-dev "$@"; }

wait_for_daemon() {
    for _ in $(seq 1 60); do
        ls "$XDG_RUNTIME_DIR"/dplayer*.sock >/dev/null 2>&1 && return 0
        sleep 0.5
    done
    return 1
}

rpc() {
    python3 -I - "$1" "$2" <<'P'
import glob, json, os, socket, sys
s = socket.socket(socket.AF_UNIX)
s.connect(sorted(glob.glob(os.environ["XDG_RUNTIME_DIR"] + "/dplayer*.sock"))[-1])
f = s.makefile("rwb")
f.readline()
f.write(json.dumps({"id": 1, "method": sys.argv[1], "params": json.loads(sys.argv[2])}).encode() + b"\n")
f.flush()
print(f.readline().decode().strip()[:2000])
P
}

ui() { qs ipc -p "$W/tree" call headless "$@" >/dev/null; }
go() { ui go "$1"; sleep "${SETTLE:-1.5}"; }
expand() { ui expand "$1"; sleep "${SETTLE:-1.5}"; }
look() { ui look "$1" "$2"; sleep "${SETTLE:-1.5}"; }
element() { ui element "$1" "$2"; }
shot() { grim "$W/out/$1.png"; }
# frames NAME CMD...: runs CMD, then records the ms between UI frames for 1.5 s in out/NAME.frames
frames() {
    local name=$1
    shift
    qs ipc -p "$W/tree" call headless framesStart >/dev/null
    "$@"
    sleep 1.5
    qs ipc -p "$W/tree" call headless framesStop >"$W/out/$name.frames"
}

pointer() {
    if [ ! -p "$W/ptr" ]; then
        mkfifo "$W/ptr"
        ptr <"$W/ptr" 2>"$W/out/ptr.log" &
        exec 9>"$W/ptr"
    fi
    echo "$*" >&9
}
click() { pointer m "$(($1 - 2))" "$2"; sleep 0.1; pointer m "$1" "$2"; sleep 0.3; pointer c; }
