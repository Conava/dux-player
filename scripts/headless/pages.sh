# The pages, Home and navigation features end to end with real clicks: the playing tile, shelf titles and arrows, the
# Library entry and its tabs, the letter index, the page header tint, the album page side by side, Speed dial and
# Genre chips, the rail menu button and the title-bars-off rail, the header field on a narrow window, the three Pages
# style rows and the Playlists page's collection tiles. Needs SERVER=1: the Downloads tile shows only with a server.
failed=0
fact() {
    echo "$1" >>"$W/out/facts.txt"
}
fail() {
    echo "FAIL $1" | tee -a "$W/out/facts.txt"
    failed=1
}
expect() { # expect NAME ACTUAL WANTED
    if [ "$2" = "$3" ]; then fact "ok   $1: $2"; else fail "$1: wanted '$3', got '$2'"; fi
}
# check NAME COMMAND...: ok when the command succeeds
check() {
    local name=$1
    shift
    if "$@"; then fact "ok   $name"; else fail "$name"; fi
}
jsonq() { python3 -I -c 'import json,sys
d = json.load(sys.stdin).get("result")
print(eval(sys.argv[1]))' "$1"; }
lt() { [ "$1" -lt "$2" ]; }
gt() { [ "$1" -gt "$2" ]; }
# fgt A B: A > B for decimals
fgt() { awk -v a="$1" -v b="$2" 'BEGIN { exit !(a > b) }'; }
count() { grep -c .; }
yesno() { "$@" && echo yes || echo no; }
resize() {
    hyprctl dispatch "hl.dsp.window.resize({x=$1,y=$2})" >/dev/null
    sleep 2
}
# pixel SHOT X Y: "R G B" averaged over 6x6 px at a point relative to the main window
pixel() {
    local wx wy
    read -r wx wy < <(winat)
    python3 -I -c 'import sys
from PIL import Image
x, y = int(sys.argv[2]), int(sys.argv[3])
print(*Image.open(sys.argv[1]).convert("RGB").crop((x, y, x + 6, y + 6)).resize((1, 1), Image.BOX).getpixel((0, 0)))' "$W/out/$1.png" "$((wx + $2))" "$((wy + $3))"
}
# spread "R G B": how far apart the channels are, 0 for a grey
spread() { python3 -I -c 'import sys; v = [int(a) for a in sys.argv[1].split()]; print(max(v) - min(v))' "$1"; }
# apart "R G B" "R G B": the largest channel difference
apart() { python3 -I -c 'import sys; a = [int(v) for v in sys.argv[1].split()]; b = [int(v) for v in sys.argv[2].split()]; print(max(abs(x - y) for x, y in zip(a, b)))' "$1" "$2"; }
tone() { # tone FILE TITLE ARTIST ALBUM TRACK GENRE SECONDS
    ffmpeg -loglevel error -f lavfi -i "sine=frequency=$((200 + $5 * 20)):duration=$7" -metadata title="$2" -metadata artist="$3" -metadata album="$4" -metadata track="$5" -metadata genre="$6" "$1" </dev/null
}
home_shelves() { # home_shelves TYPE:SIZE...: a Home with just these shelves
    local rows="" spec
    for spec in "$@"; do
        rows="$rows${rows:+,}{\"id\":\"${spec%%:*}\",\"type\":\"${spec%%:*}\",\"hidden\":false,\"placement\":\"auto\",\"options\":{\"layout\":\"carousel\",\"size\":\"${spec##*:}\"}}"
    done
    ui surface home "{\"zones\":{\"shelves\":[$rows]}}"
    sleep 1.5
}
settle_scan() { # settle_scan ALBUM...: until every album shows in the UI
    local a
    for a in "$@"; do
        for _ in $(seq 1 80); do
            [ -n "$(ask albumId "$a" | tr -d '\n')" ] && break
            sleep 0.5
        done
    done
}
# pick_option LABEL PREF WANTED OTHER: a real click in the settings window sets the app setting; it still does after the
# setting changed by other means (a row whose binding broke would think the option is still selected)
pick_option() {
    local point
    point=$(ask settingsPoint "$1" 0)
    [ -n "$point" ] || {
        fail "settings row '$1' shows"
        return
    }
    wclick ${point% *} ${point#* } "Dux Player Settings"
    sleep 0.8
    expect "clicking '$1' sets $2" "$(ask prefValue "$2")" "$3"
    ui pref "$2" "$4"
    sleep 0.8
    wclick ${point% *} ${point#* } "Dux Player Settings"
    sleep 0.8
    expect "clicking '$1' again after $2 changed" "$(ask prefValue "$2")" "$3"
}

lib=$(rpc libraries.list '{}' | jsonq '[l["id"] for l in d if l["kind"] == "subsonic"][0]' 2>/dev/null)
if [ -z "$lib" ]; then
    echo "FAIL no server library (run with SERVER=1)" | tee -a "$W/out/facts.txt"
    exit 1
fi
local_lib=$(rpc libraries.list '{}' | jsonq '[l["id"] for l in d if l["kind"] == "local"][0]')
hyprctl dispatch 'hl.dsp.window.float({action="set"})' >/dev/null
resize 1600 1000

# One played album first: a play counts after half of a 40 s track, which is heard while the rest is generated.
mkdir -p "$W/music/Played Artist/Played Album"
tone "$W/music/Played Artist/Played Album/1.flac" "Played Track" "Played Artist" "Played Album" 1 Rock 40 || exit 1
rpc libraries.scan "{\"library\":\"$local_lib\"}" >/dev/null
settle_scan "Played Album"
rpc player.play "{\"album\":\"$(ask albumId "Played Album" | tr -d '\n')\"}" >/dev/null
play_start=$SECONDS

genres=(Rock Jazz Pop Folk)
n=0
for name in Aurora Bravo Cobalt Crimson Delta Echo Fjord Gamma Harbor Indigo Juniper Kilo Lantern Mosaic Nimbus Opal Prism Quartz Raven Sierra Slate Tundra Umbra Velvet Willow Xenon Yonder Zephyr; do
    mkdir -p "$W/music/Artist $name/$name"
    tone "$W/music/Artist $name/$name/1.flac" "$name One" "Artist $name" "$name" 1 "${genres[$((n % 4))]}" 3 || exit 1
    n=$((n + 1))
done
# a red and a grey cover: the page tint follows the colour, and a grey cover gives a neutral tint
ffmpeg -loglevel error -f lavfi -i color=c=0xC02020:s=240x240 -frames:v 1 "$W/music/Artist Crimson/Crimson/cover.png" </dev/null
ffmpeg -loglevel error -f lavfi -i color=c=0x808080:s=240x240 -frames:v 1 "$W/music/Artist Slate/Slate/cover.png" </dev/null
mkdir -p "$W/music/Long Artist/Long Album"
for i in $(seq -w 1 30); do
    tone "$W/music/Long Artist/Long Album/$i.flac" "Long $i" "Long Artist" "Long Album" "$((10#$i))" Jazz 2 || exit 1
done
rpc libraries.scan "{\"library\":\"$local_lib\"}" >/dev/null
settle_scan Zephyr "Long Album" Crimson Slate
sleep 3
# heard past half of the track, then paused: the album stays the queue's origin and the play counted
while [ $((SECONDS - play_start)) -lt 24 ]; do sleep 1; done
dp pause >/dev/null
crimson=$(ask albumId Crimson | tr -d '\n')
slate=$(ask albumId Slate | tr -d '\n')
long=$(ask albumId "Long Album" | tr -d '\n')

# ---- Library entry and its tabs
expect "the default navigation has no Library entry" "$(yesno grep -q page:collection <<<"$(ask navEntries)")" no
ui pinFirst page collection
sleep 1
go home
clicknamed Library
sleep 2
expect "the Library entry opens its page" "$(ask route)" "collection/"
expect "Library opens on Albums" "$(ask prefValue libraryTab)" '"albums"'
shot library-albums
clickunder libraryTabs Artists
sleep 2
expect "the Artists tab is remembered" "$(ask prefValue libraryTab)" '"artists"'
expect "the Artists tab lists artists" "$(ask topItem)" "Artist Aurora"
shot library-artists
go home
clicknamed Library
sleep 2
expect "Library reopens on the remembered tab" "$(ask topItem)" "Artist Aurora"
clickunder libraryTabs Albums
sleep 2
expect "the Albums tab is remembered" "$(ask prefValue libraryTab)" '"albums"'
ui open album "$long"
sleep 2
expect "an album opened with Albums in the navigation lights Albums" "$(ask navLit)" page:albums
ui pinFirst page albums
ui unpinFirst
sleep 1
expect "Albums leaves the navigation" "$(yesno grep -q page:albums <<<"$(ask navEntries)")" no
go collection
sleep 1.5
clicknamed "Aurora, Artist Aurora"
sleep 2
expect "an album opens from Library's Albums tab" "$(ask route | cut -d/ -f1)" album
expect "Library lights for the opened album when Albums isn't in the navigation" "$(ask navLit)" page:collection
shot library-album-lit

# ---- Letter index, on the Albums page and the Tracks page
ui pageOptions albums '{"sort":"title"}'
go albums
header=$(ask rect scopedSearch)
expect "no letter index while the setting is off" "$(ask rect letterIndex)" ""
ui pref letterIndex true
sleep 2
check "the letter index shows when the setting is on and the list is sorted by name" test -n "$(ask rect letterIndex)"
expect "the index leaves the page header where it was" "$(ask rect scopedSearch)" "$header"
before=$(ask namedAll "Mosaic, Artist Mosaic" | head -1 | cut -d' ' -f2)
shot letters-albums
clicknamed "Jump to M"
sleep 1.5
after=$(ask namedAll "Mosaic, Artist Mosaic" | head -1 | cut -d' ' -f2)
check "a letter click brings its first album to the top ($before -> $after)" test "${after:-9999}" -lt 330 -a "${after:-0}" -gt 20
expect "the letter of the top row is the current one" "$(ask props .currentLetter currentLetter)" '["M"]'
shot letters-jumped
read -r lx ly lw lh < <(ask namedAll "Jump to P" | head -1)
read -r wx wy < <(winat)
fact "hover at $lx $ly in the window at $wx $wy"
pointer m "$((wx + ${lx%.*} + ${lw%.*} / 2))" "$((wy + ${ly%.*} + ${lh%.*} / 2))"
sleep 0.6
shot letters-hover
check "the letters near the pointer grow" fgt "$(ask props .currentLetter hoverAmount | tr -d '[]')" 0.9
pointer m 5 5
expect "the index doesn't show for a sort that isn't by name" "$(ui pageOptions albums '{"sort":"added"}'; sleep 1.5; ask rect letterIndex)" ""
ui pageOptions albums '{"sort":"title"}'
sleep 1.5
check "the index shows again by name" test -n "$(ask rect letterIndex)"
check "a tall window lists every letter" test -n "$(ask namedAll "Jump to B")"
resize 1600 560
check "a short window drops letters" test -z "$(ask namedAll "Jump to B")"
check "a short window keeps the last letter" test -n "$(ask namedAll "Jump to Z")"
shot letters-short
resize 1600 1000
go tracks
sleep 1.5
# the taller header leaves room for a subset of the letters, so M is only a dot here
clicknamed "Jump to L"
sleep 1.5
expect "a letter click on the Tracks page puts its first track at the top" "$(ask topItem | cut -c1)" "L"
ui pref letterIndex false
sleep 1.5
expect "no letter index when the setting goes off" "$(ask rect letterIndex)" ""

# ---- Page header tint
ui pref pageHeader '"tint"'
ui pref albumLayout '"stacked"'
ui open album "$crimson"
sleep 2.5
read -r shown sat <<<"$(ask tint)"
expect "a red cover tints the page" "$shown" shown
check "the red tint is saturated ($sat)" fgt "$sat" 0.3
shot tint-red
red=$(pixel tint-red 800 90)
ui open album "$slate"
sleep 2.5
read -r shown sat <<<"$(ask tint)"
expect "a grey cover tints the page" "$shown" shown
check "the grey tint is neutral ($sat)" fgt 0.08 "$sat"
shot tint-grey
grey=$(pixel tint-grey 800 90)
ui pref pageHeader '"plain"'
sleep 2
expect "Plain draws no tint" "$(ask tint)" ""
shot tint-plain
plain=$(pixel tint-plain 800 90)
check "the red tint changes the header's pixels ($red against $plain)" gt "$(apart "$red" "$plain")" 10
check "the grey tint adds no hue ($grey against $red)" gt "$(spread "$red")" "$(($(spread "$grey") + 8))"
ui pref pageHeader '"tint"'

# ---- Album page side by side
ui pref albumLayout '"sideBySide"'
ui open album "$long"
sleep 2.5
read -r hx hy hw hh <<<"$(ask heroRect)"
check "side by side: the card is narrower than the page ($hw)" lt "$hw" 720
read -r wx wy < <(winat)
pointer m "$((wx + 900))" "$((wy + 600))"
sleep 0.3
pointer w 12
sleep 1.5
scrolled=$(printf '%.0f' "$(ask offset)")
check "the tracks scroll ($scrolled)" gt "$scrolled" 0
read -r _ hy2 _ <<<"$(ask heroRect)"
expect "the card stays while the tracks scroll" "$hy2" "$hy"
shot album-side-by-side
ui pref albumLayout '"stacked"'
ui open album "$long"
sleep 2.5
read -r _ sy _ <<<"$(ask heroRect)"
pointer m "$((wx + 900))" "$((wy + 600))"
sleep 0.3
pointer w 12
sleep 1.5
read -r _ sy2 _ <<<"$(ask heroRect)"
check "stacked: the cover scrolls away with the tracks ($sy -> $sy2)" lt "$sy2" "$sy"
ui pref albumLayout '"sideBySide"'
sleep 2
resize 2400 1200
read -r _ _ w2400 _ <<<"$(ask heroRect)"
resize 2000 1200
read -r _ _ w2000 _ <<<"$(ask heroRect)"
expect "the card width is capped" "$w2000" "$w2400"
check "the capped card is at most 480 wide ($w2400)" lt "$w2400" 481
shot album-wide
resize 1000 1000
read -r _ _ w1000 _ <<<"$(ask heroRect)"
check "under the breakpoint the album stacks ($w1000)" gt "$w1000" 600
resize 700 900
read -r _ _ w700 _ <<<"$(ask heroRect)"
check "a compact window stacks ($w700)" gt "$w700" 450
resize 1600 1000
read -r _ _ w1600 _ <<<"$(ask heroRect)"
check "back to the wide window the card returns ($w1600)" lt "$w1600" 720
clicknamed "Edit page"
sleep 1.5
expect "edit mode stacks: no side card" "$(ask heroRect)" ""
read -r gx _ <<<"$(ask namedAll "Group by disc" | head -1)"
check "edit mode: the editor takes the page's width ($gx)" lt "${gx:-9999}" 400
shot album-edit
clicknamed Done
sleep 1.5
ui pref albumLayout '"stacked"'

# ---- Home: the playing tile, shelf titles and arrows
resize 1600 1000
home_shelves recentlyPlayed:m recentlyAdded:s
go home
expect "the album the queue plays from has the plate" "$(ask playingTiles | tr ';' '\n' | sort -u)" "Played Album"
shot home-playing
home_shelves recentlyPlayed:m
expect "no arrows when the row fits" "$(ask rect shelfNext)" ""
home_shelves recentlyPlayed:m recentlyAdded:s
read -r _ _ _ _ prev <<<"$(ask rect shelfPrev)"
read -r _ _ _ _ next <<<"$(ask rect shelfNext)"
expect "the first arrow starts disabled" "$prev" false
expect "the next arrow starts enabled" "$next" true
x0=$(ask carouselX)
clickrect shelfNext
sleep 1.2
x1=$(ask carouselX)
check "next moves the row ($x0 -> $x1)" gt "$x1" "$x0"
read -r _ _ _ _ prev <<<"$(ask rect shelfPrev)"
expect "previous is enabled after paging" "$prev" true
shot home-arrows
for _ in $(seq 1 12); do
    read -r _ _ _ _ next <<<"$(ask rect shelfNext)"
    [ "$next" = true ] || break
    clickrect shelfNext
    sleep 1
done
expect "the last page disables next" "$next" false
shot home-arrows-end
for _ in $(seq 1 12); do
    read -r _ _ _ _ prev <<<"$(ask rect shelfPrev)"
    [ "$prev" = true ] || break
    clickrect shelfPrev
    sleep 1
done
expect "paging back disables previous" "$prev" false
expect "paging back returns to the start" "$(ask carouselX)" 0
resize 740 900
expect "a compact window of 740 shows no arrows" "$(ask rect shelfNext)" ""
resize 640 900
expect "a window of the minimum width shows no arrows" "$(ask rect shelfNext)" ""
resize 1600 1000
clicknamed "Recently added, see all"
sleep 2
expect "the shelf title opens its page" "$(ask route)" "albums/recent"
ui back
sleep 1.5

# ---- Genre chips and Speed dial
home_shelves genreChips:m speedDial:m
go home
shot home-chips
clicknamed Jazz
sleep 2
route=$(ask route)
expect "a genre chip opens its genre" "$(echo "$route" | cut -d/ -f1)" genre
genre=${route#genre/}
ui back
sleep 1.5
ui pinFirst genre "$genre"
ui pinFirst playlist "$lib:pl-1"
for _ in 1 2 3 4 5 6 7 8; do ui pinFirst view ""; done
sleep 1.5
tiles=$(ask dialTiles)
expect "Speed dial starts with Shuffle all" "$(echo "$tiles" | head -1 | cut -d' ' -f1-2)" "Shuffle all"
check "pinned entries come before the recent albums" test "$(echo "$tiles" | grep -n "^Server Mix" | cut -d: -f1)" -lt "$(echo "$tiles" | grep -n "^Played Album" | cut -d: -f1)"
check "the pinned genre is a tile" grep -q "^Jazz " <<<"$tiles"
check "the recently played album is a tile" grep -q "^Played Album " <<<"$tiles"
expect "cover-less tiles keep one size" "$(echo "$tiles" | grep "^First " | awk '{print $(NF-2), $(NF-1)}' | sort -u | wc -l)" 1
expect "a pinned custom page has no art" "$(echo "$tiles" | grep "^First " | awk '{print $NF}' | sort -u)" false
shot home-speed-dial
rm -f "$W/out/rows.txt"
for size in s m l; do
    home_shelves speedDial:$size
    resize 700 900
    tiles=$(ask dialTiles)
    echo "$size $(echo "$tiles" | awk '{print $(NF-3)}' | sort -u | wc -l)" >>"$W/out/rows.txt"
    shot "speed-dial-$size"
    resize 1600 1000
done
expect "Speed dial rows by size on a narrow window" "$(tr '\n' ' ' <"$W/out/rows.txt")" "s 1 m 2 l 3 "
home_shelves speedDial:m genreChips:m
resize 1600 1000
go home
clicknamed "Shuffle all"
sleep 2
status=$(rpc player.status '{}')
expect "Shuffle all plays" "$(echo "$status" | jsonq 'd["state"]')" playing
expect "Shuffle all shuffles" "$(echo "$status" | jsonq 'd["shuffle"]')" True
check "Shuffle all queues the whole library" gt "$(echo "$status" | jsonq 'd["length"]')" 40
dp pause >/dev/null

# ---- Rail menu and the title-bars-off rail
go home
home_shelves recentlyPlayed:m
read -r _ _ railw _ <<<"$(ask rect railDestinations)"
clickrect railMenu
sleep 1.5
read -r _ _ labelw _ <<<"$(ask rect railDestinations)"
check "the menu button shows labels ($railw -> $labelw)" gt "$labelw" "$railw"
expect "Automatic keeps the setting" "$(ask prefValue navStyle)" '"auto"'
expect "the labels are remembered" "$(ask prefValue autoRailLabels)" true
shot rail-labels
resize 700 900
expect "a narrow window keeps the bar under Automatic" "$(ask rect railMenu)" ""
resize 1600 1000
clickrect railMenu
sleep 1.5
expect "the button hides the labels again" "$(ask prefValue autoRailLabels)" false
ui pref navStyle '"rail"'
sleep 1
clickrect railMenu
sleep 1.5
expect "with Rail the button switches to Rail with labels" "$(ask prefValue navStyle)" '"railLabels"'
clickrect railMenu
sleep 1.5
expect "and back to Rail" "$(ask prefValue navStyle)" '"rail"'
ui pref navStyle '"auto"'
ui titleBars false
sleep 2
shot rail-no-titlebar
read -r _ sy _ sh _ <<<"$(ask rect railSettings)"
check "with title bars off the rail has Settings at the bottom ($sy)" gt "${sy:-0}" 700
clickrect railSearch
sleep 1.5
expect "the Search button opens search" "$(ask route | cut -d/ -f1)" search
go home
sleep 1
# Tab from the search field around to Settings, then open it
focus=
for _ in $(seq 1 40); do
    wtype -k Tab
    sleep 0.25
    focus=$(ask focusName)
    [ "$focus" = Settings ] && break
done
expect "the rail's Settings is reachable with Tab" "$focus" Settings
ui titleBars true
sleep 1.5

# ---- Header search field on a narrow window
ui headerSearch field
resize 700 900
go albums
read -r fx fy fw _ <<<"$(ask namedAll Search | head -1)"
check "the header field takes the title's place ($fw)" gt "${fw:-0}" 250
expect "the title makes room" "$(ask namedAll "Dux Player" | count)" 0
shot header-field-narrow
wclick "$((fx + fw / 2))" "$((fy + 10))"
sleep 0.5
wtype Zeph
sleep 1.5
expect "typing in the header field opens search" "$(ask route | cut -d/ -f1)" search
shot header-field-search
ui headerSearch button
resize 1600 1000
go home

# ---- Light, right-to-left and compact variants: stills to read, plus the facts that can be checked
hover() { # hover NAME: the pointer rests on the named rect
    local x y w h e wx wy
    read -r x y w h e < <(ask rect "$1")
    [ -n "$x" ] || return 1
    read -r wx wy < <(winat)
    pointer m "$((wx + x + w / 2 - 2))" "$((wy + y + h / 2))"
    sleep 0.2
    pointer m "$((wx + x + w / 2))" "$((wy + y + h / 2))"
    sleep 1.5
}
variant() { # variant TAG: the page features in the current theme, language and window
    local t=$1
    ui pref pageHeader '"tint"'
    ui pref albumLayout '"stacked"'
    ui pref letterIndex true
    ui pageOptions albums '{"sort":"title"}'
    go albums
    shot "v-$t-letters"
    ui pref letterIndex false
    ui open album "$crimson"
    sleep 2.5
    shot "v-$t-tint-red"
    ui pref pageHeader '"plain"'
    sleep 2
    shot "v-$t-tint-plain"
    ui pref pageHeader '"tint"'
    ui pref albumLayout '"sideBySide"'
    ui open album "$long"
    sleep 2.5
    shot "v-$t-album-card"
    ui pref albumLayout '"stacked"'
    home_shelves recentlyPlayed:m recentlyAdded:s
    go home
    shot "v-$t-shelf-start"
    local read1 read2
    read1=$(ask rect shelfPrev)
    read2=$(ask rect shelfNext)
    fact "variant $t: shelf prev '$read1' next '$read2'"
    clickrect shelfNext
    sleep 1.2
    shot "v-$t-shelf-paged"
    fact "variant $t: carouselX after next $(ask carouselX), prev '$(ask rect shelfPrev)'"
    home_shelves genreChips:m speedDial:m
    go home
    shot "v-$t-speed-dial"
    ui pref navStyle '"rail"'
    sleep 1
    hover railDestinations && shot "v-$t-rail-hover"
    ui pref navStyle '"auto"'
    go home
    clicknamed Library
    sleep 2
    local i
    for i in 1 2 3 4 5 6 7 8; do
        wtype -k Tab
        sleep 0.4
        fact "variant $t: focus after Tab $i: $(ask focusName)"
        [ "$i" -ge 3 ] && shot "v-$t-tabs-$i"
    done
}
ui pref themeMode '"light"'
sleep 1
variant light
ui pref themeMode '"dark"'
ui pref language '"ar"'
sleep 2
variant rtl
resize 700 900
variant compact-rtl
ui pref language '""'
sleep 2
variant compact
resize 1600 1000
go home

# ---- Settings, Pages, Style: real clicks, and each row keeps working after its setting changed elsewhere
ui settings pages
sleep 2.5
pick_option Plain pageHeader '"plain"' '"tint"'
pick_option "Cover tint" pageHeader '"tint"' '"plain"'
pick_option "Side by side" albumLayout '"sideBySide"' '"stacked"'
pick_option Stacked albumLayout '"stacked"' '"sideBySide"'
pick_option "Letter index" letterIndex true false
shot settings-style
ui pref letterIndex false

# ---- Collection tiles on the Playlists page, then without a server
go playlists
sleep 1.5
expect "no collection tiles by default" "$(ask namedAll Favorites | count)" 1
clicknamed "Edit page"
sleep 1.5
clicknamed "Collection tiles"
sleep 1
shot collection-edit
clicknamed Done
sleep 1.5
expect "the Favorites tile shows once Collection tiles is on" "$(ask namedAll Favorites | count)" 2
expect "the Downloads tile shows with a server library" "$(ask namedAll Downloads | count)" 1
shot collection-tiles
clicknamed Favorites 2
sleep 1.5
expect "a collection tile opens its page" "$(ask route)" "favorites/"
ui back
sleep 1.5
rpc libraries.remove "{\"library\":\"$lib\"}" >/dev/null
sleep 3
expect "the Downloads tile goes without a server" "$(ask namedAll Downloads | count)" 0
expect "the other tiles stay" "$(ask namedAll Favorites | count)" 2
clicknamed "Edit page"
sleep 1.5
clicknamed "Collection tiles"
sleep 1
clicknamed Done
sleep 1.5
expect "the tiles go when Collection tiles is off" "$(ask namedAll Favorites | count)" 1

[ "$failed" = 0 ]
