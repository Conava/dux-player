# The built-in player on a private sound server: DPLAYER_BACKEND=builtin make headless SCENARIO=scripts/headless/builtin.sh
# facts.txt holds what the daemon and the sound server said at each step.
facts="$W/out/facts.txt"
note() { printf '\n--- %s\n' "$*" >>"$facts"; }
state() {
    dp status 2>&1 | cut -c1-100 >>"$facts"
    pactl list short sink-inputs 2>&1 | cut -c1-60 >>"$facts"
}
sink_of() { pactl list short sinks | awk -v n="$1" '$2 == n { print $1 }'; }

note "play"
dp play "$W/music" >/dev/null 2>&1
sleep 3
state
shot 01-playing

note "seek to 0:12 and pause"
dp seek 12 >>"$facts" 2>&1
sleep 1
dp pause >/dev/null 2>&1
sleep 1
state

note "resume"
dp resume >/dev/null 2>&1
sleep 2
state

note "switch output to headless_b (sink $(sink_of headless_b))"
rpc devices.select '{"outputs":["headless_b"]}' >>"$facts" 2>&1
sleep 2
state

note "equalizer on: must keep playing, straight to the output"
rpc settings.set '{"equalizer":{"enabled":true}}' >/dev/null 2>&1
sleep 2
state
rpc devices.list '{}' 2>&1 | grep -o '"equalizer":"[a-zA-Z]*"' >>"$facts"

note "several outputs: must be refused"
rpc devices.select '{"outputs":["headless_a","headless_b"]}' >>"$facts" 2>&1
ui settings devices
sleep 2
shot 02-devices
ui settings playback
sleep 2

note "next track whose file is gone: skipped with a snackbar"
rm "$W/music/Test Artist/Test Album/3.flac"
dp next >/dev/null 2>&1
sleep 2
state
shot 03-track-gone

note "sink removed mid-track"
dp resume >/dev/null 2>&1
sleep 1
pw-cli destroy "$(pw-cli ls Node | awk '/^\s*id /{gsub(",","",$2); id=$2} /node.name = "headless_b"/{print id; exit}')" >>"$facts" 2>&1
sleep 1
shot 04-sink-removed-1s
sleep 1
shot 05-sink-removed-2s
sleep 5
state
shot 06-sink-removed-7s
rpc devices.list '{}' 2>&1 | grep -o '"selected":\[[^]]*\]' >>"$facts"

note "play with the chosen output still gone"
dp resume >>"$facts" 2>&1
sleep 8
state
shot 06b-play-output-gone

note "play on the default output again"
rpc devices.select '{"outputs":["default"]}' >>"$facts" 2>&1
dp resume >/dev/null 2>&1
sleep 3
state

note "default output removed mid-track: pauses, then plays on the next default"
pw-cli destroy "$(pw-cli ls Node | awk '/^\s*id /{gsub(",","",$2); id=$2} /node.name = "headless_a"/{print id; exit}')" >>"$facts" 2>&1
sleep 1
shot 06c-default-removed
state
dp resume >/dev/null 2>&1
sleep 2
state

note "sound server killed mid-track"
kill "$PULSE_PID" 2>/dev/null
sleep 1
shot 07-server-gone-1s
sleep 2
shot 08-server-gone-3s
state

note "next button with no sound server"
click 1403 1371
sleep 1
shot 09-next-failed
state

note "log"
grep -E 'WARN|ERROR' "$XDG_STATE_HOME/dplayer/daemon.log" | cut -c1-140 | tail -20 >>"$facts"
