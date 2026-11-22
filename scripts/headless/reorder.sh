# Dragging a row's handle in the Pages editor: the row keeps its size while held, the drop lands where the pointer is,
# and the navigation order follows. Shots of the held row and the result go to out/.
failed=0
fact() { echo "$*" >>"$W/out/facts.txt"; }
fail() {
    echo "FAIL $*" | tee -a "$W/out/facts.txt"
    failed=1
}
hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
sleep 1
ui settings pages
sleep 2.5
S="Dux Player Settings"
read -r wx wy < <(winat "$S")
# the handles of the first three rows sit 1111 px from the left of the output; rows are 58 px apart
x=1111
y1=917
before=$(ask navEntries)
grim -g "1080,890 600x180" "$W/out/reorder-rest.png"
pointer m "$((x - 3))" "$y1"
sleep 0.2
pointer m "$x" "$y1"
sleep 0.3
pointer d
sleep 0.2
for dy in 10 25 45 70 100 118; do
    pointer m "$x" "$((y1 + dy))"
    sleep 0.08
done
sleep 0.5
grim -g "1080,890 600x180" "$W/out/reorder-held.png"
pointer u
sleep 1
grim -g "1080,890 600x180" "$W/out/reorder-dropped.png"
after=$(ask navEntries)
fact "before: $before"
fact "after:  $after"
[ "$after" = "page:playlists page:favorites page:home page:tracks page:albums page:artists" ] || fail "drop landed wrong: $after"
[ "$before" != "$after" ] || fail "order unchanged"

# the handle shows no tooltip while it is dragged
# the old tooltip was a light box at the left edge of this strip
bright=$(magick "$W/out/reorder-held.png" -crop 12x40+0+50 +repage -colorspace gray -format '%[fx:maxima]' info:)
[ "$(awk -v b="$bright" 'BEGIN { print (b < 0.5) }')" = 1 ] || fail "a tooltip shows while dragging (brightness $bright)"

# the Bottom bar's zone editor: Cover's handle dropped between Title and Artists
ui settings nowplaying/bar
sleep 3
zone() { ask zoneOrder bar left; }
zbefore=$(zone)
zx=1112
zy=781
grim -g "1080,755 600x200" "$W/out/zone-rest.png"
pointer m "$((zx - 3))" "$zy"
sleep 0.2
pointer m "$zx" "$zy"
sleep 0.3
pointer d
sleep 0.2
for dy in 10 25 45 70 80; do
    pointer m "$zx" "$((zy + dy))"
    sleep 0.08
done
sleep 0.5
grim -g "1080,755 600x200" "$W/out/zone-held.png"
pointer u
sleep 1
grim -g "1080,755 600x200" "$W/out/zone-dropped.png"
fact "zone before: $zbefore"
fact "zone after:  $(zone)"
[ "$(zone)" = "$(echo "$zbefore" | python3 -I -c 'import sys; a = sys.stdin.read().split(); a.insert(1, a.pop(0)); print(" ".join(a))')" ] || fail "zone drop landed wrong: $(zone)"

[ "$failed" = 0 ]
