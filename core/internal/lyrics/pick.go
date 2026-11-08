package lyrics

import (
	"math"
	"slices"

	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"
)

// The lyrics shown for a track, from what the sources answered:
//
//  1. A synced answer whose timing lands on the recording, measured within
//     onTimeSlack, wins: word timing first, then the provider order. An unsure
//     measure counts here, as a source off by seconds rarely lands near 0 by
//     chance. An answer found only without the track's length competes here
//     and nowhere else, unless nothing found with the length is synced.
//  2. Otherwise the best-timed answer, earliest in the order, is moved by the
//     first sure measure that speaks for it: its words, its lines, or the lines
//     of another answer it shares lines with. Without one it moves onto the
//     timing the other answers agree on.
//  3. A line-timed winner of step 1 takes the words of a word-timed answer laid
//     on its lines, when those land on the recording too.
//
// Only sure measures move anything. A lyrics file beside the track is never
// measured or moved, and without a recording only the agreement of step 2
// moves the pick.

// onTimeSlack in seconds: an answer measured this close to 0 sits on the
// recording. Measures of answers that match a song spread about this far.
const onTimeSlack = 0.35

// fitResult is a measured shift in seconds and whether the measure is sure.
type fitResult struct {
	Shift     float64 `json:"shift"`
	Confident bool    `json:"confident"`
}

// recording measures answers against the track's audio. Each reports false
// when the answer can't be measured, always for a lyrics file beside the track.
type recording interface {
	// measure fits r's word starts, or its line starts.
	measure(r *duxlyrics.Result, words bool) (fitResult, bool)
	// measureLaid fits the words of laid, which came from words and were laid
	// on another answer's lines.
	measureLaid(words, laid *duxlyrics.Result) (fitResult, bool)
}

// decide picks what to show from the answers found with the track's length
// and the loose ones found only without it, and notes what it found and did
// for each answer. rec is nil when the recording can't be measured.
func decide(order []duxlyrics.Provider, results, loose []*duxlyrics.Result, rec recording) (*duxlyrics.Result, []note) {
	if !slices.ContainsFunc(results, func(r *duxlyrics.Result) bool { return tier(answer{res: r}) >= 1 }) {
		results, loose = append(results, loose...), nil
	}
	all := append(slices.Clip(results), loose...)
	fits := map[*duxlyrics.Result]fitResult{}
	var o outcome
	if rec == nil || choose(order, results).Source == duxlyrics.Sidecar {
		o = align(order, results)
	} else {
		o = measured(order, results, all, measures{rec, fits})
	}
	notes := make([]note, len(all))
	for i, r := range all {
		notes[i].source = r.Source
		notes[i].fit, notes[i].measured = fits[r]
		if r == o.from {
			notes[i].shown, notes[i].moved = true, o.moved
			if o.lines != nil {
				notes[i].lines = o.lines.Source
			}
		}
	}
	return o.res, notes
}

func measured(order []duxlyrics.Provider, results, all []*duxlyrics.Result, rec recording) outcome {
	on, ok := onTime(order, all, rec)
	if !ok {
		return fallback(order, results, rec)
	}
	if laid, ok := lend(order, on.res, all, rec); ok {
		return laid
	}
	return on
}

// note is what deciding found for one answer and did with it.
type note struct {
	source   duxlyrics.Provider
	fit      fitResult
	measured bool
	shown    bool
	moved    float64            // seconds the shown answer was moved
	lines    duxlyrics.Provider // whose line timing the shown words were laid on
}

// outcome is what is shown: res, made from the answer from, moved by moved
// seconds, with from's words laid on the line timing of lines when set.
type outcome struct {
	res, from, lines *duxlyrics.Result
	moved            float64
}

func kept(r *duxlyrics.Result) outcome { return outcome{res: r, from: r} }

func moved(r *duxlyrics.Result, by float64) outcome {
	return outcome{res: shifted(r, by), from: r, moved: by}
}

// measures keeps the first measure of each answer.
type measures struct {
	recording
	fits map[*duxlyrics.Result]fitResult
}

func (m measures) measure(r *duxlyrics.Result, words bool) (fitResult, bool) {
	f, ok := m.recording.measure(r, words)
	if _, seen := m.fits[r]; ok && !seen {
		m.fits[r] = f
	}
	return f, ok
}

// onTime is the answer that lands on the recording, with its sure shift applied.
func onTime(order []duxlyrics.Provider, all []*duxlyrics.Result, rec recording) (outcome, bool) {
	var on []*duxlyrics.Result
	fits := map[*duxlyrics.Result]fitResult{}
	for _, r := range all {
		if f, ok := rec.measure(r, tier(answer{res: r}) == 2); ok && math.Abs(f.Shift) <= onTimeSlack {
			on = append(on, r)
			fits[r] = f
		}
	}
	if len(on) == 0 {
		return outcome{}, false
	}
	r := choose(order, on)
	return applied(r, fits[r]), true
}

// fallback is the best-timed answer, moved by the first sure measure that
// speaks for it, else onto what the others agree on.
func fallback(order []duxlyrics.Provider, results []*duxlyrics.Result, rec recording) outcome {
	top := choose(order, results)
	if f, ok := rec.measure(top, true); ok && f.Confident {
		return moved(top, f.Shift)
	}
	// A line-timed answer can be measured where a word-timed top can't, and
	// offset carries its shift over.
	others := slices.DeleteFunc(slices.Clone(results), func(r *duxlyrics.Result) bool { return r == top })
	for _, r := range append([]*duxlyrics.Result{top}, others...) {
		f, ok := rec.measure(r, false)
		if !ok || !f.Confident {
			continue
		}
		d, ok := 0.0, true
		if r != top {
			d, ok = offset(top.Synced, r.Synced)
		}
		if ok {
			return moved(top, d+f.Shift)
		}
	}
	return align(order, results)
}

// lend lays the words of a word-timed answer, in provider order, on a
// line-timed pick and keeps the first copy that lands on the recording.
// Line starts often sit a little before the singing, so a sure shift of the
// copy is applied.
func lend(order []duxlyrics.Provider, lines *duxlyrics.Result, all []*duxlyrics.Result, rec recording) (outcome, bool) {
	if tier(answer{res: lines}) != 1 {
		return outcome{}, false
	}
	words := slices.DeleteFunc(slices.Clone(all), func(r *duxlyrics.Result) bool { return tier(answer{res: r}) != 2 })
	slices.SortStableFunc(words, func(a, b *duxlyrics.Result) int { return rank(order, a.Source) - rank(order, b.Source) })
	for _, w := range words {
		laid, ok := anchored(w, lines)
		if !ok {
			continue
		}
		if f, ok := rec.measureLaid(w, laid); ok && math.Abs(f.Shift) <= onTimeSlack {
			o := applied(laid, f)
			o.from, o.lines = w, lines
			return o, true
		}
	}
	return outcome{}, false
}

func applied(r *duxlyrics.Result, f fitResult) outcome {
	if !f.Confident {
		return kept(r)
	}
	return moved(r, f.Shift)
}
