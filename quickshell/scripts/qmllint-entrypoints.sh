#!/usr/bin/env bash
# qmllint with warnings on, through the Quickshell tooling VFS (quickshell/.qmlls.ini).
#   qmllint-entrypoints.sh [FILE…]   every .qml (or FILE…, relative to quickshell/); fails on any warning
#                                   outside the known Quickshell noise
# A missing or stale VFS (fresh checkout, reboot, added or removed .qml) is rebuilt by starting qs once,
# offscreen, with throwaway XDG config/state/cache/data dirs and no daemon socket.
set -euo pipefail

quickshell_dir="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
qmlls_config="${quickshell_dir}/.qmlls.ini"
own_dirs=(Common Services Modules Widgets)

qmllint_bin="${QMLLINT:-}"
if [[ -z "${qmllint_bin}" ]]; then
    for candidate in qmllint6 qmllint-qt6 /usr/lib/qt6/bin/qmllint qmllint; do
        if command -v -- "${candidate}" >/dev/null 2>&1; then
            qmllint_bin="${candidate}"
            break
        fi
    done
fi
if [[ -z "${qmllint_bin}" ]]; then
    printf 'error: qmllint (Qt 6) not found in PATH (override with QMLLINT=/path/to/qmllint)\n' >&2
    exit 127
fi

ini_value() {
    local raw
    raw="$(sed -n "s/^$1=//p" "${qmlls_config}" 2>/dev/null | head -n 1)"
    raw="${raw#\"}"
    printf '%s\n' "${raw%\"}"
}

vfs_fresh() {
    local build_dir f
    build_dir="$(ini_value buildDir)"
    [[ -n "${build_dir}" && -f "${build_dir}/qs/qmldir" ]] || return 1
    # a removed .qml leaves a dangling link that breaks the copy below
    [[ -z "$(find "${build_dir}/qs" -xtype l -print -quit)" ]] || return 1
    while IFS= read -r f; do
        [[ -e "${build_dir}/qs/${f}" ]] || return 1
    done < <(cd "${quickshell_dir}" && find shell.qml "${own_dirs[@]}" -name '*.qml')
}

refresh_vfs() {
    command -v qs >/dev/null 2>&1 || {
        printf 'error: the Quickshell tooling VFS is missing or stale and qs (quickshell) is not installed\n' >&2
        exit 1
    }
    printf 'lint-qml: building the Quickshell tooling VFS (starts qs once, offscreen)\n'
    local sandbox pid i
    sandbox="$(mktemp -d)"
    # Quickshell fills .qmlls.ini only when the file already exists
    rm -f -- "${qmlls_config}"
    touch -- "${qmlls_config}"
    env -u DPLAYER_SOCKET -u WAYLAND_DISPLAY -u DISPLAY \
        QT_QPA_PLATFORM=offscreen DPLAYER_DISABLE_HOT_RELOAD=1 \
        XDG_CONFIG_HOME="${sandbox}/config" XDG_STATE_HOME="${sandbox}/state" \
        XDG_CACHE_HOME="${sandbox}/cache" XDG_DATA_HOME="${sandbox}/data" \
        qs -p "${quickshell_dir}" >"${sandbox}/qs.log" 2>&1 &
    pid=$!
    for ((i = 0; i < 300; i++)); do
        vfs_fresh && break
        kill -0 "${pid}" 2>/dev/null || break
        sleep 0.1
    done
    kill "${pid}" 2>/dev/null || true
    wait "${pid}" 2>/dev/null || true
    if ! vfs_fresh; then
        printf 'error: qs did not build the tooling VFS; its log:\n' >&2
        cat -- "${sandbox}/qs.log" >&2
        rm -rf -- "${sandbox}"
        exit 1
    fi
    rm -rf -- "${sandbox}"
}

vfs_fresh || refresh_vfs

build_dir="$(ini_value buildDir)"
import_paths_raw="$(ini_value importPaths)"
imports=()
IFS=':' read -r -a import_paths <<< "${import_paths_raw}"
for path in "${import_paths[@]}"; do
    [[ -n "${path}" ]] && imports+=(-I "${path}")
done

# qmllint resolves the VFS symlinks to the source tree, whose directories have no qmldir, so every singleton
# of a file's own module would be unresolved. A dereferenced copy keeps each file next to its qmldir.
root="$(mktemp -d)"
trap 'rm -rf -- "${root}"' EXIT
cp -rL "${build_dir}/qs" "${root}/qs"
(cd "${quickshell_dir}" && find -L "${own_dirs[@]}" DCommon -name '*.js' -exec cp --parents {} "${root}/qs/" \;)

# warnings are always on; the flag from the old two-mode script is accepted and ignored
[[ "${1:-}" == "--warnings" ]] && shift
if [[ $# -gt 0 ]]; then
    files=("$@")
else
    mapfile -t files < <(cd "${quickshell_dir}" && find shell.qml "${own_dirs[@]}" -name '*.qml')
fi

# known noise: Quickshell's own type info, not our code
output="$(cd "${root}/qs" && "${qmllint_bin}" --ignore-settings -I "${root}" "${imports[@]}" "${files[@]}" 2>&1 |
    grep -E '^(Warning|Error)' |
    grep -v 'not declared as singleton in qmldir' |
    grep -v 'Member "\(width\|height\)" not found on type "QuickshellScreenInfo"' |
    grep -v 'incomplete type "FileViewAdapter"' || true)"
if [[ -n "${output}" ]]; then
    printf '%s\n' "${output}"
    printf 'lint-qml: FAIL (%d files, warnings on)\n' "${#files[@]}" >&2
    exit 1
fi
printf 'lint-qml: PASS (%d files, warnings on)\n' "${#files[@]}"
