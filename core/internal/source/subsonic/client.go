// Package subsonic is the source kind for Subsonic and OpenSubsonic servers:
// Navidrome first, plus Gonic, Airsonic-Advanced, Ampache, Nextcloud Music,
// LMS and Funkwhale.
package subsonic

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/Conava/dux-player/core/internal/atomicfile"
	"github.com/Conava/dux-player/core/internal/source"
)

const (
	apiVersion = "1.16.1"
	clientName = "dplayer"
)

// Auth modes. Token (md5 of password and salt) is standard, but Nextcloud
// Music, LDAP users and Airsonic-Advanced one-way hashes can't check one and
// need the password (p=enc:<hex>). OpenSubsonic apiKeyAuthentication (LMS)
// takes an API key instead.
const (
	AuthAuto     = "auto"
	AuthToken    = "token"
	AuthPassword = "password"
	AuthAPIKey   = "apiKey"
)

func ValidAuth(mode string) bool {
	return mode == AuthAuto || mode == AuthToken || mode == AuthPassword || mode == AuthAPIKey
}

var errPasswordOverHTTP = source.ErrPasswordNeedsHTTPS

// ErrUnsupported is returned when the server doesn't implement a method.
var ErrUnsupported = errors.New("the server doesn't support this")

// APIError is an error the server reported in its response envelope.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string { return fmt.Sprintf("server error %d: %s", e.Code, e.Message) }

// AuthFailed covers wrong credentials (40), token auth not supported for
// this user (41), auth mechanism not supported (42), conflicting mechanisms
// (43) and an invalid API key (44).
func (e *APIError) AuthFailed() bool { return e.Code >= 40 && e.Code <= 44 }

// Is matches source.ErrSignInRefused for wrong credentials and a bad API
// key; the other auth codes mean another sign-in method may work.
func (e *APIError) Is(target error) bool {
	return target == source.ErrSignInRefused && (e.Code == 40 || e.Code == 44)
}

func apiCode(err error) int {
	var e *APIError
	if errors.As(err, &e) {
		return e.Code
	}
	return -1
}

func authRejected(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.AuthFailed()
}

// serverState is what the client learned about its server. It is cached
// on disk so the next start signs in the right way at once; deleting the
// file only costs a new probe.
type serverState struct {
	Auth          string   `json:"auth"`
	Version       string   `json:"version"`
	Type          string   `json:"type,omitempty"`
	ServerVersion string   `json:"serverVersion,omitempty"`
	OpenSubsonic  bool     `json:"openSubsonic"`
	Extensions    []string `json:"extensions,omitempty"`
	// Unsupported are methods the server answered with "not implemented".
	Unsupported []string `json:"unsupported,omitempty"`
}

type Client struct {
	base   *url.URL
	user   string
	secret string
	// auth is the configured mode; AuthAuto lets the probe pick.
	auth      string
	statePath string
	http      *http.Client

	mu     sync.Mutex
	st     serverState
	probed bool
}

// NewClient prepares a client; it does no network I/O. statePath may be
// empty (nothing is cached).
func NewClient(baseURL, user, secret, auth, statePath string, hc *http.Client) (*Client, error) {
	u, err := source.ServerURL(baseURL)
	if err != nil {
		return nil, err
	}
	if auth == "" {
		auth = AuthAuto
	}
	if !ValidAuth(auth) {
		return nil, fmt.Errorf("auth must be auto, token, password or apiKey, not %q", auth)
	}
	if auth == AuthPassword && u.Scheme != "https" {
		return nil, errPasswordOverHTTP
	}
	if hc == nil {
		hc = &http.Client{Timeout: source.CallTimeout, CheckRedirect: source.SameOrigin}
	}
	c := &Client{base: u, user: user, secret: secret, auth: auth, statePath: statePath, http: hc}
	c.st = c.loadState()
	return c, nil
}

func (c *Client) loadState() serverState {
	st := serverState{Auth: AuthToken, Version: apiVersion}
	if c.auth != AuthAuto {
		st.Auth = c.auth
	}
	if c.statePath == "" {
		return st
	}
	data, err := os.ReadFile(c.statePath)
	if err != nil {
		return st
	}
	var cached serverState
	if json.Unmarshal(data, &cached) != nil || !ValidAuth(cached.Auth) || cached.Auth == AuthAuto {
		return st
	}
	if c.auth != AuthAuto {
		cached.Auth = c.auth
	}
	if cached.Version == "" {
		cached.Version = apiVersion
	}
	return cached
}

func (c *Client) state() serverState {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.st
	st.Extensions = slices.Clone(st.Extensions)
	st.Unsupported = slices.Clone(st.Unsupported)
	return st
}

func (c *Client) setState(st serverState, probed bool) {
	c.mu.Lock()
	c.st = st
	c.probed = c.probed || probed
	c.mu.Unlock()
	if c.statePath == "" {
		return
	}
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = atomicfile.WriteFile(c.statePath, data, 0o600)
}

func (c *Client) supports(method string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !slices.Contains(c.st.Unsupported, method)
}

func (c *Client) markUnsupported(method string) {
	st := c.state()
	if slices.Contains(st.Unsupported, method) {
		return
	}
	st.Unsupported = append(st.Unsupported, method)
	c.setState(st, false)
}

// authParams signs a request. Token mode builds a fresh salt every time.
func (c *Client) authParams(st serverState) url.Values {
	q := url.Values{"v": {st.Version}, "c": {clientName}, "f": {"json"}}
	switch st.Auth {
	case AuthAPIKey:
		// OpenSubsonic rejects apiKey together with u.
		q.Set("apiKey", c.secret)
	case AuthPassword:
		q.Set("u", c.user)
		q.Set("p", "enc:"+hex.EncodeToString([]byte(c.secret)))
	default:
		salt := make([]byte, 8)
		_, _ = rand.Read(salt)
		s := hex.EncodeToString(salt)
		sum := md5.Sum([]byte(c.secret + s))
		q.Set("u", c.user)
		q.Set("t", hex.EncodeToString(sum[:]))
		q.Set("s", s)
	}
	return q
}

func (c *Client) endpoint(st serverState, method string, params url.Values) string {
	q := c.authParams(st)
	for k, vs := range params {
		q[k] = vs
	}
	u := *c.base
	u.Path += "/rest/" + method
	u.RawQuery = q.Encode()
	return u.String()
}

type httpStatusError struct {
	method string
	code   int
}

func (e *httpStatusError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.method, e.code) }

// get does one request. Errors never carry the URL: it holds the
// credentials.
func (c *Client) get(ctx context.Context, st serverState, method string, params url.Values) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, source.CallTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(st, method, params), nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("%s: build request", method)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		cancel()
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	resp.Body = cancelOnClose{resp.Body, cancel}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &httpStatusError{method: method, code: resp.StatusCode}
	}
	return resp, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// envelope is the common response wrapper. Payload fields are decoded by the
// caller from the same object.
type envelope struct {
	Status        string    `json:"status"`
	Version       string    `json:"version"`
	Type          string    `json:"type"`
	ServerVersion string    `json:"serverVersion"`
	OpenSubsonic  boolean   `json:"openSubsonic"`
	Error         *APIError `json:"error"`
}

// raw does a request with the given state and decodes the
// "subsonic-response" object into out.
func (c *Client) raw(ctx context.Context, st serverState, method string, params url.Values, out any) (envelope, error) {
	resp, err := c.get(ctx, st, method, params)
	if err != nil {
		return envelope{}, err
	}
	defer resp.Body.Close()
	body, err := source.ReadCapped(resp.Body, source.MaxBody)
	if err != nil {
		return envelope{}, fmt.Errorf("%s: read response: %w", method, err)
	}
	return decode(method, body, out)
}

var notImplemented = regexp.MustCompile(`(?i)not (yet )?(implemented|supported)|unknown (method|endpoint)`)

// unsupported tells "the server has no such method" apart from real
// failures. Servers say it as HTTP 404/501, as code 30 (Ampache) or as a
// generic code 0 with a message. A proxy answers 404 or 501 too while the
// server restarts, so an HTTP status only counts while ping still works.
func (c *Client) unsupported(ctx context.Context, err error) bool {
	var he *httpStatusError
	if errors.As(err, &he) {
		if he.code != http.StatusNotFound && he.code != http.StatusNotImplemented {
			return false
		}
		_, perr := c.raw(ctx, c.state(), "ping", nil, nil)
		return perr == nil
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Code == 30 || (ae.Code == 0 && notImplemented.MatchString(ae.Message))
}

// call signs in (probing the server on first use) and does one API call.
func (c *Client) call(ctx context.Context, method string, params url.Values, out any) (envelope, error) {
	if !c.supports(method) {
		return envelope{}, fmt.Errorf("%s: %w", method, ErrUnsupported)
	}
	if err := c.ensureProbed(ctx); err != nil {
		return envelope{}, err
	}
	env, err := c.raw(ctx, c.state(), method, params, out)
	if err != nil && c.unsupported(ctx, err) {
		c.markUnsupported(method)
		return env, fmt.Errorf("%s: %w", method, ErrUnsupported)
	}
	return env, err
}

func (c *Client) ensureProbed(ctx context.Context) error {
	c.mu.Lock()
	probed := c.probed
	c.mu.Unlock()
	if probed {
		return nil
	}
	_, err := c.Probe(ctx)
	return err
}

// candidates are the auth modes to try, the last one that worked first.
func (c *Client) candidates(st serverState) []string {
	if c.auth != AuthAuto {
		return []string{c.auth}
	}
	out := []string{st.Auth}
	for _, m := range []string{AuthToken, AuthAPIKey, AuthPassword} {
		if !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// Probe settles the API version (the lower of ours and the server's) and, in
// auto mode, the auth mode: token, API key if offered, then password. It reruns
// on the next call after any failure; calls run concurrently with it, so no
// lock is held while it waits on the network.
func (c *Client) Probe(ctx context.Context) (serverState, error) {
	st := c.state()
	st.Version = apiVersion
	var firstAuthErr error
	skippedPassword := false
	for _, mode := range c.candidates(st) {
		if mode == AuthToken && versionLess(st.Version, "1.13.0") {
			continue
		}
		if mode == AuthPassword && c.base.Scheme != "https" {
			skippedPassword = true
			continue
		}
		if mode == AuthAPIKey && c.auth == AuthAuto && !c.offersAPIKey(ctx, st) {
			continue
		}
		st.Auth = mode
		env, err := c.raw(ctx, st, "ping", nil, nil)
		if env.Version != "" && versionLess(env.Version, st.Version) {
			st.Version = env.Version
			if code := apiCode(err); code == 20 || code == 30 {
				env, err = c.raw(ctx, st, "ping", nil, nil)
			}
		}
		if err == nil {
			// Another server, or another version of it, may have the
			// methods this one lacked.
			if env.Type != st.Type || env.ServerVersion != st.ServerVersion {
				st.Unsupported = nil
			}
			st.Type, st.ServerVersion, st.OpenSubsonic = env.Type, env.ServerVersion, bool(env.OpenSubsonic)
			st.Extensions = nil
			if st.OpenSubsonic {
				st.Extensions, _ = c.extensionNames(ctx, st)
			}
			c.setState(st, true)
			return st, nil
		}
		if !authRejected(err) {
			return st, err
		}
		if firstAuthErr == nil {
			firstAuthErr = err
		}
	}
	switch {
	case firstAuthErr == nil && skippedPassword:
		firstAuthErr = errPasswordOverHTTP
	case firstAuthErr == nil:
		firstAuthErr = errors.New("no way to sign in to this server")
	case skippedPassword:
		firstAuthErr = fmt.Errorf("%w (%w)", firstAuthErr, errPasswordOverHTTP)
	}
	return st, firstAuthErr
}

// offersAPIKey asks for the extension list, which OpenSubsonic requires to
// be readable without signing in.
func (c *Client) offersAPIKey(ctx context.Context, st serverState) bool {
	names, err := c.extensionNames(ctx, st)
	return err == nil && slices.Contains(names, "apiKeyAuthentication")
}

func (c *Client) extensionNames(ctx context.Context, st serverState) ([]string, error) {
	var out struct {
		Extensions list[Extension] `json:"openSubsonicExtensions"`
	}
	if _, err := c.raw(ctx, st, "getOpenSubsonicExtensions", nil, &out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Extensions))
	for _, e := range out.Extensions {
		names = append(names, e.Name)
	}
	return names, nil
}

// versionLess compares dotted versions numerically ("1.9.0" < "1.13.0").
func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

func decode(method string, body []byte, out any) (envelope, error) {
	var wrap struct {
		Resp json.RawMessage `json:"subsonic-response"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil || len(wrap.Resp) == 0 {
		return envelope{}, fmt.Errorf("%s: not an (Open)Subsonic response", method)
	}
	var env envelope
	if err := json.Unmarshal(wrap.Resp, &env); err != nil {
		return envelope{}, fmt.Errorf("%s: decode response: %w", method, err)
	}
	if env.Status != "ok" {
		if env.Error != nil {
			return env, fmt.Errorf("%s: %w", method, env.Error)
		}
		return env, fmt.Errorf("%s: status %q", method, env.Status)
	}
	if out != nil {
		if err := json.Unmarshal(wrap.Resp, out); err != nil {
			return env, fmt.Errorf("%s: decode payload: %w", method, err)
		}
	}
	return env, nil
}

// StreamURL is the URL mpv opens. It carries the credentials: never log it.
// format=raw asks for the original file so gapless and ReplayGain survive.
func (c *Client) StreamURL(ctx context.Context, id string) (string, error) {
	if err := c.ensureProbed(ctx); err != nil {
		return "", err
	}
	return c.endpoint(c.state(), "stream", url.Values{"id": {id}, "format": {"raw"}}), nil
}

// CoverArt downloads cover art, capped at source.MaxCover bytes.
func (c *Client) CoverArt(ctx context.Context, id string, size int) ([]byte, string, error) {
	if err := c.ensureProbed(ctx); err != nil {
		return nil, "", err
	}
	params := url.Values{"id": {id}}
	if size > 0 {
		params.Set("size", fmt.Sprint(size))
	}
	resp, err := c.get(ctx, c.state(), "getCoverArt", params)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := source.ReadCapped(resp.Body, source.MaxCover)
	if err != nil {
		return nil, "", fmt.Errorf("getCoverArt: %w", err)
	}
	ct := http.DetectContentType(data)
	if !strings.HasPrefix(ct, "image/") {
		// Errors come back as a JSON envelope with status 200.
		if _, err := decode("getCoverArt", data, nil); err != nil {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("getCoverArt: unexpected %s", ct)
	}
	return data, ct, nil
}
