package input

import (
	"testing"
	"time"
)

func TestSwipeGesture(t *testing.T) {
	mp := NewInputMap()
	sw, _ := mp.AddAction("swipe_up")
	mp.Bind(sw.ID, &SwipeBinding{
		Ref:         DeviceRef{Kind: DeviceTouch},
		Direction:   SwipeUp,
		MinDistance: 0.1,
		MinFingers:  2,
	})
	s := NewSampler(16*time.Millisecond, mp)
	tb := newFakeBackend("touch",
		addEvent(0, DeviceTouch, 0, "touch", 0),
		// Two fingers start low and move up together.
		touchEvent(0, 0, EventTouchDown, 1, 0.4, 0.8, 10*time.Millisecond),
		touchEvent(0, 0, EventTouchDown, 2, 0.6, 0.8, 10*time.Millisecond),
		touchEvent(0, 0, EventTouchMove, 1, 0.4, 0.5, 30*time.Millisecond),
		touchEvent(0, 0, EventTouchMove, 2, 0.6, 0.5, 30*time.Millisecond),
		touchEvent(0, 0, EventTouchMove, 1, 0.4, 0.2, 50*time.Millisecond),
		touchEvent(0, 0, EventTouchMove, 2, 0.6, 0.2, 50*time.Millisecond),
	)
	s.AddBackend(tb)
	frames := tickFrames(s, 4, 16*time.Millisecond)
	// At tick 2 (t=32) the swipe has crossed the threshold (0.3 distance).
	if !frames[1][sw.ID].JustPressed {
		t.Fatal("expected two-finger swipe up to trigger JustPressed")
	}
}

func TestPinchGesture(t *testing.T) {
	mp := NewInputMap()
	pinch, _ := mp.AddAction("pinch")
	mp.Bind(pinch.ID, &PinchBinding{Ref: DeviceRef{Kind: DeviceTouch}, MinScale: 0.05})
	s := NewSampler(16*time.Millisecond, mp)
	tb := newFakeBackend("touch",
		addEvent(0, DeviceTouch, 0, "touch", 0),
		touchEvent(0, 0, EventTouchDown, 1, 0.4, 0.5, 10*time.Millisecond),
		touchEvent(0, 0, EventTouchDown, 2, 0.6, 0.5, 10*time.Millisecond),
		// Spread apart: distance grows from 0.2 to 0.6.
		touchEvent(0, 0, EventTouchMove, 1, 0.2, 0.5, 30*time.Millisecond),
		touchEvent(0, 0, EventTouchMove, 2, 0.8, 0.5, 30*time.Millisecond),
	)
	s.AddBackend(tb)
	frames := tickFrames(s, 3, 16*time.Millisecond)
	// Tick 2 (t=32): scale = 0.6/0.2 = 3 -> positive delta.
	if frames[1][pinch.ID].Value <= 0 {
		t.Fatalf("expected positive pinch delta (spread), got %v", frames[1][pinch.ID].Value)
	}
}

func TestJoystickGesture(t *testing.T) {
	mp := NewInputMap()
	mx, _ := mp.AddAction("move_x")
	mp.Bind(mx.ID, &JoystickBinding{
		Ref:     DeviceRef{Kind: DeviceTouch},
		Region:  Region{X: 0, Y: 0, W: 1, H: 1},
		Channel: ChannelX,
		Radius:  0.2,
	})
	s := NewSampler(16*time.Millisecond, mp)
	tb := newFakeBackend("touch",
		addEvent(0, DeviceTouch, 0, "touch", 0),
		// Touch at center, drag right by 0.2.
		touchEvent(0, 0, EventTouchDown, 1, 0.5, 0.5, 10*time.Millisecond),
		touchEvent(0, 0, EventTouchMove, 1, 0.7, 0.5, 30*time.Millisecond),
	)
	s.AddBackend(tb)
	frames := tickFrames(s, 3, 16*time.Millisecond)
	// Tick 3 (t=48): dx=0.2 -> full deflection +1.
	if got := frames[2][mx.ID].Value; got < 0.9 {
		t.Fatalf("expected joystick x ~1, got %v", got)
	}
}

// TestGestureCoexistence verifies that a swipe and a pinch can be recognized
// concurrently without swallowing each other's events.
func TestGestureCoexistence(t *testing.T) {
	mp := NewInputMap()
	sw, _ := mp.AddAction("swipe")
	mp.Bind(sw.ID, &SwipeBinding{Ref: DeviceRef{Kind: DeviceTouch}, Direction: SwipeLeft, MinDistance: 0.1, MinFingers: 2})
	pinch, _ := mp.AddAction("pinch")
	mp.Bind(pinch.ID, &PinchBinding{Ref: DeviceRef{Kind: DeviceTouch}, MinScale: 0.05})
	s := NewSampler(16*time.Millisecond, mp)
	tb := newFakeBackend("touch",
		addEvent(0, DeviceTouch, 0, "touch", 0),
		// Two fingers move left AND spread vertically at the same time.
		touchEvent(0, 0, EventTouchDown, 1, 0.7, 0.4, 10*time.Millisecond),
		touchEvent(0, 0, EventTouchDown, 2, 0.7, 0.6, 10*time.Millisecond),
		touchEvent(0, 0, EventTouchMove, 1, 0.4, 0.3, 30*time.Millisecond),
		touchEvent(0, 0, EventTouchMove, 2, 0.4, 0.7, 30*time.Millisecond),
		touchEvent(0, 0, EventTouchMove, 1, 0.2, 0.2, 50*time.Millisecond),
		touchEvent(0, 0, EventTouchMove, 2, 0.2, 0.8, 50*time.Millisecond),
	)
	s.AddBackend(tb)
	frames := tickFrames(s, 4, 16*time.Millisecond)
	if !frames[1][sw.ID].JustPressed {
		t.Fatal("expected left swipe to be recognized concurrently with pinch")
	}
	if frames[1][pinch.ID].Value == 0 {
		t.Fatal("expected pinch to be recognized concurrently with swipe")
	}
}
