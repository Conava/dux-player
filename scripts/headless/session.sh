# The inside of run.sh, on its private session bus.
# Whatever ends this, a killed run included, everything it started is stopped by the PID it was started with:
# the window and the daemon first, then Hyprland, then the compositor it is nested in, never both at once. A
# nested Hyprland whose host compositor vanishes can spin at full CPU and ignore SIGTERM, so each PID gets a
# grace period and then SIGKILL. The compositors also die with this shell (pdeathsig) if it is SIGKILLed, and
# have their own session so a group-wide signal from run.sh's timeout doesn't hit them out of order.
stop() {
    local p
    for p in "$@"; do
        kill "$p" 2>/dev/null || continue
        for _ in $(seq 1 50); do
            kill -0 "$p" 2>/dev/null || break
            sleep 0.1
        done
        kill -0 "$p" 2>/dev/null && kill -9 "$p" 2>/dev/null
    done
}
cleanup() {
    trap '' EXIT TERM INT HUP
    local sock daemon=
    sock=$(ls "$XDG_RUNTIME_DIR"/dplayer-*.sock 2>/dev/null | head -1)
    sock=${sock##*dplayer-}
    daemon=${sock%.sock}
    dplayer-dev quit >/dev/null 2>&1
    case "$daemon" in '' | *[!0-9]*) daemon= ;; esac
    stop ${SHOW:-} $daemon ${H:-} ${K:-} ${AUDIO_PIDS:-} ${SERVER_PIDS:-}
}
trap cleanup EXIT
# wait_for DESCRIPTION SECONDS COMMAND...: polls every 0.1 s until COMMAND succeeds; exits the run with a message when it doesn't.
wait_for() {
    local what=$1 secs=$2
    local end=$((SECONDS + secs))
    shift 2
    until "$@" >/dev/null 2>&1; do
        [ "$SECONDS" -lt "$end" ] || { echo "timed out after ${secs}s waiting for $what"; exit 1; }
        sleep 0.1
    done
}
trap 'exit 124' TERM
trap 'exit 130' INT HUP
setpriv --pdeathsig KILL setsid kwin_wayland --virtual --width 2600 --height 1500 --socket wayland-kv --no-lockscreen >"$W/out/kwin.log" 2>&1 &
K=$!
wait_for "the kwin socket $XDG_RUNTIME_DIR/wayland-kv" 20 test -S "$XDG_RUNTIME_DIR/wayland-kv"
cat >"$W/hypr.lua" <<'H'
hl.monitor({ output = "WAYLAND-1", mode = "2560x1440@60", position = "0x0", scale = 1 })
hl.config({ misc = { disable_hyprland_logo = true, disable_splash_rendering = true } })
H
WAYLAND_DISPLAY=wayland-kv setpriv --pdeathsig KILL setsid Hyprland -c "$W/hypr.lua" >"$W/out/hyprland.log" 2>&1 &
H=$!
hypr_ready() {
    export HYPRLAND_INSTANCE_SIGNATURE=$(ls -t "$XDG_RUNTIME_DIR/hypr" 2>/dev/null | head -1)
    export WAYLAND_DISPLAY=$(ls -t "$XDG_RUNTIME_DIR" | grep -E '^wayland-[0-9]+$' | head -1)
    [ -n "$HYPRLAND_INSTANCE_SIGNATURE" ] && [ -n "$WAYLAND_DISPLAY" ] && hyprctl monitors | grep -q WAYLAND-1
}
wait_for "Hyprland to answer hyprctl" 30 hypr_ready

hyprctl dismissnotify >/dev/null
[ "${DPLAYER_BACKEND:-}" = builtin ] && . "$(dirname "$LIB")/audio.sh"
# SERVER=1: a fake Subsonic server, and a fresh keyring in the sandbox HOME on this private bus for its password.
if [ -n "${SERVER:-}" ]; then
    command -v gnome-keyring-daemon >/dev/null || { echo "SERVER=1 needs gnome-keyring-daemon (the gnome-keyring package)"; exit 1; }
    # An empty password creates no login keyring, and without one storing a secret waits on a prompt.
    printf headless | gnome-keyring-daemon --foreground --unlock --components=secrets >"$W/out/keyring.log" 2>&1 &
    keyring=$!
    SERVER_PIDS=$keyring
    export GNOME_KEYRING_CONTROL="$XDG_RUNTIME_DIR/keyring"
    python3 -I "$(dirname "$LIB")/fakesubsonic.py" "$W/server/manifest.json" "$W/server/port" 2>"$W/out/fakesubsonic.log" &
    SERVER_PIDS="$SERVER_PIDS $!"
    owned=
    for _ in $(seq 1 20); do
        gdbus call --session --dest org.freedesktop.DBus --object-path /org/freedesktop/DBus \
            --method org.freedesktop.DBus.GetConnectionUnixProcessID org.freedesktop.secrets 2>/dev/null | grep -qx "(uint32 $keyring,)" && owned=1 && break
        sleep 0.25
    done
    [ -n "$owned" ] || { echo "the keyring did not take org.freedesktop.secrets on the private bus"; exit 1; }
fi
dplayer-dev show -c "$W/tree" >"$W/out/show.log" 2>&1 &
SHOW=$!
. "$LIB"
wait_for_daemon || echo "the daemon did not come up"
added=$(rpc libraries.add "{\"kind\":\"local\",\"name\":\"Test\",\"folders\":[\"$W/music\"]}")
test_library=$(printf '%s' "$added" | python3 -Ic 'import json, sys; print(json.load(sys.stdin).get("result", {}).get("id", ""))' 2>/dev/null)
[ -n "$test_library" ] || { echo "couldn't add the Test library: $added"; exit 1; }
scanned() { rpc browse.albums "{\"library\":\"$test_library\"}" 1000000 | python3 -Ic 'import json, sys; sys.exit(not json.load(sys.stdin).get("result"))'; }
wait_for "the Test library scan" 30 scanned
wait_for "the window's headless IPC target" 30 ask route
if [ -n "${SERVER:-}" ]; then
    for _ in $(seq 1 40); do
        [ -s "$W/server/port" ] && break
        sleep 0.25
    done
    [ -s "$W/server/port" ] || { echo "the fake server did not start: $(tail -3 "$W/out/fakesubsonic.log")"; exit 1; }
    added=$(rpc libraries.add "{\"kind\":\"subsonic\",\"name\":\"Server\",\"url\":\"http://127.0.0.1:$(cat "$W/server/port")\",\"username\":\"headless\",\"password\":\"headless\",\"auth\":\"token\"}")
    server=$(printf '%s' "$added" | python3 -Ic 'import json, sys; print(json.load(sys.stdin).get("result", {}).get("id", ""))' 2>/dev/null)
    [ -n "$server" ] || { echo "couldn't add the server library: $added"; exit 1; }
    synced=
    for _ in $(seq 1 120); do
        rpc libraries.list '{}' 1000000 | python3 -Ic 'import json, sys; sys.exit(not any(l.get("synced") for l in json.load(sys.stdin)["result"] if l["id"] == sys.argv[1]))' "$server" && synced=1 && break
        sleep 0.5
    done
    [ -n "$synced" ] || { echo "the server library did not finish its catalog sync"; exit 1; }
fi
. "$SCENARIO"
status=$?
qs log -p "$W/tree" >"$W/out/ui.log" 2>&1
exit $status
