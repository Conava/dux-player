import QtQml.Models

// Rows for a view whose delegates read their item by index. A view given a new model makes its first row current,
// and that row takes the focus from any field in the view's header; this model stays and only changes its row count.
ListModel {
    id: root

    property int rows: 0
    // each row's section, for a view with sections (section.property: "group")
    property var groups: []

    function sync() {
        if (rows < count)
            remove(rows, count - rows);
        else if (rows > count)
            append(new Array(rows - count).fill({
                "group": ""
            }));
        for (let i = 0; i < Math.min(rows, groups.length); i++) {
            if (get(i).group !== groups[i])
                setProperty(i, "group", groups[i]);
        }
    }

    onRowsChanged: sync()
    onGroupsChanged: sync()
}
