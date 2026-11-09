package scrobble

import (
	"context"
	"errors"
	"strings"

	"github.com/Conava/dux-player/core/internal/lastfm"
)

var errService = errors.New("service must be listenbrainz or lastfm")

// ConnectListenBrainz checks a user token and stores it in the keyring.
func (s *Service) ConnectListenBrainz(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("a ListenBrainz user token is required")
	}
	user, err := s.lb.validate(ctx, token)
	if err != nil {
		return err
	}
	if err := s.cfg.Secrets.Set(secretsLB, "token", token); err != nil {
		return err
	}
	return s.update(func(st *Settings) { st.ListenBrainz = Account{Connected: true, Enabled: true, User: user} })
}

// StartLastFM starts the desktop auth flow with the user's own API account,
// stored in the keyring, or with the built-in one when both are empty. The user approves at the returned page, then
// FinishLastFM gets the session.
func (s *Service) StartLastFM(ctx context.Context, apiKey, apiSecret string) (string, error) {
	k := lastFMKeys{key: strings.TrimSpace(apiKey), secret: strings.TrimSpace(apiSecret)}
	builtIn := k.key == "" && k.secret == "" && lastfm.APIKey != "" && lastfm.APISecret != ""
	if builtIn {
		k = lastFMKeys{key: lastfm.APIKey, secret: lastfm.APISecret}
	}
	if k.key == "" || k.secret == "" {
		return "", errors.New("a Last.fm API key and shared secret are required")
	}
	token, page, err := s.fm.token(ctx, k)
	if err != nil {
		return "", err
	}
	if !builtIn {
		if err := s.cfg.Secrets.Set(secretsFM, "apiKey", k.key); err != nil {
			return "", err
		}
		if err := s.cfg.Secrets.Set(secretsFM, "apiSecret", k.secret); err != nil {
			return "", err
		}
	}
	s.mu.Lock()
	s.fmToken = token
	s.fmBuiltIn = builtIn
	s.mu.Unlock()
	s.publish()
	return page, nil
}

func (s *Service) FinishLastFM(ctx context.Context) error {
	s.mu.Lock()
	token, builtIn := s.fmToken, s.fmBuiltIn
	s.mu.Unlock()
	if token == "" {
		return errors.New("start the Last.fm sign-in first")
	}
	k := lastFMKeys{key: lastfm.APIKey, secret: lastfm.APISecret}
	if !builtIn {
		var err error
		if k, err = s.apiKeys(); err != nil {
			return err
		}
	}
	user, session, err := s.fm.session(ctx, k, token)
	if err != nil {
		// Not approved yet keeps the token for another try; Last.fm says
		// code 14 then.
		return err
	}
	if builtIn {
		// A stored key of the user's would win over the built-in one, and only
		// goes now that the sign-in it gives way to has been approved.
		if err := s.cfg.Secrets.Delete(secretsFM); err != nil {
			return err
		}
	}
	if err := s.cfg.Secrets.Set(secretsFM, "session", session); err != nil {
		return err
	}
	s.mu.Lock()
	s.fmToken = ""
	s.mu.Unlock()
	return s.update(func(st *Settings) { st.LastFM = Account{Connected: true, Enabled: true, User: user} })
}

// Disconnect forgets a service's credentials.
func (s *Service) Disconnect(service string) error {
	id := secretsLB
	switch service {
	case ListenBrainz:
	case LastFM:
		id = secretsFM
		s.mu.Lock()
		s.fmToken = ""
		s.mu.Unlock()
	default:
		return errService
	}
	if err := s.cfg.Secrets.Delete(id); err != nil {
		return err
	}
	return s.update(func(st *Settings) {
		if service == ListenBrainz {
			st.ListenBrainz = Account{}
		} else {
			st.LastFM = Account{}
		}
	})
}

// Test checks that the stored credentials still work and returns the user.
func (s *Service) Test(ctx context.Context, service string) (string, error) {
	switch service {
	case ListenBrainz:
		token, err := s.cfg.Secrets.Get(secretsLB, "token")
		if err != nil {
			return "", err
		}
		return s.lb.validate(ctx, token)
	case LastFM:
		k, err := s.fmKeys()
		if err != nil {
			return "", err
		}
		return s.fm.user(ctx, k)
	}
	return "", errService
}

func (s *Service) SetEnabled(service string, on bool) error {
	if service != ListenBrainz && service != LastFM {
		return errService
	}
	return s.update(func(st *Settings) {
		a := &st.ListenBrainz
		if service == LastFM {
			a = &st.LastFM
		}
		a.Enabled = on && a.Connected
	})
}

// SetLibraryServer says whether a library's plays go to its own server.
func (s *Service) SetLibraryServer(id string, on bool) error {
	if id == "" {
		return errors.New("library is required")
	}
	return s.update(func(st *Settings) { st.NoServerLibraries = switchOff(st.NoServerLibraries, id, !on) })
}

// SetLibraryAccounts says whether a library's plays go to the accounts.
func (s *Service) SetLibraryAccounts(id string, on bool) error {
	if id == "" {
		return errors.New("library is required")
	}
	return s.update(func(st *Settings) { st.NoAccountsLibraries = switchOff(st.NoAccountsLibraries, id, !on) })
}
