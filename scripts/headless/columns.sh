# Shrinking the column list must not make a doomed cell assign undefined (run.sh's error grep misses this warning's format).
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

# run.sh's error grep misses "File.qml[line:col]: Unable to assign", so read the log here.
if qs log -p "$W/tree" 2>&1 | grep -q "Unable to assign"; then
    echo "FAIL columns: Unable to assign" | tee -a "$W/out/facts.txt"
    exit 1
fi
