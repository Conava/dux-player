package keyring

import (
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeService struct{ items map[string]string }

func key(a map[string]string) string { return a["library"] + "/" + a["key"] }

func (f *fakeService) get(a map[string]string) (string, bool, error) {
	v, ok := f.items[key(a)]
	return v, ok, nil
}

func (f *fakeService) set(_ string, a map[string]string, v string) error {
	f.items[key(a)] = v
	return nil
}

func (f *fakeService) delete(map[string]string) error { return nil }

// Without a keyring nothing is stored, and the bus isn't asked again on
// every call; one that starts later is picked up.
func TestMissingKeyring(t *testing.T) {
	var svc *fakeService
	dials := 0
	s := &Store{open: func() (backend, error) {
		dials++
		if svc == nil {
			return nil, errors.New("no bus")
		}
		return svc, nil
	}}
	require.ErrorIs(t, s.Set("lib", "password", "pw"), ErrUnavailable)
	_, err := s.Get("lib", "password")
	require.ErrorIs(t, err, ErrUnavailable)
	assert.Equal(t, 1, dials)

	svc = &fakeService{items: map[string]string{}}
	s.failed = s.failed.Add(-retryAfter)
	require.NoError(t, s.Set("lib", "password", "pw"))
	got, err := s.Get("lib", "password")
	require.NoError(t, err)
	assert.Equal(t, "pw", got)
	assert.Equal(t, 2, dials)
}

// A bus that accepts and never answers must not hang the caller.
func TestSilentBusTimesOut(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "bus")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path="+sock)
	old := connectTimeout
	connectTimeout = 200 * time.Millisecond
	defer func() { connectTimeout = old }()

	start := time.Now()
	_, err = openSecretService()
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}
