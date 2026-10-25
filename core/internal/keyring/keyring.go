// Package keyring keeps server credentials in the desktop's Secret Service
// (GNOME Keyring, KWallet, KeePassXC). It implements source.Secrets. Without
// a Secret Service nothing new is stored: there is no plaintext fallback.
package keyring

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	appID  = "io.github.conava.DuxPlayer"
	schema = appID + ".Credential"
)

var (
	ErrLocked = errors.New("the keyring is locked; unlock it and try again")
	// ErrUnavailable means no Secret Service runs on the session bus.
	ErrUnavailable = errors.New("no keyring (Secret Service) is running; start GNOME Keyring, KWallet or KeePassXC to store server passwords")
)

type backend interface {
	get(attrs map[string]string) (string, bool, error)
	set(label string, attrs map[string]string, value string) error
	delete(attrs map[string]string) error
}

// Store looks for the Secret Service on use until it finds one, so a
// keyring that starts after the daemon is picked up without a restart. A
// failed look is remembered for retryAfter: without a keyring every library
// build and scrobble would wait on the bus again.
type Store struct {
	mu     sync.Mutex
	b      backend
	err    error
	failed time.Time
	open   func() (backend, error)
}

const retryAfter = 30 * time.Second

// Open returns a store that connects to the Secret Service on first use.
func Open() *Store {
	return &Store{open: func() (backend, error) {
		// Not returned directly: a nil *secretService would be a non-nil backend.
		svc, err := openSecretService()
		if err != nil {
			return nil, err
		}
		return svc, nil
	}}
}

// backend dials outside the lock, so callers don't queue behind a slow bus.
func (s *Store) backend() (backend, error) {
	s.mu.Lock()
	b, err, failed := s.b, s.err, s.failed
	s.mu.Unlock()
	if b != nil {
		return b, nil
	}
	if err != nil && time.Since(failed) < retryAfter {
		return nil, err
	}
	b, err = s.open()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.err, s.failed = fmt.Errorf("%w (%v)", ErrUnavailable, err), time.Now()
		return nil, s.err
	}
	if s.b == nil {
		s.b = b
	}
	return s.b, nil
}

func attrs(libraryID, key string) map[string]string {
	a := map[string]string{"xdg:schema": schema, "application": appID, "library": libraryID}
	if key != "" {
		a["key"] = key
	}
	return a
}

func (s *Store) Get(libraryID, key string) (string, error) {
	b, err := s.backend()
	if err != nil {
		return "", err
	}
	v, ok, err := b.get(attrs(libraryID, key))
	switch {
	case err != nil:
		return "", fmt.Errorf("keyring: %w", err)
	case !ok:
		return "", fmt.Errorf("no %s stored in the keyring", key)
	}
	return v, nil
}

func (s *Store) Set(libraryID, key, value string) error {
	b, err := s.backend()
	if err != nil {
		return err
	}
	label := fmt.Sprintf("Dux Player: library %s (%s)", libraryID, key)
	if err := b.set(label, attrs(libraryID, key), value); err != nil {
		return fmt.Errorf("keyring: %w", err)
	}
	return nil
}

// Delete removes every secret of a library.
func (s *Store) Delete(libraryID string) error {
	b, err := s.backend()
	if errors.Is(err, ErrUnavailable) {
		return nil
	}
	if err != nil {
		return err
	}
	return b.delete(attrs(libraryID, ""))
}
