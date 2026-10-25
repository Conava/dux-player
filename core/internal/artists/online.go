package artists

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Conava/dux-player/core/internal/atomicfile"
	"github.com/Conava/dux-player/core/internal/source"
)

// How long an online answer is kept: found, and not found.
const (
	foundTTL    = 30 * 24 * time.Hour
	notFoundTTL = 7 * 24 * time.Hour
	callTimeout = 10 * time.Second
	maxJSON     = 2 << 20
	// lookups bounds the online lookups running at once.
	lookups = 2
	// musicBrainzGap is MusicBrainz' rate limit: one request per second.
	musicBrainzGap = time.Second
)

// Endpoints are the services' base URLs; tests point them at a fake.
type Endpoints struct {
	MusicBrainz string
	Wikidata    string
	// Wikipedia gets the language code ("en").
	Wikipedia func(lang string) string
	Commons   string
	FanartTV  string
	LastFM    string
}

var DefaultEndpoints = Endpoints{
	MusicBrainz: "https://musicbrainz.org",
	Wikidata:    "https://www.wikidata.org",
	Wikipedia:   func(lang string) string { return "https://" + lang + ".wikipedia.org" },
	Commons:     "https://commons.wikimedia.org",
	FanartTV:    "https://webservice.fanart.tv",
	LastFM:      "https://ws.audioscrobbler.com",
}

// entry is what the online services know about one artist name.
type entry struct {
	MBID        string       `json:"mbid,omitempty"`
	Wikidata    string       `json:"wikidata,omitempty"`
	Image       string       `json:"image,omitempty"` // file name in the cache dir
	ImageSource string       `json:"imageSource,omitempty"`
	ImageAt     int64        `json:"imageAt,omitempty"`
	Bio         string       `json:"bio,omitempty"`
	BioSource   *source.Link `json:"bioSource,omitempty"`
	BioAt       int64        `json:"bioAt,omitempty"`
}

func fresh(at int64, found bool, now time.Time) bool {
	ttl := notFoundTTL
	if found {
		ttl = foundTTL
	}
	return at > 0 && now.Sub(time.Unix(at, 0)) < ttl
}

// online looks artists up on the web. Answers, found or not, are cached on
// disk; a failed request (offline, rate limited) is not, so it is asked
// again next time.
type online struct {
	dir   string
	ep    Endpoints
	http  *http.Client
	agent string
	lang  string
	keys  func(provider string) string
	now   func() time.Time

	sem chan struct{}
	mu  sync.Mutex
	// inflight has one channel per artist being looked up; others wait.
	inflight map[string]chan struct{}
	mbNext   time.Time
	mbGap    time.Duration
}

func newOnline(dir, version, lang string, keys func(string) string) *online {
	if version == "" {
		version = "dev"
	}
	return &online{dir: dir, ep: DefaultEndpoints, http: &http.Client{Timeout: callTimeout},
		agent: "dplayer/" + version + " (+https://github.com/Conava/dux-player)", lang: lang, keys: keys,
		now: time.Now, sem: make(chan struct{}, lookups), inflight: map[string]chan struct{}{}, mbGap: musicBrainzGap}
}

func cacheKey(name string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(name))))
	return fmt.Sprintf("%x", h[:10])
}

func (o *online) load(key string) entry {
	var e entry
	if data, err := os.ReadFile(filepath.Join(o.dir, key+".json")); err == nil {
		_ = json.Unmarshal(data, &e)
	}
	return e
}

func (o *online) save(key string, e entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(filepath.Join(o.dir, key+".json"), data, 0o600)
}

// lookup returns what is known about name, asking the services for the
// parts (image, biography) that aren't cached yet. mbid may be empty.
func (o *online) lookup(ctx context.Context, name, mbid string, image bool) (entry, error) {
	key := cacheKey(name)
	for {
		e := o.load(key)
		if image && fresh(e.ImageAt, e.Image != "", o.now()) || !image && fresh(e.BioAt, e.Bio != "", o.now()) {
			return e, nil
		}
		o.mu.Lock()
		wait, busy := o.inflight[key]
		if !busy {
			wait = make(chan struct{})
			o.inflight[key] = wait
		}
		o.mu.Unlock()
		if busy {
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return entry{}, ctx.Err()
			}
		}
		e, err := o.fetch(ctx, key, name, mbid, e, image)
		o.mu.Lock()
		delete(o.inflight, key)
		close(wait)
		o.mu.Unlock()
		return e, err
	}
}

func (o *online) fetch(ctx context.Context, key, name, mbid string, e entry, image bool) (entry, error) {
	select {
	case o.sem <- struct{}{}:
	case <-ctx.Done():
		return e, ctx.Err()
	}
	defer func() { <-o.sem }()
	if mbid != "" && e.MBID != mbid {
		e.MBID, e.Wikidata = mbid, ""
	}
	var err error
	if e.MBID == "" {
		if e.MBID, err = o.musicBrainzID(ctx, name); err != nil {
			return e, err
		}
	}
	if e.MBID != "" && e.Wikidata == "" {
		if e.Wikidata, err = o.wikidataID(ctx, e.MBID); err != nil {
			return e, err
		}
	}
	if image {
		err = o.findImage(ctx, key, name, &e)
	} else {
		err = o.findBio(ctx, name, &e)
	}
	if err != nil {
		return e, err
	}
	return e, o.save(key, e)
}

func (o *online) findImage(ctx context.Context, key, name string, e *entry) error {
	type provider struct {
		name string
		url  func() (string, error)
	}
	providers := []provider{
		{"fanart.tv", func() (string, error) { return o.fanartImage(ctx, e.MBID) }},
		{"Wikimedia Commons", func() (string, error) { return o.commonsImage(ctx, e.Wikidata) }},
	}
	e.Image, e.ImageSource = "", ""
	for _, p := range providers {
		u, err := p.url()
		if err != nil {
			return err
		}
		if u == "" {
			continue
		}
		file, err := o.download(ctx, key, u)
		if errors.Is(err, source.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		e.Image, e.ImageSource = file, p.name
		break
	}
	e.ImageAt = o.now().Unix()
	return nil
}

func (o *online) findBio(ctx context.Context, name string, e *entry) error {
	e.Bio, e.BioSource = "", nil
	text, link, err := o.wikipediaBio(ctx, e.Wikidata)
	if err != nil {
		return err
	}
	if text == "" {
		if text, link, err = o.lastfmBio(ctx, name, e.MBID); err != nil {
			return err
		}
	}
	e.Bio, e.BioSource, e.BioAt = text, link, o.now().Unix()
	return nil
}

// errTransient marks a failure worth retrying later (offline, rate limit,
// server error), which is not cached as "not found".
var errTransient = errors.New("try again later")

// getJSON fetches u into out; a 404 leaves out empty and is no error.
func (o *online) getJSON(ctx context.Context, u string, out any) error {
	body, err := o.get(ctx, u, maxJSON)
	if errors.Is(err, source.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func (o *online) get(ctx context.Context, u string, limit int) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", o.agent)
	req.Header.Set("Accept", "application/json")
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errTransient, redact(err.Error()))
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest:
		return nil, source.ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: %s answered HTTP %d", errTransient, req.URL.Host, resp.StatusCode)
	}
	return source.ReadCapped(resp.Body, limit)
}

// redact drops query strings (API keys) from an error text.
func redact(s string) string {
	if i := strings.Index(s, "?"); i >= 0 {
		end := strings.IndexAny(s[i:], " \"")
		if end < 0 {
			return s[:i]
		}
		return s[:i] + s[i+end:]
	}
	return s
}

// download stores an image under key and returns its file name; ErrNotFound
// when the answer isn't an image.
func (o *online) download(ctx context.Context, key, u string) (string, error) {
	data, err := o.get(ctx, u, source.MaxCover)
	if err != nil {
		return "", err
	}
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[http.DetectContentType(data)]
	if ext == "" {
		return "", source.ErrNotFound
	}
	name := key + ext
	return name, atomicfile.WriteFile(filepath.Join(o.dir, name), data, 0o600)
}

// waitMusicBrainz keeps to MusicBrainz' one request per second.
func (o *online) waitMusicBrainz(ctx context.Context) error {
	o.mu.Lock()
	now := o.now()
	slot := o.mbNext
	if slot.Before(now) {
		slot = now
	}
	o.mbNext = slot.Add(o.mbGap)
	o.mu.Unlock()
	select {
	case <-time.After(slot.Sub(now)):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// musicBrainzID searches the artist by name and takes only a confident,
// same-named match.
func (o *online) musicBrainzID(ctx context.Context, name string) (string, error) {
	if err := o.waitMusicBrainz(ctx); err != nil {
		return "", err
	}
	var r struct {
		Artists []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Score int    `json:"score"`
		} `json:"artists"`
	}
	q := url.Values{"query": {`artist:"` + strings.ReplaceAll(name, `"`, ``) + `"`}, "fmt": {"json"}, "limit": {"3"}}
	if err := o.getJSON(ctx, o.ep.MusicBrainz+"/ws/2/artist/?"+q.Encode(), &r); err != nil {
		return "", err
	}
	for _, a := range r.Artists {
		if a.Score >= 90 && strings.EqualFold(a.Name, name) {
			return a.ID, nil
		}
	}
	return "", nil
}

// wikidataID follows the artist's Wikidata link on MusicBrainz.
func (o *online) wikidataID(ctx context.Context, mbid string) (string, error) {
	if err := o.waitMusicBrainz(ctx); err != nil {
		return "", err
	}
	var r struct {
		Relations []struct {
			Type string `json:"type"`
			URL  struct {
				Resource string `json:"resource"`
			} `json:"url"`
		} `json:"relations"`
	}
	if err := o.getJSON(ctx, o.ep.MusicBrainz+"/ws/2/artist/"+url.PathEscape(mbid)+"?inc=url-rels&fmt=json", &r); err != nil {
		return "", err
	}
	for _, rel := range r.Relations {
		if rel.Type == "wikidata" {
			if i := strings.LastIndex(rel.URL.Resource, "/"); i >= 0 && strings.HasPrefix(rel.URL.Resource[i+1:], "Q") {
				return rel.URL.Resource[i+1:], nil
			}
		}
	}
	return "", nil
}

type wikidataEntity struct {
	Claims map[string][]struct {
		Mainsnak struct {
			Datavalue struct {
				Value json.RawMessage `json:"value"`
			} `json:"datavalue"`
		} `json:"mainsnak"`
	} `json:"claims"`
	Sitelinks map[string]struct {
		Title string `json:"title"`
	} `json:"sitelinks"`
}

func (o *online) wikidata(ctx context.Context, qid string) (wikidataEntity, error) {
	var r struct {
		Entities map[string]wikidataEntity `json:"entities"`
	}
	err := o.getJSON(ctx, o.ep.Wikidata+"/wiki/Special:EntityData/"+url.PathEscape(qid)+".json", &r)
	return r.Entities[qid], err
}

// commonsImage is the Wikidata image (P18) as a scaled Commons file.
func (o *online) commonsImage(ctx context.Context, qid string) (string, error) {
	if qid == "" {
		return "", nil
	}
	ent, err := o.wikidata(ctx, qid)
	if err != nil || len(ent.Claims["P18"]) == 0 {
		return "", err
	}
	var file string
	if json.Unmarshal(ent.Claims["P18"][0].Mainsnak.Datavalue.Value, &file) != nil || file == "" {
		return "", nil
	}
	return o.ep.Commons + "/wiki/Special:FilePath/" + url.PathEscape(file) + "?width=600", nil
}

// wikipediaBio is the summary of the artist's Wikipedia article in the
// system language, else in English.
func (o *online) wikipediaBio(ctx context.Context, qid string) (string, *source.Link, error) {
	if qid == "" {
		return "", nil, nil
	}
	ent, err := o.wikidata(ctx, qid)
	if err != nil {
		return "", nil, err
	}
	for _, lang := range []string{o.lang, "en"} {
		link, ok := ent.Sitelinks[lang+"wiki"]
		if lang == "" || !ok {
			continue
		}
		var r struct {
			Extract     string `json:"extract"`
			ContentURLs struct {
				Desktop struct {
					Page string `json:"page"`
				} `json:"desktop"`
			} `json:"content_urls"`
		}
		title := url.PathEscape(strings.ReplaceAll(link.Title, " ", "_"))
		if err := o.getJSON(ctx, o.ep.Wikipedia(lang)+"/api/rest_v1/page/summary/"+title, &r); err != nil {
			return "", nil, err
		}
		if text := source.PlainText(r.Extract); text != "" {
			return text, &source.Link{Name: "Wikipedia", URL: r.ContentURLs.Desktop.Page}, nil
		}
	}
	return "", nil, nil
}

func (o *online) fanartImage(ctx context.Context, mbid string) (string, error) {
	key := o.keys(ProviderFanartTV)
	if key == "" || mbid == "" {
		return "", nil
	}
	var r struct {
		Thumbs []struct {
			URL string `json:"url"`
		} `json:"artistthumb"`
	}
	if err := o.getJSON(ctx, o.ep.FanartTV+"/v3/music/"+url.PathEscape(mbid)+"?"+url.Values{"api_key": {key}}.Encode(), &r); err != nil {
		return "", err
	}
	if len(r.Thumbs) == 0 {
		return "", nil
	}
	return r.Thumbs[0].URL, nil
}

// lastfmBio is Last.fm's artist summary. Last.fm serves only a placeholder
// as artist image since 2019, so it is asked for the biography alone.
func (o *online) lastfmBio(ctx context.Context, name, mbid string) (string, *source.Link, error) {
	key := o.keys(ProviderLastFM)
	if key == "" {
		return "", nil, nil
	}
	q := url.Values{"method": {"artist.getinfo"}, "artist": {name}, "api_key": {key}, "format": {"json"}, "autocorrect": {"1"}}
	if mbid != "" {
		q.Set("mbid", mbid)
	}
	if o.lang != "" {
		q.Set("lang", o.lang)
	}
	var r struct {
		Artist struct {
			URL string `json:"url"`
			Bio struct {
				Summary string `json:"summary"`
			} `json:"bio"`
		} `json:"artist"`
	}
	if err := o.getJSON(ctx, o.ep.LastFM+"/2.0/?"+q.Encode(), &r); err != nil {
		return "", nil, err
	}
	// The summary ends in a "Read more on Last.fm" link; the source link
	// carries it instead.
	text := r.Artist.Bio.Summary
	if i := strings.Index(text, "<a href"); i >= 0 {
		text = text[:i]
	}
	if text = source.PlainText(text); text == "" {
		return "", nil, nil
	}
	return text, &source.Link{Name: "Last.fm", URL: r.Artist.URL}, nil
}
