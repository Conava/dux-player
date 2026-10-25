package source

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Requests to a server carry credentials, so a redirect never leaves it.
func TestRedirectsStayOnTheServer(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("followed a redirect to another host", r.URL)
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/away":
			http.Redirect(w, r, other.URL+"/x", http.StatusTemporaryRedirect)
		case "/moved":
			http.Redirect(w, r, "/here", http.StatusFound)
		}
	}))
	defer srv.Close()
	hc := &http.Client{CheckRedirect: SameOrigin}
	_, err := hc.Get(srv.URL + "/away")
	require.Error(t, err)
	resp, err := hc.Get(srv.URL + "/moved")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "/here", resp.Request.URL.Path)
}

func TestServerURL(t *testing.T) {
	for addr, local := range map[string]bool{"http://192.168.1.4:4533": true, "http://nas.local": true, "http://nas": true,
		"http://100.101.1.2": true, "http://music.example.com": false} {
		u, err := ServerURL(addr)
		require.NoError(t, err)
		n, warn := PlainHTTPNotice(u)
		assert.Equal(t, local, !warn, addr)
		if warn {
			assert.Equal(t, NoticePlainHTTP, n.Code)
		}
	}
	_, err := ServerURL("http://u:secret@host")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")
}
