package scrobble

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ListenBrainz = "listenbrainz"
	LastFM       = "lastfm"

	callTimeout = 15 * time.Second
	maxReply    = 1 << 20
	clientName  = "Dux Player"
)

// permanentError is a refusal that retrying won't fix (bad data, a revoked
// token); such a play is dropped from the queue instead of retried forever.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

func permanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}

// fetch does one request and reads a bounded reply. Network errors drop
// the URL, which may carry a key.
func fetch(ctx context.Context, hc *http.Client, req *http.Request) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	resp, err := hc.Do(req.WithContext(ctx))
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, nil, fmt.Errorf("%s: %w", req.URL.Host, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReply))
	if err != nil {
		return 0, nil, fmt.Errorf("%s: read reply: %w", req.URL.Host, err)
	}
	return resp.StatusCode, body, nil
}

// listenBrainz talks to the ListenBrainz API with a user token.
type listenBrainz struct {
	base    string
	version string
	http    *http.Client
}

func (l listenBrainz) do(ctx context.Context, method, path, token string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = strings.NewReader(string(data))
	}
	req, err := http.NewRequest(method, l.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")
	code, reply, err := fetch(ctx, l.http, req)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(reply, &e)
		err := fmt.Errorf("ListenBrainz: HTTP %d %s", code, e.Error)
		if code >= 400 && code < 500 && code != http.StatusTooManyRequests {
			return permanentError{err}
		}
		return err
	}
	if out != nil {
		return json.Unmarshal(reply, out)
	}
	return nil
}

// validate returns the token's user name.
func (l listenBrainz) validate(ctx context.Context, token string) (string, error) {
	var out struct {
		Valid    bool   `json:"valid"`
		UserName string `json:"user_name"`
		Message  string `json:"message"`
	}
	if err := l.do(ctx, http.MethodGet, "/1/validate-token", token, nil, &out); err != nil {
		return "", err
	}
	if !out.Valid {
		return "", errors.New("ListenBrainz doesn't accept this token")
	}
	return out.UserName, nil
}

func (l listenBrainz) submit(ctx context.Context, token string, li Listen, nowPlaying bool) error {
	info := map[string]any{"media_player": clientName, "submission_client": clientName, "submission_client_version": l.version}
	if li.Duration > 0 {
		info["duration_ms"] = int(li.Duration * 1000)
	}
	meta := map[string]any{"artist_name": li.Artist, "track_name": li.Title, "additional_info": info}
	if li.Album != "" {
		meta["release_name"] = li.Album
	}
	entry := map[string]any{"track_metadata": meta}
	kind := "playing_now"
	if !nowPlaying {
		kind = "single"
		entry["listened_at"] = li.PlayedAt.Unix()
	}
	return l.do(ctx, http.MethodPost, "/1/submit-listens", token, map[string]any{"listen_type": kind, "payload": []any{entry}}, nil)
}

// lastFM talks to the Last.fm API: requests signed with the API secret,
// writes with the session key from the desktop auth flow.
type lastFM struct {
	base, authBase string
	http           *http.Client
}

type lastFMKeys struct{ key, secret, session string }

// sign is Last.fm's api_sig: the md5 of every parameter name and value,
// sorted by name, then the secret. format is not signed.
func sign(params url.Values, secret string) string {
	names := make([]string, 0, len(params))
	for k := range params {
		if k != "format" {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, k := range names {
		b.WriteString(k)
		b.WriteString(params.Get(k))
	}
	b.WriteString(secret)
	sum := md5.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// lastFMTemporary are the error codes Last.fm wants retried: service
// offline or temporarily unavailable, rate limit.
var lastFMTemporary = map[int]bool{8: true, 11: true, 16: true, 29: true}

func (f lastFM) call(ctx context.Context, post bool, k lastFMKeys, params url.Values, out any) error {
	params.Set("api_key", k.key)
	if k.session != "" {
		params.Set("sk", k.session)
	}
	params.Set("api_sig", sign(params, k.secret))
	params.Set("format", "json")
	var req *http.Request
	var err error
	if post {
		req, err = http.NewRequest(http.MethodPost, f.base, strings.NewReader(params.Encode()))
		if req != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		req, err = http.NewRequest(http.MethodGet, f.base+"?"+params.Encode(), nil)
	}
	if err != nil {
		return errors.New("Last.fm: build request")
	}
	code, reply, err := fetch(ctx, f.http, req)
	if err != nil {
		return err
	}
	var e struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(reply, &e) == nil && e.Error != 0 {
		err := fmt.Errorf("Last.fm: %s (%d)", e.Message, e.Error)
		if lastFMTemporary[e.Error] {
			return err
		}
		return permanentError{err}
	}
	if code != http.StatusOK {
		return fmt.Errorf("Last.fm: HTTP %d", code)
	}
	if out != nil {
		return json.Unmarshal(reply, out)
	}
	return nil
}

// token starts the desktop auth flow; the user approves it at authURL.
func (f lastFM) token(ctx context.Context, k lastFMKeys) (token, authURL string, err error) {
	var out struct {
		Token string `json:"token"`
	}
	k.session = ""
	if err := f.call(ctx, false, k, url.Values{"method": {"auth.getToken"}}, &out); err != nil {
		return "", "", err
	}
	if out.Token == "" {
		return "", "", errors.New("Last.fm sent no token")
	}
	return out.Token, f.authBase + "?" + url.Values{"api_key": {k.key}, "token": {out.Token}}.Encode(), nil
}

// session trades an approved token for a session key, which doesn't expire.
func (f lastFM) session(ctx context.Context, k lastFMKeys, token string) (user, key string, err error) {
	var out struct {
		Session struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"session"`
	}
	k.session = ""
	if err := f.call(ctx, false, k, url.Values{"method": {"auth.getSession"}, "token": {token}}, &out); err != nil {
		return "", "", err
	}
	if out.Session.Key == "" {
		return "", "", errors.New("Last.fm sent no session")
	}
	return out.Session.Name, out.Session.Key, nil
}

// user checks the session by asking whose it is.
func (f lastFM) user(ctx context.Context, k lastFMKeys) (string, error) {
	var out struct {
		User struct {
			Name string `json:"name"`
		} `json:"user"`
	}
	if err := f.call(ctx, false, k, url.Values{"method": {"user.getInfo"}}, &out); err != nil {
		return "", err
	}
	return out.User.Name, nil
}

func (f lastFM) submit(ctx context.Context, k lastFMKeys, li Listen, nowPlaying bool) error {
	p := url.Values{"artist": {li.Artist}, "track": {li.Title}}
	if li.Album != "" {
		p.Set("album", li.Album)
	}
	if li.Duration > 0 {
		p.Set("duration", strconv.Itoa(int(li.Duration)))
	}
	if nowPlaying {
		p.Set("method", "track.updateNowPlaying")
	} else {
		p.Set("method", "track.scrobble")
		p.Set("timestamp", strconv.FormatInt(li.PlayedAt.Unix(), 10))
	}
	return f.call(ctx, true, k, p, nil)
}
