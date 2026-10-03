package input

import "time"

// DeviceID is the stable identity of an input device. It is allocated by the
// Registry on first sight and reused across disconnect/reconnect cycles for
// the same (backend, kind, port) pair, so bindings and configuration never
// need to be re-bound after a hot-plug.
type DeviceID uint32

// DeviceKind enumerates the supported device classes.
type DeviceKind uint8

const (
	DeviceKeyboard DeviceKind = iota
	DeviceMouse
	DeviceGamepad
	DeviceTouch
)

// String returns a human readable name.
func (k DeviceKind) String() string {
	switch k {
	case DeviceKeyboard:
		return "keyboard"
	case DeviceMouse:
		return "mouse"
	case DeviceGamepad:
		return "gamepad"
	case DeviceTouch:
		return "touch"
	default:
		return "unknown"
	}
}

// TouchID identifies a single touch contact within a device.
type TouchID uint32

// DeviceInfo describes a connected device.
type DeviceInfo struct {
	// ID is the stable device identity.
	ID DeviceID
	// Kind is the device class.
	Kind DeviceKind
	// Port is the backend-local port. The (backend, kind, port) triple is the
	// identity key used for stable reconnection.
	Port uint32
	// Name is a human readable device name.
	Name string
	// Connected reports whether the device is currently attached.
	Connected bool
}

// DeviceRef addresses a device by class and optional port. Port 0 means "any
// device of this kind". Bindings use DeviceRef so they survive hot-plug and
// can target whichever device of a class is currently connected.
type DeviceRef struct {
	Kind DeviceKind
	Port uint32
}

// Touch is the state of a single touch contact.
type Touch struct {
	ID TouchID
	// X, Y are normalized coordinates in [0,1].
	X, Y float32
	// StartX, StartY are the coordinates at contact start.
	StartX, StartY float32
	// StartTime is when the contact started.
	StartTime time.Duration
}

// DeviceState is an immutable-in-practice snapshot of a device's controls.
// Pointers to DeviceState handed out by the registry must be treated as
// read-only; the registry replaces the whole struct on update.
type DeviceState struct {
	Info    DeviceInfo
	Buttons map[ControlID]bool
	Axes    map[ControlID]float32
	Touches map[TouchID]Touch
}

// Clone returns a deep copy of the device state.
func (s *DeviceState) Clone() *DeviceState {
	c := &DeviceState{Info: s.Info}
	if s.Buttons != nil {
		c.Buttons = make(map[ControlID]bool, len(s.Buttons))
		for k, v := range s.Buttons {
			c.Buttons[k] = v
		}
	}
	if s.Axes != nil {
		c.Axes = make(map[ControlID]float32, len(s.Axes))
		for k, v := range s.Axes {
			c.Axes[k] = v
		}
	}
	if s.Touches != nil {
		c.Touches = make(map[TouchID]Touch, len(s.Touches))
		for k, v := range s.Touches {
			c.Touches[k] = v
		}
	}
	return c
}
