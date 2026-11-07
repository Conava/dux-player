# Shrinking the column list must not make a doomed cell assign undefined; run.sh's error grep fails the run on that warning.
go tracks
sleep 1
ui pageOptions tracks '{"lines":["artist","album","genre"]}'
sleep 1
ui pageOptions tracks '{"lines":["artist"]}'
sleep 1
ui pageOptions tracks '{"lines":[]}'
sleep 1
shot columns-empty
ui pageOptions tracks '{"lines":["artist","album","genre"]}'
sleep 1
shot columns-restored

