package lyrics

import (
	"context"
	"math"
	"slices"
	"strings"
	"unicode"

	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"
)

// attempts are the lookups for req in turn: as asked, then without the
// length so a single edit still finds the album cut's lyrics, then the cache
// entries stored while lookups still carried the album; tracks kept for
// offline use rely on them.
func attempts(req duxlyrics.Request, album string) []duxlyrics.Request {
	out := []duxlyrics.Request{req}
	if req.Duration > 0 {
		short := req
		short.Duration = 0
		out = append(out, short)
	}
	if album != "" {
		old := req
		old.Album, old.CacheOnly = album, true
		out = append(out, old)
	}
	return out
}

// find tries the attempts until one has synced lyrics and keeps the best
// answer. With words, line timing is asked again without the length, as KuGou
// drops a cut whose length is off yet may time its words. After a failed first
// lookup only the cache is asked, so a flaky network isn't waited out again.
// With words, a better answer found only without the length comes back apart
// as loose: it may be another cut's lyrics.
func find(ctx context.Context, client *duxlyrics.Client, req duxlyrics.Request, album string, words bool) (*duxlyrics.Result, *duxlyrics.Result, error) {
	var best, other answer
	for i, r := range attempts(req, album) {
		if i > 0 && best.err != nil && !r.CacheOnly {
			continue
		}
		res, err := client.Lookup(ctx, r)
		a := answer{res, err}
		switch {
		case words && i > 0 && r.Duration == 0 && req.Duration > 0:
			other = a
		case i == 0 || tier(a) > tier(best):
			best = a
		}
		retry := words && i == 0 && req.Duration > 0 && tier(best) == 1
		if max(tier(best), tier(other)) >= 1 && !retry || ctx.Err() != nil {
			break
		}
	}
	if tier(other) > tier(best) {
		return best.res, other.res, best.err
	}
	return best.res, nil, best.err
}

type answer struct {
	res *duxlyrics.Result
	err error
}

// tier ranks a result as dux-go does: word sync 2, line sync (or
// instrumental) 1, plain 0, nothing -1.
func tier(a answer) int {
	switch {
	case a.err != nil || a.res == nil || !a.res.Found:
		return -1
	case slices.ContainsFunc(a.res.Synced, func(l duxlyrics.Line) bool { return len(l.Words) > 0 }):
		return 2
	case len(a.res.Synced) > 0 || a.res.Instrumental:
		return 1
	}
	return 0
}

// rank is p's place in order; a sidecar file comes before everything.
func rank(order []duxlyrics.Provider, p duxlyrics.Provider) int {
	if p == duxlyrics.Sidecar {
		return -1
	}
	if i := slices.Index(order, p); i >= 0 {
		return i
	}
	return len(order)
}

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
func align(order []duxlyrics.Provider, results []*duxlyrics.Result) outcome {
	best := choose(order, results)
	if !best.Found || best.Source == duxlyrics.Sidecar || len(best.Synced) == 0 {
		return kept(best)
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
		return kept(best)
	}
	return moved(best, shift)
}

// offset is how far b's lines sit after a's, taken over the lines they share.
func offset(a, b []duxlyrics.Line) (float64, bool) {
	matched := pairs(a, b, 0, maxOffset)
	if len(matched) < minMatches {
		return 0, false
	}
	return median(lateness(matched)), true
}

// pair is a line of b with the same text as a line of a, late by late seconds.
type pair struct {
	at   int
	late float64
}

// pairs matches b's lines to a's by text, in order, within reach seconds of
// around.
func pairs(a, b []duxlyrics.Line, around, reach float64) []pair {
	var out []pair
	from := 0
	for _, line := range a {
		text := folded(line.Text)
		if text == "" {
			continue
		}
		for j := from; j < len(b); j++ {
			d := float64(b[j].Start - line.Start)
			if d-around > reach {
				break
			}
			if d-around >= -reach && folded(b[j].Text) == text {
				out = append(out, pair{j, d})
				from = j + 1
				break
			}
		}
	}
	return out
}

func lateness(matched []pair) []float64 {
	out := make([]float64, len(matched))
	for i, p := range matched {
		out[i] = p.late
	}
	return out
}

const (
	// anchorReach in seconds: how far a word source can sit off a line
	// source of the same song; KuGou has been seen 7 s late.
	anchorReach = 10.0
	// anchorSlack in seconds: how far one line strays from the song's offset.
	// Further out it is a repeated line paired with the wrong one.
	anchorSlack = 2.0
	// anchorShare of the word source's sung lines must match. Sources of one
	// song matched 0.15 to 0.95 (they split lines differently), different
	// songs at most 0.02.
	anchorShare = 0.25
	// anchorPause in seconds between two line starts is an instrumental.
	anchorPause = 15.0
)

// between is how late line i of src is, between two matched lines. A pause
// longer than anchorPause splits them: the sources can differ in how long an
// instrumental lasts, and that belongs to neither side.
func between(src []duxlyrics.Line, before, after pair, i int) float64 {
	for j := before.at + 1; j <= after.at; j++ {
		if float64(src[j].Start-src[j-1].Start) > anchorPause {
			if i < j {
				return before.late
			}
			return after.late
		}
	}
	span := float64(src[after.at].Start - src[before.at].Start)
	if span <= 0 {
		return before.late
	}
	share := float64(src[i].Start-src[before.at].Start) / span
	return before.late + share*(after.late-before.late)
}

// anchored copies words onto the line timing of lines. Each matched line moves
// by its own difference, so drift goes and the rhythm within a line stays; the
// others move as their matched neighbours do. False when too few lines match.
func anchored(words, lines *duxlyrics.Result) (*duxlyrics.Result, bool) {
	rough := pairs(lines.Synced, words.Synced, 0, anchorReach)
	if len(rough) < minMatches {
		return nil, false
	}
	matched := pairs(lines.Synced, words.Synced, median(lateness(rough)), anchorSlack)
	sung := 0
	for _, l := range words.Synced {
		if folded(l.Text) != "" {
			sung++
		}
	}
	if len(matched) < minMatches || float64(len(matched)) < anchorShare*float64(sung) {
		return nil, false
	}
	src := words.Synced
	moves := make([]float64, len(src))
	next := 0
	for i := range moves {
		for next < len(matched) && matched[next].at < i {
			next++
		}
		switch {
		case next < len(matched) && matched[next].at == i:
			moves[i] = -matched[next].late
		case next == 0:
			moves[i] = -matched[0].late
		case next == len(matched):
			moves[i] = -matched[next-1].late
		default:
			moves[i] = -between(src, matched[next-1], matched[next], i)
		}
	}
	out := *words
	out.Source = lines.Source
	out.Synced = slices.Clone(src)
	for i := range out.Synced {
		line := &out.Synced[i]
		move := func(at duxlyrics.Seconds) duxlyrics.Seconds {
			at = max(0, at+duxlyrics.Seconds(moves[i]))
			if i+1 < len(src) {
				at = min(at, max(0, src[i+1].Start+duxlyrics.Seconds(moves[i+1])))
			}
			return at
		}
		line.Start, line.End = move(line.Start), move(line.End)
		line.Words = slices.Clone(line.Words)
		for j := range line.Words {
			line.Words[j].Start, line.Words[j].End = move(line.Words[j].Start), move(line.Words[j].End)
		}
	}
	return &out, true
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
