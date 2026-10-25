package ipc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

func TestRefs(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		want    []source.Ref
		wantErr bool
	}{
		{name: "absent", value: nil, want: nil},
		{name: "one cli string", value: "lib:abc", want: []source.Ref{{Library: "lib", ID: "abc"}}},
		{
			name:  "file ids keep their colons",
			value: []any{"file:/music/a:b.flac"},
			want:  []source.Ref{{Library: "file", ID: "/music/a:b.flac"}},
		},
		{
			name:  "ui objects",
			value: []any{map[string]any{"library": "nd", "id": "x1"}},
			want:  []source.Ref{{Library: "nd", ID: "x1"}},
		},
		{name: "no library", value: ":abc", wantErr: true},
		{name: "object without id", value: []any{map[string]any{"library": "nd"}}, wantErr: true},
		{name: "number", value: []any{3.0}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Refs(map[string]any{"tracks": tt.value}, "tracks")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
