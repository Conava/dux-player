.pragma library

// The letter index of a name-sorted page: which letter a value files under and where each letter first appears.

const marks = /[̀-ͯ]/g;

// The uppercased first character, "#" for nothing, digits and symbols. Only Latin letters lose their accents
// (decomposing Hangul or Cyrillic Й would split real letters); Ø and the like stay their own letter.
function letterOf(text) {
    const first = Array.from(String(text === null || text === undefined ? "" : text))[0];
    if (first === undefined)
        return "#";
    const code = first.charCodeAt(0);
    const base = code < 0x370 ? first.normalize("NFD").replace(marks, "") : first;
    const upper = base.toUpperCase();
    if (upper.length === 0)
        return "#";
    if (code < 0x80)
        return upper >= "A" && upper <= "Z" ? upper : "#";
    // cased letters, plus every script from Hebrew up (no case, but still letters)
    return upper !== base.toLowerCase() || code >= 0x590 ? upper : "#";
}

// [{letter, first}] for "#", A–Z, then other letters in list order (reversed when descending, "#" last); first is
// the index of the letter's first value in `values`, or -1.
function index(values, descending) {
    const order = ["#"];
    for (let c = 65; c <= 90; c++)
        order.push(String.fromCharCode(c));
    const firsts = {};
    for (let i = 0; i < values.length; i++) {
        const letter = letterOf(values[i]);
        if (firsts[letter] !== undefined)
            continue;
        firsts[letter] = i;
        if (order.indexOf(letter) < 0)
            order.push(letter);
    }
    const out = order.map(letter => ({
                letter: letter,
                first: firsts[letter] === undefined ? -1 : firsts[letter]
            }));
    return descending ? out.reverse() : out;
}
