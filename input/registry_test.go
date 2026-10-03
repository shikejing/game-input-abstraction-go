package input

import (
	"testing"
	"time"
)

// TestHotPlugStableIdentity verifies that a disconnect/reconnect cycle reuses
// the same DeviceID and that bindings keep working.
func TestHotPlugStableIdentity(t *testing.T) {
	mp := NewInputMap()
	jump, _ := mp.AddAction("jump")
	mp.Bind(jump.ID, &ButtonBinding{
		Ref:     DeviceRef{Kind: DeviceGamepad},
		Control: GamepadButtonControl(GamepadSouth),
	})

	s := NewSampler(16*time.Millisecond, mp)
	backend := newFakeBackend("gp",
		addEvent(0, DeviceGamepad, 0, "pad", 0),
		buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadSouth), true, 10*time.Millisecond),
		removeEvent(0, DeviceGamepad, 0, 50*time.Millisecond),
		// Events during the disconnect window: must be dropped, not applied.
		buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadSouth), true, 60*time.Millisecond),
		buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadEast), true, 65*time.Millisecond),
		// Reconnect on the same port.
		addEvent(0, DeviceGamepad, 0, "pad", 70*time.Millisecond),
		buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadSouth), true, 80*time.Millisecond),
	)
	s.AddBackend(backend)

	// Tick 1 (t=16ms): add + press -> jump pressed.
	s.Update(16 * time.Millisecond)
	f, _ := s.Snapshot()
	if len(f.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(f.Devices))
	}
	var padID DeviceID
	for id := range f.Devices {
		padID = id
	}
	if !f.Actions[jump.ID].JustPressed {
		t.Fatal("expected jump JustPressed on first press")
	}

	// Ticks 2-3: held.
	s.Update(16 * time.Millisecond)
	s.Update(16 * time.Millisecond)
	f, _ = s.Snapshot()
	if !f.Actions[jump.ID].Pressed {
		t.Fatal("expected jump held")
	}

	// Tick 4 (t=64ms): device removed at 50ms; the 60ms event is a dead
	// letter (the 65ms event arrives in the next tick).
	s.Update(16 * time.Millisecond)
	if dl := s.Registry().DeadLetters(); dl != 1 {
		t.Fatalf("expected 1 dead letter during disconnect, got %d", dl)
	}
	f, _ = s.Snapshot()
	if len(f.Devices) != 0 {
		t.Fatalf("expected 0 connected devices after remove, got %d", len(f.Devices))
	}
	if f.Actions[jump.ID].Pressed {
		t.Fatal("jump must not be pressed while pad disconnected")
	}

	// Tick 5 (t=80ms): the 65ms event is dropped (still disconnected), then
	// the 70ms add reuses the same id and the 80ms press is applied.
	s.Update(16 * time.Millisecond)
	if dl := s.Registry().DeadLetters(); dl != 2 {
		t.Fatalf("expected 2 dead letters total, got %d", dl)
	}
	f, _ = s.Snapshot()
	if len(f.Devices) != 1 {
		t.Fatalf("expected 1 device after reconnect, got %d", len(f.Devices))
	}
	for id := range f.Devices {
		if id != padID {
			t.Fatalf("device id changed after reconnect: %d -> %d", padID, id)
		}
	}
	if !f.Actions[jump.ID].JustPressed {
		t.Fatal("expected jump JustPressed after reconnect press")
	}
}

// TestHotPlugNoMisattribution verifies that events for a disconnected port are
// never applied to a different connected device.
func TestHotPlugNoMisattribution(t *testing.T) {
	mp := NewInputMap()
	p1, _ := mp.AddAction("p1")
	mp.Bind(p1.ID, &ButtonBinding{Ref: DeviceRef{Kind: DeviceGamepad, Port: 1}, Control: GamepadButtonControl(GamepadSouth)})
	p2, _ := mp.AddAction("p2")
	mp.Bind(p2.ID, &ButtonBinding{Ref: DeviceRef{Kind: DeviceGamepad, Port: 2}, Control: GamepadButtonControl(GamepadSouth)})

	s := NewSampler(16*time.Millisecond, mp)
	backend := newFakeBackend("gp",
		addEvent(0, DeviceGamepad, 1, "pad1", 0),
		addEvent(0, DeviceGamepad, 2, "pad2", 0),
		// Port 1 disconnects; port 2 stays connected.
		removeEvent(0, DeviceGamepad, 1, 20*time.Millisecond),
		// Event intended for port 1 during disconnect.
		buttonEvent(0, DeviceGamepad, 1, GamepadButtonControl(GamepadSouth), true, 30*time.Millisecond),
	)
	s.AddBackend(backend)

	runTicks(s, 3, 16*time.Millisecond)
	f, _ := s.Snapshot()
	if f.Actions[p1.ID].Pressed {
		t.Fatal("disconnected port 1 action must not be active")
	}
	if f.Actions[p2.ID].Pressed {
		t.Fatal("port 2 must not receive port 1's event")
	}
	if dl := s.Registry().DeadLetters(); dl != 1 {
		t.Fatalf("expected 1 dead letter, got %d", dl)
	}
}

// TestDeadLettersForUnknownDevice verifies events for never-seen devices are
// dropped.
func TestDeadLettersForUnknownDevice(t *testing.T) {
	mp := NewInputMap()
	s := NewSampler(16*time.Millisecond, mp)
	backend := newFakeBackend("gp",
		buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadSouth), true, 5*time.Millisecond),
	)
	s.AddBackend(backend)
	s.Update(16 * time.Millisecond)
	if dl := s.Registry().DeadLetters(); dl != 1 {
		t.Fatalf("expected 1 dead letter for unknown device, got %d", dl)
	}
}
