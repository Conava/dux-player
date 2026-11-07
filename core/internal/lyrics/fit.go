package lyrics

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/cmplx"
	"slices"
	"sort"

	duxlyrics "github.com/duxlabs-dev/dux-go/lyrics"
)

// Word timing is fitted to the recording: the attacks of the vocals are
// matched against every word start over the whole song. One song's words make
// a pattern a source's offset can't hide in, where its first line alone would.
const (
	// FitRate is the sample rate the fit reads, stereo 32-bit floats.
	FitRate = 16000
	// The frames: 64 ms long, every 20 ms.
	fitWindow = 1024
	fitHop    = 320
	// Vocals, without the bass and the cymbals.
	fitBandLow, fitBandHigh = 300.0, 3400.0
	// fitCentre is how alike both channels must be to count as the centre,
	// where vocals sit; guitars are usually panned away from it.
	fitCentre = 0.8
	// fitReach in frames: how far a source can be off, 2.5 s.
	fitReach = 125
	// fitSmooth in frames is what an attack must stand out from, 2 s.
	fitSmooth = 100
	// fitPulse in frames is how close to a word start an attack counts.
	fitPulse = 3
	// fitEarly in seconds: a frame sees an attack before it reaches the
	// frame's middle, so fits land this much too early.
	fitEarly = 0.06
	// fitConfidence is how far the best fit must stand above all other
	// shifts, in standard deviations. Below it fits on real songs were wrong.
	fitConfidence = 4.0
	// fitMinWords: too few words fit anywhere.
	fitMinWords = 30
	// fitMaxFrames caps a decoded track at an hour.
	fitMaxFrames = 3600 * FitRate / fitHop
)

// attacks reads stereo samples and returns where the vocals start something
// new, one value per frame.
func attacks(r io.Reader) ([]float64, error) {
	in := bufio.NewReaderSize(r, 1<<16)
	left, right := make([]float64, fitWindow), make([]float64, fitWindow)
	read := func(from int) error {
		var pair [8]byte
		for i := from; i < fitWindow; i++ {
			if _, err := io.ReadFull(in, pair[:]); err != nil {
				return err
			}
			left[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(pair[:4])))
			right[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(pair[4:])))
		}
		return nil
	}
	hann := make([]float64, fitWindow)
	for i := range hann {
		hann[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/(fitWindow-1))
	}
	low := int(math.Floor(fitBandLow*fitWindow/FitRate)) + 1
	high := int(math.Ceil(fitBandHigh*fitWindow/FitRate)) - 1
	bins := make([]complex128, fitWindow)
	var energy []float64
	err := read(0)
	for err == nil && len(energy) < fitMaxFrames {
		// both channels in one transform: left as real, right as imaginary
		for i := range bins {
			bins[i] = complex(left[i]*hann[i], right[i]*hann[i])
		}
		fft(bins)
		sum := 0.0
		for k := low; k <= high; k++ {
			a, b := bins[k], cmplx.Conj(bins[fitWindow-k])
			l, r := (a+b)/2, (a-b)/complex(0, 2)
			power := real(l)*real(l) + imag(l)*imag(l) + real(r)*real(r) + imag(r)*imag(r)
			alike := 2 * (real(l)*real(r) + imag(l)*imag(r)) / (power + 1e-9)
			sum += cmplx.Abs((l+r)/2) * min(1, max(0, (alike-fitCentre)/(1-fitCentre)))
		}
		energy = append(energy, math.Log1p(sum*5))
		copy(left, left[fitHop:])
		copy(right, right[fitHop:])
		err = read(fitWindow - fitHop)
	}
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	return onsets(energy), nil
}

// fft transforms in place; len(x) is a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Rect(1, -2*math.Pi/float64(size))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := range size / 2 {
				even, odd := x[start+k], x[start+k+size/2]*w
				x[start+k], x[start+k+size/2] = even+odd, even-odd
				w *= step
			}
		}
	}
}

// onsets keeps each frame's rise, minus the rises around it.
func onsets(energy []float64) []float64 {
	rise := make([]float64, len(energy))
	for i := 1; i < len(energy); i++ {
		rise[i] = max(0, energy[i]-energy[i-1])
	}
	total := make([]float64, len(rise)+1)
	for i, v := range rise {
		total[i+1] = total[i] + v
	}
	out := make([]float64, len(rise))
	for i, v := range rise {
		from, to := max(0, i-fitSmooth/2), min(len(rise), i+fitSmooth/2)
		out[i] = v - (total[to]-total[from])/fitSmooth
	}
	return out
}

// fit is the shift in seconds that lays the word starts on the attacks, and
// how far it stands above all other shifts.
func fit(starts, attacks []float64) (float64, float64) {
	seconds := float64(fitHop) / FitRate
	var pulses []int
	for _, at := range starts {
		frame := int(at / seconds)
		for i := frame; i < frame+fitPulse && i < len(attacks); i++ {
			pulses = append(pulses, i)
		}
	}
	slices.Sort(pulses)
	pulses = slices.Compact(pulses)
	scores := make([]float64, 2*fitReach+1)
	for s := range scores {
		for _, p := range pulses {
			if i := p + s - fitReach; i >= 0 && i < len(attacks) {
				scores[s] += attacks[i]
			}
		}
	}
	best, mean := 0, 0.0
	for s, v := range scores {
		mean += v / float64(len(scores))
		if v > scores[best] {
			best = s
		}
	}
	deviation := 0.0
	for _, v := range scores {
		deviation += (v - mean) * (v - mean) / float64(len(scores))
	}
	sorted := slices.Clone(scores)
	sort.Float64s(sorted)
	spread := math.Sqrt(deviation) + 1e-9
	return float64(best-fitReach)*seconds + fitEarly, (scores[best] - sorted[len(sorted)/2]) / spread
}

func wordStarts(r *duxlyrics.Result) []float64 {
	var out []float64
	for _, line := range r.Synced {
		for _, word := range line.Words {
			out = append(out, float64(word.Start))
		}
	}
	return out
}
