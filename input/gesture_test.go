package input

import (
	"math"
	"testing"
)

func TestSwipeGesture(t *testing.T) {
	g := NewGestureRecognizer(GestureConfig{
		SwipeMinDistance: 0.1,
		SwipeMinFingers:  2,
	})
	// 单指移动不触发（阈值要求双指）
	g.Handle(Event{Kind: EventTouchBegin, TouchIdx: 0, X: 0.5, Y: 0.5})
	if out := g.Handle(Event{Kind: EventTouchMove, TouchIdx: 0, X: 0.9, Y: 0.5}); len(out) != 0 {
		t.Fatalf("single finger must not trigger 2-finger swipe, got %+v", out)
	}
	// 双指同向滑动超过阈值
	g.Handle(Event{Kind: EventTouchBegin, TouchIdx: 1, X: 0.4, Y: 0.5})
	out := g.Handle(Event{Kind: EventTouchMove, TouchIdx: 1, X: 0.7, Y: 0.5})
	var swipe *GestureEvent
	for i := range out {
		if out[i].Kind == GestureSwipe {
			swipe = &out[i]
		}
	}
	if swipe == nil {
		t.Fatal("expected swipe gesture")
	}
	if swipe.Fingers != 2 || swipe.DX < 0.1 {
		t.Fatalf("bad swipe: %+v", swipe)
	}
}

func TestPinchGesture(t *testing.T) {
	g := NewGestureRecognizer(GestureConfig{PinchMinScale: 0.2})
	g.Handle(Event{Kind: EventTouchBegin, TouchIdx: 0, X: 0.4, Y: 0.5})
	g.Handle(Event{Kind: EventTouchBegin, TouchIdx: 1, X: 0.6, Y: 0.5})
	// 双指远离：距离 0.2 → 0.4，scale=2
	out := g.Handle(Event{Kind: EventTouchMove, TouchIdx: 1, X: 0.8, Y: 0.5})
	var pinch *GestureEvent
	for i := range out {
		if out[i].Kind == GesturePinch {
			pinch = &out[i]
		}
	}
	if pinch == nil {
		t.Fatal("expected pinch gesture")
	}
	if math.Abs(pinch.Scale-2.0) > 1e-9 {
		t.Fatalf("pinch scale should be 2.0, got %v", pinch.Scale)
	}
}

func TestVirtualStick(t *testing.T) {
	g := NewGestureRecognizer(GestureConfig{
		StickZones: []StickZone{
			{Name: "move", X: 0, Y: 0, W: 0.5, H: 1, Radius: 0.2, Deadzone: 0.1},
		},
	})
	// 区域外按下不捕获
	if out := g.Handle(Event{Kind: EventTouchBegin, TouchIdx: 0, X: 0.8, Y: 0.5}); len(out) != 0 {
		t.Fatalf("touch outside zone must not capture stick, got %+v", out)
	}
	g.Handle(Event{Kind: EventTouchEnd, TouchIdx: 0, X: 0.8, Y: 0.5})
	// 区域内按下捕获
	out := g.Handle(Event{Kind: EventTouchBegin, TouchIdx: 1, X: 0.25, Y: 0.5})
	if len(out) != 1 || out[0].Kind != GestureVirtualStick || !out[0].Active {
		t.Fatalf("expected stick capture, got %+v", out)
	}
	// 拖动：偏移 0.1 / radius 0.2 = 0.5，死区 0.1 → (0.5-0.1)/0.9 ≈ 0.444
	out = g.Handle(Event{Kind: EventTouchMove, TouchIdx: 1, X: 0.35, Y: 0.5})
	var stick *GestureEvent
	for i := range out {
		if out[i].Kind == GestureVirtualStick {
			stick = &out[i]
		}
	}
	if stick == nil {
		t.Fatalf("expected stick update, got %+v", out)
	}
	want := (0.5 - 0.1) / 0.9
	if math.Abs(stick.DX-want) > 1e-9 || stick.DY != 0 {
		t.Fatalf("bad stick value: got %v want %v", stick.DX, want)
	}
	// 松开：释放并归零
	out = g.Handle(Event{Kind: EventTouchEnd, TouchIdx: 1, X: 0.35, Y: 0.5})
	if len(out) != 1 || out[0].Active {
		t.Fatalf("expected stick release, got %+v", out)
	}
}

func TestGestureCoexistWithButtons(t *testing.T) {
	// 手势动作与按键动作并发：互不吞事件
	reg := NewDeviceRegistry()
	touch, _ := reg.Connect(DeviceTouch, "Touch", "touch-1")
	m := NewActionMap()
	m.Bind(Binding{Action: "move_x", Device: DeviceTouch, Control: "stick:move/x", Deadzone: 0})
	m.Bind(Binding{Action: "tap_button", Device: DeviceTouch, Control: "btn_a"})
	g := NewGestureRecognizer(GestureConfig{
		StickZones: []StickZone{{Name: "move", X: 0, Y: 0, W: 0.5, H: 1, Radius: 0.2}},
	})
	e := NewEngine(m, reg, g)
	const dt = 1.0 / 60

	// 同一帧内：摇杆拖动 + 触屏按钮按下
	e.Submit(Event{Kind: EventTouchBegin, Device: touch, TouchIdx: 0, X: 0.25, Y: 0.5})
	e.Submit(Event{Kind: EventTouchMove, Device: touch, TouchIdx: 0, X: 0.45, Y: 0.5})
	e.Submit(Event{Kind: EventButtonDown, Device: touch, Control: "btn_a"})
	e.Tick(dt)

	snap := e.Snapshot()
	if st := snap.Action("move_x"); !st.Active || st.Value <= 0.9 {
		t.Fatalf("stick action lost: %+v", st)
	}
	if st := snap.Action("tap_button"); !st.Active {
		t.Fatalf("button action swallowed by gesture: %+v", st)
	}
}

func TestGestureResetOnDisconnect(t *testing.T) {
	reg := NewDeviceRegistry()
	touch, _ := reg.Connect(DeviceTouch, "Touch", "touch-1")
	m := NewActionMap()
	m.Bind(Binding{Action: "move_x", Device: DeviceTouch, Control: "stick:move/x"})
	g := NewGestureRecognizer(GestureConfig{
		StickZones: []StickZone{{Name: "move", X: 0, Y: 0, W: 0.5, H: 1, Radius: 0.2}},
	})
	e := NewEngine(m, reg, g)
	const dt = 1.0 / 60

	e.Submit(Event{Kind: EventTouchBegin, Device: touch, TouchIdx: 0, X: 0.25, Y: 0.5})
	e.Submit(Event{Kind: EventTouchMove, Device: touch, TouchIdx: 0, X: 0.45, Y: 0.5})
	e.Tick(dt)
	if !e.Snapshot().Action("move_x").Active {
		t.Fatal("stick should be active")
	}
	// 触屏断开：手势状态复位，摇杆归零
	e.Submit(Event{Kind: EventDeviceDisconnected, Device: touch})
	reg.Disconnect(touch)
	e.Tick(dt)
	if st := e.Snapshot().Action("move_x"); st.Active {
		t.Fatalf("stick should reset on disconnect: %+v", st)
	}
}
