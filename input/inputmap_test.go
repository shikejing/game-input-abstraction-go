package input

import (
	"testing"
	"time"
)

func TestBindConflicts(t *testing.T) {
	mp := NewInputMap()
	jump, _ := mp.AddAction("jump")
	punch, _ := mp.AddAction("punch")

	a := &ButtonBinding{Ref: DeviceRef{Kind: DeviceGamepad}, Control: GamepadButtonControl(GamepadSouth)}
	b := &ButtonBinding{Ref: DeviceRef{Kind: DeviceGamepad}, Control: GamepadButtonControl(GamepadSouth)}

	// First bind succeeds.
	if r := mp.Bind(jump.ID, a); !r.Added || r.Conflict != ConflictNone {
		t.Fatalf("expected first bind to succeed, got %+v", r)
	}
	// Identical duplicate on the same action is a no-op, not an error.
	if r := mp.Bind(jump.ID, b); r.Added || r.Conflict != ConflictDuplicate {
		t.Fatalf("expected duplicate, got %+v", r)
	}
	// Same control on another action is a distinguishable conflict.
	if r := mp.Bind(punch.ID, &ButtonBinding{Ref: DeviceRef{Kind: DeviceGamepad}, Control: GamepadButtonControl(GamepadSouth)}); r.Added || r.Conflict != ConflictControlTaken || r.ConflictingAction != jump.ID {
		t.Fatalf("expected control_taken conflict against jump, got %+v", r)
	}
	// Force bind reassigns the control.
	if r := mp.ForceBind(punch.ID, &ButtonBinding{Ref: DeviceRef{Kind: DeviceGamepad}, Control: GamepadButtonControl(GamepadSouth)}); !r.Added || r.Conflict != ConflictNone {
		t.Fatalf("expected force bind to succeed, got %+v", r)
	}
	if len(mp.BindingsOf(jump.ID)) != 0 {
		t.Fatal("force bind should have removed jump's binding")
	}
	if len(mp.BindingsOf(punch.ID)) != 1 {
		t.Fatal("force bind should have added punch's binding")
	}
}

func TestComboConflict(t *testing.T) {
	mp := NewInputMap()
	a, _ := mp.AddAction("a")
	b, _ := mp.AddAction("b")

	combo := &ComboBinding{
		Ref:     DeviceRef{Kind: DeviceGamepad},
		Buttons: []ControlID{GamepadButtonControl(GamepadSouth), GamepadButtonControl(GamepadEast)},
	}
	mp.Bind(a.ID, combo)
	// Binding a control that is part of a combo to another action conflicts.
	r := mp.Bind(b.ID, &ButtonBinding{Ref: DeviceRef{Kind: DeviceGamepad}, Control: GamepadButtonControl(GamepadSouth)})
	if r.Conflict != ConflictControlTaken || r.ConflictingAction != a.ID {
		t.Fatalf("expected combo control conflict, got %+v", r)
	}
}

func TestLongPressAndDoubleTap(t *testing.T) {
	mp := NewInputMap()
	lp, _ := mp.AddAction("long")
	mp.Bind(lp.ID, &LongPressBinding{
		Ref: DeviceRef{Kind: DeviceKeyboard}, Control: KeyControlID(KeySpace), Duration: 500 * time.Millisecond,
	})
	dt, _ := mp.AddAction("double")
	mp.Bind(dt.ID, &DoubleTapBinding{
		Ref: DeviceRef{Kind: DeviceKeyboard}, Control: KeyControlID(KeyEnter), MaxInterval: 300 * time.Millisecond,
	})

	s := NewSampler(100*time.Millisecond, mp)
	kb := newFakeBackend("kb",
		addEvent(0, DeviceKeyboard, 0, "kb", 0),
		buttonEvent(0, DeviceKeyboard, 0, KeyControlID(KeySpace), true, 50*time.Millisecond),
		buttonEvent(0, DeviceKeyboard, 0, KeyControlID(KeySpace), false, 600*time.Millisecond),
		buttonEvent(0, DeviceKeyboard, 0, KeyControlID(KeyEnter), true, 700*time.Millisecond),
		buttonEvent(0, DeviceKeyboard, 0, KeyControlID(KeyEnter), false, 750*time.Millisecond),
		buttonEvent(0, DeviceKeyboard, 0, KeyControlID(KeyEnter), true, 850*time.Millisecond),
		buttonEvent(0, DeviceKeyboard, 0, KeyControlID(KeyEnter), false, 900*time.Millisecond),
	)
	s.AddBackend(kb)

	frames := tickFrames(s, 10, 100*time.Millisecond)
	// Long press: space held from t=50; at tick 6 (t=600) held 550ms -> active.
	if !frames[5][lp.ID].Pressed {
		t.Fatal("expected long press active at tick 6")
	}
	// Double tap: enter pressed at 700 and 850 (150ms apart) -> triggers at tick 9 (t=900).
	if !frames[8][dt.ID].JustPressed {
		t.Fatal("expected double tap JustPressed at tick 9")
	}
}

func TestDeadzone(t *testing.T) {
	mp := NewInputMap()
	mx, _ := mp.AddAction("move_x")
	mp.Bind(mx.ID, &StickBinding{
		Ref: DeviceRef{Kind: DeviceGamepad},
		XControl: GamepadAxisControl(AxisLeftX),
		YControl: GamepadAxisControl(AxisLeftY),
		Channel:  ChannelX,
		Deadzone: DeadzoneScaledRadial,
		Radius:   0.2,
	})
	s := NewSampler(16*time.Millisecond, mp)
	gp := newFakeBackend("gp",
		addEvent(0, DeviceGamepad, 0, "pad", 0),
		axisEvent(0, DeviceGamepad, 0, GamepadAxisControl(AxisLeftX), 0.1, 5*time.Millisecond),
		axisEvent(0, DeviceGamepad, 0, GamepadAxisControl(AxisLeftX), 0.5, 20*time.Millisecond),
	)
	s.AddBackend(gp)
	frames := tickFrames(s, 2, 16*time.Millisecond)
	if frames[0][mx.ID].Value != 0 {
		t.Fatalf("expected deadzone to zero 0.1 deflection, got %v", frames[0][mx.ID].Value)
	}
	// 0.5 with scaled radial deadzone radius 0.2: (0.5-0.2)/0.8 = 0.375.
	want := float32(0.375)
	if got := frames[1][mx.ID].Value; got < want-0.01 || got > want+0.01 {
		t.Fatalf("expected scaled deadzone value ~%v, got %v", want, got)
	}
}
