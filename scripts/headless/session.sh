# The inside of run.sh, on its private session bus.
# Whatever ends this, a killed run included, nothing it started stays behind.
cleanup() {
    dplayer-dev quit >/dev/null 2>&1
    kill ${AUDIO_PIDS:-} ${H:-} ${K:-} 2>/dev/null
}
trap cleanup EXIT
trap 'exit 124' TERM
kwin_wayland --virtual --width 2600 --height 1500 --socket wayland-kv --no-lockscreen >"$W/out/kwin.log" 2>&1 &
K=$!
sleep 2
cat >"$W/hypr.lua" <<'H'
hl.monitor({ output = "WAYLAND-1", mode = "2560x1440@60", position = "0x0", scale = 1 })
hl.config({ misc = { disable_hyprland_logo = true, disable_splash_rendering = true } })
H
WAYLAND_DISPLAY=wayland-kv Hyprland -c "$W/hypr.lua" >"$W/out/hyprland.log" 2>&1 &
H=$!
sleep 3
export HYPRLAND_INSTANCE_SIGNATURE=$(ls -t "$XDG_RUNTIME_DIR/hypr" | head -1)
export WAYLAND_DISPLAY=$(ls -t "$XDG_RUNTIME_DIR" | grep -E '^wayland-[0-9]+$' | head -1)

hyprctl dismissnotify >/dev/null
[ "${DPLAYER_BACKEND:-}" = builtin ] && . "$(dirname "$LIB")/audio.sh"
dplayer-dev show -c "$W/tree" >"$W/out/show.log" 2>&1 &
. "$LIB"
wait_for_daemon || echo "the daemon did not come up"
rpc libraries.add "{\"kind\":\"local\",\"name\":\"Test\",\"folders\":[\"$W/music\"]}"
sleep 4
. "$SCENARIO"
status=$?
qs log -p "$W/tree" >"$W/out/ui.log" 2>&1
exit $status
