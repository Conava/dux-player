// Package similar picks tracks that fit a listening session: it scores
// candidates from every session track's artists, their collaborators,
// genres, shared playlists and the server's own suggestions, then draws them
// weighted at random.
package similar

import (
	"cmp"
	"context"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/Conava/dux-player/core/internal/source"
	"github.com/duxlabs-dev/dux-go/log"
)

// Profile is how far picks stray from the session; the values match the
// similarTracks setting.
type Profile string

const (
	Close       Profile = "close"
	Balanced    Profile = "balanced"
	Adventurous Profile = "adventurous"
)

// Collaborator is an artist that shares tracks or albums with the asked
// artists, Count times.
type Collaborator struct {
	ID    string
	Count int
}

// Catalog reads candidates from one library's catalog; every call is bounded
// by limit.
type Catalog interface {
	TracksByArtists(ctx context.Context, lib string, artistIDs []string, limit int) ([]source.Track, error)
	Collaborators(ctx context.Context, lib string, artistIDs []string, limit int) ([]Collaborator, error)
	TracksByGenre(ctx context.Context, lib, genre string, limit int) ([]source.Track, error)
}

// Playlist is a playlist holding some of the asked refs.
type Playlist struct {
	// Len is the playlist's full length.
	Len int
	// Seeds are the asked refs it holds.
	Seeds []source.Ref
	// Tracks are some of its tracks, at most the limit asked for.
	Tracks []source.Track
}

// Playlists returns the playlists (local or server) holding any of refs, by
// playlist id.
type Playlists interface {
	PlaylistsWith(ctx context.Context, refs []source.Ref, limit int) (map[string]Playlist, error)
}

// Server is a library's own similar-tracks call for one seed.
type Server func(ctx context.Context, seed source.Track) ([]source.Track, error)

// Input is everything Pick reads. Playlists, Server and Recent are optional.
type Input struct {
	// Seeds are the session's tracks, oldest first.
	Seeds []source.Track
	// Queued and Skip are never returned.
	Queued    map[source.Ref]bool
	Skip      func(source.Track) bool
	Catalog   Catalog
	Playlists Playlists
	Server    Server
	// ServerTimeout bounds each server call; Pick never waits longer.
	ServerTimeout time.Duration
	// Recent maps lately played tracks to when they played.
	Recent  map[source.Ref]time.Time
	Now     time.Time
	Profile Profile
	Rand    *rand.Rand
}

type tuning struct {
	temperature float64
	capShare    float64 // per-artist cap as a share of the batch
	// countSeeds counts the newest seeds' artists toward the cap in batches
	// of more than one; without it only single-track fills do. Close's small
	// cap would otherwise leave no room for the artist just played.
	countSeeds bool
	artist     float64
	collab     float64
	genre      float64
	playlist   float64
	server     float64
}

var profiles = map[Profile]tuning{
	Close:       {temperature: 0.5, capShare: 0.2, artist: 1, collab: 0.15, genre: 0.05, playlist: 0.3, server: 0.5},
	Balanced:    {temperature: 1, capShare: 0.3, countSeeds: true, artist: 1, collab: 0.5, genre: 0.2, playlist: 0.5, server: 0.8},
	Adventurous: {temperature: 2, capShare: 0.4, countSeeds: true, artist: 0.6, collab: 0.8, genre: 0.5, playlist: 0.7, server: 1},
}

const (
	queryLimit    = 300
	topArtists    = 24
	collabSeeds   = 8 // seed artists whose collaborators are asked for
	topCollabs    = 24
	topGenres     = 3
	serverSeeds   = 3
	capSeeds      = 2    // newest seeds whose artists count toward the cap
	seedHalfLife  = 10.0 // seeds back from the newest at which weight halves
	defaultServer = 3 * time.Second
)

type candidate struct {
	track source.Track
	score float64
	// related is set by any signal but genre.
	related bool
}

type picker struct {
	in    Input
	tune  tuning
	pool  map[source.Ref]*candidate
	order []source.Ref // first-seen order, so a fixed Rand draws the same
	seeds map[source.Ref]bool
	warn  map[string]bool
}

// Pick returns n tracks like the session, fewer only when candidates run
// out. Errors from the catalog, playlists or server are logged and ignored.
func Pick(ctx context.Context, in Input, n int) []source.Track {
	if n <= 0 || len(in.Seeds) == 0 || in.Catalog == nil {
		return nil
	}
	tune, ok := profiles[in.Profile]
	if !ok {
		tune = profiles[Balanced]
	}
	if in.Rand == nil {
		in.Rand = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	if in.Now.IsZero() {
		in.Now = time.Now()
	}
	p := &picker{in: in, tune: tune, pool: map[source.Ref]*candidate{}, seeds: map[source.Ref]bool{}, warn: map[string]bool{}}
	for _, s := range in.Seeds {
		p.seeds[s.Ref()] = true
	}
	weights := seedWeights(len(in.Seeds))
	server := p.startServer(ctx, weights)
	p.gatherCatalog(ctx, weights)
	p.gatherPlaylists(ctx, weights)
	server()
	return p.draw(n)
}

// seedWeights rise toward the newest seed and sum to 1, so a key's summed
// weight is its share of the session.
func seedWeights(m int) []float64 {
	w := make([]float64, m)
	total := 0.0
	for i := range w {
		w[i] = math.Pow(0.5, float64(m-1-i)/seedHalfLife)
		total += w[i]
	}
	for i := range w {
		w[i] /= total
	}
	return w
}

func (p *picker) warnOnce(what string, err error) {
	if err == nil || p.warn[what] {
		return
	}
	p.warn[what] = true
	log.Warnf("similar tracks: %s: %v", what, err)
}

func (p *picker) add(t source.Track, score float64, related bool) {
	r := t.Ref()
	if score <= 0 || t.ID == "" || p.in.Queued[r] || p.seeds[r] || t.Unavailable != "" {
		return
	}
	if p.in.Skip != nil && p.in.Skip(t) {
		return
	}
	if c := p.pool[r]; c != nil {
		c.score += score
		c.related = c.related || related
		return
	}
	p.pool[r] = &candidate{track: t, score: score, related: related}
	p.order = append(p.order, r)
}

// artistKeys are a track's artists as library-scoped keys: their ids, else
// the lower-cased name.
func artistKeys(t source.Track) []string {
	refs := t.Artists
	if len(refs) == 0 {
		refs = []source.ArtistRef{{ID: t.ArtistID, Name: t.Artist}}
	}
	var keys []string
	for _, a := range refs {
		switch {
		case a.ID != "":
			keys = append(keys, t.Library+"\x00"+a.ID)
		case strings.TrimSpace(a.Name) != "":
			keys = append(keys, t.Library+"\x00name:"+strings.ToLower(strings.TrimSpace(a.Name)))
		}
	}
	return keys
}

// weighted is a key with the summed weight of the seeds that named it.
type weighted map[string]float64

func (w weighted) top(k int) []string {
	keys := make([]string, 0, len(w))
	for key := range w {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(w[b], w[a]), strings.Compare(a, b))
	})
	return keys[:min(k, len(keys))]
}

func (p *picker) gatherCatalog(ctx context.Context, weights []float64) {
	artists := map[string]weighted{} // library -> artist id -> weight
	genres := map[string]weighted{}  // library -> lower-cased genre -> weight
	for i, s := range p.in.Seeds {
		for _, a := range s.Artists {
			if a.ID != "" {
				addTo(artists, s.Library, a.ID, weights[i])
			}
		}
		if len(s.Artists) == 0 && s.ArtistID != "" {
			addTo(artists, s.Library, s.ArtistID, weights[i])
		}
		if g := strings.ToLower(strings.TrimSpace(s.Genre)); g != "" {
			addTo(genres, s.Library, g, weights[i])
		}
	}
	for _, lib := range sortedKeys(artists) {
		p.gatherArtists(ctx, lib, artists[lib])
	}
	for _, lib := range sortedKeys(genres) {
		for _, g := range genres[lib].top(topGenres) {
			ts, err := p.in.Catalog.TracksByGenre(ctx, lib, g, queryLimit)
			p.warnOnce("tracks by genre", err)
			for _, t := range ts {
				p.add(t, p.tune.genre*genres[lib][g], false)
			}
		}
	}
}

func (p *picker) gatherArtists(ctx context.Context, lib string, artists weighted) {
	top := artists.top(topArtists)
	ts, err := p.in.Catalog.TracksByArtists(ctx, lib, top, queryLimit)
	p.warnOnce("tracks by artist", err)
	for _, t := range ts {
		p.add(t, p.tune.artist*matchWeight(t, artists), true)
	}

	collabs := weighted{}
	for _, a := range top[:min(collabSeeds, len(top))] {
		cs, err := p.in.Catalog.Collaborators(ctx, lib, []string{a}, topCollabs)
		p.warnOnce("collaborators", err)
		most := 0
		for _, c := range cs {
			most = max(most, c.Count)
		}
		for _, c := range cs {
			if c.ID != "" && c.Count > 0 && artists[c.ID] == 0 {
				collabs[c.ID] += artists[a] * float64(c.Count) / float64(most)
			}
		}
	}
	if len(collabs) == 0 {
		return
	}
	ts, err = p.in.Catalog.TracksByArtists(ctx, lib, collabs.top(topCollabs), queryLimit)
	p.warnOnce("tracks by collaborator", err)
	for _, t := range ts {
		p.add(t, p.tune.collab*matchWeight(t, collabs), true)
	}
}

// matchWeight sums the weights of t's artists found in w, so a track by
// several session artists scores higher.
func matchWeight(t source.Track, w weighted) float64 {
	sum := 0.0
	for _, a := range t.Artists {
		sum += w[a.ID]
	}
	if len(t.Artists) == 0 {
		sum = w[t.ArtistID]
	}
	return sum
}

func (p *picker) gatherPlaylists(ctx context.Context, weights []float64) {
	if p.in.Playlists == nil {
		return
	}
	seedWeight := make(map[source.Ref]float64, len(p.in.Seeds))
	refs := make([]source.Ref, 0, len(p.in.Seeds))
	for i, s := range p.in.Seeds {
		if _, ok := seedWeight[s.Ref()]; !ok {
			refs = append(refs, s.Ref())
		}
		seedWeight[s.Ref()] = weights[i]
	}
	lists, err := p.in.Playlists.PlaylistsWith(ctx, refs, queryLimit)
	p.warnOnce("playlists", err)
	for _, id := range sortedKeys(lists) {
		pl := lists[id]
		shared := 0.0
		for _, r := range pl.Seeds {
			shared += seedWeight[r]
		}
		// A large catch-all playlist says less about each of its tracks.
		score := p.tune.playlist * shared / math.Sqrt(float64(max(pl.Len, len(pl.Tracks), 1)))
		for _, t := range pl.Tracks {
			p.add(t, score, true)
		}
	}
}

// startServer asks the server about the newest seeds in the background and
// returns a func that adds what arrived within the timeout.
func (p *picker) startServer(ctx context.Context, weights []float64) func() {
	if p.in.Server == nil {
		return func() {}
	}
	timeout := p.in.ServerTimeout
	if timeout <= 0 {
		timeout = defaultServer
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	type reply struct {
		weight float64
		tracks []source.Track
		err    error
	}
	first := max(0, len(p.in.Seeds)-serverSeeds)
	replies := make(chan reply, len(p.in.Seeds)-first)
	for i := first; i < len(p.in.Seeds); i++ {
		go func(seed source.Track, w float64) {
			ts, err := p.in.Server(ctx, seed)
			replies <- reply{w, ts, err}
		}(p.in.Seeds[i], weights[i])
	}
	return func() {
		defer cancel()
		got := make([]reply, 0, cap(replies))
		for range cap(replies) {
			select {
			case r := <-replies:
				got = append(got, r)
			case <-ctx.Done():
				p.warnOnce("server", ctx.Err())
			}
			if ctx.Err() != nil {
				got = append(got, drain(replies)...)
				break
			}
		}
		// Arrival order varies; sort so a fixed Rand draws the same.
		slices.SortStableFunc(got, func(a, b reply) int { return cmp.Compare(b.weight, a.weight) })
		for _, r := range got {
			p.warnOnce("server", r.err)
			for _, t := range r.tracks {
				p.add(t, p.tune.server*r.weight, true)
			}
		}
	}
}

// draw takes n candidates weighted at random without replacement, at most
// a few per artist; the cap gives way once only capped candidates remain.
func (p *picker) draw(n int) []source.Track {
	limit := max(1, int(math.Ceil(float64(n)*p.tune.capShare)))
	now := p.in.Now
	related, genreOnly := 0, 0
	for _, c := range p.pool {
		if c.related {
			related++
		} else {
			genreOnly++
		}
	}
	// Genre widens the pool but must not become its bulk: genre-only
	// candidates count as at most half as many as the related ones.
	genreScale := 1.0
	if related > 0 && genreOnly > 0 {
		genreScale = min(1, 0.5*float64(related)/float64(genreOnly))
	}
	cands := make([]candidate, 0, len(p.order))
	for _, r := range p.order {
		c := *p.pool[r]
		if !c.related {
			c.score *= genreScale
		}
		if at, ok := p.in.Recent[r]; ok {
			c.score *= recentFactor(now.Sub(at))
		}
		c.score = math.Pow(c.score, 1/p.tune.temperature)
		cands = append(cands, c)
	}
	// The newest seeds' artists count toward the cap, so small repeated
	// fills don't keep returning to the artist that just played.
	perArtist := map[string]int{}
	seen := map[string]bool{}
	newest := p.in.Seeds[max(0, len(p.in.Seeds)-capSeeds):]
	if n > 1 && !p.tune.countSeeds {
		newest = nil
	}
	for _, s := range newest {
		for _, k := range artistKeys(s) {
			if !seen[k] {
				seen[k] = true
				perArtist[k]++
			}
		}
	}
	capped := func(c candidate) bool {
		for _, k := range artistKeys(c.track) {
			if perArtist[k] >= limit {
				return true
			}
		}
		return false
	}
	var out []source.Track
	for len(out) < n && len(cands) > 0 {
		i := p.choose(cands, capped)
		if i < 0 {
			i = p.choose(cands, func(candidate) bool { return false })
		}
		c := cands[i]
		out = append(out, c.track)
		for _, k := range artistKeys(c.track) {
			perArtist[k]++
		}
		cands = slices.Delete(cands, i, i+1)
	}
	return out
}

// choose returns a random index among the cands not excluded, weighted by
// score, or -1 when all are excluded.
func (p *picker) choose(cands []candidate, excluded func(candidate) bool) int {
	total := 0.0
	for _, c := range cands {
		if !excluded(c) {
			total += c.score
		}
	}
	if total <= 0 {
		return -1
	}
	r := p.in.Rand.Float64() * total
	last := -1
	for i, c := range cands {
		if excluded(c) {
			continue
		}
		last = i
		if r -= c.score; r < 0 {
			return i
		}
	}
	return last
}

// drain returns the replies already waiting without blocking.
func drain[T any](ch <-chan T) []T {
	var out []T
	for {
		select {
		case v := <-ch:
			out = append(out, v)
		default:
			return out
		}
	}
}

// recentFactor down-weights a track played age ago.
func recentFactor(age time.Duration) float64 {
	switch {
	case age < 24*time.Hour:
		return 0.2
	case age < 7*24*time.Hour:
		return 0.5
	default:
		return 1
	}
}

func addTo(m map[string]weighted, lib, key string, w float64) {
	if m[lib] == nil {
		m[lib] = weighted{}
	}
	m[lib][key] += w
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// maxRounds bounds TopUp against a feed that keeps returning duplicates.
const maxRounds = 8

// TopUp calls next until it has n tracks that are not queued, skipped or
// repeated, or next returns none. It returns what it has with next's error.
func TopUp(ctx context.Context, n int, next func(ctx context.Context, k int) ([]source.Track, error), queued map[source.Ref]bool, skip func(source.Track) bool) ([]source.Track, error) {
	var out []source.Track
	seen := map[source.Ref]bool{}
	for round := 0; round < maxRounds && len(out) < n; round++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		ts, err := next(ctx, n) // a full batch each round, so duplicates cost few rounds
		if err != nil {
			return out, err
		}
		if len(ts) == 0 {
			break
		}
		for _, t := range ts {
			r := t.Ref()
			if len(out) == n || queued[r] || seen[r] || (skip != nil && skip(t)) {
				continue
			}
			seen[r] = true
			out = append(out, t)
		}
	}
	return out, nil
}
