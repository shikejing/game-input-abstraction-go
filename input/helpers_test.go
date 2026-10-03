package input

import "time"

// fakeBackend is a scripted backend that returns events with timestamp <= now
// in the order they were scripted.
type fakeBackend struct {
	name   string
	events []Event
	pos    int
}

func newFakeBackend(name string, events ...Event) *fakeBackend {
	// Copy into a fresh backing array so the caller's slice is never mutated
	// by Poll's in-place filtering.
	cp := make([]Event, len(events))
	copy(cp, events)
	return &fakeBackend{name: name, events: cp}
}

func (b *fakeBackend) Name() string { return b.name }

func (b *fakeBackend) Poll(now time.Duration) []Event {
	var out []Event
	kept := b.events[:0]
	for _, e := range b.events {
		if e.Timestamp <= now {
			out = append(out, e)
		} else {
			kept = append(kept, e)
		}
	}
	b.events = kept
	return out
}

// Event builder helpers.

func addEvent(backendID uint16, kind DeviceKind, port uint32, name string, ts time.Duration) Event {
	return Event{
		BackendID: backendID, Kind: kind, Port: port, DeviceName: name,
		Type: EventDeviceAdd, Timestamp: ts,
	}
}

func removeEvent(backendID uint16, kind DeviceKind, port uint32, ts time.Duration) Event {
	return Event{
		BackendID: backendID, Kind: kind, Port: port,
		Type: EventDeviceRemove, Timestamp: ts,
	}
}

func buttonEvent(backendID uint16, kind DeviceKind, port uint32, ctrl ControlID, down bool, ts time.Duration) Event {
	typ := EventButtonDown
	if !down {
		typ = EventButtonUp
	}
	return Event{
		BackendID: backendID, Kind: kind, Port: port,
		Type: typ, Control: ctrl, Timestamp: ts,
	}
}

func axisEvent(backendID uint16, kind DeviceKind, port uint32, ctrl ControlID, value float32, ts time.Duration) Event {
	return Event{
		BackendID: backendID, Kind: kind, Port: port,
		Type: EventAxisMove, Control: ctrl, Value: value, Timestamp: ts,
	}
}

func touchEvent(backendID uint16, port uint32, typ EventType, id TouchID, x, y float32, ts time.Duration) Event {
	return Event{
		BackendID: backendID, Kind: DeviceTouch, Port: port,
		Type: typ, TouchID: id, X: x, Y: y, Timestamp: ts,
	}
}

// runTicks drives a sampler for n ticks at the given frame dt.
func runTicks(s *Sampler, n int, frameDt time.Duration) {
	for i := 0; i < n; i++ {
		s.Update(frameDt)
	}
}

// tickFrames returns the action states of n consecutive ticks.
func tickFrames(s *Sampler, n int, frameDt time.Duration) []map[ActionID]ActionState {
	out := make([]map[ActionID]ActionState, 0, n)
	for i := 0; i < n; i++ {
		s.Update(frameDt)
		f, _ := s.Snapshot()
		cp := make(map[ActionID]ActionState, len(f.Actions))
		for k, v := range f.Actions {
			cp[k] = v
		}
		out = append(out, cp)
	}
	return out
}
