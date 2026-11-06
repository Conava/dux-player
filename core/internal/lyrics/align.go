package lyrics

import (
	"math"
	"slices"
	"strings"
	"unicode"

	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"
)

const (
	// minVoters is how many other sources must time the same lines before
	// their agreement moves anything.
	minVoters = 2
	// agreeShare is the share of them that must agree, exclusive: a majority.
	agreeShare = 0.5
	// agreeSlack in seconds: good sources for one song spread this far.
	agreeSlack = 0.5
	// moveSlack in seconds: a result this close to the others stays as it is.
	moveSlack = 0.35
	// maxOffset in seconds bounds the search for a line in another source,
	// so a repeated chorus isn't matched with a later one.
	maxOffset = 5.0
	// minMatches is how many lines must match for a source to vote.
	minMatches = 5
)

// choose picks the result with the best timing; between equals the one
// earlier in order wins.
func choose(order []duxlyrics.Provider, results []*duxlyrics.Result) *duxlyrics.Result {
	best := results[0]
	for _, r := range results[1:] {
		ours, theirs := tier(answer{res: best}), tier(answer{res: r})
		if theirs > ours || theirs == ours && theirs >= 0 && rank(order, r.Source) < rank(order, best.Source) {
			best = r
		}
	}
	return best
}

// align chooses the best result and moves it onto the timing the other
// sources agree on. A lyrics file of the user's own is never moved.
func align(order []duxlyrics.Provider, results []*duxlyrics.Result) *duxlyrics.Result {
	best := choose(order, results)
	if !best.Found || best.Source == duxlyrics.Sidecar || len(best.Synced) == 0 {
		return best
	}
	var offsets []float64
	for _, r := range results {
		if r.Source == best.Source {
			continue
		}
		if d, ok := offset(best.Synced, r.Synced); ok {
			offsets = append(offsets, d)
		}
	}
	shift, ok := consensus(offsets)
	if !ok {
		return best
	}
	return shifted(best, shift)
}

// offset is how far b's lines sit after a's, taken over the lines they share.
func offset(a, b []duxlyrics.Line) (float64, bool) {
	var deltas []float64
	from := 0
	for _, line := range a {
		text := folded(line.Text)
		if text == "" {
			continue
		}
		for j := from; j < len(b); j++ {
			d := float64(b[j].Start - line.Start)
			if d > maxOffset {
				break
			}
			if d >= -maxOffset && folded(b[j].Text) == text {
				deltas = append(deltas, d)
				from = j + 1
				break
			}
		}
	}
	if len(deltas) < minMatches {
		return 0, false
	}
	return median(deltas), true
}

// consensus is the shift the voters agree on, if they do and it matters.
func consensus(offsets []float64) (float64, bool) {
	if len(offsets) < minVoters {
		return 0, false
	}
	m := median(offsets)
	agree := 0
	for _, o := range offsets {
		if math.Abs(o-m) <= agreeSlack {
			agree++
		}
	}
	if float64(agree) <= agreeShare*float64(len(offsets)) || math.Abs(m) <= moveSlack {
		return 0, false
	}
	return m, true
}

func median(values []float64) float64 {
	sorted := slices.Sorted(slices.Values(values))
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func folded(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, text)
}

// shifted copies the result: a result can be shared with dux-go's lookups.
func shifted(r *duxlyrics.Result, by float64) *duxlyrics.Result {
	out := *r
	move := func(at duxlyrics.Seconds) duxlyrics.Seconds {
		return max(0, at+duxlyrics.Seconds(by))
	}
	out.Synced = slices.Clone(r.Synced)
	for i := range out.Synced {
		line := &out.Synced[i]
		line.Start, line.End = move(line.Start), move(line.End)
		line.Words = slices.Clone(line.Words)
		for j := range line.Words {
			line.Words[j].Start, line.Words[j].End = move(line.Words[j].Start), move(line.Words[j].End)
		}
	}
	return &out
}
