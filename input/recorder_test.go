package input

import (
	"bytes"
	"testing"
	"time"
)

// buildScenario creates the action map and scripted events used by the
// determinism tests.
func buildScenario() (*InputMap, []Event) {
	mp := NewInputMap()
	jump, _ := mp.AddAction("jump")
	mp.Bind(jump.ID, &ButtonBinding{Ref: DeviceRef{Kind: DeviceGamepad}, Control: GamepadButtonControl(GamepadSouth)})
	fire, _ := mp.AddAction("fire")
	mp.Bind(fire.ID, &ButtonBinding{Ref: DeviceRef{Kind: DeviceKeyboard}, Control: KeyControlID(KeySpace)})
	move, _ := mp.AddAction("move")
	mp.Bind(move.ID, &StickBinding{
		Ref: DeviceRef{Kind: DeviceGamepad}, XControl: GamepadAxisControl(AxisLeftX), YControl: GamepadAxisControl(AxisLeftY),
		Channel: ChannelX, Deadzone: DeadzoneScaledRadial, Radius: 0.2,
	})

	events := []Event{
		addEvent(0, DeviceGamepad, 0, "pad", 0),
		addEvent(0, DeviceKeyboard, 0, "kb", 0),
		buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadSouth), true, 10*time.Millisecond),
		buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadSouth), false, 30*time.Millisecond),
		buttonEvent(0, DeviceKeyboard, 0, KeyControlID(KeySpace), true, 50*time.Millisecond),
		axisEvent(0, DeviceGamepad, 0, GamepadAxisControl(AxisLeftX), 0.8, 70*time.Millisecond),
		buttonEvent(0, DeviceKeyboard, 0, KeyControlID(KeySpace), false, 90*time.Millisecond),
		buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadSouth), true, 110*time.Millisecond),
		buttonEvent(0, DeviceGamepad, 0, GamepadButtonControl(GamepadSouth), false, 130*time.Millisecond),
	}
	return mp, events
}

func evaluate(mp *InputMap, events []Event, frameDt time.Duration, updates int) []Frame {
	s := NewSampler(16*time.Millisecond, mp)
	s.AddBackend(newFakeBackend("b", events...))
	out := make([]Frame, 0, updates)
	lastTick := int64(-1)
	for i := 0; i < updates; i++ {
		s.Update(frameDt)
		if f, ok := s.Snapshot(); ok && int64(f.Tick) > lastTick {
			out = append(out, f)
			lastTick = int64(f.Tick)
		}
	}
	return out
}

func compareFrames(t *testing.T, want, got []Frame) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("frame count mismatch: %d vs %d", len(want), len(got))
	}
	for i := range want {
		w, g := want[i], got[i]
		if w.Tick != g.Tick {
			t.Fatalf("frame %d: tick mismatch %d vs %d", i, w.Tick, g.Tick)
		}
		for id, wa := range w.Actions {
			ga, ok := g.Actions[id]
			if !ok {
				t.Fatalf("frame %d: missing action %d", i, id)
			}
			if wa.Pressed != ga.Pressed || wa.JustPressed != ga.JustPressed || wa.JustReleased != ga.JustReleased {
				t.Fatalf("frame %d action %d: state mismatch: %+v vs %+v", i, id, wa, ga)
			}
			if abs32(wa.Value-ga.Value) > 0.001 {
				t.Fatalf("frame %d action %d: value mismatch: %v vs %v", i, id, wa.Value, ga.Value)
			}
		}
	}
}

// TestRecordReplayDeterminism records a scenario and replays it, expecting
// identical per-frame results.
func TestRecordReplayDeterminism(t *testing.T) {
	mp, events := buildScenario()

	// Record run.
	rec := NewRecorder()
	s := NewSampler(16*time.Millisecond, mp)
	s.SetRecorder(rec)
	s.AddBackend(newFakeBackend("b", events...))
	recorded := make([]Frame, 10)
	for i := 0; i < 10; i++ {
		s.Update(16 * time.Millisecond)
		f, _ := s.Snapshot()
		recorded[i] = f
	}

	// Save/load round trip.
	var buf bytes.Buffer
	if err := rec.Save(&buf); err != nil {
		t.Fatal(err)
	}
	loaded := NewRecorder()
	if err := loaded.Load(&buf); err != nil {
		t.Fatal(err)
	}

	// Replay run with the same frame rate.
	mp2, _ := buildScenario()
	s2 := NewSampler(16*time.Millisecond, mp2)
	s2.AddBackend(NewReplayBackend("b", loaded.Events()))
	replayed := make([]Frame, 10)
	for i := 0; i < 10; i++ {
		s2.Update(16 * time.Millisecond)
		f, _ := s2.Snapshot()
		replayed[i] = f
	}
	compareFrames(t, recorded, replayed)
}

// TestDeterminismAcrossFrameRates verifies that different frame rates produce
// identical per-frame results for the same event sequence.
func TestDeterminismAcrossFrameRates(t *testing.T) {
	mp, events := buildScenario()
	base := evaluate(mp, events, 16*time.Millisecond, 10)

	mp2, _ := buildScenario()
	fast := evaluate(mp2, events, 8*time.Millisecond, 20)
	// fast has 20 frames but only 10 ticks; keep one frame per tick.
	aligned := uniqueByTick(fast)
	compareFrames(t, base, aligned)

	// Odd frame rate that does not divide the tick rate.
	mp3, _ := buildScenario()
	odd := evaluate(mp3, events, 7*time.Millisecond, 23)
	alignedOdd := uniqueByTick(odd)
	compareFrames(t, base, alignedOdd)
}

// uniqueByTick keeps only the first frame seen for each tick.
func uniqueByTick(frames []Frame) []Frame {
	seen := make(map[uint64]bool)
	out := make([]Frame, 0, len(frames))
	for _, f := range frames {
		if !seen[f.Tick] {
			seen[f.Tick] = true
			out = append(out, f)
		}
	}
	return out
}

// TestDeterminismAcrossEventOrder verifies that shuffled event arrival order
// does not change per-frame results.
func TestDeterminismAcrossEventOrder(t *testing.T) {
	mp, events := buildScenario()
	base := evaluate(mp, events, 16*time.Millisecond, 10)

	// Reverse the script order; the sampler sorts by timestamp then seq.
	reversed := make([]Event, len(events))
	for i := range events {
		reversed[i] = events[len(events)-1-i]
	}
	mp2, _ := buildScenario()
	shuffled := evaluate(mp2, reversed, 16*time.Millisecond, 10)
	compareFrames(t, base, shuffled)
}
