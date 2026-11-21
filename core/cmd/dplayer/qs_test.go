package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPickRuntime(t *testing.T) {
	loads := func() (string, error) { return "/lib/x.so", nil }
	broken := func() (string, error) { return "", errors.New("version `Qt_6.11' not found") }
	probed := func() (string, error) { t.Fatal("a forced qs must not load the library"); return "", nil }
	hasQS := func(string) (string, error) { return "/usr/bin/qs", nil }
	noQS := func(string) (string, error) { return "", errors.New("not found") }

	tests := []struct {
		name   string
		force  bool
		load   func() (string, error)
		look   func(string) (string, error)
		want   runtimeChoice
		wantEr error
	}{
		{"library loads", false, loads, hasQS, runtimeChoice{lib: "/lib/x.so"}, nil},
		{"library loads without qs installed", false, loads, noQS, runtimeChoice{lib: "/lib/x.so"}, nil},
		{"load error falls back with the reason", false, broken, hasQS, runtimeChoice{qs: "/usr/bin/qs", reason: "version `Qt_6.11' not found"}, nil},
		{"forced qs is silent and skips the library", true, probed, hasQS, runtimeChoice{qs: "/usr/bin/qs"}, nil},
		{"neither runtime", false, broken, noQS, runtimeChoice{}, errNoRuntime},
		{"forced qs that is missing", true, probed, noQS, runtimeChoice{}, errNoRuntime},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pickRuntime(tt.force, tt.load, tt.look)
			if tt.wantEr != nil {
				require.ErrorIs(t, err, tt.wantEr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
