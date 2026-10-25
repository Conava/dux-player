package scrobble

import (
	"context"
	"errors"
	"slices"
	"strings"
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

// StartLastFM starts the desktop auth flow with the user's own API account
// (stored as an override) or, given none, the build's. The user approves
// at the returned page, then FinishLastFM gets the session.
func (s *Service) StartLastFM(ctx context.Context, apiKey, apiSecret string) (string, error) {
	k := lastFMKeys{key: strings.TrimSpace(apiKey), secret: strings.TrimSpace(apiSecret)}
	own := k.key != "" || k.secret != ""
	switch {
	case own && (k.key == "" || k.secret == ""):
		return "", errors.New("a Last.fm API key needs its shared secret")
	case !own && (LastFMAPIKey == "" || LastFMAPISecret == ""):
		return "", errors.New("a Last.fm API key and shared secret are required")
	case !own:
		k = lastFMKeys{key: LastFMAPIKey, secret: LastFMAPISecret}
	}
	token, page, err := s.fm.token(ctx, k)
	if err != nil {
		return "", err
	}
	if own {
		if err := s.cfg.Secrets.Set(secretsFM, "apiKey", k.key); err != nil {
			return "", err
		}
		if err := s.cfg.Secrets.Set(secretsFM, "apiSecret", k.secret); err != nil {
			return "", err
		}
	} else if err := s.cfg.Secrets.Delete(secretsFM); err != nil {
		// An old override would win over the build's key at the session step.
		return "", err
	}
	s.mu.Lock()
	s.fmToken = token
	s.mu.Unlock()
	s.publish()
	return page, nil
}

func (s *Service) FinishLastFM(ctx context.Context) error {
	s.mu.Lock()
	token := s.fmToken
	s.mu.Unlock()
	if token == "" {
		return errors.New("start the Last.fm sign-in first")
	}
	k, err := s.apiKeys()
	if err != nil {
		return err
	}
	user, session, err := s.fm.session(ctx, k, token)
	if err != nil {
		// Not approved yet keeps the token for another try; Last.fm says
		// code 14 then.
		return err
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

func (s *Service) SetLibrary(id string, on bool) error {
	if id == "" {
		return errors.New("library is required")
	}
	return s.update(func(st *Settings) {
		st.DisabledLibraries = slices.DeleteFunc(st.DisabledLibraries, func(x string) bool { return x == id })
		if !on {
			st.DisabledLibraries = append(st.DisabledLibraries, id)
		}
	})
}
