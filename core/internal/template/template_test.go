package template

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Conava/dux-player/core/internal/source"
)

func TestCases(t *testing.T) {
	data, err := os.ReadFile("testdata/cases.json")
	require.NoError(t, err)
	var table struct {
		Fields map[string]string `json:"fields"`
		Cases  []struct{ Name, Template, Want string }
	}
	require.NoError(t, json.Unmarshal(data, &table))
	require.NotEmpty(t, table.Cases)
	for _, c := range table.Cases {
		t.Run(c.Name, func(t *testing.T) {
			assert.Equal(t, c.Want, Format(c.Template, table.Fields))
		})
	}
}

func TestOutputIsCapped(t *testing.T) {
	got := Format("%a%%a%%a%", map[string]string{"a": strings.Repeat("x", 300)})
	assert.Len(t, got, maxOutput)
}

func TestTrackFieldsKeepZeroPlays(t *testing.T) {
	f := TrackFields(source.Track{Year: 0})
	require.Equal(t, "0", f["playcount"])
	require.Equal(t, "", f["year"])
}
