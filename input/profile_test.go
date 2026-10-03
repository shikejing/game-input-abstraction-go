package input

import (
	"errors"
	"testing"
)

func testProfile(t *testing.T, fb FallbackPolicy) Profile {
	m := NewActionMap()
	if err := m.Bind(Binding{Action: "fire", Device: DeviceKeyboard, Control: "f"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Bind(Binding{Action: "fire", Device: DeviceGamepad, Control: "rt"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Bind(Binding{Action: "look", Device: DeviceMouse, Control: "dx"}); err != nil {
		t.Fatal(err)
	}
	return Profile{
		Platform: PlatformDesktop,
		Map:      m,
		Gestures: GestureConfig{SwipeMinDistance: 0.1, SwipeMinFingers: 2},
		Fallback: fb,
	}
}

func TestProfileValidate(t *testing.T) {
	p := testProfile(t, FallbackDisable)
	if err := p.Validate(); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}

	// 非法平台
	bad := p
	bad.Platform = "toaster"
	var ce ConfigError
	if err := bad.Validate(); !errors.As(err, &ce) || ce.Field != "Platform" {
		t.Fatalf("expected ConfigError on Platform, got %v", err)
	}

	// 非法手势阈值
	bad2 := p
	bad2.Gestures.SwipeMinDistance = -1
	if err := bad2.Validate(); !errors.As(err, &ce) {
		t.Fatalf("expected ConfigError on gestures, got %v", err)
	}

	// 空映射
	bad3 := p
	bad3.Map = nil
	if err := bad3.Validate(); !errors.As(err, &ce) {
		t.Fatalf("expected ConfigError on nil map, got %v", err)
	}
}

func TestResolveNoMissingDevices(t *testing.T) {
	reg := NewDeviceRegistry()
	reg.Connect(DeviceKeyboard, "KB", "kb-1")
	reg.Connect(DeviceMouse, "Mouse", "ms-1")
	reg.Connect(DeviceGamepad, "Pad", "pad-1")

	p := testProfile(t, FallbackError) // 即使策略是 Error，设备齐全也不报错
	m, err := Resolve(p, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.All()) != 3 {
		t.Fatalf("expected 3 bindings, got %d", len(m.All()))
	}
}

func TestResolveFallbackDisable(t *testing.T) {
	reg := NewDeviceRegistry()
	reg.Connect(DeviceKeyboard, "KB", "kb-1")
	reg.Connect(DeviceMouse, "Mouse", "ms-1")
	// 无手柄

	p := testProfile(t, FallbackDisable)
	m, err := Resolve(p, reg)
	if err != nil {
		t.Fatal(err)
	}
	// 手柄绑定被移除，键鼠保留
	if got := m.Bindings("fire"); len(got) != 1 || got[0].Device != DeviceKeyboard {
		t.Fatalf("gamepad binding should be dropped: %+v", got)
	}
	if len(m.Bindings("look")) != 1 {
		t.Fatal("mouse binding should be kept")
	}
}

func TestResolveFallbackError(t *testing.T) {
	reg := NewDeviceRegistry()
	reg.Connect(DeviceKeyboard, "KB", "kb-1")
	reg.Connect(DeviceMouse, "Mouse", "ms-1")

	p := testProfile(t, FallbackError)
	_, err := Resolve(p, reg)
	if !errors.Is(err, ErrDeviceMissing) {
		t.Fatalf("expected ErrDeviceMissing, got %v", err)
	}
}

func TestResolveFallbackToKeyboard(t *testing.T) {
	reg := NewDeviceRegistry()
	reg.Connect(DeviceKeyboard, "KB", "kb-1")
	reg.Connect(DeviceMouse, "Mouse", "ms-1")

	p := testProfile(t, FallbackToKeyboard)
	m, err := Resolve(p, reg)
	if err != nil {
		t.Fatal(err)
	}
	// 只保留键鼠绑定
	for _, b := range m.All() {
		if b.Device != DeviceKeyboard && b.Device != DeviceMouse {
			t.Fatalf("non-keyboard binding survived: %+v", b)
		}
	}
	if len(m.All()) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(m.All()))
	}
}

func TestResolveInvalidProfileDeterministic(t *testing.T) {
	reg := NewDeviceRegistry()
	p := Profile{Platform: "nope"}
	_, err := Resolve(p, reg)
	var ce ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("invalid profile must yield ConfigError, got %v", err)
	}
}
