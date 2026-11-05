package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeWAV writes a 16-bit WAV holding v in every sample.
func writeWAV(t *testing.T, path string, rate, channels int, seconds float64, v float64) {
	t.Helper()
	n := int(seconds*float64(rate)) * channels
	data := make([]byte, 44+2*n)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(36+2*n))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], uint16(channels))
	binary.LittleEndian.PutUint32(data[24:], uint32(rate))
	binary.LittleEndian.PutUint32(data[28:], uint32(rate*channels*2))
	binary.LittleEndian.PutUint16(data[32:], uint16(channels*2))
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(2*n))
	s := int16(math.Round(v * 32767))
	for i := range n {
		binary.LittleEndian.PutUint16(data[44+2*i:], uint16(s))
	}
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func openFile(t *testing.T, path string) decoder {
	t.Helper()
	in, err := openInput(context.Background(), path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = in.close() })
	d, err := newDecoder(in)
	require.NoError(t, err)
	return d
}

// firstSound is the first frame of d louder than -26 dB.
func firstSound(t *testing.T, d decoder) int64 {
	t.Helper()
	ch := d.format().channels
	buf := make([]float32, 1024*ch)
	var at int64
	for {
		n, err := d.read(buf)
		for i := 0; i < n; i += ch {
			if math.Abs(float64(buf[i])) > 0.05 {
				return at + int64(i/ch)
			}
		}
		at += int64(n / ch)
		if err != nil {
			require.ErrorIs(t, err, io.EOF)
			return -1
		}
	}
}

// The fixtures are 0.1 s of silence, then a tone, 0.5 s in all (testdata,
// made with ffmpeg). Each decoder must report the length, start the sound
// where it is (Opus: after its pre-skip) and seek to the frame asked for.
func TestDecoders(t *testing.T) {
	for _, tt := range []struct {
		file string
		// tolerance in seconds; MP3 carries encoder delay and padding.
		tol float64
	}{
		{"tone.flac", 0.001},
		{"tone.ogg", 0.005},
		{"tone.opus", 0.003},
		{"tone.mp3", 0.06},
	} {
		t.Run(tt.file, func(t *testing.T) {
			path := filepath.Join("testdata", tt.file)
			d := openFile(t, path)
			rate := float64(d.format().rate)
			assert.InDelta(t, 0.5, float64(d.length())/rate, tt.tol, "length")
			assert.InDelta(t, 0.1, float64(firstSound(t, d))/rate, tt.tol, "sound starts")

			d = openFile(t, path)
			require.NoError(t, d.seek(int64(0.05*rate)))
			assert.InDelta(t, 0.05, float64(firstSound(t, d))/rate, tt.tol, "sound after a seek")
		})
	}
}

func TestSniff(t *testing.T) {
	for _, tt := range []struct {
		head string
		want string
	}{
		{"fLaC\x00\x00\x00\x22", "flac"},
		{"ID3\x04\x00", "mp3"},
		{"\xff\xfb\x90\x00", "mp3"},
		{"OggS\x00\x02" + string(make([]byte, 22)) + "OpusHead", "opus"},
		{"OggS\x00\x02" + string(make([]byte, 22)) + "\x01vorbis", "vorbis"},
		{"OggS\x00\x02" + string(make([]byte, 22)) + "\x7fFLAC", ""},
		{"RIFF\x00\x00\x00\x00WAVEfmt ", "wav"},
		{"FORM\x00\x00\x00\x00AIFFCOMM", "aiff"},
		{"\x00\x00\x00\x20ftypM4A ", ""},
		{"wvpk", ""},
		{"MAC \x96\x0f", ""},
	} {
		assert.Equal(t, tt.want, sniff([]byte(tt.head)), "%q", tt.head)
	}
}

// Seeking over http: with Range support both ways, without only forward.
func TestHTTPSeek(t *testing.T) {
	body := make([]byte, 1<<20)
	for i := range body {
		body[i] = byte(i * 7)
	}
	for _, ranges := range []bool{true, false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ranges {
				http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
				return
			}
			_, _ = w.Write(body)
		}))
		f, err := openHTTP(context.Background(), srv.URL)
		require.NoError(t, err)
		assert.Equal(t, ranges, f.ranges)

		read := func(off int64) []byte {
			_, err := f.Seek(off, io.SeekStart)
			require.NoError(t, err)
			b := make([]byte, 16)
			_, err = io.ReadFull(f, b)
			require.NoError(t, err)
			return b
		}
		assert.Equal(t, body[:16], read(0))
		assert.Equal(t, body[700_000:700_016], read(700_000), "forward, ranges %v", ranges)
		if ranges {
			assert.Equal(t, body[100:116], read(100), "backward")
		} else {
			_, err := f.Seek(100, io.SeekStart)
			assert.ErrorIs(t, err, errNotSeekable)
		}
		f.Close()
		srv.Close()
	}
}
