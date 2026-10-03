package input

import (
	"encoding/gob"
	"io"
	"sync"
	"time"
)

// RecordedEvent is a timestamped raw event captured during recording.
type RecordedEvent struct {
	Timestamp time.Duration
	Event     Event
}

// Recorder captures the raw event stream so it can be replayed deterministically.
type Recorder struct {
	mu        sync.Mutex
	events    []RecordedEvent
	recording bool
}

// NewRecorder creates a recorder.
func NewRecorder() *Recorder { return &Recorder{} }

// Start begins recording.
func (r *Recorder) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recording = true
}

// Stop ends recording.
func (r *Recorder) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recording = false
}

// IsRecording reports whether recording is active.
func (r *Recorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.recording
}

// Events returns a copy of the recorded events.
func (r *Recorder) Events() []RecordedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RecordedEvent, len(r.events))
	copy(out, r.events)
	return out
}

// record is called by the sampler each tick.
func (r *Recorder) record(events []Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.recording {
		return
	}
	for _, e := range events {
		r.events = append(r.events, RecordedEvent{Timestamp: e.Timestamp, Event: e})
	}
}

// Reset clears all recorded events.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

// Save writes the recorded events to w in a deterministic binary format.
func (r *Recorder) Save(w io.Writer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	enc := gob.NewEncoder(w)
	return enc.Encode(r.events)
}

// Load reads events previously written by Save, replacing the recording.
func (r *Recorder) Load(rd io.Reader) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var events []RecordedEvent
	if err := gob.NewDecoder(rd).Decode(&events); err != nil {
		return err
	}
	r.events = events
	return nil
}

// ReplayBackend feeds recorded events back through the sampler. Events are
// delivered according to their recorded timestamps, so fixed-tick assignment
// is identical to the original run.
type ReplayBackend struct {
	name   string
	events []RecordedEvent
	pos    int
}

// NewReplayBackend creates a backend that replays the given events.
func NewReplayBackend(name string, events []RecordedEvent) *ReplayBackend {
	return &ReplayBackend{name: name, events: events}
}

// Name returns the backend name.
func (b *ReplayBackend) Name() string { return b.name }

// Poll returns all recorded events with timestamp <= now.
func (b *ReplayBackend) Poll(now time.Duration) []Event {
	var out []Event
	for b.pos < len(b.events) && b.events[b.pos].Timestamp <= now {
		out = append(out, b.events[b.pos].Event)
		b.pos++
	}
	return out
}

// Reset restarts replay from the beginning.
func (b *ReplayBackend) Reset() { b.pos = 0 }
