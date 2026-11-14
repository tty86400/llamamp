package audio

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// State is the transport state reported to the UI.
type State string

const (
	StateStopped State = "stopped"
	StatePlaying State = "playing"
	StatePaused  State = "paused"
)

// Status is the full snapshot the UI polls or receives as an event.
type Status struct {
	State    State     `json:"state"`
	Position float64   `json:"position"` // seconds
	Duration float64   `json:"duration"` // seconds, 0 when unknown
	Track    TrackMeta `json:"track"`
	Error    string    `json:"error,omitempty"`
}

// Engine owns the oto context, the current track, and the spectrum tap.
type Engine struct {
	mu     sync.Mutex
	ctx    *oto.Context
	player *oto.Player
	src    *normalized
	spec   *Spectrum

	state State
	track TrackMeta
	vol   float64

	onTrackEnd func() // set by the app; fires on natural EOF
}

// NewEngine creates the audio output. oto hands back a ready channel; we
// wait for it here so callers never see a half-initialized context.
func NewEngine() (*Engine, error) {
	op := &oto.NewContextOptions{
		SampleRate:   targetRate,
		ChannelCount: targetCh,
		Format:       oto.FormatSignedInt16LE,
	}
	ctx, ready, err := oto.NewContext(op)
	if err != nil {
		return nil, err
	}
	<-ready
	e := &Engine{ctx: ctx, spec: NewSpectrum(), state: StateStopped, vol: 0.75}
	return e, nil
}

// SetOnTrackEnd registers the EOF callback. Called once at startup.
func (e *Engine) SetOnTrackEnd(fn func()) {
	e.mu.Lock()
	e.onTrackEnd = fn
	e.mu.Unlock()
	e.watchEnd()
}

// Spectrum exposes the analyser for the UI push loop.
func (e *Engine) Spectrum() *Spectrum { return e.spec }

// watchEnd polls player activity; oto has no completion callback, so the
// natural end of a track shows up as "was playing, player went idle".
func (e *Engine) watchEnd() {
	t := time.NewTicker(150 * time.Millisecond)
	go func() {
		for range t.C {
			e.mu.Lock()
			playing := e.state == StatePlaying
			player := e.player
			e.mu.Unlock()
			if !playing || player == nil || player.IsPlaying() {
				continue
			}
			e.mu.Lock()
			fn := e.onTrackEnd
			e.state = StateStopped
			e.mu.Unlock()
			if fn != nil {
				fn()
			}
		}
	}()
}

// Load opens a file, replacing whatever is playing.
func (e *Engine) Load(path string) (Status, error) {
	src, err := openSource(path)
	if err != nil {
		return e.Status(), err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.stopLocked()
	norm := newNormalized(src)
	norm.tapFn = e.spec.Tap
	player := e.ctx.NewPlayer(norm)
	player.SetVolume(e.vol)

	e.src = norm
	e.player = player
	e.track = readTags(path)
	e.track.Duration = int(norm.TotalSeconds() + 0.5)
	e.state = StateStopped
	return e.statusLocked(), nil
}

// Play starts or resumes playback.
func (e *Engine) Play() (Status, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.player == nil {
		return e.statusLocked(), fmt.Errorf("no track loaded")
	}
	e.player.Play()
	e.state = StatePlaying
	return e.statusLocked(), nil
}

// Pause suspends playback, keeping the position.
func (e *Engine) Pause() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.player != nil && e.state == StatePlaying {
		e.player.Pause()
		e.state = StatePaused
	}
	return e.statusLocked()
}

// Stop halts playback and rewinds to the start of the track.
func (e *Engine) Stop() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopLocked()
	return e.statusLocked()
}

// stopLocked pauses and seeks back to zero. Callers must hold e.mu.
func (e *Engine) stopLocked() {
	if e.player != nil {
		e.player.Pause()
		if e.src != nil {
			e.src.SeekTo(0)
		}
	}
	e.state = StateStopped
}

// Seek jumps to a position in the current track.
func (e *Engine) Seek(seconds float64) (Status, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.src == nil {
		return e.statusLocked(), fmt.Errorf("no track loaded")
	}
	if _, err := e.src.SeekTo(seconds); err != nil {
		return e.statusLocked(), err
	}
	return e.statusLocked(), nil
}

// SetVolume takes 0..100 from the UI slider.
func (e *Engine) SetVolume(percent float64) Status {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.vol = percent / 100
	if e.player != nil {
		e.player.SetVolume(e.vol)
	}
	return e.statusLocked()
}

// Status snapshots the current state; safe from any goroutine.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.statusLocked()
}

func (e *Engine) statusLocked() Status {
	s := Status{State: e.state, Track: e.track}
	if e.src != nil {
		s.Position = e.src.Position()
		s.Duration = e.src.TotalSeconds()
	}
	return s
}

// Close tears down output; called on app exit.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.player != nil {
		e.player.Close()
	}
}

// ScanFolder lists playable tracks in dir. Durations are filled in when
// each track loads (reading them here would fully decode every WAV), so
// the playlist shows them as unknown until played. A stray non-audio file
// never breaks the scan.
func ScanFolder(dir string) ([]TrackMeta, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var tracks []TrackMeta
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		switch extOf(ent.Name()) {
		case ".mp3", ".wav":
		default:
			continue
		}
		tracks = append(tracks, readTags(dir+string(os.PathSeparator)+ent.Name()))
	}
	if len(tracks) == 0 {
		log.Printf("scan: no mp3/wav files in %s", dir)
	}
	return tracks, nil
}
