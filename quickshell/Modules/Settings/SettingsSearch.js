.pragma library

// Best match first: the title (exact, prefix, word start, anywhere); then entries holding every word of the query
// in their title, keywords, section and page; then in their description and option labels too; then the same
// with word endings and small typos forgiven. Ties put hidden entries last, then keep the index order.
function search(entries, query) {
    const q = normalize(query);
    if (q === "")
        return [];
    const words = q.split(" ");
    const scored = [];
    for (let i = 0; i < entries.length; i++) {
        const score = scoreOf(entries[i], q, words);
        if (score > 0)
            scored.push({
                "entry": entries[i],
                "score": score,
                "order": i
            });
    }
    scored.sort((a, b) => b.score - a.score || (a.entry.hidden === true) - (b.entry.hidden === true) || a.order - b.order);
    return scored.map(s => s.entry);
}

// The index entries updated with the rows read from the pages ({page, title, hidden, reason, subView}), matched by
// page and title; rows the index lacks are added. Copies of a row give one entry per sub-view they are shown on,
// or one hidden entry when none is shown.
function merge(entries, live) {
    const groups = {};
    const keys = [];
    for (let i = 0; i < live.length; i++) {
        const key = live[i].page + "\n" + live[i].title;
        if (!groups[key]) {
            groups[key] = [];
            keys.push(key);
        }
        groups[key].push(live[i]);
    }
    for (let i = 0; i < keys.length; i++) {
        const copies = groups[keys[i]];
        const shown = copies.filter((e, j) => !e.hidden && copies.findIndex(o => !o.hidden && o.subView === e.subView) === j);
        groups[keys[i]] = shown.length > 0 ? shown : [copies.find(e => e.reason) ?? copies[0]];
    }
    const out = [];
    for (let i = 0; i < entries.length; i++) {
        const key = entries[i].page + "\n" + entries[i].title;
        const copies = entries[i].isPage ? null : groups[key];
        if (!copies) {
            out.push(entries[i]);
            continue;
        }
        delete groups[key];
        for (let j = 0; j < copies.length; j++)
            out.push(Object.assign({}, entries[i], {
                "section": entries[i].section || copies[j].section,
                "hidden": copies[j].hidden,
                "reason": copies[j].reason,
                "subView": copies[j].subView
            }));
    }
    for (let i = 0; i < keys.length; i++)
        out.push(...(groups[keys[i]] ?? []));
    return out;
}

function normalize(text) {
    return String(text ?? "").toLowerCase().replace(/[\s.,:;!?()"'…·\/+-]+/g, " ").trim();
}

function titleScore(title, q) {
    if (title === q)
        return 7;
    if (title.startsWith(q))
        return 6;
    if ((" " + title).includes(" " + q))
        return 5;
    return title.includes(q) ? 4 : 0;
}

function scoreOf(entry, q, words) {
    const title = normalize(entry.title);
    const byTitle = titleScore(title, q);
    if (byTitle > 0)
        return byTitle;
    const strong = [title, normalize(entry.keywords), normalize(entry.section), normalize(entry.pageLabel)].join(" ");
    if (words.every(w => strong.includes(w)))
        return 3;
    const all = strong + " " + normalize(entry.text);
    if (words.every(w => all.includes(w)))
        return 2;
    const known = all.split(" ");
    return words.every(w => known.some(k => similar(w, k))) ? 1 : 0;
}

// "transitions" finds "transition", "crosfade" finds "crossfade"
function similar(word, known) {
    const a = stem(word);
    const b = stem(known);
    if (a.length < 3)
        return b.startsWith(a);
    if (b.startsWith(a))
        return true;
    // a typo rarely hits the first letter, and short words are too close to each other to forgive one
    const allowed = a.length >= 8 ? 2 : a.length >= 5 ? 1 : 0;
    return allowed > 0 && a[0] === b[0] && (distance(a, b) <= allowed || distance(a, b.slice(0, a.length)) <= allowed);
}

function stem(word) {
    const m = word.match(/^(.{3,}?)(?:ing|ed|es|s)$/);
    return m ? m[1] : word;
}

// edit distance with transpositions, given up past 2
function distance(a, b) {
    if (Math.abs(a.length - b.length) > 2)
        return 3;
    let before = [];
    let previous = [];
    for (let j = 0; j <= b.length; j++)
        previous.push(j);
    for (let i = 1; i <= a.length; i++) {
        const row = [i];
        for (let j = 1; j <= b.length; j++) {
            const cost = a[i - 1] === b[j - 1] ? 0 : 1;
            let d = Math.min(previous[j] + 1, row[j - 1] + 1, previous[j - 1] + cost);
            if (i > 1 && j > 1 && a[i - 1] === b[j - 2] && a[i - 2] === b[j - 1])
                d = Math.min(d, before[j - 2] + 1);
            row.push(d);
        }
        before = previous;
        previous = row;
    }
    return previous[b.length];
}
