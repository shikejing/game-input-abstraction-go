package input

import (
	"testing"
	"time"
)

// TestCrossBackendConsistency verifies that the same physical operation on
// different backends produces identical action states.
func TestCrossBackendConsistency(t *testing.T) {
	// Two maps: one bound to keyboard, one to gamepad, same action names.
	buildMap := func(kind DeviceKind) *InputMap {
		mp := NewInputMap()
		jump, _ := mp.AddAction("jump")
		mp.Bind(jump.ID, &ButtonBinding{Ref: DeviceRef{Kind: kind}, Control: controlFor(kind)})
		return mp
	}
	kbMap := buildMap(DeviceKeyboard)
	gpMap := buildMap(DeviceGamepad)

	kbEvents := []Event{
		addEvent(0, DeviceKeyboard, 0, "kb", 0),
		buttonEvent(0, DeviceKeyboard, 0, controlFor(DeviceKeyboard), true, 10*time.Millisecond),
		buttonEvent(0, DeviceKeyboard, 0, controlFor(DeviceKeyboard), false, 30*time.Millisecond),
	}
	gpEvents := []Event{
		addEvent(0, DeviceGamepad, 0, "gp", 0),
		buttonEvent(0, DeviceGamepad, 0, controlFor(DeviceGamepad), true, 10*time.Millisecond),
		buttonEvent(0, DeviceGamepad, 0, controlFor(DeviceGamepad), false, 30*time.Millisecond),
	}

	kbFrames := evaluate(kbMap, kbEvents, 16*time.Millisecond, 3)
	gpFrames := evaluate(gpMap, gpEvents, 16*time.Millisecond, 3)

	for i := range kbFrames {
		k := kbFrames[i].Actions[1]
		g := gpFrames[i].Actions[1]
		if k.Pressed != g.Pressed || k.JustPressed != g.JustPressed || k.JustReleased != g.JustReleased {
			t.Fatalf("frame %d: keyboard (%+v) and gamepad (%+v) action states differ", i, k, g)
		}
	}
}

func controlFor(kind DeviceKind) ControlID {
	switch kind {
	case DeviceKeyboard:
		return KeyControlID(KeySpace)
	case DeviceGamepad:
		return GamepadButtonControl(GamepadSouth)
	}
	return 0
}

// TestMouseButtonsAndAxes verifies mouse button and axis events are applied.
func TestMouseButtonsAndAxes(t *testing.T) {
	mp := NewInputMap()
	fire, _ := mp.AddAction("fire")
	mp.Bind(fire.ID, &ButtonBinding{Ref: DeviceRef{Kind: DeviceMouse}, Control: MouseButtonControl(MouseLeft)})
	s := NewSampler(16*time.Millisecond, mp)
	m := newFakeBackend("mouse",
		addEvent(0, DeviceMouse, 0, "mouse", 0),
		buttonEvent(0, DeviceMouse, 0, MouseButtonControl(MouseLeft), true, 5*time.Millisecond),
		axisEvent(0, DeviceMouse, 0, AxisControl(AxisMouseX), 0.7, 5*time.Millisecond),
	)
	s.AddBackend(m)
	s.Update(16 * time.Millisecond)
	f, _ := s.Snapshot()
	if !f.Actions[fire.ID].JustPressed {
		t.Fatal("expected mouse left click to trigger fire")
	}
	mouse := f.Devices[DeviceID(1)]
	if mouse == nil {
		t.Fatal("mouse device missing")
	}
	if mouse.Axes[AxisControl(AxisMouseX)] != 0.7 {
		t.Fatalf("expected mouse x axis 0.7, got %v", mouse.Axes[AxisControl(AxisMouseX)])
	}
}
