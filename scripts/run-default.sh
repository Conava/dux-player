#!/usr/bin/env bash
# make run-default: the dev window as on a first start. The player's own config, data, state and cache dirs start
# empty on every run; the libraries are copied from the real library.db (their passwords stay in the keyring), and
# everything else under the XDG dirs (the shell's colors and settings, fonts, mpv) is the real one, linked in. The
# real player dirs are only read.
set -euo pipefail
shopt -s nullglob dotglob
binary=$1
shell_dir=$2
root=$3

rm -rf "$root"
for kind in config data state cache; do
    case $kind in
    config) real=${XDG_CONFIG_HOME:-$HOME/.config} ;;
    data) real=${XDG_DATA_HOME:-$HOME/.local/share} ;;
    state) real=${XDG_STATE_HOME:-$HOME/.local/state} ;;
    cache) real=${XDG_CACHE_HOME:-$HOME/.cache} ;;
    esac
    mkdir -p "$root/$kind"
    for entry in "$real"/*; do
        [ "${entry##*/}" = dplayer ] || ln -s "$entry" "$root/$kind/"
    done
done
real_data=${XDG_DATA_HOME:-$HOME/.local/share}/dplayer
mkdir -p "$root/data/dplayer"
for f in "$real_data"/library.db "$real_data"/library.db-wal "$real_data"/library.db-shm; do
    cp "$f" "$root/data/dplayer/"
done

XDG_CONFIG_HOME=$root/config XDG_DATA_HOME=$root/data XDG_STATE_HOME=$root/state XDG_CACHE_HOME=$root/cache \
    exec "$binary" show -c "$shell_dir"
