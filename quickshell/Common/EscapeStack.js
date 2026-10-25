.pragma library

// What Escape closes, in the order it was activated: the last one is on top.

function raise(stack, layer) {
    return stack.filter(l => l !== layer).concat([layer]);
}

// the same stack when layer isn't in it
function drop(stack, layer) {
    return stack.indexOf(layer) < 0 ? stack : stack.filter(l => l !== layer);
}

// the topmost layer that passes accept, or null
function top(stack, accept) {
    for (let i = stack.length - 1; i >= 0; i--) {
        if (accept(stack[i]))
            return stack[i];
    }
    return null;
}
