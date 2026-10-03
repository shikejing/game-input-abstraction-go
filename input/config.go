package input

import (
	"fmt"
	"time"
)

// ConfigErrorCode identifies a class of configuration error.
type ConfigErrorCode uint8

const (
	// ErrConfigUnknownAction: a binding references an action that does not
	// exist.
	ErrConfigUnknownAction ConfigErrorCode = iota
	// ErrConfigDuplicateAction: an action name is defined more than once.
	ErrConfigDuplicateAction
	// ErrConfigInvalidBinding: a binding definition is malformed.
	ErrConfigInvalidBinding
	// ErrConfigInvalidDeadzone: a dead zone radius is out of range.
	ErrConfigInvalidDeadzone
	// ErrConfigInvalidThreshold: a gesture/timing threshold is out of range.
	ErrConfigInvalidThreshold
	// ErrConfigInvalidPlatform: an unknown platform value was used.
	ErrConfigInvalidPlatform
	// ErrConfigMissingDevice: a required device is not connected and no
	// fallback is available.
	ErrConfigMissingDevice
)

// String returns a stable machine readable error code name.
func (c ConfigErrorCode) String() string {
	switch c {
	case ErrConfigUnknownAction:
		return "unknown_action"
	case ErrConfigDuplicateAction:
		return "duplicate_action"
	case ErrConfigInvalidBinding:
		return "invalid_binding"
	case ErrConfigInvalidDeadzone:
		return "invalid_deadzone"
	case ErrConfigInvalidThreshold:
		return "invalid_threshold"
	case ErrConfigInvalidPlatform:
		return "invalid_platform"
	case ErrConfigMissingDevice:
		return "missing_device"
	default:
		return "unknown"
	}
}

// ConfigError is a deterministic, distinguishable configuration error.
type ConfigError struct {
	Code    ConfigErrorCode
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("input config error [%s] %s: %s", e.Code.String(), e.Field, e.Message)
}

// BindingType enumerates the serializable binding kinds.
type BindingType uint8

const (
	BindingButton BindingType = iota
	BindingAxis
	BindingStick
	BindingCombo
	BindingLongPress
	BindingDoubleTap
	BindingSwipe
	BindingPinch
	BindingJoystick
)

// ActionDef is the serializable definition of an action.
type ActionDef struct {
	Name string
}

// BindingDef is the serializable definition of a binding.
type BindingDef struct {
	Action    string
	Type      BindingType
	Device    DeviceRef
	Control   ControlID
	Controls  []ControlID
	Deadzone  DeadzoneMode
	Radius    float32
	Scale     float32
	Invert    bool
	Duration  time.Duration
	Interval  time.Duration
	Direction SwipeDirection
	MinDistance float32
	MaxTime   time.Duration
	MinFingers int
	MinScale  float32
	Region    Region
	Channel   AxisChannel
}

// GestureConfig holds gesture recognizer thresholds.
type GestureConfig struct {
	SwipeMinDistance float32
	SwipeMaxTime     time.Duration
}

// Config is a serializable input configuration.
type Config struct {
	Actions  []ActionDef
	Bindings []BindingDef
	Gestures GestureConfig
}

// Validate checks the configuration for errors without building anything.
func (c *Config) Validate() error {
	names := make(map[string]bool, len(c.Actions))
	for _, a := range c.Actions {
		if a.Name == "" {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "actions", Message: "action name must not be empty"}
		}
		if names[a.Name] {
			return &ConfigError{Code: ErrConfigDuplicateAction, Field: "actions", Message: "duplicate action name " + a.Name}
		}
		names[a.Name] = true
	}
	for i := range c.Bindings {
		b := &c.Bindings[i]
		if b.Action == "" {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings", Message: "binding action must not be empty"}
		}
		if !names[b.Action] {
			return &ConfigError{Code: ErrConfigUnknownAction, Field: "bindings." + b.Action, Message: "binding references unknown action " + b.Action}
		}
		if err := validateBinding(b); err != nil {
			return err
		}
	}
	if c.Gestures.SwipeMinDistance != 0 && (c.Gestures.SwipeMinDistance <= 0 || c.Gestures.SwipeMinDistance > 1) {
		return &ConfigError{Code: ErrConfigInvalidThreshold, Field: "gestures.swipe_min_distance", Message: "must be in (0,1]"}
	}
	if c.Gestures.SwipeMaxTime < 0 {
		return &ConfigError{Code: ErrConfigInvalidThreshold, Field: "gestures.swipe_max_time", Message: "must not be negative"}
	}
	return nil
}

func validateBinding(b *BindingDef) error {
	if b.Device.Kind > DeviceTouch {
		return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action, Message: "invalid device kind"}
	}
	switch b.Type {
	case BindingButton:
		if b.Control == 0 {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action, Message: "button binding requires a control"}
		}
	case BindingAxis:
		if b.Control == 0 {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action, Message: "axis binding requires a control"}
		}
		if b.Radius < 0 || b.Radius > 1 {
			return &ConfigError{Code: ErrConfigInvalidDeadzone, Field: "bindings." + b.Action + ".radius", Message: "deadzone radius must be in [0,1]"}
		}
		if b.Scale < 0 {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action + ".scale", Message: "scale must not be negative"}
		}
	case BindingStick:
		if b.Control == 0 || b.Controls == nil {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action, Message: "stick binding requires two axes"}
		}
		if b.Radius < 0 || b.Radius > 1 {
			return &ConfigError{Code: ErrConfigInvalidDeadzone, Field: "bindings." + b.Action + ".radius", Message: "deadzone radius must be in [0,1]"}
		}
	case BindingCombo:
		if len(b.Controls) == 0 {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action, Message: "combo binding requires at least one control"}
		}
	case BindingLongPress:
		if b.Control == 0 {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action, Message: "long press binding requires a control"}
		}
		if b.Duration <= 0 {
			return &ConfigError{Code: ErrConfigInvalidThreshold, Field: "bindings." + b.Action + ".duration", Message: "long press duration must be positive"}
		}
	case BindingDoubleTap:
		if b.Control == 0 {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action, Message: "double tap binding requires a control"}
		}
		if b.Interval <= 0 {
			return &ConfigError{Code: ErrConfigInvalidThreshold, Field: "bindings." + b.Action + ".interval", Message: "double tap interval must be positive"}
		}
	case BindingSwipe:
		if b.MinDistance <= 0 || b.MinDistance > 1 {
			return &ConfigError{Code: ErrConfigInvalidThreshold, Field: "bindings." + b.Action + ".min_distance", Message: "swipe min distance must be in (0,1]"}
		}
		if b.MinFingers < 1 {
			return &ConfigError{Code: ErrConfigInvalidThreshold, Field: "bindings." + b.Action + ".min_fingers", Message: "min fingers must be >= 1"}
		}
		if b.MaxTime < 0 {
			return &ConfigError{Code: ErrConfigInvalidThreshold, Field: "bindings." + b.Action + ".max_time", Message: "max time must not be negative"}
		}
	case BindingPinch:
		if b.MinScale < 0 {
			return &ConfigError{Code: ErrConfigInvalidThreshold, Field: "bindings." + b.Action + ".min_scale", Message: "pinch min scale must not be negative"}
		}
	case BindingJoystick:
		if b.Region.W <= 0 || b.Region.H <= 0 {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action + ".region", Message: "joystick region must have positive size"}
		}
		if b.Region.X < 0 || b.Region.Y < 0 || b.Region.X+b.Region.W > 1 || b.Region.Y+b.Region.H > 1 {
			return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action + ".region", Message: "joystick region must be within [0,1]"}
		}
	default:
		return &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action, Message: "unknown binding type"}
	}
	return nil
}

// Build constructs an InputMap from the configuration. The configuration is
// validated first; an invalid configuration produces a distinguishable
// ConfigError and no partial map is returned.
func (c *Config) Build() (*InputMap, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	m := NewInputMap()
	for _, a := range c.Actions {
		if _, err := m.AddAction(a.Name); err != nil {
			return nil, err
		}
	}
	for i := range c.Bindings {
		b := &c.Bindings[i]
		action, ok := m.Action(b.Action)
		if !ok {
			return nil, &ConfigError{Code: ErrConfigUnknownAction, Field: "bindings." + b.Action, Message: "unknown action"}
		}
		binding, err := buildBinding(b)
		if err != nil {
			return nil, err
		}
		m.Bind(action.ID, binding)
	}
	return m, nil
}

func buildBinding(b *BindingDef) (Binding, error) {
	switch b.Type {
	case BindingButton:
		return &ButtonBinding{Ref: b.Device, Control: b.Control}, nil
	case BindingAxis:
		return &AxisBinding{Ref: b.Device, Control: b.Control, Deadzone: b.Deadzone, Radius: b.Radius, Scale: scaleOr(b.Scale), Invert: b.Invert}, nil
	case BindingStick:
		if len(b.Controls) != 2 {
			return nil, &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action, Message: "stick binding requires exactly two axes"}
		}
		return &StickBinding{Ref: b.Device, XControl: b.Controls[0], YControl: b.Controls[1], Channel: b.Channel, Deadzone: b.Deadzone, Radius: b.Radius, Scale: scaleOr(b.Scale), Invert: b.Invert}, nil
	case BindingCombo:
		return &ComboBinding{Ref: b.Device, Buttons: b.Controls}, nil
	case BindingLongPress:
		return &LongPressBinding{Ref: b.Device, Control: b.Control, Duration: b.Duration}, nil
	case BindingDoubleTap:
		return &DoubleTapBinding{Ref: b.Device, Control: b.Control, MaxInterval: b.Interval}, nil
	case BindingSwipe:
		return &SwipeBinding{Ref: b.Device, Direction: b.Direction, MinDistance: b.MinDistance, MaxTime: b.MaxTime, MinFingers: maxInt(b.MinFingers, 1)}, nil
	case BindingPinch:
		return &PinchBinding{Ref: b.Device, MinScale: b.MinScale}, nil
	case BindingJoystick:
		return &JoystickBinding{Ref: b.Device, Region: b.Region, Channel: b.Channel, Radius: b.Radius, Invert: b.Invert}, nil
	default:
		return nil, &ConfigError{Code: ErrConfigInvalidBinding, Field: "bindings." + b.Action, Message: "unknown binding type"}
	}
}

func scaleOr(v float32) float32 {
	if v == 0 {
		return 1
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
