package jellyfin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

func TestFactoryNeedsNoRequestToReportQuickConnect(t *testing.T) {
	src, err := Factory(source.Library{ID: "x", Kind: Kind, Config: map[string]any{"url": "https://caps.invalid", "username": "caps", "auth": "quickConnect"}}, source.Env{Secrets: memSecrets{}, CacheDir: t.TempDir()})
	require.NoError(t, err)
	defer src.Close()
	assert.Contains(t, source.Capabilities(src), source.CapQuickConnect)
}
