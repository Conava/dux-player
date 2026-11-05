# Every main page, then the player with a track playing.
for page in home libraries history search; do
    go "$page"
    shot "$page"
done
dp play "$W/music" >/dev/null
sleep 3
go home
shot playing
expand true
shot now-playing
expand false
