package source

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Limits for talking to a server: one request's time, an API reply and a
// cover image.
const (
	CallTimeout = 20 * time.Second
	MaxBody     = 32 << 20
	MaxCover    = 20 << 20
)

// ReadCapped reads r to the end and fails when it holds more than limit
// bytes.
func ReadCapped(r io.Reader, limit int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err == nil && len(data) > limit {
		err = fmt.Errorf("larger than %d bytes", limit)
	}
	return data, err
}

// ServerURL checks a server address: http(s) with a host and no user info,
// which would be stored in the config in plain text.
func ServerURL(addr string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(addr, "/"))
	if err == nil && u.User != nil {
		return nil, ErrCredentialsInAddress
	}
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("server address %q: %w", addr, ErrBadAddress)
	}
	return u, nil
}

// Failures server sources wrap, so the library manager can give them codes.
var (
	ErrBadAddress           = errors.New("not an http(s) server address")
	ErrCredentialsInAddress = errors.New("put the username and password in their own fields, not in the server address")
	ErrRedirected           = errors.New("the server redirected to another address; use that address as the server")
	ErrSignInRefused        = errors.New("the server refused the sign-in")
	ErrPasswordNeedsHTTPS   = errors.New("signing in with the password itself needs an https server address")
)

// SameOrigin is the CheckRedirect of clients whose requests carry
// credentials: a redirect may change the path, never the scheme, host or
// port.
func SameOrigin(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if o := via[0].URL; req.URL.Scheme != o.Scheme || req.URL.Host != o.Host {
		return ErrRedirected
	}
	return nil
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// Local is true for a host on this machine or the local network (or a
// Tailscale-style 100.64/10 address), where plain http is the norm.
func Local(u *url.URL) bool {
	h := strings.ToLower(u.Hostname())
	if ip, err := netip.ParseAddr(h); err == nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || cgnat.Contains(ip)
	}
	if !strings.Contains(h, ".") {
		return true
	}
	for _, suffix := range []string{".local", ".lan", ".home.arpa", ".internal", ".localhost"} {
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return false
}

// PlainHTTPNotice warns about a server outside the local network that is
// reached without encryption; false when there is nothing to warn about.
func PlainHTTPNotice(u *url.URL) (Notice, bool) {
	if u.Scheme == "https" || Local(u) {
		return Notice{}, false
	}
	return Notice{Code: NoticePlainHTTP,
		Message: "The connection isn't encrypted: sign-in tokens and what you play are readable on the network. Use an https address."}, true
}
