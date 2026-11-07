.pragma library

// Text templates: %field%, %a|b% (first non-empty), [ section ] printed only when a field inside has a value,
// 'quoted' literals, '' for an apostrophe. A name missing from fields stays literal so a typo is visible.
// core/internal/template is the same formatter; both run testdata/cases.json there.

const maxOutput = 512;
const fieldName = /^[a-z0-9]+$/;

function has(fields, name) {
    return Object.prototype.hasOwnProperty.call(fields, name);
}

function render(s, inSection) {
    let text = "";
    let filled = false;
    while (s.at < s.template.length) {
        const c = s.template[s.at];
        if (c === "]" && inSection) {
            s.at++;
            break;
        }
        if (c === "[") {
            s.at++;
            const inner = render(s, true);
            if (inner.filled) {
                text += inner.text;
                filled = true;
            }
            continue;
        }
        const end = c === "'" || c === "%" ? s.template.indexOf(c, s.at + 1) : -1;
        if (c === "'" && end > 0) {
            text += end === s.at + 1 ? "'" : s.template.slice(s.at + 1, end);
            s.at = end + 1;
            continue;
        }
        const names = c === "%" && end > 0 ? s.template.slice(s.at + 1, end).split("|") : [];
        if (names.length > 0 && names.every(n => fieldName.test(n))) {
            const known = names.filter(n => has(s.fields, n));
            const value = known.map(n => s.fields[n]).find(v => v !== "") ?? "";
            text += known.length > 0 ? value : s.template.slice(s.at, end + 1);
            filled = filled || value !== "" || known.length === 0;
            s.at = end + 1;
            continue;
        }
        text += c;
        s.at++;
    }
    return {
        "text": text,
        "filled": filled
    };
}

// fields: {name: string}, every known name present, "" when it has no value
function format(template, fields) {
    return render({
        "template": template,
        "at": 0,
        "fields": fields
    }, false).text.slice(0, maxOutput);
}
