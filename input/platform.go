package input

// Platform selects a platform-specific mapping scheme.
type Platform uint8

const (
	// PlatformPC: keyboard + mouse is the primary input.
	PlatformPC Platform = iota
	// PlatformConsole: gamepad is the primary input.
	PlatformConsole
	// PlatformMobile: touch is the primary input.
	PlatformMobile
)

// String returns a human readable platform name.
func (p Platform) String() string {
	switch p {
	case PlatformPC:
		return "pc"
	case PlatformConsole:
		return "console"
	case PlatformMobile:
		return "mobile"
	default:
		return "unknown"
	}
}

// FallbackPolicy determines what happens when a platform's required devices
// are not connected.
type FallbackPolicy uint8

const (
	// FallbackNone: resolution fails with ErrConfigMissingDevice.
	FallbackNone FallbackPolicy = iota
	// FallbackKeyboardAsGamepad: use the fallback map (keyboard bindings)
	// and report a degraded scheme.
	FallbackKeyboardAsGamepad
	// FallbackTouchAsMouse: use the fallback map (touch bindings) and report
	// a degraded scheme.
	FallbackTouchAsMouse
)

// PlatformScheme bundles an input map with the devices it requires and the
// fallback policy to apply when they are missing.
type PlatformScheme struct {
	Platform    Platform
	Map         *InputMap
	Required    []DeviceKind
	Fallback    FallbackPolicy
	FallbackMap *InputMap
}

// ResolvedScheme is the outcome of scheme resolution.
type ResolvedScheme struct {
	Platform Platform
	// Map is the input map to use (primary or fallback).
	Map *InputMap
	// Degraded reports whether a fallback map is in use.
	Degraded bool
	// Missing lists the required device kinds that are not connected.
	Missing []DeviceKind
}

// Resolve checks device availability against the registry and returns the
// scheme to use. The decision is deterministic for a given registry state.
func (s *PlatformScheme) Resolve(reg *Registry) (*ResolvedScheme, error) {
	connected := make(map[DeviceKind]bool)
	for _, st := range reg.Devices() {
		connected[st.Info.Kind] = true
	}
	var missing []DeviceKind
	for _, k := range s.Required {
		if !connected[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return &ResolvedScheme{Platform: s.Platform, Map: s.Map}, nil
	}
	if s.Fallback != FallbackNone && s.FallbackMap != nil {
		return &ResolvedScheme{
			Platform: s.Platform,
			Map:      s.FallbackMap,
			Degraded: true,
			Missing:  missing,
		}, nil
	}
	return nil, &ConfigError{
		Code:    ErrConfigMissingDevice,
		Field:   "scheme",
		Message: "required device(s) missing: " + kindsString(missing),
	}
}

func kindsString(kinds []DeviceKind) string {
	out := ""
	for i, k := range kinds {
		if i > 0 {
			out += ","
		}
		out += k.String()
	}
	return out
}
