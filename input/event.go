package input

import "time"

// EventType identifies the kind of a raw input event.
type EventType uint8

const (
	EventDeviceAdd EventType = iota
	EventDeviceRemove
	EventButtonDown
	EventButtonUp
	EventAxisMove
	EventTouchDown
	EventTouchMove
	EventTouchUp
)

// Event is a single raw input event produced by a Backend. Events carry the
// stable DeviceID of the device they originated from; events for devices that
// are not currently connected are dropped by the registry so that events
// arriving during a reconnect window can never be misattributed.
type Event struct {
	// Seq is a backend-local monotonic sequence number.
	Seq uint64
	// Timestamp is the time at which the event occurred, relative to backend
	// start. It drives deterministic fixed-tick assignment.
	Timestamp time.Duration
	// BackendID identifies the backend that produced the event; it is set by
	// the sampler after polling. Together with Kind and Port it forms the
	// stable device identity key.
	BackendID uint16
	// DeviceID is the stable identity of the target device. Backend-produced
	// events leave this zero and identify the device via Kind+Port; synthetic
	// (gesture) events address a device directly.
	DeviceID DeviceID
	// Kind is the device class for device lifecycle events.
	Kind DeviceKind
	// Port is the backend-local device port.
	Port uint32
	// DeviceName is the human readable name for device lifecycle events.
	DeviceName string
	// Type is the event kind.
	Type EventType
	// Control identifies the button/axis/touch control on the device.
	Control ControlID
	// Value is the analog value for EventAxisMove (range depends on axis).
	Value float32
	// X, Y are normalized touch coordinates in [0,1] for touch events.
	X, Y float32
	// TouchID identifies a contact point for touch events.
	TouchID TouchID
}

// ControlID identifies a physical control (key, mouse button, gamepad button
// or axis). The high byte encodes the device kind so control spaces of
// different device kinds never collide.
type ControlID uint32

const (
	controlKindMask  ControlID = 0xFF000000
	controlIndexMask ControlID = 0x00FFFFFF
)

func makeControl(kind DeviceKind, index uint32) ControlID {
	return ControlID(kind)<<24 | ControlID(index&0x00FFFFFF)
}

// Kind returns the device kind the control belongs to.
func (c ControlID) Kind() DeviceKind { return DeviceKind(c >> 24) }

// Index returns the kind-local control index.
func (c ControlID) Index() uint32 { return uint32(c & controlIndexMask) }

// Key is a keyboard key code (backend-independent).
type Key uint16

const (
	KeyNone Key = iota
	KeyA
	KeyB
	KeyC
	KeyD
	KeyE
	KeyF
	KeyG
	KeyH
	KeyI
	KeyJ
	KeyK
	KeyL
	KeyM
	KeyN
	KeyO
	KeyP
	KeyQ
	KeyR
	KeyS
	KeyT
	KeyU
	KeyV
	KeyW
	KeyX
	KeyY
	KeyZ
	Key0
	Key1
	Key2
	Key3
	Key4
	Key5
	Key6
	Key7
	Key8
	Key9
	KeySpace
	KeyEnter
	KeyEscape
	KeyTab
	KeyBackspace
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyShift
	KeyControl
	KeyAlt
	KeySuper
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
)

// MouseButton identifies a mouse button.
type MouseButton uint8

const (
	MouseLeft MouseButton = iota
	MouseRight
	MouseMiddle
	MouseButton4
	MouseButton5
)

// GamepadButton identifies a gamepad button using Xbox-like naming.
type GamepadButton uint8

const (
	GamepadSouth GamepadButton = iota // A / cross
	GamepadEast                        // B / circle
	GamepadWest                        // X / square
	GamepadNorth                       // Y / triangle
	GamepadLeftShoulder
	GamepadRightShoulder
	GamepadLeftTrigger  // digital trigger
	GamepadRightTrigger // digital trigger
	GamepadLeftStick
	GamepadRightStick
	GamepadStart
	GamepadSelect
	GamepadHome
	GamepadDPadUp
	GamepadDPadDown
	GamepadDPadLeft
	GamepadDPadRight
)

// Axis identifies an analog axis (mouse axes, sticks, triggers).
type Axis uint8

const (
	AxisMouseX Axis = iota
	AxisMouseY
	AxisMouseDeltaX
	AxisMouseDeltaY
	AxisMouseWheel
	AxisLeftX
	AxisLeftY
	AxisRightX
	AxisRightY
	AxisLeftTrigger
	AxisRightTrigger
)

// Keyboard control constructors.
func KeyControlID(k Key) ControlID { return makeControl(DeviceKeyboard, uint32(k)) }

// MouseButtonControl returns the control id for a mouse button.
func MouseButtonControl(b MouseButton) ControlID { return makeControl(DeviceMouse, uint32(b)) }

// AxisControl returns the control id for an analog axis.
func AxisControl(a Axis) ControlID { return makeControl(DeviceMouse, uint32(a)+0x100) }

// GamepadButtonControl returns the control id for a gamepad button.
func GamepadButtonControl(b GamepadButton) ControlID {
	return makeControl(DeviceGamepad, uint32(b))
}

// GamepadAxisControl returns the control id for a gamepad axis.
func GamepadAxisControl(a Axis) ControlID { return makeControl(DeviceGamepad, uint32(a)+0x100) }

// TouchControl is the virtual control id used for touch gesture outputs.
func TouchControl() ControlID { return makeControl(DeviceTouch, 0) }
