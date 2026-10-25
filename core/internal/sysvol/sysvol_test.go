package sysvol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	cases := []struct {
		out     string
		vol     float64
		muted   bool
		wantErr bool
	}{
		{out: "Volume: 0.40\n", vol: 40},
		{out: "Volume: 1.00 [MUTED]\n", vol: 100, muted: true},
		{out: "Volume: 1.25\n", vol: 125},
		{out: "Translate: 0.40\n", wantErr: true},
		{out: "Volume: loud\n", wantErr: true},
		{out: "", wantErr: true},
	}
	for _, tc := range cases {
		vol, muted, err := parse(tc.out)
		if tc.wantErr {
			assert.Error(t, err, tc.out)
			continue
		}
		assert.NoError(t, err, tc.out)
		assert.InDelta(t, tc.vol, vol, 0.001, tc.out)
		assert.Equal(t, tc.muted, muted, tc.out)
	}
}
