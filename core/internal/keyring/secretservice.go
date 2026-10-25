package keyring

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	busName         = "org.freedesktop.secrets"
	servicePath     = dbus.ObjectPath("/org/freedesktop/secrets")
	serviceIface    = "org.freedesktop.Secret.Service"
	collectionIface = "org.freedesktop.Secret.Collection"
	itemIface       = "org.freedesktop.Secret.Item"
	promptIface     = "org.freedesktop.Secret.Prompt"
	sessionIface    = "org.freedesktop.Secret.Session"

	loginCollection = dbus.ObjectPath("/org/freedesktop/secrets/collection/login")

	callTimeout = 10 * time.Second
	// promptTimeout includes the user typing a keyring master password.
	promptTimeout = 2 * time.Minute
)

// secretService talks org.freedesktop.secrets (GNOME Keyring, KWallet,
// KeePassXC) over the session bus. Values travel in a "plain" session: the
// bus is local to the user.
type secretService struct {
	conn *dbus.Conn
}

type wireSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

func openSecretService() (*secretService, error) {
	conn, err := connectSession()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	s := &secretService{conn: conn}
	session, err := s.openSession()
	if err != nil {
		conn.Close()
		return nil, err
	}
	s.closeSession(session)
	return s, nil
}

// connectTimeout bounds the connect and handshake: a bus that accepts but
// never answers would otherwise hold up daemon startup.
var connectTimeout = callTimeout

// connectSession dials a private connection; the conn's context only ends
// when the handshake overran, which closes the conn.
func connectSession() (*dbus.Conn, error) {
	ctx, cancel := context.WithCancel(context.Background())
	t := time.AfterFunc(connectTimeout, cancel)
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if !t.Stop() {
		if conn != nil {
			conn.Close()
		}
		return nil, fmt.Errorf("no answer within %s", connectTimeout)
	}
	return conn, err
}

func (s *secretService) call(path dbus.ObjectPath, method string, args ...any) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	return s.conn.Object(busName, path).CallWithContext(ctx, method, 0, args...)
}

func (s *secretService) openSession() (dbus.ObjectPath, error) {
	var out dbus.Variant
	var session dbus.ObjectPath
	if err := s.call(servicePath, serviceIface+".OpenSession", "plain", dbus.MakeVariant("")).Store(&out, &session); err != nil {
		return "", fmt.Errorf("secret service: %w", err)
	}
	return session, nil
}

func (s *secretService) closeSession(session dbus.ObjectPath) {
	s.call(session, sessionIface+".Close")
}

func (s *secretService) search(attrs map[string]string) (unlocked, locked []dbus.ObjectPath, err error) {
	err = s.call(servicePath, serviceIface+".SearchItems", attrs).Store(&unlocked, &locked)
	return unlocked, locked, err
}

func (s *secretService) unlock(paths []dbus.ObjectPath) error {
	var done []dbus.ObjectPath
	var prompt dbus.ObjectPath
	if err := s.call(servicePath, serviceIface+".Unlock", paths).Store(&done, &prompt); err != nil {
		return err
	}
	_, err := s.completePrompt(prompt)
	return err
}

func (s *secretService) get(attrs map[string]string) (string, bool, error) {
	unlocked, locked, err := s.search(attrs)
	if err != nil {
		return "", false, err
	}
	items := append(unlocked, locked...)
	if len(items) == 0 {
		// KeePassXC hides every item while its database is locked.
		if s.defaultLocked() {
			return "", false, ErrLocked
		}
		return "", false, nil
	}
	if len(unlocked) == 0 {
		if err := s.unlock(locked[:1]); err != nil {
			return "", false, err
		}
	}
	session, err := s.openSession()
	if err != nil {
		return "", false, err
	}
	defer s.closeSession(session)
	var secret wireSecret
	if err := s.call(items[0], itemIface+".GetSecret", session).Store(&secret); err != nil {
		return "", false, err
	}
	return string(secret.Value), true, nil
}

func (s *secretService) defaultCollection() dbus.ObjectPath {
	var path dbus.ObjectPath
	if err := s.call(servicePath, serviceIface+".ReadAlias", "default").Store(&path); err != nil || path == "/" {
		return loginCollection
	}
	return path
}

func (s *secretService) defaultLocked() bool {
	v, err := s.conn.Object(busName, s.defaultCollection()).GetProperty(collectionIface + ".Locked")
	if err != nil {
		return false
	}
	locked, _ := v.Value().(bool)
	return locked
}

// ensureDefault returns the default collection, creating one when the
// service has none yet (a fresh keyring).
func (s *secretService) ensureDefault() (dbus.ObjectPath, error) {
	collection := s.defaultCollection()
	v, err := s.conn.Object(busName, servicePath).GetProperty(serviceIface + ".Collections")
	if paths, ok := v.Value().([]dbus.ObjectPath); err == nil && ok && slices.Contains(paths, collection) {
		return collection, nil
	}
	props := map[string]dbus.Variant{collectionIface + ".Label": dbus.MakeVariant("Login")}
	var created, prompt dbus.ObjectPath
	if err := s.call(servicePath, serviceIface+".CreateCollection", props, "default").Store(&created, &prompt); err != nil {
		return "", err
	}
	if created != "/" {
		return created, nil
	}
	result, err := s.completePrompt(prompt)
	if err != nil {
		return "", err
	}
	if created, _ = result.Value().(dbus.ObjectPath); created == "" {
		return "", errors.New("the keyring created no collection")
	}
	return created, nil
}

func (s *secretService) set(label string, attrs map[string]string, value string) error {
	collection, err := s.ensureDefault()
	if err != nil {
		return err
	}
	if err := s.unlock([]dbus.ObjectPath{collection}); err != nil {
		return err
	}
	session, err := s.openSession()
	if err != nil {
		return err
	}
	defer s.closeSession(session)
	props := map[string]dbus.Variant{
		itemIface + ".Label":      dbus.MakeVariant(label),
		itemIface + ".Attributes": dbus.MakeVariant(attrs),
	}
	secret := wireSecret{Session: session, Parameters: []byte{}, Value: []byte(value), ContentType: "text/plain"}
	var item, prompt dbus.ObjectPath
	if err := s.call(collection, collectionIface+".CreateItem", props, secret, true).Store(&item, &prompt); err != nil {
		return err
	}
	_, err = s.completePrompt(prompt)
	return err
}

func (s *secretService) delete(attrs map[string]string) error {
	unlocked, locked, err := s.search(attrs)
	if err != nil {
		return err
	}
	for _, item := range append(unlocked, locked...) {
		var prompt dbus.ObjectPath
		if err := s.call(item, itemIface+".Delete").Store(&prompt); err != nil {
			return err
		}
		if _, err := s.completePrompt(prompt); err != nil {
			return err
		}
	}
	return nil
}

// completePrompt subscribes before calling Prompt so Completed can't be
// missed, and filters by path because the bus connection is shared.
func (s *secretService) completePrompt(prompt dbus.ObjectPath) (dbus.Variant, error) {
	if prompt == "" || prompt == "/" {
		return dbus.Variant{}, nil
	}
	match := []dbus.MatchOption{
		dbus.WithMatchObjectPath(prompt),
		dbus.WithMatchInterface(promptIface),
		dbus.WithMatchMember("Completed"),
	}
	if err := s.conn.AddMatchSignal(match...); err != nil {
		return dbus.Variant{}, err
	}
	defer func() { _ = s.conn.RemoveMatchSignal(match...) }()
	signals := make(chan *dbus.Signal, 16)
	s.conn.Signal(signals)
	defer s.conn.RemoveSignal(signals)

	if err := s.call(prompt, promptIface+".Prompt", "").Err; err != nil {
		return dbus.Variant{}, err
	}
	timeout := time.NewTimer(promptTimeout)
	defer timeout.Stop()
	for {
		select {
		case sig := <-signals:
			if sig == nil || sig.Path != prompt || sig.Name != promptIface+".Completed" || len(sig.Body) != 2 {
				continue
			}
			if dismissed, _ := sig.Body[0].(bool); dismissed {
				return dbus.Variant{}, errors.New("the keyring prompt was dismissed")
			}
			result, _ := sig.Body[1].(dbus.Variant)
			return result, nil
		case <-timeout.C:
			return dbus.Variant{}, fmt.Errorf("the keyring prompt got no answer within %s", promptTimeout)
		}
	}
}
