package input

import (
	"errors"
	"testing"
)

func TestBindAndConflictDetection(t *testing.T) {
	m := NewActionMap()
	b1 := Binding{Action: "fire", Device: DeviceKeyboard, Control: "ctrl", Modifiers: []ControlID{"shift"}}
	if err := m.Bind(b1); err != nil {
		t.Fatal(err)
	}
	// 修饰键顺序不同视为同一绑定 → 冲突
	b2 := Binding{Action: "reload", Device: DeviceKeyboard, Control: "ctrl", Modifiers: []ControlID{"shift"}}
	err := m.Bind(b2)
	var c Conflict
	if !errors.As(err, &c) {
		t.Fatalf("expected Conflict, got %v", err)
	}
	if c.ActionA != "fire" || c.ActionB != "reload" {
		t.Fatalf("conflict actions wrong: %+v", c)
	}
	// 冲突后映射未被修改
	if got := m.Bindings("reload"); len(got) != 0 {
		t.Fatal("conflicting bind must not take effect")
	}
	// 不同触发方式不冲突
	b3 := Binding{Action: "reload", Device: DeviceKeyboard, Control: "ctrl", Modifiers: []ControlID{"shift"}, Activation: ActivateLongPress, LongPressDuration: 0.5}
	if err := m.Bind(b3); err != nil {
		t.Fatalf("different activation should not conflict: %v", err)
	}
}

func TestRebindAtomic(t *testing.T) {
	m := NewActionMap()
	m.Bind(Binding{Action: "fire", Device: DeviceMouse, Control: "left"})
	m.Bind(Binding{Action: "jump", Device: DeviceKeyboard, Control: "space"})

	// 重绑定与既有映射冲突 → 整体不生效，原绑定保留
	conflicts := m.Rebind("jump", []Binding{
		{Action: "jump", Device: DeviceMouse, Control: "left"},
	})
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if got := m.Bindings("jump"); len(got) != 1 || got[0].Control != "space" {
		t.Fatal("failed rebind must leave original binding intact")
	}

	// 合法重绑定 → 原子替换
	if cs := m.Rebind("jump", []Binding{
		{Action: "jump", Device: DeviceGamepad, Control: "a"},
		{Action: "jump", Device: DeviceKeyboard, Control: "w", Modifiers: []ControlID{"shift"}},
	}); len(cs) != 0 {
		t.Fatalf("valid rebind failed: %v", cs)
	}
	if got := m.Bindings("jump"); len(got) != 2 {
		t.Fatalf("rebind should install 2 bindings, got %d", len(got))
	}
}

func TestInvalidBindingRejected(t *testing.T) {
	m := NewActionMap()
	for _, b := range []Binding{
		{Action: "", Control: "a"},
		{Action: "x", Control: ""},
		{Action: "x", Control: "a", Deadzone: 1.5},
		{Action: "x", Control: "a", Deadzone: -0.1},
		{Action: "x", Control: "a", LongPressDuration: -1},
		{Action: "x", Control: "a", Modifiers: []ControlID{"a"}},
	} {
		if err := m.Bind(b); !errors.Is(err, ErrInvalidBinding) {
			t.Fatalf("binding %+v: expected ErrInvalidBinding, got %v", b, err)
		}
	}
	if len(m.All()) != 0 {
		t.Fatal("invalid bindings must not be stored")
	}
}

func TestComboAndActivationEvaluation(t *testing.T) {
	reg := NewDeviceRegistry()
	kb, _ := reg.Connect(DeviceKeyboard, "KB", "kb-1")
	m := NewActionMap()
	m.Bind(Binding{Action: "skill", Device: DeviceKeyboard, Control: "q", Modifiers: []ControlID{"shift"}})
	m.Bind(Binding{Action: "charge", Device: DeviceKeyboard, Control: "e", Activation: ActivateLongPress, LongPressDuration: 0.5})
	m.Bind(Binding{Action: "dash", Device: DeviceKeyboard, Control: "d", Activation: ActivateDoubleTap, DoubleTapWindow: 0.3})
	e := NewEngine(m, reg, nil)
	const dt = 0.1

	// 组合键：单独按 q 不触发
	e.Submit(Event{Kind: EventButtonDown, Device: kb, Control: "q"})
	e.Tick(dt)
	if e.Snapshot().Action("skill").Active {
		t.Fatal("combo should require modifier")
	}
	// 按住 shift 后触发
	e.Submit(Event{Kind: EventButtonDown, Device: kb, Control: "shift"})
	e.Tick(dt)
	if !e.Snapshot().Action("skill").Active {
		t.Fatal("combo should trigger with modifier held")
	}

	// 长按：不足时长不触发，达到时长后触发
	e.Submit(Event{Kind: EventButtonDown, Device: kb, Control: "e"})
	e.Tick(dt) // held 0.1
	e.Tick(dt) // held 0.2
	e.Tick(dt) // held 0.3
	if e.Snapshot().Action("charge").Active {
		t.Fatal("long press fired too early")
	}
	e.Tick(dt) // held 0.4
	e.Tick(dt) // held 0.5
	if !e.Snapshot().Action("charge").Active {
		t.Fatal("long press should fire after duration")
	}

	// 双击：窗口内第二次按下触发
	e.Submit(Event{Kind: EventButtonDown, Device: kb, Control: "d"})
	e.Submit(Event{Kind: EventButtonUp, Device: kb, Control: "d"})
	e.Tick(dt)
	e.Tick(dt)
	if e.Snapshot().Action("dash").Active {
		t.Fatal("single tap must not trigger double tap")
	}
	e.Submit(Event{Kind: EventButtonDown, Device: kb, Control: "d"})
	e.Tick(dt)
	if !e.Snapshot().Action("dash").Active {
		t.Fatal("double tap should fire within window")
	}
}

func TestAxisDeadzoneAndNormalize(t *testing.T) {
	reg := NewDeviceRegistry()
	pad, _ := reg.Connect(DeviceGamepad, "Pad", "pad-1")
	m := NewActionMap()
	m.Bind(Binding{Action: "move_x", Device: DeviceGamepad, Control: "lx", Deadzone: 0.2})
	e := NewEngine(m, reg, nil)
	const dt = 1.0 / 60

	// 死区内 → 0
	e.Submit(Event{Kind: EventAxisMove, Device: pad, Control: "lx", Value: 0.15})
	e.Tick(dt)
	if st := e.Snapshot().Action("move_x"); st.Active || st.Value != 0 {
		t.Fatalf("value inside deadzone must be 0, got %+v", st)
	}
	// 死区外 → 重新归一化：(0.6-0.2)/(1-0.2)=0.5
	e.Submit(Event{Kind: EventAxisMove, Device: pad, Control: "lx", Value: 0.6})
	e.Tick(dt)
	if st := e.Snapshot().Action("move_x"); !st.Active || st.Value < 0.49 || st.Value > 0.51 {
		t.Fatalf("normalized value should be ~0.5, got %+v", st)
	}
}

func TestCrossBackendConsistency(t *testing.T) {
	// 同一动作映射到键鼠与手柄：各后端结果必须一致
	reg := NewDeviceRegistry()
	kb, _ := reg.Connect(DeviceKeyboard, "KB", "kb-1")
	pad, _ := reg.Connect(DeviceGamepad, "Pad", "pad-1")
	m := NewActionMap()
	m.Bind(Binding{Action: "fire", Device: DeviceKeyboard, Control: "f"})
	m.Bind(Binding{Action: "fire", Device: DeviceGamepad, Control: "rt_btn"})
	e := NewEngine(m, reg, nil)
	const dt = 1.0 / 60

	e.Submit(Event{Kind: EventButtonDown, Device: kb, Control: "f"})
	e.Tick(dt)
	kbResult := e.Snapshot().Action("fire")
	e.Submit(Event{Kind: EventButtonUp, Device: kb, Control: "f"})
	e.Tick(dt)

	e.Submit(Event{Kind: EventButtonDown, Device: pad, Control: "rt_btn"})
	e.Tick(dt)
	padResult := e.Snapshot().Action("fire")

	if kbResult.Active != padResult.Active || kbResult.Value != padResult.Value {
		t.Fatalf("backends disagree: kb=%+v pad=%+v", kbResult, padResult)
	}
}
