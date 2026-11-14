// Package audio implements the LLamAmp playback engine: file decoding,
// sample normalization, and playback through oto. Everything is pure Go —
// no cgo — so the whole app stays a single cross-compilable binary.
package audio

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/dhowden/tag"
	"github.com/go-audio/wav"
	mp3 "github.com/hajimehoshi/go-mp3"
)

// Target is the format every source is normalized to before hitting oto:
// 44.1 kHz interleaved stereo, signed 16-bit LE.
const (
	targetRate = 44100
	targetCh   = 2
	frameBytes = targetCh * 2 // bytes per stereo frame of decoded output
)

// TrackMeta is what the frontend sees in the playlist.
type TrackMeta struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Title    string `json:"title"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	TrackNum int    `json:"trackNum,omitempty"`
	Duration int    `json:"duration"` // seconds, 0 when unknown before load
}

// source is a decoded PCM stream for one file. Read produces interleaved
// signed 16-bit samples at the source's own rate and channel count.
type source interface {
	io.Reader
	// SeekTo jumps to the given position. Returns the new position in seconds.
	SeekTo(seconds float64) (float64, error)
	// Rate returns the source sample rate.
	Rate() int
	// Channels returns the source channel count.
	Channels() int
	// TotalSeconds returns the duration, or 0 when unknown.
	TotalSeconds() float64
	Close() error
}

// mp3Source wraps go-mp3. The decoder always outputs signed 16-bit LE
// stereo and implements io.Seeker, mapping byte offsets onto frame
// boundaries for us.
type mp3Source struct {
	dec *mp3.Decoder
	f   *os.File
}

func newMP3Source(path string) (*mp3Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	dec, err := mp3.NewDecoder(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &mp3Source{dec: dec, f: f}, nil
}

func (s *mp3Source) Read(p []byte) (int, error) { return s.dec.Read(p) }
func (s *mp3Source) Rate() int                  { return s.dec.SampleRate() }
func (s *mp3Source) Channels() int              { return 2 }
func (s *mp3Source) TotalSeconds() float64 {
	// decoded output is always 16-bit stereo
	return float64(s.dec.Length()) / float64(s.dec.SampleRate()*2*2)
}
func (s *mp3Source) Close() error { return s.f.Close() }

func (s *mp3Source) SeekTo(seconds float64) (float64, error) {
	bps := int64(s.dec.SampleRate()) * 2 * 2
	target := int64(seconds * float64(bps))
	if target < 0 {
		target = 0
	}
	n, err := s.dec.Seek(target, io.SeekStart)
	if err != nil {
		return 0, err
	}
	return float64(n) / float64(bps), nil
}

// wavSource wraps go-audio/wav, loading the whole file into memory. WAV
// files are small enough that streaming buys nothing, and it makes seeking
// trivial and correct. go-audio/wav normalizes every PCM bit depth into
// int samples, so all common WAV variants are accepted.
type wavSource struct {
	data []int16 // interleaved PCM samples
	rate int
	ch   int
	pos  int // sample index into data
}

func newWAVSource(path string) (*wavSource, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := wav.NewDecoder(f)
	if !dec.IsValidFile() {
		return nil, fmt.Errorf("%s: not a valid WAV file", path)
	}
	format := dec.Format()
	buf, err := dec.FullPCMBuffer()
	if err != nil {
		return nil, err
	}
	// FullPCMBuffer hands back int samples in the source's native range;
	// clamp into 16-bit signed space for the normalization chain.
	data := make([]int16, len(buf.Data))
	for i, v := range buf.Data {
		if v > 32767 {
			v = 32767
		} else if v < -32768 {
			v = -32768
		}
		data[i] = int16(v)
	}
	return &wavSource{data: data, rate: format.SampleRate, ch: format.NumChannels}, nil
}

func (s *wavSource) Read(p []byte) (int, error) {
	n := len(p) / 2
	if s.pos >= len(s.data) {
		return 0, io.EOF
	}
	if s.pos+n > len(s.data) {
		n = len(s.data) - s.pos
	}
	for i := 0; i < n; i++ {
		v := s.data[s.pos+i]
		p[2*i] = byte(v)
		p[2*i+1] = byte(v >> 8)
	}
	s.pos += n
	return n * 2, nil
}

func (s *wavSource) Rate() int     { return s.rate }
func (s *wavSource) Channels() int { return s.ch }
func (s *wavSource) Close() error  { return nil }

func (s *wavSource) TotalSeconds() float64 {
	frame := s.rate * s.ch
	if frame == 0 {
		return 0
	}
	return float64(len(s.data)) / float64(frame)
}

func (s *wavSource) SeekTo(seconds float64) (float64, error) {
	frame := s.rate * s.ch
	if frame == 0 {
		return 0, fmt.Errorf("wav: cannot seek, bad format")
	}
	target := int(seconds * float64(frame))
	if target < 0 {
		target = 0
	}
	if target > len(s.data) {
		target = len(s.data)
	}
	s.pos = target
	return float64(target) / float64(frame), nil
}

// openSource picks the right decoder by extension.
func openSource(path string) (source, error) {
	switch extOf(path) {
	case ".mp3":
		return newMP3Source(path)
	case ".wav":
		return newWAVSource(path)
	default:
		return nil, fmt.Errorf("unsupported format: %s (supported: mp3, wav)", extOf(path))
	}
}

// normalized wraps a source and converts everything to the target format:
// mono→stereo duplication and linear-interpolation resampling to 44.1 kHz.
type normalized struct {
	src source
	// srcCh is the source channel count; mono is expanded to stereo when
	// filling the input buffer so Read below only sees stereo frames.
	srcCh  int
	ratio  float64 // srcRate / targetRate
	intPos float64 // fractional read position within the current input pair
	prev   [2]int16
	cur    [2]int16
	hasPair bool

	mu    sync.Mutex
	inBuf []byte
	inPos int

	framesOut int64 // stereo frames emitted since load/seek (position tracking)
	posOffset float64

	tapFn func(left, right int16) // called once per output frame
}

func newNormalized(src source) *normalized {
	return &normalized{
		src:   src,
		srcCh: src.Channels(),
		ratio: float64(src.Rate()) / float64(targetRate),
	}
}

func (n *normalized) Read(p []byte) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	wrote := 0
	for wrote+frameBytes <= len(p) {
		if !n.hasPair {
			n.advancePair()
			if !n.hasPair {
				if wrote == 0 {
					return 0, io.EOF
				}
				return wrote, nil
			}
		}
		outL := n.lerp(n.prev[0], n.cur[0])
		outR := n.lerp(n.prev[1], n.cur[1])
		p[wrote] = byte(outL)
		p[wrote+1] = byte(outL >> 8)
		p[wrote+2] = byte(outR)
		p[wrote+3] = byte(outR >> 8)
		if n.tapFn != nil {
			n.tapFn(outL, outR)
		}
		wrote += frameBytes
		n.framesOut++

		n.intPos += n.ratio
		for n.intPos >= 1 {
			n.intPos--
			n.prev = n.cur
			n.nextPair()
			if !n.hasPair {
				break
			}
		}
	}
	return wrote, nil
}

func (n *normalized) lerp(from, to int16) int16 {
	return int16(int(from) + int(float64(int(to)-int(from))*n.intPos))
}

// advancePair loads the next input stereo pair into prev/cur.
func (n *normalized) advancePair() {
	n.prev = n.cur
	if !n.nextPair() {
		n.hasPair = false
	}
}

// nextPair reads one stereo frame from the input buffer, refilling from
// the source when drained. Returns false at end of stream.
func (n *normalized) nextPair() bool {
	for n.inPos+4 > len(n.inBuf) {
		n.fill()
		if n.inPos+4 > len(n.inBuf) {
			n.hasPair = false
			return false
		}
	}
	n.cur = [2]int16{
		int16(n.inBuf[n.inPos]) | int16(n.inBuf[n.inPos+1])<<8,
		int16(n.inBuf[n.inPos+2]) | int16(n.inBuf[n.inPos+3])<<8,
	}
	n.inPos += 4
	n.hasPair = true
	return true
}

// fill reads more PCM from the source, expanding mono to stereo in place.
func (n *normalized) fill() {
	if n.inBuf == nil {
		n.inBuf = make([]byte, 0, 64*1024)
	}
	raw := make([]byte, 48*1024)
	m, err := n.src.Read(raw)
	if m <= 0 {
		n.inBuf = n.inBuf[:0]
		return
	}
	if err != nil && err != io.EOF {
		n.inBuf = n.inBuf[:0]
		return
	}
	raw = raw[:m]
	if n.srcCh == 1 {
		st := make([]byte, 0, m*2)
		for i := 0; i+1 < m; i += 2 {
			st = append(st, raw[i], raw[i+1], raw[i], raw[i+1])
		}
		raw = st
	}
	n.inBuf = append(n.inBuf[:0], raw...)
	n.inPos = 0
}

func (n *normalized) SeekTo(seconds float64) (float64, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	pos, err := n.src.SeekTo(seconds)
	n.inBuf = n.inBuf[:0]
	n.inPos = 0
	n.hasPair = false
	n.intPos = 0
	n.framesOut = 0
	n.posOffset = pos
	return pos, err
}

// Position returns the playback position in seconds.
func (n *normalized) Position() float64 {
	return n.posOffset + float64(n.framesOut)/float64(targetRate)
}

func (n *normalized) Rate() int             { return n.src.Rate() }
func (n *normalized) Channels() int         { return n.src.Channels() }
func (n *normalized) TotalSeconds() float64 { return n.src.TotalSeconds() }
func (n *normalized) Close() error          { return n.src.Close() }

// TrackDuration reads the duration of a file without playing it.
func TrackDuration(path string) int {
	src, err := openSource(path)
	if err != nil {
		return 0
	}
	defer src.Close()
	return int(src.TotalSeconds() + 0.5)
}

// readTags extracts ID3 metadata, falling back to the file name.
func readTags(path string) TrackMeta {
	meta := TrackMeta{ID: path, Path: path, Title: baseName(path)}
	f, err := os.Open(path)
	if err != nil {
		return meta
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return meta
	}
	if t := m.Title(); t != "" {
		meta.Title = t
	}
	meta.Artist = m.Artist()
	meta.Album = m.Album()
	if tr, _ := m.Track(); tr > 0 {
		meta.TrackNum = tr
	}
	return meta
}

// extOf returns the lowercase extension of path.
func extOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		c := p[i]
		if c == '.' {
			return toLower(p[i:])
		}
		if c == '\\' || c == '/' {
			break
		}
	}
	return ""
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '\\' || p[i] == '/' {
			p = p[i+1:]
			break
		}
	}
	if ext := extOf(p); ext != "" {
		p = p[:len(p)-len(ext)]
	}
	return p
}
