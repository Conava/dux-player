package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/Conava/dux-player/core/internal/source"
)

var _ source.QuickConnect = (*Source)(nil)

// authQuickConnect is the config value of a library that signs in with
// Quick Connect: no password is stored for it.
const authQuickConnect = "quickConnect"

// quickConnectErr words the failures every Quick Connect route shares: 401
// is the feature turned off, 404 a code the server doesn't know.
func quickConnectErr(err error, notFound error) error {
	var he *httpError
	switch {
	case errors.Is(err, ErrUnauthorized):
		return source.ErrQuickConnectOff
	case notFound != nil && errors.As(err, &he) && he.code == http.StatusNotFound:
		return notFound
	}
	return err
}

// QuickConnectEnabled reads a public route. A server without the route
// (Emby) counts as off.
func (s *Source) QuickConnectEnabled(ctx context.Context) (bool, error) {
	var on bool
	err := s.client.do(ctx, http.MethodGet, "/QuickConnect/Enabled", nil, nil, &on, "")
	switch err = quickConnectErr(err, source.ErrQuickConnectOff); {
	case errors.Is(err, source.ErrQuickConnectOff):
		return false, nil
	case err != nil:
		return false, err
	}
	return on, nil
}

func (s *Source) QuickConnectStart(ctx context.Context) (string, string, error) {
	var res struct{ Secret, Code string }
	err := s.client.do(ctx, http.MethodPost, "/QuickConnect/Initiate", nil, nil, &res, "")
	var he *httpError
	if errors.As(err, &he) && he.code == http.StatusMethodNotAllowed {
		// Jellyfin 10.8 takes only GET here.
		err = s.client.do(ctx, http.MethodGet, "/QuickConnect/Initiate", nil, nil, &res, "")
	}
	if err != nil {
		return "", "", quickConnectErr(err, source.ErrQuickConnectOff)
	}
	if res.Secret == "" || res.Code == "" {
		return "", "", errors.New("QuickConnect/Initiate: no code in the reply")
	}
	return source.Clean(res.Code), res.Secret, nil
}

func (s *Source) QuickConnectPoll(ctx context.Context, secret string) (bool, error) {
	var res struct{ Authenticated bool }
	err := s.client.do(ctx, http.MethodGet, "/QuickConnect/Connect", url.Values{"secret": {secret}}, nil, &res, "")
	if err != nil {
		return false, quickConnectErr(err, source.ErrQuickConnectExpired)
	}
	return res.Authenticated, nil
}

func (s *Source) QuickConnectFinish(ctx context.Context, secret string) (string, error) {
	var res authResult
	err := s.client.do(ctx, http.MethodPost, "/Users/AuthenticateWithQuickConnect", nil,
		map[string]string{"Secret": secret}, &res, "")
	if err != nil {
		return "", quickConnectErr(err, source.ErrQuickConnectExpired)
	}
	user := source.Clean(res.User.Name)
	if res.AccessToken == "" || res.User.ID == "" || user == "" {
		return "", errors.New("Users/AuthenticateWithQuickConnect: no session in the reply")
	}
	s.client.setSession(res.AccessToken, res.User.ID)
	if s.client.onToken != nil {
		s.client.onToken(res.AccessToken, res.User.ID, user)
	}
	return user, nil
}
