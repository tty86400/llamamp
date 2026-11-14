// LLamAmp resurrects the MP3-era desktop music player for the modern
// desktop: a single pure-Go binary with a classic Winamp-style skin, real
// spectrum analysis, and local-first playback. No accounts, no cloud —
// your files, your machine.
package main

import (
	"log"
	"sync"
	"time"

	"llamamp/audio"

	"github.com/egoist/mygo"
)

// UIState is the single payload pushed to the frontend every 50 ms: it
// carries the spectrum bars and the transport state in one event, so the
// UI never needs to poll.
type UIState struct {
	Bands    []float64      `json:"bands"`    // 32 values in [0,1]
	Peaks    []float64      `json:"peaks"`    // falling peak caps
	Position float64        `json:"position"` // seconds
	Duration float64        `json:"duration"` // seconds
	State    string         `json:"state"`
	Track    audio.TrackMeta `json:"track"`
	Index    int            `json:"index"` // playlist index, -1 when none
}

// Tick carries UIState to the page.
var Tick = mygo.NewEvent[UIState]("tick")

// Service is bound to the frontend: `mygo generate` turns its exported
// methods into typed TypeScript functions in src/mygo.ts.
type Service struct {
	eng *audio.Engine

	win *mygo.Window // set once the window exists

	mu     sync.Mutex
	tracks []audio.TrackMeta
	folder string
	index  int // -1 when nothing selected
}

// FolderReply is what PickFolder returns.
type FolderReply struct {
	Folder string            `json:"folder"`
	Tracks []audio.TrackMeta `json:"tracks"`
}

// PickFolder opens a native directory picker and scans it for audio.
func (s *Service) PickFolder() (FolderReply, error) {
	dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Title:       "Choose a music folder",
		Directory:   true,
		ButtonLabel: "Play this folder",
	})
	if err != nil {
		return FolderReply{}, err
	}
	if len(dirs) == 0 {
		return FolderReply{}, nil // cancelled
	}
	tracks, err := audio.ScanFolder(dirs[0])
	if err != nil {
		return FolderReply{}, err
	}
	s.mu.Lock()
	s.folder = dirs[0]
	s.tracks = tracks
	s.index = -1
	s.mu.Unlock()
	return FolderReply{Folder: dirs[0], Tracks: tracks}, nil
}

// Load selects a playlist entry by index and loads it (stopped, not playing).
func (s *Service) Load(index int) (audio.Status, error) {
	s.mu.Lock()
	tracks := s.tracks
	s.mu.Unlock()
	if index < 0 || index >= len(tracks) {
		return s.eng.Status(), nil
	}
	status, err := s.eng.Load(tracks[index].Path)
	if err != nil {
		return status, err
	}
	s.mu.Lock()
	s.index = index
	s.mu.Unlock()
	return status, nil
}

// Play starts or resumes the current track.
func (s *Service) Play() (audio.Status, error) { return s.eng.Play() }

// Pause suspends playback.
func (s *Service) Pause() audio.Status { return s.eng.Pause() }

// Stop rewinds to the start of the track.
func (s *Service) Stop() audio.Status { return s.eng.Stop() }

// Seek jumps to a position in seconds.
func (s *Service) Seek(seconds float64) (audio.Status, error) { return s.eng.Seek(seconds) }

// SetVolume takes the slider value 0..100.
func (s *Service) SetVolume(percent float64) audio.Status { return s.eng.SetVolume(percent) }

// Snapshot returns the transport state; used for the initial page load.
func (s *Service) Snapshot() audio.Status { return s.eng.Status() }

// Playlist returns the current folder's tracks.
func (s *Service) Playlist() []audio.TrackMeta {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tracks
}

// onTrackEnd advances to the next track and keeps playing. Runs from the
// engine's EOF watcher goroutine.
func (s *Service) onTrackEnd() {
	s.mu.Lock()
	index := s.index
	tracks := s.tracks
	s.mu.Unlock()
	if len(tracks) == 0 {
		return
	}
	next := index + 1
	if next >= len(tracks) {
		next = 0 // loop the playlist
	}
	status, err := s.eng.Load(tracks[next].Path)
	if err != nil {
		log.Printf("auto-advance: %v", err)
		return
	}
	if _, err := s.eng.Play(); err != nil {
		log.Printf("auto-advance play: %v", err)
		return
	}
	s.mu.Lock()
	s.index = next
	s.mu.Unlock()
	s.emitStatus(status)
}

// emitStatus pushes a status change to the page. Safe to call before the
// window exists — Emit just reports an error then.
func (s *Service) emitStatus(status audio.Status) {
	if s.win == nil {
		return
	}
	state := UIState{
		Position: status.Position,
		Duration: status.Duration,
		State:    string(status.State),
		Track:    status.Track,
	}
	s.mu.Lock()
	index := s.index
	s.mu.Unlock()
	state.Index = index
	if err := Tick.Emit(s.win, state); err != nil {
		log.Printf("emit status: %v", err)
	}
}

// pushLoop streams spectrum + transport state to the page at 20 Hz.
func (s *Service) pushLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		win := s.win
		if win == nil {
			continue
		}
		bands, peaks := s.eng.Spectrum().Analyse()
		status := s.eng.Status()
		s.mu.Lock()
		index := s.index
		s.mu.Unlock()
		state := UIState{
			Bands:    toSlice(bands),
			Peaks:    toSlice(peaks),
			Position: status.Position,
			Duration: status.Duration,
			State:    string(status.State),
			Track:    status.Track,
			Index:    index,
		}
		if Tick.Emit(win, state) != nil {
			return // window closed
		}
	}
}

func toSlice(arr [audio.Bands]float64) []float64 {
	return append([]float64{}, arr[:]...)
}

func main() {
	eng, err := audio.NewEngine()
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()

	svc := &Service{eng: eng, index: -1}
	eng.SetOnTrackEnd(svc.onTrackEnd)
	mygo.Bind(svc)

	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:           "LLamAmp — it really whips the llama's ass",
			Width:           760,
			Height:          480,
			MinWidth:        560,
			MinHeight:       360,
			BackgroundColor: "#211a23",
			StateKey:        "main",
			URL:             "/",
		})
		svc.win = win
		go svc.pushLoop()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
