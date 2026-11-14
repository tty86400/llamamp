package audio

import (
	"math"
	"math/cmplx"
	"sync"
)

// fftSize is the analysis window: 2048 mono samples ≈ 46 ms at 44.1 kHz.
const fftSize = 2048

// Bands is the number of spectrum bands the UI renders.
const Bands = 32

// ringBuffer keeps the most recent mono samples fed to the speaker, so the
// analyser always sees what the listener is currently hearing.
type ringBuffer struct {
	mu   sync.Mutex
	data [fftSize * 2]int16 // twice the window: read-side slack
	pos  int
}

func (r *ringBuffer) write(left, right int16) {
	// Mono downmix — spectrum analysis is mono.
	mix := int32(left) + int32(right)
	if mix > 32767 {
		mix = 32767
	} else if mix < -32768 {
		mix = -32768
	}
	r.mu.Lock()
	r.data[r.pos] = int16(mix)
	r.pos = (r.pos + 1) % len(r.data)
	r.mu.Unlock()
}

func (r *ringBuffer) latest(n int) []int16 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]int16, n)
	// pos is the next write slot, so the previous n slots end at pos-1.
	start := (r.pos - n + len(r.data)) % len(r.data)
	for i := 0; i < n; i++ {
		out[i] = r.data[(start+i)%len(r.data)]
	}
	return out
}

// Spectrum turns a rolling PCM window into log-spaced frequency bands for
// the LED-style visualizer. Its tap is called from the playback goroutine
// via normalized.tapFn; Analyse is called on the UI push interval.
type Spectrum struct {
	ring ringBuffer

	// Precomputed tables — reused across analyses.
	window []float64
	edges  [Bands + 1]int // band → FFT bin range
	peaks  [Bands]float64 // falling peak caps, classic Winamp behaviour
}

func NewSpectrum() *Spectrum {
	s := &Spectrum{}
	// Hamming window tames spectral leakage.
	s.window = make([]float64, fftSize)
	for i := range s.window {
		s.window[i] = 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/(fftSize-1))
	}
	// 32 bands distributed logarithmically from 50 Hz to 16 kHz.
	const fMin, fMax = 50.0, 16000.0
	for i := 0; i <= Bands; i++ {
		f := fMin * math.Pow(fMax/fMin, float64(i)/Bands)
		bin := int(f / (targetRate / fftSize))
		if bin > fftSize/2 {
			bin = fftSize / 2
		}
		if i > 0 && bin <= s.edges[i-1] {
			bin = s.edges[i-1] + 1
		}
		if bin > fftSize/2 {
			bin = fftSize / 2
		}
		s.edges[i] = bin
	}
	return s
}

// Tap receives one stereo output frame. Called from the playback path.
func (s *Spectrum) Tap(left, right int16) { s.ring.write(left, right) }

// Analyse returns Bands values in [0,1] plus the smoothed peaks.
func (s *Spectrum) Analyse() (bands, peaks [Bands]float64) {
	samples := s.ring.latest(fftSize)
	in := make([]float64, fftSize)
	for i, v := range samples {
		in[i] = float64(v) / 32768 * s.window[i]
	}
	coef := fftRadix2(in)

	for b := 0; b < Bands; b++ {
		lo, hi := s.edges[b], s.edges[b+1]
		if hi <= lo {
			bands[b] = 0
			continue
		}
		sum := 0.0
		for i := lo; i < hi; i++ {
			sum += cmplx.Abs(coef[i])
		}
		avg := sum / float64(hi-lo)
		// Map magnitude to dB then normalize: floor −50 dB, ceiling +25 dB.
		db := 20 * math.Log10(avg+1e-9)
		v := (db + 50) / 75
		if v < 0 {
			v = 0
		} else if v > 1 {
			v = 1
		}
		// Slight perceptual boost for the upper register.
		v = math.Pow(v, 1.2)
		bands[b] = v

		// Peak caps fall slowly, like the classic visualizer.
		if v > s.peaks[b] {
			s.peaks[b] = v
		} else {
			s.peaks[b] -= 0.03
			if s.peaks[b] < 0 {
				s.peaks[b] = 0
			}
		}
	}
	copy(peaks[:], s.peaks[:])
	return bands, peaks
}

// fftRadix2 is an in-place iterative radix-2 FFT. Size must be a power of
// two; fftSize is a compile-time constant so no runtime check is needed.
func fftRadix2(in []float64) []complex128 {
	n := len(in)
	out := make([]complex128, n)
	for i, v := range in {
		out[i] = complex(v, 0)
	}
	// Bit reversal permutation.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			out[i], out[j] = out[j], out[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		wLen := cmplx.Exp(complex(0, angle))
		for start := 0; start < n; start += length {
			w := complex(1, 0)
			half := length / 2
			for k := start; k < start+half; k++ {
				even := out[k]
				odd := out[k+half] * w
				out[k] = even + odd
				out[k+half] = even - odd
				w *= wLen
			}
		}
	}
	return out[:n/2] // upper half mirrors the lower for real input
}
