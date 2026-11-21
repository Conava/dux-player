# Palette style and base color end to end: with a palette from a color, Vibrant changes the theme, Tonal spot brings
# back exactly the first colors and Monochrome has no chroma; a click on a Base color swatch sets the seed color; the
# settings search for "pause" finds the fixed shortcut on General. Shots of the pages go to out/.
failed=0
fact() { echo "$*" >>"$W/out/facts.txt"; }
fail() {
    echo "FAIL $*" | tee -a "$W/out/facts.txt"
    failed=1
}
expect() { # NAME ACTUAL WANTED
    if [ "$2" = "$3" ]; then fact "ok   $1: $2"; else fail "$1: wanted '$3', got '$2'"; fi
}
differs() { # NAME A B
    if [ "$2" != "$3" ]; then fact "ok   $1: $2 -> $3"; else fail "$1: still $2"; fi
}
S="Dux Player Settings"
# primary alone can stay put for a seed whose hue has no room for more chroma at tone 80, so the style is read from four roles
primary() { ask themeColor primary; }
palette() { echo "$(ask themeColor primary) $(ask themeColor secondary) $(ask themeColor tertiary) $(ask themeColor surfaceContainer)"; }
settings_shot() { # NAME
    local x y w h
    read -r x y w h < <(hyprctl -j clients | python3 -I -c '
import json, sys
c = next(c for c in json.load(sys.stdin) if c["title"] == sys.argv[1])
print(*c["at"], *c["size"])' "$S")
    grim -g "$x,$y ${w}x$h" "$W/out/$1.png"
}

hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
hyprctl dispatch 'hl.dsp.window.resize({x=1600,y=1000})' >/dev/null
sleep 1
look dark m3e
ui pref colorScheme '"seed"'
ui pref seedColor '"#6750a4"'
ui pref paletteStyle '"tonalSpot"'
sleep 1
first=$(palette)
ui pref paletteStyle '"vibrant"'
sleep 1
differs "vibrant changes the theme" "$first" "$(palette)"
ui pref paletteStyle '"tonalSpot"'
sleep 1
expect "tonal spot brings the first colors back" "$(palette)" "$first"
ui pref paletteStyle '"monochrome"'
sleep 1
mono=$(primary)
case $mono in
"#"[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]) [ "${mono:1:2}" = "${mono:3:2}" ] && [ "${mono:3:2}" = "${mono:5:2}" ] && fact "ok   monochrome primary is grey: $mono" || fail "monochrome primary has color: $mono" ;;
*) fail "monochrome primary unreadable: '$mono'" ;;
esac
ui pref paletteStyle '"tonalSpot"'
sleep 1
expect "tonal spot again" "$(palette)" "$first"

# Base color: a click on a swatch that isn't the picked one
ui settings appearance
sleep 2
settings_shot appearance-dark
mapfile -t sw < <(ask swatches)
fact "swatches: ${#sw[@]}"
[ "${#sw[@]}" -gt 1 ] || fail "fewer than two Base color swatches show"
pick=
for line in "${sw[@]}"; do
    read -r sx sy sww shh chosen <<<"$line"
    [ "$chosen" = false ] && { pick="$sx $sy $sww $shh"; break; }
done
before=$(ask prefValue seedColor)
if [ -n "$pick" ]; then
    read -r sx sy sww shh <<<"$pick"
    wclick "$((sx + sww / 2))" "$((sy + shh / 2))" "$S"
    sleep 1
    differs "a swatch click sets the seed color" "$before" "$(ask prefValue seedColor)"
    settings_shot appearance-swatch
else
    fail "no unpicked swatch"
fi
look light m3e
ui settings appearance
sleep 2
settings_shot appearance-light

# General: the fixed keys, found by the settings search
ui settings general
sleep 2
settings_shot general
ui settingsFocusSearch
sleep 0.5
wtype pause
sleep 2
results=$(ask settingsResults)
fact "search for pause: $(echo "$results" | tr '\n' ';')"
expect "search for pause finds one row on General" "$(echo "$results" | grep -c '^general ')" 1
# the general row is not the first result: step to it by its place in the list
idx=$(echo "$results" | grep -n "^general " | head -1 | cut -d: -f1)
for _ in $(seq 2 "${idx:-1}"); do wtype -k Down; done
wtype -k Return
sleep 2.5
settings_shot general-search

ui pref paletteStyle '"tonalSpot"'
ui pref colorScheme '""'
look dark m3e
exit $failed
