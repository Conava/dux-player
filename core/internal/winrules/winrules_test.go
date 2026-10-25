package winrules

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetect(t *testing.T) {
	for env, want := range map[string]string{
		"HYPRLAND_INSTANCE_SIGNATURE=x": "hyprland",
		"NIRI_SOCKET=/run/niri.sock":    "niri",
		"MANGO_INSTANCE_SIGNATURE=/s":   "mango",
		"XDG_CURRENT_DESKTOP=Mango":     "mango",
		"XDG_CURRENT_DESKTOP=GNOME":     "",
	} {
		k, v, _ := strings.Cut(env, "=")
		assert.Equal(t, want, Detect(func(key string) string {
			if key == k {
				return v
			}
			return ""
		}), env)
	}
}

// The rule is what the shell's own floating rule looks like, for our app id.
func TestArgv(t *testing.T) {
	add := addArgv("niri")
	assert.Equal(t, []string{"dms", "config", "windowrules", "add", "niri"}, add[:5])
	var r map[string]any
	require.NoError(t, json.Unmarshal([]byte(add[5]), &r))
	assert.Equal(t, map[string]any{
		"id": RuleID, "enabled": true, "name": "Dux Player Floating Windows",
		"matchCriteria": map[string]any{"appId": `^(io\.github\.conava\.DuxPlayer)$`},
		"actions":       map[string]any{"openFloating": true},
	}, r)
	assert.Equal(t, []string{"dms", "config", "windowrules", "remove", "hyprland", RuleID}, removeArgv("hyprland"))
}

func TestActive(t *testing.T) {
	for out, want := range map[string]bool{
		`{"rules":[{"id":"dplayer-floating-windows","enabled":true,"actions":{"openFloating":true}}]}`:  true,
		`{"rules":[{"id":"dplayer-floating-windows","actions":{"openFloating":true}}]}`:                 true,
		`{"rules":[{"id":"dplayer-floating-windows","enabled":false,"actions":{"openFloating":true}}]}`: false,
		`{"rules":[{"id":"dplayer-floating-windows","enabled":true,"actions":{}}]}`:                     false,
		`{"rules":[{"id":"dms-floating-windows","enabled":true,"actions":{"openFloating":true}}]}`:      false,
		`{"rules":null}`: false,
	} {
		got, _, err := active(out)
		require.NoError(t, err)
		assert.Equal(t, want, got, out)
	}
}

// listed is a dms list with our rule matching appID.
func listed(appID string, enabled bool) string {
	m, _ := json.Marshal(appID)
	return `{"rules":[{"id":"` + RuleID + `","enabled":` + strconv.FormatBool(enabled) + `,"matchCriteria":{"appId":` + string(m) + `},"actions":{"openFloating":true}}]}`
}

// A seeded start rewrites the rule only when it is on and matches another
// app id (one from before a rename).
func TestSeededStartReplacesOutdatedRule(t *testing.T) {
	old := `^(org\.example\.Old)$`
	for _, tc := range []struct {
		name, list string
		cmds       []string
		active     bool
	}{
		{"current", listed(appIDMatch, true), []string{"list"}, true},
		{"outdated", listed(old, true), []string{"list", "add", "reload"}, true},
		{"outdated but off", listed(old, false), []string{"list"}, false},
		{"absent", `{"rules":[]}`, []string{"list"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, fileName), []byte(`{"floatingSeeded":["hyprland"]}`), 0o600))
			f := &fake{}
			s := service(t, dir, f)
			s.run = func(_ context.Context, argv []string) (string, error) {
				f.cmds = append(f.cmds, argv[3])
				if argv[3] == "add" {
					assert.Equal(t, addArgv("hyprland"), argv)
				}
				return tc.list, nil
			}
			st, err := s.Seed(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tc.cmds, f.cmds)
			assert.Equal(t, tc.active, st.Active)
		})
	}
}

// fake keeps a rules list the way dms would and records every command.
type fake struct {
	on   bool
	cmds []string
}

func (f *fake) run(_ context.Context, argv []string) (string, error) {
	f.cmds = append(f.cmds, argv[3])
	switch argv[3] {
	case "add":
		f.on = true
	case "remove":
		f.on = false
	}
	if f.on {
		return listed(appIDMatch, true), nil
	}
	return `{"rules":[]}`, nil
}

func service(t *testing.T, dir string, f *fake) *Service {
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "test")
	s := New(dir)
	s.shell, s.status.Mode = true, ModeShell
	s.run = f.run
	s.reload = func(context.Context, string) error { f.cmds = append(f.cmds, "reload"); return nil }
	return s
}

// Seeded once per compositor: turning it off sticks across restarts.
func TestSeedOnce(t *testing.T) {
	dir, f := t.TempDir(), &fake{}
	ctx := context.Background()
	st, err := service(t, dir, f).Seed(ctx)
	require.NoError(t, err)
	assert.True(t, st.Active)
	assert.Equal(t, []string{"add", "reload"}, f.cmds)

	s := service(t, dir, f)
	_, err = s.Set(ctx, false)
	require.NoError(t, err)
	f.cmds = nil
	_, err = service(t, dir, f).Seed(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"list"}, f.cmds, "a seeded start only reads the rule")
}

// winrules.status runs in parallel with writes: a list that started before
// a Set must not overwrite what the Set did.
func TestRefreshOverlappingSetKeepsTheSet(t *testing.T) {
	ctx := context.Background()
	s := service(t, t.TempDir(), &fake{})
	listing, release := make(chan struct{}), make(chan struct{})
	s.run = func(_ context.Context, argv []string) (string, error) {
		if argv[3] == "list" {
			close(listing)
			<-release
		}
		return `{"rules":[]}`, nil
	}
	done := make(chan Status)
	go func() {
		st, _ := s.Refresh(ctx)
		done <- st
	}()
	<-listing
	_, err := s.Set(ctx, true)
	require.NoError(t, err)
	close(release)
	assert.True(t, (<-done).Active)
	assert.True(t, s.Status().Active)
}
