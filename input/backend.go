package input

import "time"

// Backend is a platform-specific source of raw input events. Implementations
// must be non-blocking: Poll returns the events accumulated since the previous
// call and never waits for input.
type Backend interface {
	// Name returns a unique backend name, used as part of the stable device
	// identity key.
	Name() string
	// Poll returns all events accumulated since the last call. It must be
	// safe to call from a single goroutine (the sampling goroutine).
	Poll(now time.Duration) []Event
}

// DeviceEnumerator is an optional Backend capability for backends that can
// report the devices they currently know about.
type DeviceEnumerator interface {
	Devices() []DeviceInfo
}
