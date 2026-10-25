package subsonic

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
)

// Servers disagree on JSON types: Funkwhale sends ids as numbers, older
// Ampache and Subsonic convert their XML and send numbers and booleans as
// strings and a one-element list as a bare object. These types accept every
// variant, so one odd field doesn't fail a whole response.

// str is a JSON string or number.
type str string

func (s *str) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = str(v)
		return nil
	}
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*s = str(n)
	return nil
}

// num is a JSON number or a numeric string; anything else, NaN and the
// infinities included, reads as 0.
type num float64

func (n *num) UnmarshalJSON(b []byte) error {
	s := string(bytes.Trim(b, `"`))
	f, err := strconv.ParseFloat(s, 64)
	if err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		*n = num(f)
	}
	return nil
}

func (n num) int() int { return int(n) }

// boolean is a JSON bool or "true"/"false".
type boolean bool

func (v *boolean) UnmarshalJSON(b []byte) error {
	*v = boolean(string(bytes.Trim(b, `"`)) == "true")
	return nil
}

// list is a JSON array or a single object.
type list[T any] []T

func (l *list[T]) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '[' {
		return json.Unmarshal(b, (*[]T)(l))
	}
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	var one T
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*l = list[T]{one}
	return nil
}
