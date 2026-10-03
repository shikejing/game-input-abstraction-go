package input

import (
	"math"
	"time"
)

// SwipeDirection enumerates swipe directions.
type SwipeDirection uint8

const (
	SwipeUp SwipeDirection = iota
	SwipeDown
	SwipeLeft
	SwipeRight
)

// String returns a human readable direction name.
func (d SwipeDirection) String() string {
	switch d {
	case SwipeUp:
		return "up"
	case SwipeDown:
		return "down"
	case SwipeLeft:
		return "left"
	case SwipeRight:
		return "right"
	default:
		return "unknown"
	}
}

// SwipeEvent is emitted when one or more fingers perform a swipe.
type SwipeEvent struct {
	Direction SwipeDirection
	// Distance is the normalized travel distance of the group.
	Distance float32
	// Duration is the elapsed time from first contact to recognition.
	Duration time.Duration
	// Fingers is the number of contacts that participated.
	Fingers int
}

// PinchEvent reports two-finger pinch/spread motion.
type PinchEvent struct {
	// Scale is the current finger distance relative to the initial distance.
	Scale float32
	// Delta is the scale change since the previous tick (negative = pinch).
	Delta float32
}

// DeviceGestureEvents holds all gesture outputs for one device in a tick.
type DeviceGestureEvents struct {
	Swipes []SwipeEvent
	Pinch  PinchEvent
}

// GestureFrame is the per-tick collection of gesture outputs, keyed by device.
type GestureFrame struct {
	devices map[DeviceID]*DeviceGestureEvents
}

// Get returns the gesture events for a device, or nil if none.
func (f *GestureFrame) Get(id DeviceID) *DeviceGestureEvents {
	if f == nil {
		return nil
	}
	return f.devices[id]
}

// ---------------------------------------------------------------------------
// Swipe recognizer
// ---------------------------------------------------------------------------

// SwipeRecognizer detects multi-finger swipes from touch histories.
type SwipeRecognizer struct {
	// MinDistance is the normalized travel distance required to emit a swipe.
	MinDistance float32
	// MaxTime is the maximum allowed duration of the gesture.
	MaxTime time.Duration

	hist map[TouchID]swipeTouch
}

type swipeTouch struct {
	startX, startY float32
	startTime      time.Duration
	reported       bool
}

// NewSwipeRecognizer creates a swipe recognizer with the given thresholds.
func NewSwipeRecognizer(minDistance float32, maxTime time.Duration) *SwipeRecognizer {
	return &SwipeRecognizer{MinDistance: minDistance, MaxTime: maxTime, hist: make(map[TouchID]swipeTouch)}
}

// Reset clears all contact history.
func (r *SwipeRecognizer) Reset() { r.hist = make(map[TouchID]swipeTouch) }

// Process evaluates the current touches and returns the swipes recognized this
// tick.
func (r *SwipeRecognizer) Process(now time.Duration, touches []Touch) []SwipeEvent {
	active := make(map[TouchID]Touch, len(touches))
	for _, t := range touches {
		active[t.ID] = t
		if _, ok := r.hist[t.ID]; !ok {
			r.hist[t.ID] = swipeTouch{startX: t.X, startY: t.Y, startTime: now}
		}
	}
	// Drop history for contacts that lifted.
	for id := range r.hist {
		if _, ok := active[id]; !ok {
			delete(r.hist, id)
		}
	}

	var out []SwipeEvent
	// Find candidate contacts that crossed the distance threshold this tick.
	for id, t := range active {
		h := r.hist[id]
		if h.reported {
			continue
		}
		dx := t.X - h.startX
		dy := t.Y - h.startY
		dist := hypot32(dx, dy)
		if dist < r.MinDistance {
			continue
		}
		if r.MaxTime > 0 && now-h.startTime > r.MaxTime {
			h.reported = true
			r.hist[id] = h
			continue
		}
		dir := swipeDirection(dx, dy)
		// Group with other active contacts moving in the same direction.
		fingers := 1
		var groupDist = dist
		for id2, t2 := range active {
			if id2 == id {
				continue
			}
			h2 := r.hist[id2]
			if h2.reported {
				continue
			}
			dx2 := t2.X - h2.startX
			dy2 := t2.Y - h2.startY
			d2 := hypot32(dx2, dy2)
			if d2 < r.MinDistance*0.6 {
				continue
			}
			if r.MaxTime > 0 && now-h2.startTime > r.MaxTime {
				continue
			}
			if sameDirection(dir, dx, dy, dx2, dy2) {
				fingers++
				groupDist += d2
				h2.reported = true
				r.hist[id2] = h2
			}
		}
		h.reported = true
		r.hist[id] = h
		out = append(out, SwipeEvent{
			Direction: dir,
			Distance:  groupDist / float32(fingers),
			Duration:  now - h.startTime,
			Fingers:   fingers,
		})
	}
	return out
}

func swipeDirection(dx, dy float32) SwipeDirection {
	if math.Abs(float64(dx)) > math.Abs(float64(dy)) {
		if dx > 0 {
			return SwipeRight
		}
		return SwipeLeft
	}
	if dy > 0 {
		return SwipeDown
	}
	return SwipeUp
}

func sameDirection(dir SwipeDirection, dx1, dy1, dx2, dy2 float32) bool {
	switch dir {
	case SwipeRight:
		return dx2 > 0 && math.Abs(float64(dy2)) < math.Abs(float64(dx2))
	case SwipeLeft:
		return dx2 < 0 && math.Abs(float64(dy2)) < math.Abs(float64(dx2))
	case SwipeDown:
		return dy2 > 0 && math.Abs(float64(dx2)) < math.Abs(float64(dy2))
	case SwipeUp:
		return dy2 < 0 && math.Abs(float64(dx2)) < math.Abs(float64(dy2))
	}
	return false
}

// ---------------------------------------------------------------------------
// Pinch recognizer
// ---------------------------------------------------------------------------

// PinchRecognizer detects two-finger pinch/spread gestures.
type PinchRecognizer struct {
	startDist float32
	lastScale float32
	active    bool
}

// NewPinchRecognizer creates a pinch recognizer.
func NewPinchRecognizer() *PinchRecognizer { return &PinchRecognizer{} }

// Reset clears the internal state.
func (r *PinchRecognizer) Reset() { r.startDist = 0; r.lastScale = 1; r.active = false }

// Process evaluates the current touches and returns the pinch event for this
// tick.
func (r *PinchRecognizer) Process(touches []Touch) PinchEvent {
	if len(touches) != 2 {
		r.Reset()
		return PinchEvent{Scale: 1}
	}
	d := hypot32(touches[0].X-touches[1].X, touches[0].Y-touches[1].Y)
	if d <= 0 {
		return PinchEvent{Scale: 1}
	}
	if !r.active {
		r.active = true
		r.startDist = d
		r.lastScale = 1
	}
	scale := d / r.startDist
	delta := scale - r.lastScale
	r.lastScale = scale
	return PinchEvent{Scale: scale, Delta: delta}
}

// ---------------------------------------------------------------------------
// Gesture system
// ---------------------------------------------------------------------------

// GestureSystem owns the per-device recognizers and produces a GestureFrame
// each tick.
type GestureSystem struct {
	swipe map[DeviceID]*SwipeRecognizer
	pinch map[DeviceID]*PinchRecognizer

	minSwipe float32
	maxSwipe time.Duration
}

// NewGestureSystem creates a gesture system with default thresholds.
func NewGestureSystem() *GestureSystem {
	return &GestureSystem{
		swipe:    make(map[DeviceID]*SwipeRecognizer),
		pinch:    make(map[DeviceID]*PinchRecognizer),
		minSwipe: 0.15,
		maxSwipe: 500 * time.Millisecond,
	}
}

// SetSwipeThresholds configures the swipe recognizer thresholds.
func (g *GestureSystem) SetSwipeThresholds(minDistance float32, maxTime time.Duration) {
	g.minSwipe = minDistance
	g.maxSwipe = maxTime
	for _, r := range g.swipe {
		r.MinDistance = minDistance
		r.MaxTime = maxTime
	}
}

// Reset clears all recognizer state.
func (g *GestureSystem) Reset() {
	for _, r := range g.swipe {
		r.Reset()
	}
	for _, r := range g.pinch {
		r.Reset()
	}
}

// Process advances all recognizers using the current device states and
// returns the gesture frame for the tick.
func (g *GestureSystem) Process(now time.Duration, reg *Registry) *GestureFrame {
	frame := &GestureFrame{devices: make(map[DeviceID]*DeviceGestureEvents)}
	for _, st := range reg.Devices() {
		if st.Info.Kind != DeviceTouch {
			continue
		}
		touches := make([]Touch, 0, len(st.Touches))
		for _, t := range st.Touches {
			touches = append(touches, t)
		}
		// Sort by touch id for deterministic recognizer behavior.
		sortTouches(touches)

		sr := g.swipe[st.Info.ID]
		if sr == nil {
			sr = NewSwipeRecognizer(g.minSwipe, g.maxSwipe)
			g.swipe[st.Info.ID] = sr
		}
		pr := g.pinch[st.Info.ID]
		if pr == nil {
			pr = NewPinchRecognizer()
			g.pinch[st.Info.ID] = pr
		}
		frame.devices[st.Info.ID] = &DeviceGestureEvents{
			Swipes: sr.Process(now, touches),
			Pinch:  pr.Process(touches),
		}
	}
	return frame
}

func sortTouches(t []Touch) {
	// Simple insertion sort (touch counts are tiny).
	for i := 1; i < len(t); i++ {
		for j := i; j > 0 && t[j-1].ID > t[j].ID; j-- {
			t[j-1], t[j] = t[j], t[j-1]
		}
	}
}
