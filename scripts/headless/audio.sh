# A private PipeWire for the built-in player: null sinks only, its session manager kept away from real devices.
mkdir -p "$XDG_CONFIG_HOME/pipewire/pipewire.conf.d" "$XDG_CONFIG_HOME/wireplumber/wireplumber.conf.d"
{
    echo 'context.objects = ['
    for sink in headless_a headless_b headless_c; do
        echo "  { factory = adapter args = { factory.name = support.null-audio-sink node.name = $sink media.class = Audio/Sink object.linger = true audio.position = [ FL FR ] } }"
    done
    echo ']'
} >"$XDG_CONFIG_HOME/pipewire/pipewire.conf.d/sinks.conf"
cat >"$XDG_CONFIG_HOME/wireplumber/wireplumber.conf.d/no-devices.conf" <<'C'
wireplumber.profiles = {
  main = {
    monitor.alsa = disabled
    monitor.alsa-midi = disabled
    monitor.bluez = disabled
    monitor.bluez-midi = disabled
    monitor.libcamera = disabled
    monitor.v4l2 = disabled
  }
}
C
pipewire >"$W/out/pipewire.log" 2>&1 &
AUDIO_PIDS=$!
wait_for "pipewire" 20 pw-cli info 0
wireplumber >"$W/out/wireplumber.log" 2>&1 &
AUDIO_PIDS="$AUDIO_PIDS $!"
pipewire-pulse >"$W/out/pipewire-pulse.log" 2>&1 &
PULSE_PID=$!
AUDIO_PIDS="$AUDIO_PIDS $PULSE_PID"
wait_for "the headless sinks" 20 sh -c 'pactl list short sinks | grep -q headless_c'
