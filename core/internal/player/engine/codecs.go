package engine

import (
	"encoding/binary"
	"errors"
	"io"
	"math"

	"github.com/hajimehoshi/go-mp3"
	"github.com/jfreymuth/oggvorbis"
	"github.com/mewkiz/flac"
	"github.com/thesyncim/gopus"
	"github.com/thesyncim/gopus/container/ogg"
)

type flacDec struct {
	s     *flac.Stream
	f     format
	scale float32
	// left holds decoded frames not yet read; skip drops the head of the
	// next block after a seek.
	left []float32
	skip int
}

func newFLAC(in input) (decoder, error) {
	var s *flac.Stream
	var err error
	if rs := in.seeker(); rs != nil {
		s, err = flac.NewSeek(rs)
	} else {
		s, err = flac.New(in.r)
	}
	if err != nil {
		return nil, errBadFile
	}
	d := &flacDec{s: s, f: format{rate: int(s.Info.SampleRate), channels: int(s.Info.NChannels)}}
	if !d.f.valid() || s.Info.BitsPerSample < 4 || s.Info.BitsPerSample > 32 {
		return nil, errUnsupportedFm
	}
	d.scale = 1 / float32(uint64(1)<<(s.Info.BitsPerSample-1))
	return d, nil
}

func (d *flacDec) format() format { return d.f }
func (d *flacDec) length() int64  { return int64(d.s.Info.NSamples) }

func (d *flacDec) read(dst []float32) (int, error) {
	for len(d.left) == 0 {
		fr, err := d.s.ParseNext()
		if err != nil {
			return 0, eof(err)
		}
		if len(fr.Subframes) != d.f.channels {
			return 0, errBadFile
		}
		n := int(fr.BlockSize)
		buf := make([]float32, 0, n*d.f.channels)
		for i := range n {
			for _, sf := range fr.Subframes {
				if i >= len(sf.Samples) {
					return 0, errBadFile
				}
				buf = append(buf, float32(sf.Samples[i])*d.scale)
			}
		}
		skip := min(d.skip, n)
		d.skip -= skip
		d.left = buf[skip*d.f.channels:]
	}
	n := copy(dst[:len(dst)-len(dst)%d.f.channels], d.left)
	d.left = d.left[n:]
	return n, nil
}

func (d *flacDec) seek(frame int64) error {
	start, err := d.s.Seek(uint64(max(frame, 0)))
	if errors.Is(err, flac.ErrNoSeeker) {
		return errNotSeekable
	}
	if err != nil {
		return err
	}
	d.left, d.skip = nil, int(uint64(frame)-start)
	return nil
}

// mp3Dec wraps go-mp3, which always yields 16-bit stereo.
type mp3Dec struct {
	d   *mp3.Decoder
	buf []byte
}

func newMP3(in input) (decoder, error) {
	// With a seeker go-mp3 indexes every frame up front, which also gives
	// the length.
	r := in.r
	if rs := in.seeker(); rs == nil {
		r = struct{ io.Reader }{in.r}
	}
	d, err := mp3.NewDecoder(r)
	if err != nil {
		return nil, errBadFile
	}
	if !(format{rate: d.SampleRate(), channels: 2}).valid() {
		return nil, errUnsupportedFm
	}
	return &mp3Dec{d: d}, nil
}

func (d *mp3Dec) format() format { return format{rate: d.d.SampleRate(), channels: 2} }

func (d *mp3Dec) length() int64 {
	if n := d.d.Length(); n > 0 {
		return n / 4
	}
	return 0
}

func (d *mp3Dec) read(dst []float32) (int, error) {
	want := (len(dst) / 2) * 4
	if cap(d.buf) < want {
		d.buf = make([]byte, want)
	}
	n, err := io.ReadFull(d.d, d.buf[:want])
	n -= n % 4
	for i := 0; i < n/2; i++ {
		dst[i] = float32(int16(binary.LittleEndian.Uint16(d.buf[2*i:]))) / (1 << 15)
	}
	if n > 0 {
		return n / 2, nil
	}
	return 0, eof(err)
}

func (d *mp3Dec) seek(frame int64) error {
	if d.d.Length() < 0 {
		return errNotSeekable
	}
	_, err := d.d.Seek(max(frame, 0)*4, io.SeekStart)
	return err
}

type vorbisDec struct {
	r *oggvorbis.Reader
}

func newVorbis(in input) (decoder, error) {
	r := in.r
	if rs := in.seeker(); rs == nil {
		r = struct{ io.Reader }{in.r}
	}
	v, err := oggvorbis.NewReader(r)
	if err != nil {
		return nil, errBadFile
	}
	if !(format{rate: v.SampleRate(), channels: v.Channels()}).valid() {
		return nil, errUnsupportedFm
	}
	return &vorbisDec{r: v}, nil
}

func (d *vorbisDec) format() format { return format{rate: d.r.SampleRate(), channels: d.r.Channels()} }
func (d *vorbisDec) length() int64  { return d.r.Length() }

func (d *vorbisDec) read(dst []float32) (int, error) {
	n, err := d.r.Read(dst[:len(dst)-len(dst)%d.r.Channels()])
	if n > 0 {
		return n, nil
	}
	return 0, eof(err)
}

func (d *vorbisDec) seek(frame int64) error {
	if d.r.Length() == 0 {
		return errNotSeekable
	}
	return d.r.SetPosition(max(frame, 0))
}

// opusDec decodes Ogg Opus at 48 kHz, mono or stereo. Granule positions
// count the pre-skip, which is dropped from the output.
type opusDec struct {
	in   input
	or   *ogg.Reader
	dec  *gopus.Decoder
	ch   int
	gain float32
	pre  int64
	// at is the output frame of the next sample left holds (or decodes);
	// output before want is dropped.
	at, want int64
	out      []float32
	left     []float32
	frames   int64
}

const opusRate = 48000

func newOpus(in input) (decoder, error) {
	var frames int64
	if rs := in.seeker(); rs != nil {
		frames = opusLength(rs)
		if _, err := rs.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	}
	or, err := ogg.NewReader(in.r)
	if err != nil {
		return nil, errBadFile
	}
	if or.Header.MappingFamily != ogg.MappingFamilyRTP {
		return nil, errUnsupportedFm
	}
	ch := int(or.Channels())
	cfg := gopus.DefaultDecoderConfig(opusRate, ch)
	dec, err := gopus.NewDecoder(cfg)
	if err != nil {
		return nil, errUnsupportedFm
	}
	pre := int64(or.PreSkip())
	return &opusDec{in: in, or: or, dec: dec, ch: ch, pre: pre, at: -pre, frames: frames - pre,
		gain: float32(math.Pow(10, float64(or.Header.OutputGain)/256/20)),
		out:  make([]float32, cfg.MaxPacketSamples*ch)}, nil
}

func (d *opusDec) format() format { return format{rate: opusRate, channels: d.ch} }
func (d *opusDec) length() int64  { return max(d.frames, 0) }

func (d *opusDec) read(dst []float32) (int, error) {
	for len(d.left) == 0 {
		pkt, gp, err := d.or.ReadPacket()
		if err != nil {
			return 0, eof(err)
		}
		n, err := d.dec.Decode(pkt, d.out)
		if err != nil {
			return 0, errBadFile
		}
		// A packet's granule is where it ends; the first page's may be short.
		if gp > 0 {
			d.at = int64(gp) - int64(n) - d.pre
		}
		buf := d.out[:n*d.ch]
		if drop := d.want - d.at; drop > 0 {
			drop = min(drop, int64(n))
			buf = buf[drop*int64(d.ch):]
			d.at += drop
		}
		d.at += int64(len(buf) / d.ch)
		d.left = buf
	}
	n := copy(dst[:len(dst)-len(dst)%d.ch], d.left)
	for i := range dst[:n] {
		dst[i] *= d.gain
	}
	d.left = d.left[n:]
	return n, nil
}

func (d *opusDec) seek(frame int64) error {
	if d.in.seeker() == nil {
		return errNotSeekable
	}
	frame = max(frame, 0)
	// 80 ms of pre-roll lets the decoder converge (RFC 7845, 4.6).
	target := max(frame+d.pre-3840, 0)
	if err := d.or.SeekGranule(uint64(target)); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	d.dec.Reset()
	d.left, d.want, d.at = nil, frame, -d.pre
	return nil
}

// opusLength reads the last page's granule position.
func opusLength(rs io.ReadSeeker) int64 {
	end, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return 0
	}
	n := min(end, 64<<10)
	if _, err := rs.Seek(end-n, io.SeekStart); err != nil {
		return 0
	}
	tail := make([]byte, n)
	if _, err := io.ReadFull(rs, tail); err != nil {
		return 0
	}
	for i := len(tail) - 27; i >= 0; i-- {
		if string(tail[i:i+4]) == "OggS" {
			if g := int64(binary.LittleEndian.Uint64(tail[i+6:])); g > 0 {
				return g
			}
		}
	}
	return 0
}

// eof maps a decoder's end of input, a cut-off file's too, to io.EOF.
func eof(err error) error {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return io.EOF
	}
	return err
}
