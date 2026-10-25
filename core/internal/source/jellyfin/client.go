// Package jellyfin is the source kind for Jellyfin servers. It sticks to
// the routes Emby shares (/Users/{id}/Items, /Users/{id}/FavoriteItems,
// …), so Emby works where its API matches.
package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Conava/dux-player/core/internal/source"
)

const clientName = "Dux Player"

// httpError is a reply with an unexpected HTTP status.
type httpError struct {
	endpoint string
	code     int
}

func (e *httpError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.endpoint, e.code) }

// ErrUnauthorized means the token was refused (expired, revoked).
var ErrUnauthorized = source.ErrSignInRefused

type Client struct {
	base     *url.URL
	user     string
	password string
	deviceID string
	device   string
	version  string
	http     *http.Client
	// onToken stores a fresh token and user id (in the keyring).
	onToken func(token, userID string)

	mu     sync.Mutex
	token  string
	userID string
	// signIn runs one sign-in at a time: each one revokes the token of the
	// previous session with the same device id, so parallel sign-ins would
	// revoke each other's tokens.
	signIn sync.Mutex
	// checked is set once a call went through with the current session; a
	// token restored from the keyring may be stale, and mpv can't renew it.
	checked atomic.Bool
}

func NewClient(baseURL, user, password, deviceID, version string, hc *http.Client) (*Client, error) {
	u, err := source.ServerURL(baseURL)
	if err != nil {
		return nil, err
	}
	if hc == nil {
		hc = &http.Client{Timeout: source.CallTimeout, CheckRedirect: source.SameOrigin}
	}
	return &Client{base: u, user: user, password: password, deviceID: deviceID, device: "Linux",
		version: version, http: hc}, nil
}

func (c *Client) session() (token, userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token, c.userID
}

func (c *Client) setSession(token, userID string) {
	c.mu.Lock()
	c.token, c.userID = token, userID
	c.mu.Unlock()
}

// quote drops what would break the header's quoted values.
func quote(s string) string {
	return `"` + strings.NewReplacer(`"`, "", ",", " ", "\n", " ", "\r", " ").Replace(s) + `"`
}

func (c *Client) authHeader(token string) string {
	h := fmt.Sprintf("MediaBrowser Client=%s, Device=%s, DeviceId=%s, Version=%s",
		quote(clientName), quote(c.device), quote(c.deviceID), quote(c.version))
	if token != "" {
		h += ", Token=" + quote(token)
	}
	return h
}

func (c *Client) url(path string, q url.Values) string {
	u := *c.base
	u.Path += path
	u.RawQuery = q.Encode()
	return u.String()
}

// do sends one request with the session's token. Errors never carry the
// URL or the token.
func (c *Client) do(ctx context.Context, method, path string, q url.Values, body, out any, token string) error {
	ctx, cancel := context.WithTimeout(ctx, source.CallTimeout)
	defer cancel()
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path, q), rd)
	if err != nil {
		return fmt.Errorf("%s: build request", path)
	}
	auth := c.authHeader(token)
	req.Header.Set("Authorization", auth)
	// Emby reads only the legacy header name.
	req.Header.Set("X-Emby-Authorization", auth)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("%s: %w", endpointName(path), err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%s: %w", endpointName(path), ErrUnauthorized)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return &httpError{endpoint: endpointName(path), code: resp.StatusCode}
	}
	data, err := source.ReadCapped(resp.Body, source.MaxBody)
	if err != nil {
		return fmt.Errorf("%s: read response: %w", endpointName(path), err)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: decode response: %w", endpointName(path), err)
	}
	return nil
}

// endpointName keeps ids (which may be a user id) out of error text.
func endpointName(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	out := parts[:0]
	for _, p := range parts {
		if len(p) >= 16 {
			p = "…"
		}
		out = append(out, p)
	}
	return strings.Join(out, "/")
}

type authResult struct {
	AccessToken string `json:"AccessToken"`
	User        struct {
		ID string `json:"Id"`
	} `json:"User"`
}

// Authenticate signs in with the password and keeps the new token.
func (c *Client) Authenticate(ctx context.Context) error {
	var res authResult
	err := c.do(ctx, http.MethodPost, "/Users/AuthenticateByName", nil,
		map[string]string{"Username": c.user, "Pw": c.password}, &res, "")
	if err != nil {
		return err
	}
	if res.AccessToken == "" || res.User.ID == "" {
		return errors.New("Users/AuthenticateByName: no token in the reply")
	}
	c.setSession(res.AccessToken, res.User.ID)
	if c.onToken != nil {
		c.onToken(res.AccessToken, res.User.ID)
	}
	return nil
}

// renew signs in again unless another request already replaced the
// refused token stale.
func (c *Client) renew(ctx context.Context, stale string) error {
	c.signIn.Lock()
	defer c.signIn.Unlock()
	if token, _ := c.session(); token != "" && token != stale {
		return nil
	}
	return c.Authenticate(ctx)
}

// call signs in when there is no token yet, and once more when the token
// is refused.
func (c *Client) call(ctx context.Context, method, path string, q url.Values, body, out any) error {
	if _, err := c.UserID(ctx); err != nil {
		return err
	}
	token, _ := c.session()
	err := c.do(ctx, method, c.userPath(path), c.userQuery(q), body, out, token)
	if errors.Is(err, ErrUnauthorized) {
		if err := c.renew(ctx, token); err != nil {
			return err
		}
		token, _ = c.session()
		err = c.do(ctx, method, c.userPath(path), c.userQuery(q), body, out, token)
	}
	if err == nil {
		c.checked.Store(true)
	}
	return err
}

// userParam in a path or query value stands for the user id, which is
// only known after signing in.
const userParam = "{user}"

func (c *Client) userPath(path string) string {
	_, uid := c.session()
	return strings.ReplaceAll(path, userParam, url.PathEscape(uid))
}

func (c *Client) userQuery(q url.Values) url.Values {
	_, uid := c.session()
	out := url.Values{}
	for k, vs := range q {
		for _, v := range vs {
			out.Add(k, strings.ReplaceAll(v, userParam, uid))
		}
	}
	return out
}

// UserID signs in if needed and returns the user's id.
func (c *Client) UserID(ctx context.Context) (string, error) {
	if token, uid := c.session(); token != "" {
		return uid, nil
	}
	if err := c.renew(ctx, ""); err != nil {
		return "", err
	}
	_, uid := c.session()
	return uid, nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	return c.call(ctx, http.MethodGet, path, q, nil, out)
}

// StreamURL asks for the original file in every container mpv plays (direct
// play) and Opus in Ogg when the server must transcode. It carries the
// token: never log it.
func (c *Client) StreamURL(ctx context.Context, id string) (string, error) {
	if !c.checked.Load() {
		if err := c.get(ctx, "/Users/{user}", nil, nil); err != nil {
			return "", err
		}
	}
	uid, err := c.UserID(ctx)
	if err != nil {
		return "", err
	}
	token, _ := c.session()
	q := url.Values{
		"UserId":               {uid},
		"DeviceId":             {c.deviceID},
		"Container":            {"flac,mp3,opus,ogg,oga,m4a,m4b,aac,alac,wav,wv,ape,mpc,aiff,aif,dsf,dff,webm,mka,wma"},
		"TranscodingContainer": {"ogg"},
		"TranscodingProtocol":  {"http"},
		"AudioCodec":           {"opus"},
		"MaxStreamingBitrate":  {"999999999"},
		// Jellyfin 12 refuses the legacy api_key (401) and wants ApiKey;
		// Emby knows only api_key. Sending both works on each.
		"ApiKey":  {token},
		"api_key": {token},
	}
	return c.url("/Audio/"+url.PathEscape(id)+"/universal", q), nil
}

// Image downloads an item's primary image, capped at source.MaxCover bytes.
func (c *Client) Image(ctx context.Context, id string, size int) ([]byte, string, error) {
	data, err := c.image(ctx, id, size)
	if errors.Is(err, ErrUnauthorized) {
		token, _ := c.session()
		if err := c.renew(ctx, token); err != nil {
			return nil, "", err
		}
		data, err = c.image(ctx, id, size)
	}
	if err != nil {
		return nil, "", err
	}
	ct := http.DetectContentType(data)
	if !strings.HasPrefix(ct, "image/") {
		return nil, "", fmt.Errorf("image: unexpected %s", ct)
	}
	return data, ct, nil
}

func (c *Client) image(ctx context.Context, id string, size int) ([]byte, error) {
	q := url.Values{}
	if size > 0 {
		q.Set("maxHeight", fmt.Sprint(size))
		q.Set("maxWidth", fmt.Sprint(size))
	}
	ctx, cancel := context.WithTimeout(ctx, source.CallTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/Items/"+url.PathEscape(id)+"/Images/Primary", q), nil)
	if err != nil {
		return nil, errors.New("image: build request")
	}
	token, _ := c.session()
	req.Header.Set("Authorization", c.authHeader(token))
	resp, err := c.http.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("image: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, fmt.Errorf("image: %w", ErrUnauthorized)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("image: HTTP %d", resp.StatusCode)
	}
	data, err := source.ReadCapped(resp.Body, source.MaxCover)
	if err != nil {
		return nil, fmt.Errorf("image: %w", err)
	}
	return data, nil
}
