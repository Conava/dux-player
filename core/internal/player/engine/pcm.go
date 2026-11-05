package engine

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
)

// pcm reads uncompressed samples from a WAV or AIFF data chunk.
type pcm struct {
	in     input
	f      format
	bits   int
	float  bool
	big    bool
	start  int64 // file offset of the first frame
	frames int64
	pos    int64
	buf    []byte
}

func (d *pcm) format() format { return d.f }
func (d *pcm) length() int64  { return d.frames }

func (d *pcm) frameBytes() int { return d.f.channels * d.bits / 8 }

func (d *pcm) read(dst []float32) (int, error) {
	frames := min(int64(len(dst)/d.f.channels), d.frames-d.pos)
	if frames <= 0 {
		return 0, io.EOF
	}
	need := int(frames) * d.frameBytes()
	if cap(d.buf) < need {
		d.buf = make([]byte, need)
	}
	n, err := io.ReadFull(d.in.r, d.buf[:need])
	n -= n % d.frameBytes()
	size := d.bits / 8
	for i := 0; i < n/size; i++ {
		dst[i] = d.sample(d.buf[i*size : (i+1)*size])
	}
	d.pos += int64(n / d.frameBytes())
	if errors.Is(err, io.ErrUnexpectedEOF) || (err == io.EOF && n == 0) {
		// A truncated file ends where its data does.
		d.frames = d.pos
		err = nil
		if n == 0 {
			err = io.EOF
		}
	}
	return n / size, err
}

func (d *pcm) sample(b []byte) float32 {
	order := binary.ByteOrder(binary.LittleEndian)
	if d.big {
		order = binary.BigEndian
	}
	switch {
	case d.float && d.bits == 32:
		return float32(math.Float32frombits(order.Uint32(b)))
	case d.float && d.bits == 64:
		return float32(math.Float64frombits(order.Uint64(b)))
	case d.bits == 8 && !d.big:
		return float32(int(b[0])-128) / 128
	case d.bits == 8:
		return float32(int8(b[0])) / 128
	case d.bits == 16:
		return float32(int16(order.Uint16(b))) / (1 << 15)
	case d.bits == 24:
		var v int32
		if d.big {
			v = int32(b[0])<<24 | int32(b[1])<<16 | int32(b[2])<<8
		} else {
			v = int32(b[2])<<24 | int32(b[1])<<16 | int32(b[0])<<8
		}
		return float32(v) / (1 << 31)
	default:
		return float32(int32(order.Uint32(b))) / (1 << 31)
	}
}

func (d *pcm) seek(frame int64) error {
	rs := d.in.seeker()
	if rs == nil {
		return errNotSeekable
	}
	frame = min(max(frame, 0), d.frames)
	if _, err := rs.Seek(d.start+frame*int64(d.frameBytes()), io.SeekStart); err != nil {
		return err
	}
	d.pos = frame
	return nil
}

func (d *pcm) check() error {
	if !d.f.valid() {
		return errUnsupportedFm
	}
	switch {
	case d.float && d.bits != 32 && d.bits != 64:
		return errUnsupportedFm
	case !d.float && d.bits != 8 && d.bits != 16 && d.bits != 24 && d.bits != 32:
		return errUnsupportedFm
	}
	return nil
}

// chunks walks RIFF/IFF chunks after the 12-byte file header. fn may read
// a chunk (padded to an even size) whole and return true for read; the
// others are skipped. On done the reader stays at that chunk's data, whose
// file offset is returned.
func chunks(r io.Reader, order binary.ByteOrder, fn func(id string, size int64) (read, done bool, err error)) (int64, error) {
	off := int64(12)
	var hdr [8]byte
	for {
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return 0, errBadFile
		}
		id, size := string(hdr[:4]), int64(order.Uint32(hdr[4:]))
		off += 8
		read, done, err := fn(id, size)
		switch {
		case err != nil:
			return 0, err
		case done:
			return off, nil
		}
		padded := size + size&1
		if !read {
			if _, err := io.CopyN(io.Discard, r, padded); err != nil {
				return 0, errBadFile
			}
		}
		off += padded
	}
}

func newWAV(in input) (decoder, error) {
	d := &pcm{in: in}
	var hdr [12]byte
	if _, err := io.ReadFull(in.r, hdr[:]); err != nil {
		return nil, errBadFile
	}
	haveFmt := false
	var dataSize int64
	start, err := chunks(in.r, binary.LittleEndian, func(id string, size int64) (bool, bool, error) {
		switch id {
		case "fmt ":
			if size < 16 || size > 64 {
				return false, false, errBadFile
			}
			b := make([]byte, size+size&1)
			if _, err := io.ReadFull(in.r, b); err != nil {
				return false, false, errBadFile
			}
			tag := binary.LittleEndian.Uint16(b)
			if tag == 0xFFFE && size >= 26 {
				// WAVE_FORMAT_EXTENSIBLE: the subformat GUID starts with the tag.
				tag = binary.LittleEndian.Uint16(b[24:])
			}
			d.f = format{rate: int(binary.LittleEndian.Uint32(b[4:])), channels: int(binary.LittleEndian.Uint16(b[2:]))}
			d.bits = int(binary.LittleEndian.Uint16(b[14:]))
			switch tag {
			case 1:
			case 3:
				d.float = true
			default:
				return false, false, errNeedsMPV
			}
			haveFmt = true
			return true, false, nil
		case "data":
			if !haveFmt {
				return false, false, errBadFile
			}
			dataSize = size
			return false, true, nil
		}
		return false, false, nil
	})
	if err != nil {
		return nil, err
	}
	if err := d.check(); err != nil {
		return nil, err
	}
	d.start = start
	d.frames = dataSize / int64(d.frameBytes())
	return d, nil
}

func newAIFF(in input) (decoder, error) {
	d := &pcm{in: in, big: true}
	var hdr [12]byte
	if _, err := io.ReadFull(in.r, hdr[:]); err != nil {
		return nil, errBadFile
	}
	aifc := string(hdr[8:12]) == "AIFC"
	haveComm := false
	start, err := chunks(in.r, binary.BigEndian, func(id string, size int64) (bool, bool, error) {
		switch id {
		case "COMM":
			if size < 18 || size > 256 {
				return false, false, errBadFile
			}
			b := make([]byte, size+size&1)
			if _, err := io.ReadFull(in.r, b); err != nil {
				return false, false, errBadFile
			}
			d.f.channels = int(binary.BigEndian.Uint16(b))
			d.frames = int64(binary.BigEndian.Uint32(b[2:]))
			d.bits = int(binary.BigEndian.Uint16(b[6:]))
			d.f.rate = int(extended(b[8:18]))
			if aifc {
				if size < 22 {
					return false, false, errBadFile
				}
				switch string(b[18:22]) {
				case "NONE":
				case "sowt":
					d.big = false
				case "fl32", "FL32":
					d.float = true
				default:
					return false, false, errNeedsMPV
				}
			}
			// Sample sizes round up to whole bytes.
			d.bits = (d.bits + 7) / 8 * 8
			haveComm = true
			return true, false, nil
		case "SSND":
			if !haveComm {
				return false, false, errBadFile
			}
			var b [8]byte
			if _, err := io.ReadFull(in.r, b[:]); err != nil {
				return false, false, errBadFile
			}
			if off := int64(binary.BigEndian.Uint32(b[:])); off > 0 {
				if _, err := io.CopyN(io.Discard, in.r, off); err != nil {
					return false, false, errBadFile
				}
				d.start = off
			}
			if avail := (size - 8 - d.start) / int64(max(d.f.channels*d.bits/8, 1)); avail < d.frames {
				d.frames = max(avail, 0)
			}
			return false, true, nil
		}
		return false, false, nil
	})
	if err != nil {
		return nil, err
	}
	if err := d.check(); err != nil {
		return nil, err
	}
	d.start += start + 8
	return d, nil
}

// extended decodes an 80-bit IEEE 754 extended float (AIFF's sample rate).
func extended(b []byte) float64 {
	exp := int(binary.BigEndian.Uint16(b)&0x7FFF) - 16383 - 63
	mant := binary.BigEndian.Uint64(b[2:])
	v := math.Ldexp(float64(mant), exp)
	if b[0]&0x80 != 0 {
		v = -v
	}
	if v < 0 || v > maxRate || math.IsNaN(v) {
		return 0
	}
	return v
}
