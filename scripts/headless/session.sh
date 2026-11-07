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
    stop ${SHOW:-} $daemon ${H:-} ${K:-} ${AUDIO_PIDS:-}
}
trap cleanup EXIT
trap 'exit 124' TERM
trap 'exit 130' INT HUP
setpriv --pdeathsig KILL setsid kwin_wayland --virtual --width 2600 --height 1500 --socket wayland-kv --no-lockscreen >"$W/out/kwin.log" 2>&1 &
K=$!
sleep 2
cat >"$W/hypr.lua" <<'H'
hl.monitor({ output = "WAYLAND-1", mode = "2560x1440@60", position = "0x0", scale = 1 })
hl.config({ misc = { disable_hyprland_logo = true, disable_splash_rendering = true } })
H
WAYLAND_DISPLAY=wayland-kv setpriv --pdeathsig KILL setsid Hyprland -c "$W/hypr.lua" >"$W/out/hyprland.log" 2>&1 &
H=$!
sleep 3
export HYPRLAND_INSTANCE_SIGNATURE=$(ls -t "$XDG_RUNTIME_DIR/hypr" | head -1)
export WAYLAND_DISPLAY=$(ls -t "$XDG_RUNTIME_DIR" | grep -E '^wayland-[0-9]+$' | head -1)

hyprctl dismissnotify >/dev/null
[ "${DPLAYER_BACKEND:-}" = builtin ] && . "$(dirname "$LIB")/audio.sh"
dplayer-dev show -c "$W/tree" >"$W/out/show.log" 2>&1 &
SHOW=$!
. "$LIB"
wait_for_daemon || echo "the daemon did not come up"
rpc libraries.add "{\"kind\":\"local\",\"name\":\"Test\",\"folders\":[\"$W/music\"]}"
sleep 4
. "$SCENARIO"
status=$?
qs log -p "$W/tree" >"$W/out/ui.log" 2>&1
exit $status
