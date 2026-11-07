package library

import (
	"context"
	"errors"
	"maps"
	"time"

	"github.com/duxlabs-dev/dux-go/log"

	"github.com/Conava/dux-player/core/internal/keyring"
	"github.com/Conava/dux-player/core/internal/source"
)

// Quick Connect timing: the server keeps a code for 10 minutes and the
// Jellyfin clients poll every 5 seconds.
const (
	quickConnectEvery    = 5 * time.Second
	quickConnectLifetime = 10 * time.Minute
)

const quickConnectMaxFailures = 3

// Quick Connect states, sent to the UI in quickConnect events.
const (
	QuickConnectApproved = "approved"
	QuickConnectDone     = "done"
	QuickConnectExpired  = "expired"
	QuickConnectOff      = "off"
	QuickConnectFailed   = "failed"
)

type quickConnectSession struct {
	id     string
	cancel context.CancelFunc
}

// QuickConnectKinds lists the kinds that sign in with Quick Connect, so a form
// can offer it before any server answered.
func (m *Manager) QuickConnectKinds() []string {
	kinds := []string{}
	for _, kind := range m.registry.Kinds() {
		config := withAuth(map[string]any{"url": "https://caps.invalid", "username": "caps"})
		lib := source.Library{ID: "caps-" + source.NewID(), Kind: kind, Name: "caps", Enabled: true, Config: config}
		src, err := m.registry.Build(lib, source.Env{Secrets: overlay{nil, map[string]string{}}, CacheDir: m.env.CacheDir, Version: m.env.Version})
		if err != nil {
			continue
		}
		if _, ok := src.(source.QuickConnect); ok {
			kinds = append(kinds, kind)
		}
		src.Close()
	}
	return kinds
}

// failReason words a failure of building a source: a keyring problem reads
// as that, anything else as a server problem.
func failReason(err error) Reason {
	if errors.Is(err, keyring.ErrLocked) || errors.Is(err, keyring.ErrUnavailable) {
		return buildReason(err)
	}
	return testFailure(err)
}

const authQuickConnect = "quickConnect"

var errNoQuickConnectSession = errors.New("signing in with Quick Connect needs a session from a Quick Connect request")

// withAuth copies a config and marks it as signing in with Quick Connect.
func withAuth(config map[string]any) map[string]any {
	out := maps.Clone(config)
	if out == nil {
		out = map[string]any{}
	}
	out["auth"] = authQuickConnect
	return out
}

// QuickConnectStart requests a code from the server of a new library (kind,
// name, config) or of the existing library libID, which signs in again. It
// returns the session id and the code to show. The library is added or
// updated once the code is approved, and a quickConnect event on the
// libraries topic reports how it ended. One request runs at a time.
func (m *Manager) QuickConnectStart(ctx context.Context, libID, kind, name string, config map[string]any) (session, code string, err error) {
	var lib source.Library
	if libID != "" {
		if lib, err = m.library(ctx, libID); err != nil {
			return "", "", err
		}
		config = maps.Clone(lib.Config)
	} else {
		if _, ok := m.registry[kind]; !ok {
			return "", "", errors.New("unknown library kind " + kind)
		}
		if name = source.Clean(name); name == "" {
			return "", "", errors.New("a library needs a name")
		}
		lib = source.Library{ID: source.NewID(), Kind: kind, Name: name, Enabled: true}
	}
	lib.Config = withAuth(config)
	vals := map[string]string{}
	var base source.Secrets
	if libID != "" {
		base = m.env.Secrets
	}
	src, err := m.registry.Build(lib, source.Env{Secrets: overlay{base, vals}, CacheDir: m.env.CacheDir, Version: m.env.Version})
	if err != nil {
		return "", "", failReason(err)
	}
	q, ok := src.(source.QuickConnect)
	if !ok {
		_ = src.Close()
		return "", "", errors.New("this library can't sign in with Quick Connect")
	}
	sctx, cancel := context.WithTimeout(ctx, source.CallTimeout)
	code, secret, err := q.QuickConnectStart(sctx)
	cancel()
	if err != nil {
		_ = src.Close()
		log.Infof("quick connect start: %v", err)
		return "", "", testFailure(err)
	}
	s := &quickConnectSession{id: source.NewID()}
	var rctx context.Context
	rctx, s.cancel = context.WithTimeout(m.ctx, m.qcLifetime)
	m.qcMu.Lock()
	if m.qc != nil {
		m.qc.cancel()
	}
	m.qc = s
	m.qcMu.Unlock()
	if !m.spawn(func() { m.quickConnectRun(rctx, s, src, q, secret, libID == "", lib, vals, code) }) {
		_ = src.Close()
		s.cancel()
		m.qcMu.Lock()
		if m.qc == s {
			m.qc = nil
		}
		m.qcMu.Unlock()
		return "", "", errors.New("the library manager has stopped")
	}
	return s.id, code, nil
}

// QuickConnectCancel ends a request the user gave up on.
func (m *Manager) QuickConnectCancel(session string) {
	m.qcMu.Lock()
	defer m.qcMu.Unlock()
	if m.qc != nil && m.qc.id == session {
		m.qc.cancel()
		m.qc = nil
	}
}

func (m *Manager) quickConnectEvent(s *quickConnectSession, state, code, libID string) {
	m.emit(TopicLibraries, map[string]any{"type": "quickConnect", "session": s.id, "state": state, "code": code, "library": libID})
}

// quickConnectRun polls until the code is approved, then signs in and adds
// or updates the library. It ends silently when the session was replaced or
// cancelled.
func (m *Manager) quickConnectRun(ctx context.Context, s *quickConnectSession, src source.Source, q source.QuickConnect,
	secret string, isNew bool, lib source.Library, vals map[string]string, code string) {
	defer func() {
		_ = src.Close()
		s.cancel()
		m.qcMu.Lock()
		if m.qc == s {
			m.qc = nil
		}
		m.qcMu.Unlock()
	}()
	end := func(state, errCode string) {
		if ctx.Err() == context.Canceled {
			return
		}
		m.emit(TopicLibraries, map[string]any{"type": "quickConnect", "session": s.id, "state": state, "code": code, "library": lib.ID, "errorCode": errCode})
	}
	fail := func(err error) {
		switch {
		case errors.Is(err, source.ErrQuickConnectOff):
			end(QuickConnectOff, "")
		case errors.Is(err, source.ErrQuickConnectExpired):
			end(QuickConnectExpired, "")
		default:
			log.Infof("quick connect: %v", err)
			end(QuickConnectFailed, failReason(err).Code)
		}
	}
	tick := time.NewTicker(m.qcEvery)
	defer tick.Stop()
	failures := 0
	for approved := false; !approved; {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				end(QuickConnectExpired, "")
			}
			return
		case <-tick.C:
		}
		var err error
		approved, err = q.QuickConnectPoll(ctx, secret)
		switch {
		case err == nil:
			failures = 0
		case ctx.Err() != nil:
			continue
		case errors.Is(err, source.ErrQuickConnectOff), errors.Is(err, source.ErrQuickConnectExpired):
			fail(err)
			return
		default:
			if failures++; failures >= quickConnectMaxFailures {
				fail(err)
				return
			}
		}
	}
	m.quickConnectEvent(s, QuickConnectApproved, code, lib.ID)
	user, err := q.QuickConnectFinish(ctx, secret)
	if err != nil {
		fail(err)
		return
	}
	// Past this point the library changes, so the user giving up no longer applies.
	config := maps.Clone(lib.Config)
	config["username"] = user
	actx := context.WithoutCancel(ctx)
	if isNew {
		_, err = m.AddWithID(actx, lib.ID, lib.Kind, lib.Name, config, vals)
	} else {
		_, err = m.Update(actx, lib.ID, "", config, vals)
	}
	if err != nil {
		// Only the keyring and the index are involved now, never the server.
		log.Infof("quick connect: %v", err)
		end(QuickConnectFailed, buildReason(err).Code)
		return
	}
	m.emit(TopicLibraries, map[string]any{"type": "changed"})
	m.quickConnectEvent(s, QuickConnectDone, code, lib.ID)
}
