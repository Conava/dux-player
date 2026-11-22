.pragma library

// The letter index of a name-sorted page: which letter a value files under and where each letter first appears.

const marks = /[̀-ͯ]/g;

// punctuation and symbols, CJK punctuation, variation selectors and the emoji planes: caseless, but not letters
function isSymbol(cp) {
    return (cp >= 0x2000 && cp <= 0x2bff) || (cp >= 0x3000 && cp <= 0x303f) || (cp >= 0xfe00 && cp <= 0xfe0f) || (cp >= 0x1f000 && cp <= 0x1faff);
}

// The uppercased first character, "#" for nothing, digits and symbols. Only Latin letters lose their accents
// (decomposing Hangul or Cyrillic Й would split real letters); Ø and the like stay their own letter.
function letterOf(text) {
    const s = String(text === null || text === undefined ? "" : text);
    const first = s === "" ? undefined : String.fromCodePoint(s.codePointAt(0));
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
    return upper !== base.toLowerCase() || (code >= 0x590 && !isSymbol(first.codePointAt(0))) ? upper : "#";
}

// [{letter, first}] for "#", A–Z, then other letters in list order; descending reverses all of it, "#" last. first is
// the index of the letter's first value in `values`, or -1.
function index(values, descending) {
    const order = ["#"];
    for (let c = 65; c <= 90; c++)
        order.push(String.fromCharCode(c));
    const firsts = {};
    const others = [];
    for (let i = 0; i < values.length; i++) {
        const letter = letterOf(values[i]);
        if (firsts[letter] !== undefined)
            continue;
        firsts[letter] = i;
        if (order.indexOf(letter) < 0)
            others.push(letter);
    }
    // a descending list meets the other letters last to first; the reversal below puts them back
    const out = order.concat(descending ? others.reverse() : others).map(letter => ({
                letter: letter,
                first: firsts[letter] === undefined ? -1 : firsts[letter]
            }));
    return descending ? out.reverse() : out;
}
