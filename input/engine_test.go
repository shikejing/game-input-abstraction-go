package input

import (
	"fmt"
	"sync"
	"testing"
)

// buildDeterminismFixture 构造一组跨多帧的事件，帧内顺序打乱。
func buildDeterminismFixture(reg *DeviceRegistry, kb DeviceID) (frames [][]Event) {
	seq := uint64(0)
	mk := func(kind EventKind, c ControlID, v float64) Event {
		seq++
		return Event{Kind: kind, Device: kb, Control: c, Value: v, Seq: seq}
	}
	_ = reg
	// 5 帧事件
	frames = [][]Event{
		{mk(EventButtonDown, "a", 1)},
		{mk(EventButtonDown, "b", 1), mk(EventButtonUp, "a", 0)},
		{mk(EventAxisMove, "lx", 0.8)},
		{mk(EventButtonUp, "b", 0), mk(EventAxisMove, "lx", -0.4)},
		{},
	}
	return frames
}

func runFrames(m *ActionMap, reg *DeviceRegistry, frames [][]Event, shuffle bool) []Snapshot {
	e := NewEngine(m, reg, nil)
	const dt = 1.0 / 60
	var out []Snapshot
	for _, evs := range frames {
		if shuffle {
			// 帧内逆序投递，模拟乱序到达
			for i := len(evs) - 1; i >= 0; i-- {
				e.Submit(evs[i])
			}
		} else {
			for _, ev := range evs {
				e.Submit(ev)
			}
		}
		e.Tick(dt)
		out = append(out, e.Snapshot())
	}
	return out
}

func testMap(t *testing.T) *ActionMap {
	m := NewActionMap()
	if err := m.Bind(Binding{Action: "a_btn", Device: DeviceKeyboard, Control: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Bind(Binding{Action: "b_btn", Device: DeviceKeyboard, Control: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Bind(Binding{Action: "move", Device: DeviceKeyboard, Control: "lx", Deadzone: 0.1}); err != nil {
		t.Fatal(err)
	}
	return m
}

func snapshotsEqual(a, b []Snapshot) error {
	if len(a) != len(b) {
		return fmt.Errorf("frame count %d != %d", len(a), len(b))
	}
	for i := range a {
		for _, name := range []string{"a_btn", "b_btn", "move"} {
			sa, sb := a[i].Action(name), b[i].Action(name)
			if sa != sb {
				return fmt.Errorf("frame %d action %s: %+v != %+v", i, name, sa, sb)
			}
		}
	}
	return nil
}

func TestDeterministicUnderArrivalOrder(t *testing.T) {
	reg := NewDeviceRegistry()
	kb, _ := reg.Connect(DeviceKeyboard, "KB", "kb-1")
	m := testMap(t)
	frames := buildDeterminismFixture(reg, kb)

	// 正序与乱序投递，逐帧结果必须一致
	// 注意：两次运行使用独立注册表，避免设备状态互相影响
	reg2 := NewDeviceRegistry()
	reg2.Connect(DeviceKeyboard, "KB", "kb-1")
	ordered := runFrames(m, reg, frames, false)
	shuffled := runFrames(m, reg2, frames, true)
	if err := snapshotsEqual(ordered, shuffled); err != nil {
		t.Fatal(err)
	}
}

func TestRecordingReplayDeterministic(t *testing.T) {
	reg := NewDeviceRegistry()
	kb, _ := reg.Connect(DeviceKeyboard, "KB", "kb-1")
	m := testMap(t)
	frames := buildDeterminismFixture(reg, kb)

	// 录制
	e := NewEngine(m, reg, nil)
	const dt = 1.0 / 60
	rec := NewRecorder(e, dt)
	var live []Snapshot
	for _, evs := range frames {
		for _, ev := range evs {
			e.Submit(ev)
		}
		e.Tick(dt)
		live = append(live, e.Snapshot())
	}
	recording := rec.Recording()
	if len(recording.Frames) != len(frames) {
		t.Fatalf("recorded %d frames, want %d", len(recording.Frames), len(frames))
	}

	// 回放（全新注册表与引擎）
	reg2 := NewDeviceRegistry()
	reg2.Connect(DeviceKeyboard, "KB", "kb-1")
	replayed := Replay(recording, m, reg2, nil)
	if err := snapshotsEqual(live, replayed); err != nil {
		t.Fatalf("replay diverges: %v", err)
	}

	// 再次回放仍一致（可重复）
	reg3 := NewDeviceRegistry()
	reg3.Connect(DeviceKeyboard, "KB", "kb-1")
	replayed2 := Replay(recording, m, reg3, nil)
	if err := snapshotsEqual(replayed, replayed2); err != nil {
		t.Fatalf("second replay diverges: %v", err)
	}
}

func TestConcurrentSnapshotConsistency(t *testing.T) {
	reg := NewDeviceRegistry()
	kb, _ := reg.Connect(DeviceKeyboard, "KB", "kb-1")
	m := testMap(t)
	e := NewEngine(m, reg, nil)
	const dt = 1.0 / 60

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// 采集协程：高速投递
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			select {
			case <-stop:
				return
			default:
			}
			kind := EventButtonDown
			if i%2 == 1 {
				kind = EventButtonUp
			}
			e.Submit(Event{Kind: kind, Device: kb, Control: "a"})
		}
	}()

	// 逻辑协程：推进帧
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			e.Tick(dt)
		}
	}()

	// 读取协程：持续读快照，校验一致性（帧号单调、状态完整）
	wg.Add(1)
	go func() {
		defer wg.Done()
		var lastFrame uint64
		for i := 0; i < 5000; i++ {
			snap := e.Snapshot()
			if snap.Frame < lastFrame {
				t.Errorf("frame went backwards: %d -> %d", lastFrame, snap.Frame)
				return
			}
			lastFrame = snap.Frame
			// 读取动作状态必须是完整快照的一部分
			_ = snap.Action("a_btn")
			_ = snap.Action("move")
		}
	}()

	wg.Wait()
	close(stop)
}
