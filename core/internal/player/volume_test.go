package player

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSystem struct {
	mu     sync.Mutex
	volume float64
	muted  bool
}

func (f *fakeSystem) Get(context.Context) (float64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.volume, f.muted, nil
}

func (f *fakeSystem) SetVolume(_ context.Context, v float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.volume = v
	return nil
}

func (f *fakeSystem) SetMute(_ context.Context, m bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.muted = m
	return nil
}

// With the system target, volume and mute drive the system output and
// status reports it; the app volume stays as it was.
func TestVolumeTarget(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		noSystem   bool
		wantSystem float64
		wantStatus float64
		wantTarget string
	}{
		{name: "system", target: VolumeSystem, wantSystem: 60, wantStatus: 60, wantTarget: VolumeSystem},
		{name: "app", target: VolumeApp, wantSystem: 30, wantStatus: 50, wantTarget: VolumeApp},
		{name: "system without a mixer", target: VolumeSystem, noSystem: true, wantSystem: 30, wantStatus: 50, wantTarget: VolumeApp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sys := &fakeSystem{volume: 30}
			opts := DefaultOptions()
			opts.VolumeTarget = tc.target
			cfg := Config{Binary: filepath.Join(t.TempDir(), "no-mpv"), Options: opts, System: sys}
			if tc.noSystem {
				cfg.System = nil
			}
			p := New(cfg)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			go p.Run(ctx)
			require.NoError(t, p.Configure(ctx, opts))

			require.NoError(t, p.SetVolume(ctx, 50, false))
			require.NoError(t, p.SetVolume(ctx, 10, true))
			if tc.target == VolumeSystem && !tc.noSystem {
				// changed outside the player, then by the player again
				require.NoError(t, sys.SetVolume(ctx, 50))
				require.NoError(t, p.SystemChanged(ctx, 50, false))
				require.NoError(t, p.SetVolume(ctx, 10, true))
			} else {
				require.NoError(t, p.SetVolume(ctx, -10, true))
			}
			require.NoError(t, p.SetMute(ctx, true))
			st, err := p.Status(ctx)
			require.NoError(t, err)
			assert.InDelta(t, tc.wantStatus, st.Volume, 0.001)
			assert.Equal(t, tc.wantTarget, st.VolumeTarget)
			assert.True(t, st.Muted)
			assert.Equal(t, !tc.noSystem, st.SystemVolume)
			assert.InDelta(t, tc.wantSystem, sys.volume, 0.001)
			assert.Equal(t, tc.wantTarget == VolumeSystem, sys.muted)
		})
	}
}
