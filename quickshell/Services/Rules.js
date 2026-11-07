.pragma library

// Filter rules of a browse list: the fields and conditions of each type, and the test they compile to. No QML
// here, so tests/rules.test.js runs them under node. Stored form: {match: "all"|"any", list: [{field, op, value}]}.
// A rule without a value, or one this version doesn't know, does not filter.

const conditions = {
    "text": ["contains", "notContains", "is", "isNot", "startsWith", "isEmpty", "isNotEmpty"],
    "pick": ["isAny", "isNone", "contains"],
    "number": ["is", "isNot", "atLeast", "atMost", "between"],
    "date": ["inLast", "notInLast"],
    "flag": ["is"]
};

// the order is the order of the field dropdown
const fieldTables = {
    "tracks": {
        "title": "text",
        "artist": "text",
        "album": "text",
        "albumArtist": "text",
        "composer": "text",
        "genre": "pick",
        "library": "pick",
        "format": "pick",
        "year": "number",
        "duration": "number",
        "rating": "number",
        "playCount": "number",
        "bitrate": "number",
        "added": "date",
        "lastPlayed": "date",
        "favorite": "flag"
    },
    "albums": {
        "title": "text",
        "artist": "text",
        "genre": "pick",
        "library": "pick",
        "year": "number",
        "duration": "number",
        "rating": "number",
        "playCount": "number",
        "trackCount": "number",
        "added": "date",
        "lastPlayed": "date",
        "favorite": "flag"
    },
    "artists": {
        "name": "text",
        "genre": "pick",
        "library": "pick",
        "rating": "number",
        "albumCount": "number",
        "trackCount": "number",
        "favorite": "flag"
    }
};

// cheap checks run first, so a match-all list leaves most items before reaching a text search
const costs = {
    "flag": 0,
    "number": 1,
    "date": 1,
    "pick": 2,
    "text": 3
};

const daySeconds = 86400;
const minuteSeconds = 60;

function table(type) {
    return Object.prototype.hasOwnProperty.call(fieldTables, type) ? fieldTables[type] : {};
}

function fieldKind(type, field) {
    const fields = table(type);
    return Object.prototype.hasOwnProperty.call(fields, field) ? fields[field] : "";
}

function fields(type) {
    return Object.keys(table(type));
}

function ops(type, field) {
    const kind = fieldKind(type, field);
    if (kind === "")
        return [];
    if (field === "lastPlayed")
        return conditions.date.concat(["never"]);
    // a library rule holds ids, which a text search can't mean
    if (field === "library")
        return conditions.pick.filter(o => o !== "contains");
    return conditions[kind];
}

// What value a rule takes: "none", "text", "list", "number", "range", "days" or "flag"; "" for a rule this
// version doesn't know.
function shape(type, rule) {
    if (ops(type, rule?.field).indexOf(rule?.op) < 0)
        return "";
    switch (fieldKind(type, rule.field)) {
    case "text":
        return rule.op === "isEmpty" || rule.op === "isNotEmpty" ? "none" : "text";
    case "pick":
        return rule.op === "contains" ? "text" : "list";
    case "number":
        return rule.op === "between" ? "range" : "number";
    case "date":
        return rule.op === "never" ? "none" : "days";
    }
    return "flag";
}

function finite(v) {
    return typeof v === "number" && isFinite(v);
}

function hasValue(kind, value) {
    switch (kind) {
    case "none":
        return true;
    case "text":
        return typeof value === "string" && value.trim() !== "";
    case "list":
        return Array.isArray(value) && value.length > 0 && value.every(v => typeof v === "string");
    case "number":
        return finite(value);
    case "range":
        return Array.isArray(value) && value.length === 2 && value.every(finite);
    case "days":
        return finite(value) && value > 0;
    case "flag":
        return typeof value === "boolean";
    }
    return false;
}

// the rules that filter
function active(rules, type) {
    const list = rules?.list;
    return Array.isArray(list) ? list.filter(r => hasValue(shape(type, r), r?.value)) : [];
}

function uses(rules, type, field) {
    return active(rules, type).some(r => r.field === field);
}

// everything the rules let through is a favorite, so the favorites list holds all of it
function onlyFavorites(rules, type) {
    const list = active(rules, type);
    return (rules?.match !== "any" || list.length === 1) && list.some(r => r.field === "favorite" && r.value === true);
}

function newRule(type) {
    const field = fields(type)[0];
    return {
        "field": field,
        "op": ops(type, field)[0]
    };
}

// a rule's value survives a change only while it still means the same kind of thing
function keepValue(type, before, after) {
    if (before.value !== undefined && shape(type, before) === shape(type, after))
        after.value = before.value;
    return after;
}

function withField(type, rule, field) {
    const list = ops(type, field);
    return keepValue(type, rule, {
        "field": field,
        "op": list.indexOf(rule.op) >= 0 ? rule.op : list[0]
    });
}

function withOp(type, rule, op) {
    return keepValue(type, rule, {
        "field": rule.field,
        "op": op
    });
}

// the item's own field: an artist holds lists, gathered from its albums
function itemKey(type, field) {
    if (type !== "artists")
        return field;
    return field === "genre" ? "genres" : field === "library" ? "libraries" : field;
}

function textTest(key, op, value) {
    const needle = typeof value === "string" ? value.trim().toLowerCase() : "";
    const text = i => (i[key] ?? "").toLowerCase();
    switch (op) {
    case "contains":
        return i => text(i).indexOf(needle) >= 0;
    case "notContains":
        return i => text(i).indexOf(needle) < 0;
    case "is":
        return i => text(i) === needle;
    case "isNot":
        return i => text(i) !== needle;
    case "startsWith":
        return i => text(i).startsWith(needle);
    case "isEmpty":
        return i => (i[key] ?? "") === "";
    }
    return i => (i[key] ?? "") !== "";
}

// a library holds few distinct values, so each is judged once
function pickTest(key, op, value) {
    const wanted = op === "contains" ? null : new Set(value.map(v => v.toLowerCase()));
    const needle = op === "contains" ? value.trim().toLowerCase() : "";
    const judged = new Map();
    const hit = s => {
        let known = judged.get(s);
        if (known === undefined) {
            const lower = String(s).toLowerCase();
            known = wanted ? wanted.has(lower) : lower.indexOf(needle) >= 0;
            judged.set(s, known);
        }
        return known;
    };
    const holds = i => {
        const v = i[key] ?? "";
        if (!Array.isArray(v))
            return hit(v);
        for (let n = 0; n < v.length; n++) {
            if (hit(v[n]))
                return true;
        }
        return false;
    };
    return op === "isNone" ? i => !holds(i) : holds;
}

function numberTest(key, field, op, value) {
    // duration is entered in minutes and held in seconds; "is 3" means any length that shows as 3 minutes
    const scale = field === "duration" ? minuteSeconds : 1;
    const whole = field === "duration" ? i => Math.floor((i[key] ?? 0) / minuteSeconds) : i => i[key] ?? 0;
    switch (op) {
    case "is":
        return i => whole(i) === value;
    case "isNot":
        return i => whole(i) !== value;
    case "atLeast":
        {
            const low = value * scale;
            return i => (i[key] ?? 0) >= low;
        }
    case "atMost":
        {
            const high = value * scale;
            return i => (i[key] ?? 0) <= high;
        }
    }
    const low = Math.min(value[0], value[1]) * scale;
    const high = Math.max(value[0], value[1]) * scale;
    return i => {
        const v = i[key] ?? 0;
        return v >= low && v <= high;
    };
}

function dateTest(key, field, op, value, now) {
    const cut = now - value * daySeconds;
    switch (op) {
    case "inLast":
        return i => {
            const v = i[key] ?? 0;
            return v > 0 && v >= cut;
        };
    case "never":
        return i => !((i[key] ?? 0) > 0);
    }
    // never played is not played lately; an unknown date added says nothing either way
    if (field === "lastPlayed")
        return i => (i[key] ?? 0) < cut;
    return i => {
        const v = i[key] ?? 0;
        return v > 0 && v < cut;
    };
}

function ruleTest(type, rule, now) {
    const key = itemKey(type, rule.field);
    switch (fieldKind(type, rule.field)) {
    case "text":
        return textTest(key, rule.op, rule.value);
    case "pick":
        return pickTest(key, rule.op, rule.value);
    case "number":
        return numberTest(key, rule.field, rule.op, rule.value);
    case "date":
        return dateTest(key, rule.field, rule.op, rule.value, now);
    }
    return i => (i[key] === true) === rule.value;
}

// A list rule's picked values; a scalar from a hand-edited file counts as none.
function picked(value) {
    return Array.isArray(value) ? value : [];
}

// One test for a whole list, or null when no rule filters. now: unix seconds. Everything that doesn't depend
// on the item is prepared here, once per query.
function compile(rules, type, now) {
    const list = active(rules, type).sort((a, b) => costs[fieldKind(type, a.field)] - costs[fieldKind(type, b.field)]);
    if (list.length === 0)
        return null;
    const tests = list.map(r => ruleTest(type, r, now));
    const count = tests.length;
    if (count === 1)
        return tests[0];
    if (rules.match === "any")
        return i => {
            for (let n = 0; n < count; n++) {
                if (tests[n](i))
                    return true;
            }
            return false;
        };
    return i => {
        for (let n = 0; n < count; n++) {
            if (!tests[n](i))
                return false;
        }
        return true;
    };
}
