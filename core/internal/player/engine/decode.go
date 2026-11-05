package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

// NeedsMPV is the file_error of an entry in a format the engine doesn't
// decode.
const NeedsMPV = "format requires mpv"

// OutputError is the event sent when the output won't open or fails; file_error
// says why. It stays paused: the next track would fail the same way.
const OutputError = "output-error"

// Containers are the file types the engine decodes, as file extensions and
// server container names.
var Containers = []string{"flac", "mp3", "ogg", "oga", "opus", "wav", "aif", "aiff"}

// Caps on what a file's header may ask for.
const (
	maxChannels = 8
	maxRate     = 384000
)

var (
	errNeedsMPV      = errors.New(NeedsMPV)
	errNotSeekable   = errors.New("this stream can't seek")
	errBadFile       = errors.New("not a valid audio file")
	errUnsupportedFm = errors.New("unsupported audio layout")
)

type format struct{ rate, channels int }

func (f format) valid() bool {
	return f.rate > 0 && f.rate <= maxRate && f.channels > 0 && f.channels <= maxChannels
}

// decoder yields interleaved float32 frames in [-1, 1].
type decoder interface {
	format() format
	// read fills dst with whole frames; io.EOF at the end.
	read(dst []float32) (int, error)
	// seek moves to a frame; errNotSeekable on a stream that can't.
	seek(frame int64) error
	// length is the file's length in frames; 0 when unknown.
	length() int64
}

// input is an opened file or stream.
type input struct {
	r        io.Reader
	seekable bool
	path     string // local files only
	close    func() error
}

func (in input) seeker() io.ReadSeeker {
	if !in.seekable {
		return nil
	}
	rs, _ := in.r.(io.ReadSeeker)
	return rs
}

func openInput(ctx context.Context, rawURL string) (input, error) {
	u, err := url.Parse(rawURL)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		f, err := openHTTP(ctx, rawURL)
		if err != nil {
			return input{}, err
		}
		return input{r: f, seekable: f.ranges, close: f.Close}, nil
	}
	path := strings.TrimPrefix(rawURL, "file://")
	f, err := os.Open(path)
	if err != nil {
		return input{}, err
	}
	return input{r: f, seekable: true, path: path, close: f.Close}, nil
}

// sniff reads the head of in and picks a decoder by content, never by name.
func newDecoder(in input) (decoder, error) {
	head := make([]byte, 64)
	n, err := io.ReadFull(in.r, head)
	switch {
	case errors.Is(err, io.EOF):
		return nil, errBadFile
	case err != nil && !errors.Is(err, io.ErrUnexpectedEOF):
		return nil, fmt.Errorf("read: %w", err)
	}
	head = head[:n]
	if rs := in.seeker(); rs != nil {
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	} else {
		in.r = io.MultiReader(bytes.NewReader(head), in.r)
	}
	switch sniff(head) {
	case "flac":
		return newFLAC(in)
	case "mp3":
		return newMP3(in)
	case "vorbis":
		return newVorbis(in)
	case "opus":
		return newOpus(in)
	case "wav":
		return newWAV(in)
	case "aiff":
		return newAIFF(in)
	}
	return nil, errNeedsMPV
}

func sniff(h []byte) string {
	switch {
	case bytes.HasPrefix(h, []byte("fLaC")):
		return "flac"
	case bytes.HasPrefix(h, []byte("OggS")):
		switch {
		case bytes.Contains(h, []byte("OpusHead")):
			return "opus"
		case bytes.Contains(h, []byte("\x01vorbis")):
			return "vorbis"
		}
	case len(h) >= 12 && string(h[:4]) == "RIFF" && string(h[8:12]) == "WAVE":
		return "wav"
	case len(h) >= 12 && string(h[:4]) == "FORM" && (string(h[8:12]) == "AIFF" || string(h[8:12]) == "AIFC"):
		return "aiff"
	case bytes.HasPrefix(h, []byte("ID3")):
		return "mp3"
	case len(h) >= 2 && h[0] == 0xFF && h[1]&0xE6 == 0xE2:
		// MPEG audio frame sync, layer III.
		return "mp3"
	}
	return ""
}

// skipFrames reads and drops n frames, for a seek on a stream that can't.
func skipFrames(d decoder, n int64) error {
	buf := make([]float32, 4096*d.format().channels)
	for n > 0 {
		want := min(int64(len(buf)), n*int64(d.format().channels))
		got, err := d.read(buf[:want])
		n -= int64(got / d.format().channels)
		if err != nil {
			return err
		}
	}
	return nil
}
