package input

import (
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		code ConfigErrorCode
	}{
		{
			name: "duplicate action",
			cfg:  Config{Actions: []ActionDef{{Name: "a"}, {Name: "a"}}},
			code: ErrConfigDuplicateAction,
		},
		{
			name: "unknown action",
			cfg: Config{
				Actions:  []ActionDef{{Name: "a"}},
				Bindings: []BindingDef{{Action: "b", Type: BindingButton, Device: DeviceRef{Kind: DeviceKeyboard}, Control: KeyControlID(KeyA)}},
			},
			code: ErrConfigUnknownAction,
		},
		{
			name: "missing control",
			cfg: Config{
				Actions:  []ActionDef{{Name: "a"}},
				Bindings: []BindingDef{{Action: "a", Type: BindingButton, Device: DeviceRef{Kind: DeviceKeyboard}}},
			},
			code: ErrConfigInvalidBinding,
		},
		{
			name: "bad deadzone",
			cfg: Config{
				Actions: []ActionDef{{Name: "a"}},
				Bindings: []BindingDef{{
					Action: "a", Type: BindingAxis, Device: DeviceRef{Kind: DeviceGamepad},
					Control: GamepadAxisControl(AxisLeftX), Radius: 1.5,
				}},
			},
			code: ErrConfigInvalidDeadzone,
		},
		{
			name: "long press without duration",
			cfg: Config{
				Actions:  []ActionDef{{Name: "a"}},
				Bindings: []BindingDef{{Action: "a", Type: BindingLongPress, Device: DeviceRef{Kind: DeviceKeyboard}, Control: KeyControlID(KeyA)}},
			},
			code: ErrConfigInvalidThreshold,
		},
		{
			name: "swipe without distance",
			cfg: Config{
				Actions:  []ActionDef{{Name: "a"}},
				Bindings: []BindingDef{{Action: "a", Type: BindingSwipe, Device: DeviceRef{Kind: DeviceTouch}, Direction: SwipeUp, MinFingers: 2}},
			},
			code: ErrConfigInvalidThreshold,
		},
		{
			name: "joystick bad region",
			cfg: Config{
				Actions:  []ActionDef{{Name: "a"}},
				Bindings: []BindingDef{{Action: "a", Type: BindingJoystick, Device: DeviceRef{Kind: DeviceTouch}, Region: Region{X: 0.8, Y: 0, W: 0.5, H: 0.5}}},
			},
			code: ErrConfigInvalidBinding,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if err == nil {
				t.Fatal("expected validation error")
			}
			ce, ok := err.(*ConfigError)
			if !ok {
				t.Fatalf("expected *ConfigError, got %T", err)
			}
			if ce.Code != tc.code {
				t.Fatalf("expected code %s, got %s", tc.code, ce.Code)
			}
		})
	}
}

func TestConfigBuild(t *testing.T) {
	cfg := Config{
		Actions: []ActionDef{{Name: "jump"}, {Name: "move_x"}},
		Bindings: []BindingDef{
			{Action: "jump", Type: BindingButton, Device: DeviceRef{Kind: DeviceGamepad}, Control: GamepadButtonControl(GamepadSouth)},
			{Action: "move_x", Type: BindingAxis, Device: DeviceRef{Kind: DeviceGamepad}, Control: GamepadAxisControl(AxisLeftX), Deadzone: DeadzoneAxial, Radius: 0.2},
		},
	}
	mp, err := cfg.Build()
	if err != nil {
		t.Fatal(err)
	}
	jump, ok := mp.Action("jump")
	if !ok {
		t.Fatal("jump action missing")
	}
	if len(mp.BindingsOf(jump.ID)) != 1 {
		t.Fatal("jump binding missing")
	}
}

func TestPlatformFallback(t *testing.T) {
	primary := NewInputMap()
	primary.AddAction("jump")
	fallback := NewInputMap()
	fallback.AddAction("jump")

	scheme := &PlatformScheme{
		Platform:    PlatformConsole,
		Map:         primary,
		Required:    []DeviceKind{DeviceGamepad},
		Fallback:    FallbackKeyboardAsGamepad,
		FallbackMap: fallback,
	}

	// No gamepad connected -> degraded fallback.
	reg := NewRegistry()
	resolved, err := scheme.Resolve(reg)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.Degraded || resolved.Map != fallback {
		t.Fatalf("expected degraded fallback, got %+v", resolved)
	}

	// Gamepad connected -> primary map.
	reg2 := NewRegistry()
	reg2.Process([]Event{{Kind: DeviceGamepad, Port: 0, Type: EventDeviceAdd}})
	resolved2, err := scheme.Resolve(reg2)
	if err != nil {
		t.Fatal(err)
	}
	if resolved2.Degraded || resolved2.Map != primary {
		t.Fatalf("expected primary map, got %+v", resolved2)
	}
}

func TestPlatformFallbackNone(t *testing.T) {
	primary := NewInputMap()
	primary.AddAction("jump")
	scheme := &PlatformScheme{
		Platform: PlatformConsole,
		Map:      primary,
		Required: []DeviceKind{DeviceGamepad},
		Fallback: FallbackNone,
	}
	reg := NewRegistry()
	_, err := scheme.Resolve(reg)
	if err == nil {
		t.Fatal("expected missing device error")
	}
	ce, ok := err.(*ConfigError)
	if !ok || ce.Code != ErrConfigMissingDevice {
		t.Fatalf("expected ErrConfigMissingDevice, got %v", err)
	}
}

// TestLongPressNotActiveBeforeDuration verifies long press timing.
func TestLongPressNotActiveBeforeDuration(t *testing.T) {
	mp := NewInputMap()
	lp, _ := mp.AddAction("long")
	mp.Bind(lp.ID, &LongPressBinding{Ref: DeviceRef{Kind: DeviceKeyboard}, Control: KeyControlID(KeySpace), Duration: 500 * time.Millisecond})
	s := NewSampler(100*time.Millisecond, mp)
	kb := newFakeBackend("kb",
		addEvent(0, DeviceKeyboard, 0, "kb", 0),
		buttonEvent(0, DeviceKeyboard, 0, KeyControlID(KeySpace), true, 50*time.Millisecond),
	)
	s.AddBackend(kb)
	frames := tickFrames(s, 4, 100*time.Millisecond)
	for i, f := range frames {
		if f[lp.ID].Pressed {
			t.Fatalf("tick %d: long press should not be active before duration", i)
		}
	}
}
