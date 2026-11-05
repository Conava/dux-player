#!/usr/bin/env bash
# Prints the license text of everything compiled or embedded into the release binary.
# Works on a module cache and on the vendored tree of the source tarball.
set -euo pipefail
cd "$(dirname "$0")/.."

section() { # name, files...
    printf '%s\n%s\n' '================================================================' "$1"
    shift
    for f in "$@"; do
        printf '\n---- %s\n\n' "$(basename "$f")"
        cat "$f"
        echo
    done
    echo
}

# Distributions move this file out of GOROOT, so the repo keeps a copy.
section "Go standard library and runtime" assets/LICENSE.go
section "dux-qml-common" dux-qml-common/LICENSE
for font in dux-qml-common/DCommon/assets/fonts/*/; do
    section "Font: $(basename "$font")" "$(find "$font" -maxdepth 1 -type f ! -iname '*.ttf')"
done

cd core
list=$(CGO_ENABLED=0 go list -tags withshell -deps \
    -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}}|{{.Dir}}|{{$.Dir}}{{end}}{{end}}' ./cmd/dplayer)

# A module's notices sit at its root and sometimes next to a package, so walk from each package up to the root.
declare -A files
while IFS='|' read -r module moddir dir; do
    [ -n "$moddir" ] || moddir=$PWD/vendor/${module% *}
    while :; do
        while IFS= read -r f; do
            files[$module]+="$f"$'\n'
        done < <(find "$dir" -maxdepth 1 -type f \( -iname 'copying*' -o -iname 'copyright*' -o -iname 'licen[cs]e*' \
            -o -iname 'notice*' -o -iname 'patents*' -o -iname 'unlicense*' \))
        [ "$dir" != "$moddir" ] && [ "$dir" != / ] || break
        dir=$(dirname "$dir")
    done
done <<<"$list"

while IFS= read -r module; do
    # go mod vendor drops UNLICENSE files; these are public domain, which asks for no text.
    case $module in github.com/mewkiz/pkg\ * | github.com/mewpkg/term\ *)
        [ -n "${files[$module]:-}" ] || continue ;;
    esac
    [ -n "${files[$module]:-}" ] || { echo "no license file found for $module" >&2; exit 1; }
    mapfile -t found < <(sort -u <<<"${files[$module]%$'\n'}")
    section "$module" "${found[@]}"
done < <(cut -d'|' -f1 <<<"$list" | sort -u)
