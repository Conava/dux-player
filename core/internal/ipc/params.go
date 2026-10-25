package ipc

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Conava/dux-player/core/internal/library"
	"github.com/Conava/dux-player/core/internal/source"
)

// Param readers for every handler. The CLI sends every value as a string
// (a repeated key as a list), the UI sends JSON types; both are read.

func Str(p map[string]any, key string) string {
	s, _ := p[key].(string)
	return s
}

func Strs(p map[string]any, key string) []string {
	switch v := p[key].(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// Num reads a number; ok is false when the key is absent.
func Num(p map[string]any, key string) (n float64, ok bool, err error) {
	switch v := p[key].(type) {
	case nil:
		return 0, false, nil
	case float64:
		return v, true, nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, false, fmt.Errorf("%s: %q is not a number", key, v)
		}
		return f, true, nil
	}
	return 0, false, fmt.Errorf("%s: not a number", key)
}

// Int reads a whole number; def when absent or not a number.
func Int(p map[string]any, key string, def int) (int, error) {
	f, ok, err := Num(p, key)
	if err != nil || !ok {
		return def, err
	}
	return int(f), nil
}

func Ints(p map[string]any, key string) ([]int, error) {
	raw, ok := p[key].([]any)
	if !ok && p[key] != nil {
		raw = []any{p[key]}
	}
	out := make([]int, 0, len(raw))
	for _, x := range raw {
		i, err := Int(map[string]any{key: x}, key, 0)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, nil
}

// Bool reads true/false; ok is false when the key is absent or not a bool.
func Bool(p map[string]any, key string) (v, ok bool) {
	switch b := p[key].(type) {
	case bool:
		return b, true
	case string:
		v, err := strconv.ParseBool(b)
		return v, err == nil
	}
	return false, false
}

// Refs reads track references given as {"library","id"} objects or as
// "library:id" strings.
func Refs(p map[string]any, key string) ([]source.Ref, error) {
	var raw []any
	switch v := p[key].(type) {
	case nil:
		return nil, nil
	case []any:
		raw = v
	default:
		raw = []any{v}
	}
	out := make([]source.Ref, 0, len(raw))
	for _, x := range raw {
		switch v := x.(type) {
		case string:
			lib, id, ok := strings.Cut(v, ":")
			if !ok || lib == "" || id == "" {
				return nil, fmt.Errorf("%s: %q is not library:id", key, v)
			}
			out = append(out, source.Ref{Library: lib, ID: id})
		case map[string]any:
			ref := source.Ref{Library: Str(v, "library"), ID: Str(v, "id")}
			if ref.Library == "" || ref.ID == "" {
				return nil, fmt.Errorf("%s: a track needs library and id", key)
			}
			out = append(out, ref)
		default:
			return nil, fmt.Errorf("%s: unsupported track reference", key)
		}
	}
	return out, nil
}

// Reply writes v or err, for calls that return both.
func Reply(w *ConnWriter, id int) func(any, error) {
	return func(v any, err error) {
		var r library.Reason
		switch {
		case errors.As(err, &r):
			_ = w.WriteResponse(codedError{ID: id, Error: r.Message, Code: r.Code})
		case err != nil:
			w.RespondError(id, err.Error())
		default:
			w.Respond(id, v)
		}
	}
}

// codedError is an error reply with a code the UI words; Error is plain
// English.
type codedError struct {
	ID    int    `json:"id,omitempty"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

// ReplyOK answers {ok: true} or the error.
func ReplyOK(w *ConnWriter, id int, err error) {
	Reply(w, id)(map[string]bool{"ok": err == nil}, err)
}

// Capability returns a library's source as the optional interface T, or
// says the library doesn't support what.
func Capability[T any](libs *library.Manager, lib, what string) (T, error) {
	var zero T
	src, err := libs.Get(lib)
	if err != nil {
		return zero, err
	}
	c, ok := src.(T)
	if !ok {
		return zero, errors.New("library " + lib + " does not support " + what)
	}
	return c, nil
}
