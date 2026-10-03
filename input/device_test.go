package input

import "testing"

func TestHotplugStableIdentity(t *testing.T) {
	reg := NewDeviceRegistry()

	id1, isNew := reg.Connect(DeviceGamepad, "Pad A", "serial-AAA")
	if !isNew {
		t.Fatal("first connect should be new")
	}

	// 断开再重连：身份必须保持
	reg.Disconnect(id1)
	if d, _ := reg.Lookup(id1); d.Connected {
		t.Fatal("device should be disconnected")
	}
	id2, isNew := reg.Connect(DeviceGamepad, "Pad A", "serial-AAA")
	if isNew {
		t.Fatal("reconnect should not be new")
	}
	if id1 != id2 {
		t.Fatalf("identity changed across reconnect: %s -> %s", id1, id2)
	}

	// 不同持久身份是新设备，且不复用已断开设备的 ID
	id3, _ := reg.Connect(DeviceGamepad, "Pad B", "serial-BBB")
	if id3 == id1 {
		t.Fatal("different device reused identity")
	}
}

func TestDisconnectedDeviceEventsNotMisrouted(t *testing.T) {
	reg := NewDeviceRegistry()
	idA, _ := reg.Connect(DeviceGamepad, "Pad A", "serial-AAA")
	reg.Connect(DeviceGamepad, "Pad B", "serial-BBB")

	// A 断开期间，A 的事件不得解析到 B
	reg.Disconnect(idA)
	if _, ok := reg.Resolve("serial-AAA"); ok {
		t.Fatal("disconnected device should not resolve")
	}
	idB, ok := reg.Resolve("serial-BBB")
	if !ok {
		t.Fatal("connected device B should resolve")
	}
	if idB == idA {
		t.Fatal("event misrouted to disconnected device A")
	}
}

func TestDisconnectReleasesControls(t *testing.T) {
	reg := NewDeviceRegistry()
	devID, _ := reg.Connect(DeviceKeyboard, "KB", "kb-1")
	m := NewActionMap()
	if err := m.Bind(Binding{Action: "jump", Device: DeviceKeyboard, Control: "space"}); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(m, reg, nil)
	const dt = 1.0 / 60

	e.Submit(Event{Kind: EventButtonDown, Device: devID, Control: "space"})
	e.Tick(dt)
	if !e.Snapshot().Action("jump").Active {
		t.Fatal("jump should be active")
	}

	// 断开设备：按键状态必须释放，不能卡住
	e.Submit(Event{Kind: EventDeviceDisconnected, Device: devID})
	reg.Disconnect(devID)
	e.Tick(dt)
	st := e.Snapshot().Action("jump")
	if st.Active {
		t.Fatal("jump should be released after disconnect")
	}
	if !st.Completed {
		t.Fatal("jump should report Completed on disconnect frame")
	}
}
