package loudness

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const summary = `[Parsed_ebur128_0 @ 0x1] t: 3.9  TARGET:-23 LUFS    M: -41.7 S: -41.7     I: -41.6 LUFS       LRA:   0.0 LU
[Parsed_ebur128_0 @ 0x1] Summary:

  Integrated loudness:
    I:         %s LUFS
    Threshold: -51.7 LUFS
`

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		lufs    string
		want    float64
		wantErr bool
	}{
		{name: "quiet track is raised", lufs: "-23.0", want: 5},
		{name: "loud track is lowered", lufs: "-8.5", want: -9.5},
		{name: "silence is capped", lufs: "-70.0", want: maxGain},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parse(fmt.Sprintf(summary, tt.lufs))
			require.NoError(t, err)
			assert.InDelta(t, tt.want, got, 0.001)
		})
	}
	_, err := parse("Invalid data found when processing input")
	assert.Error(t, err)
}

func TestMeterWithoutFFmpeg(t *testing.T) {
	m := &Meter{slot: make(chan struct{}, 1)}
	assert.False(t, m.Available())
	_, err := m.Measure(t.Context(), "/x.flac")
	assert.Error(t, err)
}

// Recorded from ffmpeg 9 with the filters Measure runs.
const (
	logLeadMiddleTrail = `[Parsed_silencedetect_0 @ 0x7f02440015c0] silence_start: 0
[Parsed_silencedetect_0 @ 0x7f02440015c0] silence_end: 2.000045 | silence_duration: 2.000045
[Parsed_silencedetect_0 @ 0x7f02440015c0] silence_start: 6
[Parsed_silencedetect_0 @ 0x7f02440015c0] silence_end: 7.000023 | silence_duration: 1.000023
[Parsed_silencedetect_0 @ 0x7f02440015c0] silence_start: 10
[Parsed_silencedetect_0 @ 0x7f02440015c0] silence_end: 13 | silence_duration: 3
[Parsed_ebur128_1 @ 0x7f02440017c0] Summary:
    I:         -22.0 LUFS
size=N/A time=00:00:13.00 bitrate=N/A speed= 947x elapsed=0:00:00.01
`
	logAllSilent = `[Parsed_silencedetect_0 @ 0x7f2bc80015c0] silence_start: 0
[Parsed_silencedetect_0 @ 0x7f2bc80015c0] silence_end: 5 | silence_duration: 5
    I:         -70.0 LUFS
size=N/A time=00:00:05.00 bitrate=N/A speed=1.29e+03x elapsed=0:00:00.00
`
	logNoSilence = `    I:         -21.8 LUFS
size=N/A time=00:00:05.00 bitrate=N/A speed=1.04e+03x elapsed=0:00:00.00
`
	logMiddleOnly = `[Parsed_silencedetect_0 @ 0x7f3c140015c0] silence_start: 2
[Parsed_silencedetect_0 @ 0x7f3c140015c0] silence_end: 3.500023 | silence_duration: 1.500023
    I:         -22.1 LUFS
size=N/A time=00:00:05.50 bitrate=N/A speed=1.09e+03x elapsed=0:00:00.00
`
)

func TestParseSilence(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   Silence
	}{
		{name: "leading and trailing, not the middle", stderr: logLeadMiddleTrail, want: Silence{Start: 2.000045, End: 10}},
		{name: "all silent plays whole", stderr: logAllSilent},
		{name: "no silence", stderr: logNoSilence},
		{name: "silence only in the middle", stderr: logMiddleOnly},
		{name: "trailing silence too short", stderr: "silence_start: 4.6\nsilence_end: 5 | silence_duration: 0.4\ntime=00:00:05.00\n"},
		{name: "older ffmpeg leaves the trailing span open", stderr: "silence_start: 7\nsize=N/A time=00:00:10.00 bitrate=N/A\n", want: Silence{End: 7}},
		{name: "end unknown keeps the end", stderr: "silence_start: 0\nsilence_end: 1 | silence_duration: 1\nsilence_start: 4\nsilence_end: 5 | silence_duration: 1\n", want: Silence{Start: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseSilence(tt.stderr))
		})
	}
}

// Runs real ffmpeg: one decode yields the gain and the silence.
func TestMeasureFindsSilence(t *testing.T) {
	m := Find()
	if !m.Available() {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "t.flac")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	gen := exec.CommandContext(ctx, m.ffmpeg, "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono:d=1",
		"-f", "lavfi", "-i", "sine=f=440:r=8000:d=2", "-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono:d=1",
		"-filter_complex", "[1]aformat=channel_layouts=mono[s];[0][s][2]concat=n=3:v=0:a=1", path)
	require.NoError(t, gen.Run())
	r, err := m.Measure(ctx, path)
	require.NoError(t, err)
	assert.InDelta(t, 1, r.Silence.Start, 0.05)
	assert.InDelta(t, 3, r.Silence.End, 0.05)
	assert.Less(t, r.Gain, maxGain)
}
