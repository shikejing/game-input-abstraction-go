package input

import (
	"sort"
	"sync"
	"time"
)

// Frame is the consistent per-tick output of the sampler. Action states,
// device states and gesture outputs all refer to the same tick.
type Frame struct {
	// Tick is the fixed-tick index (starting at 0).
	Tick uint64
	// Time is the simulated time at the end of the tick.
	Time time.Duration
	// Actions are the evaluated action states keyed by action id.
	Actions map[ActionID]ActionState
	// Devices are deep copies of the connected device states.
	Devices map[DeviceID]*DeviceState
	// Gestures are the gesture outputs for this tick.
	Gestures *GestureFrame
	// Events are the raw events applied during this tick (in order).
	Events []Event
}

// Clone returns a deep copy of the frame.
func (f Frame) Clone() Frame {
	c := Frame{
		Tick:     f.Tick,
		Time:     f.Time,
		Actions:  make(map[ActionID]ActionState, len(f.Actions)),
		Devices:  make(map[DeviceID]*DeviceState, len(f.Devices)),
		Events:   make([]Event, len(f.Events)),
	}
	for k, v := range f.Actions {
		c.Actions[k] = v
	}
	for k, v := range f.Devices {
		c.Devices[k] = v.Clone()
	}
	if f.Gestures != nil {
		gf := &GestureFrame{devices: make(map[DeviceID]*DeviceGestureEvents, len(f.Gestures.devices))}
		for k, v := range f.Gestures.devices {
			cp := *v
			cp.Swipes = append([]SwipeEvent(nil), v.Swipes...)
			gf.devices[k] = &cp
		}
		c.Gestures = gf
	}
	copy(c.Events, f.Events)
	return c
}

type registeredBackend struct {
	id      uint16
	backend Backend
}

// Sampler drives fixed-timestep input sampling. Each Update call accumulates
// real elapsed time and runs zero or more fixed ticks; every tick drains the
// backends, applies events in deterministic order, recognizes gestures and
// evaluates actions.
type Sampler struct {
	mu sync.Mutex

	registry *Registry
	inputMap *InputMap
	scheme   *ResolvedScheme
	gestures *GestureSystem
	recorder *Recorder

	backends []registeredBackend

	tickRate    time.Duration
	accumulator time.Duration
	tick        uint64

	prevActions map[ActionID]ActionState
	current     Frame

	bus *SnapshotBus
}

// NewSampler creates a sampler with the given fixed tick rate and action map.
func NewSampler(tickRate time.Duration, m *InputMap) *Sampler {
	if tickRate <= 0 {
		tickRate = time.Second / 60
	}
	return &Sampler{
		registry:    NewRegistry(),
		inputMap:    m,
		gestures:    NewGestureSystem(),
		tickRate:    tickRate,
		prevActions: make(map[ActionID]ActionState),
		bus:         NewSnapshotBus(),
	}
}

// Registry returns the device registry.
func (s *Sampler) Registry() *Registry { return s.registry }

// Gestures returns the gesture system.
func (s *Sampler) Gestures() *GestureSystem { return s.gestures }

// Bus returns the snapshot bus used for concurrent frame reads.
func (s *Sampler) Bus() *SnapshotBus { return s.bus }

// SetScheme installs a resolved platform scheme; when set, the scheme's map
// is evaluated instead of the primary input map.
func (s *Sampler) SetScheme(scheme *ResolvedScheme) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheme = scheme
}

// SetRecorder installs a recorder and starts recording immediately. All
// subsequently drained events are recorded.
func (s *Sampler) SetRecorder(r *Recorder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorder = r
	if r != nil {
		r.Start()
	}
}

// AddBackend registers an input backend.
func (s *Sampler) AddBackend(b Backend) {
	id := s.registry.RegisterBackend(b.Name())
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backends = append(s.backends, registeredBackend{id: id, backend: b})
}

// TickRate returns the fixed tick duration.
func (s *Sampler) TickRate() time.Duration { return s.tickRate }

// Tick returns the index of the next tick to be evaluated.
func (s *Sampler) Tick() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tick
}

// Update accumulates real elapsed time and runs fixed ticks.
func (s *Sampler) Update(realDt time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accumulator += realDt
	for s.accumulator >= s.tickRate {
		s.accumulator -= s.tickRate
		s.runTickLocked()
	}
}

// runTickLocked executes one fixed tick. Caller holds s.mu.
func (s *Sampler) runTickLocked() {
	now := time.Duration(s.tick+1) * s.tickRate

	// 1. Drain backends (non-blocking) and tag events with backend ids.
	events := make([]Event, 0)
	for _, rb := range s.backends {
		batch := rb.backend.Poll(now)
		for i := range batch {
			batch[i].BackendID = rb.id
		}
		events = append(events, batch...)
	}

	// 2. Deterministic order: timestamp, then backend-local sequence.
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp != events[j].Timestamp {
			return events[i].Timestamp < events[j].Timestamp
		}
		return events[i].Seq < events[j].Seq
	})

	// 3. Record before applying so replay sees the raw stream.
	if s.recorder != nil {
		s.recorder.record(events)
	}

	// 4. Apply events to device states.
	s.registry.Process(events)

	// 5. Gesture recognition.
	gf := s.gestures.Process(now, s.registry)

	// 6. Evaluate actions.
	ctx := &EvalContext{
		Registry: s.registry,
		Gestures: gf,
		Events:   events,
		DT:       s.tickRate,
		Tick:     s.tick,
	}
	actions := s.activeMap().Evaluate(ctx, s.prevActions)
	s.prevActions = actions

	// 7. Build the consistent frame snapshot.
	devs := make(map[DeviceID]*DeviceState)
	for _, st := range s.registry.Devices() {
		devs[st.Info.ID] = st
	}
	frame := Frame{
		Tick:     s.tick,
		Time:     now,
		Actions:  actions,
		Devices:  devs,
		Gestures: gf,
		Events:   events,
	}
	s.current = frame
	s.bus.Publish(frame)
	s.tick++
}

func (s *Sampler) activeMap() *InputMap {
	if s.scheme != nil && s.scheme.Map != nil {
		return s.scheme.Map
	}
	return s.inputMap
}

// Snapshot returns a deep copy of the latest frame.
func (s *Sampler) Snapshot() (Frame, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tick == 0 {
		return Frame{}, false
	}
	return s.current.Clone(), true
}

// Reset clears the sampler state (registry, gestures, timing) for a fresh
// run, e.g. before replaying a recording.
func (s *Sampler) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry = NewRegistry()
	// Re-register backends so ids stay stable.
	bes := s.backends
	s.backends = nil
	for _, rb := range bes {
		rb.id = s.registry.RegisterBackend(rb.backend.Name())
		s.backends = append(s.backends, rb)
	}
	s.gestures = NewGestureSystem()
	s.accumulator = 0
	s.tick = 0
	s.prevActions = make(map[ActionID]ActionState)
	s.current = Frame{}
	s.bus = NewSnapshotBus()
	if s.inputMap != nil {
		s.inputMap.ResetBindings()
	}
}
