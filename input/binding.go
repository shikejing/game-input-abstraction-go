package input

import (
	"math"
	"time"
)

// Binding evaluates the value of a logical action from device state.
// Implementations must be safe for single-goroutine use (the sampling
// goroutine) and must be deterministic: given the same EvalContext they
// produce the same value.
type Binding interface {
	// Value returns the current value of the binding. Digital bindings report
	// 0 or 1; analog bindings report a normalized value.
	Value(ctx *EvalContext) float32
	// Controls returns the physical controls this binding reads, used for
	// conflict detection. Gesture bindings return nil.
	Controls() []ControlRef
	// Reset clears all internal timing state.
	Reset()
}

// ControlRef addresses a physical control on a class of devices.
type ControlRef struct {
	Ref     DeviceRef
	Control ControlID
}

// EvalContext is the read-only view of the world handed to bindings during
// per-tick evaluation.
type EvalContext struct {
	// Registry gives access to connected device states.
	Registry *Registry
	// Gestures holds gesture outputs for the current tick.
	Gestures *GestureFrame
	// Events are the raw events applied during this tick, in deterministic
	// order. Timing-sensitive bindings (long press, double tap) use them to
	// detect edges that occur within a tick.
	Events []Event
	// DT is the fixed tick duration.
	DT time.Duration
	// Tick is the current fixed-tick index.
	Tick uint64
}

// Button reports whether the given control is currently held.
func (c *EvalContext) Button(ref DeviceRef, ctrl ControlID) bool {
	id, ok := c.Registry.Resolve(ref)
	if !ok {
		return false
	}
	st, ok := c.Registry.Device(id)
	if !ok {
		return false
	}
	return st.Buttons[ctrl]
}

// Axis reports the current value of the given analog control.
func (c *EvalContext) Axis(ref DeviceRef, ctrl ControlID) float32 {
	id, ok := c.Registry.Resolve(ref)
	if !ok {
		return 0
	}
	st, ok := c.Registry.Device(id)
	if !ok {
		return 0
	}
	return st.Axes[ctrl]
}

// Touches returns the active contacts of the first matching device.
func (c *EvalContext) Touches(ref DeviceRef) []Touch {
	id, ok := c.Registry.Resolve(ref)
	if !ok {
		return nil
	}
	st, ok := c.Registry.Device(id)
	if !ok {
		return nil
	}
	out := make([]Touch, 0, len(st.Touches))
	for _, t := range st.Touches {
		out = append(out, t)
	}
	return out
}

// GestureEvents returns the gesture outputs for the first matching device.
func (c *EvalContext) GestureEvents(ref DeviceRef) *DeviceGestureEvents {
	if c.Gestures == nil {
		return nil
	}
	id, ok := c.Registry.Resolve(ref)
	if !ok {
		return nil
	}
	return c.Gestures.Get(id)
}

// ---------------------------------------------------------------------------
// Digital binding
// ---------------------------------------------------------------------------

// ButtonBinding is a simple one-control digital binding.
type ButtonBinding struct {
	Ref     DeviceRef
	Control ControlID
}

func (b *ButtonBinding) Value(ctx *EvalContext) float32 {
	if ctx.Button(b.Ref, b.Control) {
		return 1
	}
	return 0
}

func (b *ButtonBinding) Controls() []ControlRef {
	return []ControlRef{{Ref: b.Ref, Control: b.Control}}
}

func (b *ButtonBinding) Reset() {}

// ---------------------------------------------------------------------------
// Analog axis binding with dead zone
// ---------------------------------------------------------------------------

// DeadzoneMode selects how analog dead zones are applied.
type DeadzoneMode uint8

const (
	// DeadzoneNone passes values through unchanged.
	DeadzoneNone DeadzoneMode = iota
	// DeadzoneAxial zeroes values whose absolute magnitude is below Radius.
	DeadzoneAxial
	// DeadzoneRadial zeroes the whole vector when its magnitude is below
	// Radius (stick bindings only).
	DeadzoneRadial
	// DeadzoneScaledRadial rescales the [Radius,1] magnitude range to [0,1],
	// eliminating the dead-band jump (stick bindings only).
	DeadzoneScaledRadial
)

// AxisBinding reads a single analog control and applies an axial dead zone.
type AxisBinding struct {
	Ref      DeviceRef
	Control  ControlID
	Deadzone DeadzoneMode
	Radius   float32
	Scale    float32
	Invert   bool
}

func (b *AxisBinding) Value(ctx *EvalContext) float32 {
	v := ctx.Axis(b.Ref, b.Control)
	if b.Invert {
		v = -v
	}
	if b.Scale != 0 {
		v *= b.Scale
	}
	switch b.Deadzone {
	case DeadzoneAxial:
		if abs32(v) < b.Radius {
			v = 0
		}
	}
	return clamp32(v, -1, 1)
}

func (b *AxisBinding) Controls() []ControlRef {
	return []ControlRef{{Ref: b.Ref, Control: b.Control}}
}

func (b *AxisBinding) Reset() {}

// AxisChannel selects the X or Y channel of a 2D stick.
type AxisChannel uint8

const (
	ChannelX AxisChannel = iota
	ChannelY
)

// StickBinding reads a 2D stick (pair of axes) and applies a radial dead zone
// to the vector before reporting the selected channel.
type StickBinding struct {
	Ref      DeviceRef
	XControl ControlID
	YControl ControlID
	Channel  AxisChannel
	Deadzone DeadzoneMode
	Radius   float32
	Scale    float32
	Invert   bool
}

func (b *StickBinding) Value(ctx *EvalContext) float32 {
	x := ctx.Axis(b.Ref, b.XControl)
	y := ctx.Axis(b.Ref, b.YControl)
	if b.Scale != 0 {
		x *= b.Scale
		y *= b.Scale
	}
	mag := hypot32(x, y)
	switch b.Deadzone {
	case DeadzoneRadial:
		if mag < b.Radius {
			x, y = 0, 0
		}
	case DeadzoneScaledRadial:
		if mag < b.Radius {
			x, y = 0, 0
		} else if mag > 0 {
			// Rescale magnitude from [Radius,1] to [0,1].
			nx, ny := x/mag, y/mag
			scaled := (mag - b.Radius) / (1 - b.Radius)
			x, y = nx*scaled, ny*scaled
		}
	}
	v := x
	if b.Channel == ChannelY {
		v = y
	}
	if b.Invert {
		v = -v
	}
	return clamp32(v, -1, 1)
}

func (b *StickBinding) Controls() []ControlRef {
	return []ControlRef{
		{Ref: b.Ref, Control: b.XControl},
		{Ref: b.Ref, Control: b.YControl},
	}
}

func (b *StickBinding) Reset() {}

// ---------------------------------------------------------------------------
// Combo binding (all controls held simultaneously)
// ---------------------------------------------------------------------------

// ComboBinding requires all of its controls to be held at the same time.
type ComboBinding struct {
	Ref      DeviceRef
	Buttons  []ControlID
}

func (b *ComboBinding) Value(ctx *EvalContext) float32 {
	if len(b.Buttons) == 0 {
		return 0
	}
	for _, c := range b.Buttons {
		if !ctx.Button(b.Ref, c) {
			return 0
		}
	}
	return 1
}

func (b *ComboBinding) Controls() []ControlRef {
	out := make([]ControlRef, len(b.Buttons))
	for i, c := range b.Buttons {
		out[i] = ControlRef{Ref: b.Ref, Control: c}
	}
	return out
}

func (b *ComboBinding) Reset() {}

// ---------------------------------------------------------------------------
// Long press binding
// ---------------------------------------------------------------------------

// LongPressBinding activates after its control has been held continuously for
// at least Duration.
type LongPressBinding struct {
	Ref      DeviceRef
	Control  ControlID
	Duration time.Duration

	pressStart time.Duration
	wasDown    bool
}

func (b *LongPressBinding) Value(ctx *EvalContext) float32 {
	down := ctx.Button(b.Ref, b.Control)
	now := ctxTime(ctx)

	// Look for this tick's down/up events to get accurate press timing even
	// when the press starts or ends between tick boundaries.
	var downTS, upTS time.Duration
	var hasDown, hasUp bool
	for i := range ctx.Events {
		e := &ctx.Events[i]
		if e.Control != b.Control {
			continue
		}
		switch e.Type {
		case EventButtonDown:
			hasDown = true
			downTS = e.Timestamp
		case EventButtonUp:
			hasUp = true
			upTS = e.Timestamp
		}
	}
	if down && !b.wasDown {
		if hasDown {
			b.pressStart = downTS
		} else {
			b.pressStart = now
		}
	}
	b.wasDown = down

	// Active while held past the duration, or on the release tick if the
	// press had already lasted long enough.
	if down && now-b.pressStart >= b.Duration {
		return 1
	}
	if !down && hasUp && upTS-b.pressStart >= b.Duration {
		return 1
	}
	return 0
}

func (b *LongPressBinding) Controls() []ControlRef {
	return []ControlRef{{Ref: b.Ref, Control: b.Control}}
}

func (b *LongPressBinding) Reset() { b.wasDown = false; b.pressStart = 0 }

// ---------------------------------------------------------------------------
// Double tap binding
// ---------------------------------------------------------------------------

// DoubleTapBinding triggers on the tick where its control is tapped twice
// within MaxInterval. It reports 1 only on that tick (edge semantics).
type DoubleTapBinding struct {
	Ref         DeviceRef
	Control     ControlID
	MaxInterval time.Duration

	wasDown  bool
	lastTap  time.Duration
	reported bool
}

func (b *DoubleTapBinding) Value(ctx *EvalContext) float32 {
	// Scan this tick's down events so taps that start and end within the
	// same tick are still detected.
	triggered := false
	for i := range ctx.Events {
		e := &ctx.Events[i]
		if e.Control != b.Control || e.Type != EventButtonDown {
			continue
		}
		if b.lastTap != 0 && e.Timestamp-b.lastTap <= b.MaxInterval {
			triggered = true
		}
		b.lastTap = e.Timestamp
	}
	b.wasDown = ctx.Button(b.Ref, b.Control)
	if triggered {
		return 1
	}
	return 0
}

func (b *DoubleTapBinding) Controls() []ControlRef {
	return []ControlRef{{Ref: b.Ref, Control: b.Control}}
}

func (b *DoubleTapBinding) Reset() { b.wasDown = false; b.lastTap = 0 }

// ---------------------------------------------------------------------------
// Gesture bindings
// ---------------------------------------------------------------------------

// SwipeBinding triggers on a multi-finger swipe gesture in Direction over at
// least MinDistance (normalized), involving at least MinFingers contacts.
type SwipeBinding struct {
	Ref         DeviceRef
	Direction   SwipeDirection
	MinDistance float32
	MaxTime     time.Duration
	MinFingers  int
}

func (b *SwipeBinding) Value(ctx *EvalContext) float32 {
	ge := ctx.GestureEvents(b.Ref)
	if ge == nil {
		return 0
	}
	for _, s := range ge.Swipes {
		if s.Direction != b.Direction {
			continue
		}
		if s.Distance < b.MinDistance {
			continue
		}
		if b.MaxTime > 0 && s.Duration > b.MaxTime {
			continue
		}
		if s.Fingers < b.MinFingers {
			continue
		}
		return 1
	}
	return 0
}

func (b *SwipeBinding) Controls() []ControlRef { return nil }
func (b *SwipeBinding) Reset()                {}

// PinchBinding reports the per-tick pinch delta (positive = spread, negative
// = pinch) once the gesture exceeds MinScale.
type PinchBinding struct {
	Ref      DeviceRef
	MinScale float32
}

func (b *PinchBinding) Value(ctx *EvalContext) float32 {
	ge := ctx.GestureEvents(b.Ref)
	if ge == nil {
		return 0
	}
	d := ge.Pinch.Delta
	if abs32(d) < b.MinScale {
		return 0
	}
	return clamp32(d, -1, 1)
}

func (b *PinchBinding) Controls() []ControlRef { return nil }
func (b *PinchBinding) Reset()                {}

// ---------------------------------------------------------------------------
// Virtual joystick (floating stick region)
// ---------------------------------------------------------------------------

// Region is a normalized rectangular region of the touch surface.
type Region struct {
	X, Y, W, H float32
}

// Contains reports whether the normalized point lies in the region.
func (r Region) Contains(x, y float32) bool {
	return x >= r.X && x <= r.X+r.W && y >= r.Y && y <= r.Y+r.H
}

// JoystickBinding implements a floating virtual joystick: the first contact
// that lands in Region becomes the stick center; dragging moves the stick,
// and the selected channel reports a normalized -1..1 value.
type JoystickBinding struct {
	Ref     DeviceRef
	Region  Region
	Channel AxisChannel
	Radius  float32 // normalized drag distance for full deflection
	Invert  bool

	activeTouch TouchID
	hasTouch    bool
	centerX     float32
	centerY     float32
}

func (b *JoystickBinding) Value(ctx *EvalContext) float32 {
	touches := ctx.Touches(b.Ref)
	if !b.hasTouch {
		for _, t := range touches {
			if b.Region.Contains(t.X, t.Y) {
				b.hasTouch = true
				b.activeTouch = t.ID
				b.centerX, b.centerY = t.X, t.Y
				break
			}
		}
	}
	if !b.hasTouch {
		return 0
	}
	var cur *Touch
	for i := range touches {
		if touches[i].ID == b.activeTouch {
			cur = &touches[i]
			break
		}
	}
	if cur == nil {
		// Contact lifted.
		b.hasTouch = false
		return 0
	}
	dx := cur.X - b.centerX
	dy := cur.Y - b.centerY
	// Clamp the vector to the stick radius.
	mag := hypot32(dx, dy)
	r := b.Radius
	if r <= 0 {
		r = 0.2
	}
	if mag > r {
		dx = dx / mag * r
		dy = dy / mag * r
	}
	v := dx / r
	if b.Channel == ChannelY {
		v = dy / r
	}
	if b.Invert {
		v = -v
	}
	return clamp32(v, -1, 1)
}

func (b *JoystickBinding) Controls() []ControlRef { return nil }

func (b *JoystickBinding) Reset() { b.hasTouch = false; b.activeTouch = 0 }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func ctxTime(ctx *EvalContext) time.Duration { return time.Duration(ctx.Tick) * ctx.DT }

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }

func hypot32(x, y float32) float32 { return float32(math.Hypot(float64(x), float64(y))) }

func clamp32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
