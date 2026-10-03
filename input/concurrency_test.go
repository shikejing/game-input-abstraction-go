package input

import (
	"sync"
	"testing"
	"time"
)

// streamBackend is a thread-safe backend that accepts events while the
// sampler is running.
type streamBackend struct {
	name   string
	mu     sync.Mutex
	events []Event
	seq    uint64
}

func (b *streamBackend) Name() string { return b.name }

func (b *streamBackend) Poll(now time.Duration) []Event {
	b.mu.Lock()
	defer b.mu.Unlock()
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

func (b *streamBackend) push(events ...Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range events {
		b.seq++
		e.Seq = b.seq
		b.events = append(b.events, e)
	}
}

func validateFrame(t *testing.T, f Frame) {
	t.Helper()
	for id, a := range f.Actions {
		if a.JustPressed && !a.Pressed {
			t.Fatalf("frame %d action %d: JustPressed but not Pressed", f.Tick, id)
		}
		if a.JustReleased && a.Pressed {
			t.Fatalf("frame %d action %d: JustReleased but Pressed", f.Tick, id)
		}
		if a.Value < -1.0001 || a.Value > 1.0001 {
			t.Fatalf("frame %d action %d: value out of range: %v", f.Tick, id, a.Value)
		}
	}
	for id, d := range f.Devices {
		if !d.Info.Connected {
			t.Fatalf("frame %d device %d: disconnected device in snapshot", f.Tick, id)
		}
		if d.Buttons == nil || d.Axes == nil || d.Touches == nil {
			t.Fatalf("frame %d device %d: nil control map", f.Tick, id)
		}
	}
}

// TestConcurrentSnapshotReads hammers the sampler with concurrent readers and
// verifies every observed frame is internally consistent.
func TestConcurrentSnapshotReads(t *testing.T) {
	mp := NewInputMap()
	jump, _ := mp.AddAction("jump")
	mp.Bind(jump.ID, &ButtonBinding{Ref: DeviceRef{Kind: DeviceGamepad}, Control: GamepadButtonControl(GamepadSouth)})
	s := NewSampler(16*time.Millisecond, mp)
	backend := &streamBackend{name: "stream"}
	s.AddBackend(backend)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Producer: advance time and occasionally push events.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		added := false
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s.Update(5 * time.Millisecond)
				if !added && s.Tick() >= 2 {
					backend.push(addEvent(0, DeviceGamepad, 0, "pad", time.Duration(s.Tick())*16*time.Millisecond))
					added = true
				}
				if s.Tick() == 5 {
					backend.push(buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadSouth), true, time.Duration(s.Tick())*16*time.Millisecond))
				}
			}
		}
	}()

	// Fast readers.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var lastTick uint64
			for {
				select {
				case <-stop:
					return
				default:
				}
				f, ok := s.Bus().Latest()
				if !ok {
					continue
				}
				validateFrame(t, f)
				if f.Tick < lastTick {
					t.Errorf("tick went backwards: %d -> %d", lastTick, f.Tick)
					return
				}
				lastTick = f.Tick
			}
		}()
	}

	// Slow consumer: must not block the producer.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			time.Sleep(20 * time.Millisecond)
			f, ok := s.Bus().Latest()
			if ok {
				validateFrame(t, f)
			}
		}
	}()

	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()

	if s.Tick() < 15 {
		t.Fatalf("producer stalled: only reached tick %d", s.Tick())
	}
}

// TestSnapshotIsACopy verifies that mutating a returned frame does not affect
// subsequent reads.
func TestSnapshotIsACopy(t *testing.T) {
	mp := NewInputMap()
	jump, _ := mp.AddAction("jump")
	mp.Bind(jump.ID, &ButtonBinding{Ref: DeviceRef{Kind: DeviceGamepad}, Control: GamepadButtonControl(GamepadSouth)})
	s := NewSampler(16*time.Millisecond, mp)
	backend := newFakeBackend("b",
		addEvent(0, DeviceGamepad, 0, "pad", 0),
		buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadSouth), true, 5*time.Millisecond),
	)
	s.AddBackend(backend)
	s.Update(16 * time.Millisecond)

	f1, ok := s.Snapshot()
	if !ok {
		t.Fatal("expected a frame")
	}
	f1.Actions[jump.ID] = ActionState{Pressed: false, Value: 999}
	f2, _ := s.Snapshot()
	if f2.Actions[jump.ID].Value == 999 {
		t.Fatal("snapshot mutation leaked into the sampler state")
	}
}
