package input

import (
	"sort"
	"sync"
)

// portKey is the stable identity key of a device: the backend that produced
// it, its device class and its backend-local port.
type portKey struct {
	backendID uint16
	kind      DeviceKind
	port      uint32
}

// Registry tracks device lifecycles. It allocates stable DeviceIDs on first
// sight and reuses them across disconnect/reconnect cycles for the same
// (backend, kind, port) triple. Events that arrive while a device is
// disconnected (or before it was ever announced) are dropped and counted as
// dead letters; they can never be misattributed to another device.
type Registry struct {
	mu sync.RWMutex

	nextBackendID uint16
	nextDeviceID  DeviceID

	ports   map[portKey]DeviceID
	devices map[DeviceID]*DeviceState
	info    map[DeviceID]DeviceInfo

	// deadLetters counts events dropped because their device was not
	// connected at the time they were processed.
	deadLetters uint64
}

// NewRegistry creates an empty device registry.
func NewRegistry() *Registry {
	return &Registry{
		nextDeviceID: 1,
		ports:        make(map[portKey]DeviceID),
		devices:      make(map[DeviceID]*DeviceState),
		info:         make(map[DeviceID]DeviceInfo),
	}
}

// RegisterBackend assigns a stable backend id used in event identity keys.
func (r *Registry) RegisterBackend(name string) uint16 {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.nextBackendID
	r.nextBackendID++
	return id
}

// Process applies a batch of raw events to the device states. Device
// add/remove events update the lifecycle; input events are only applied when
// their target device is currently connected.
func (r *Registry) Process(events []Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ev := range events {
		switch ev.Type {
		case EventDeviceAdd:
			r.handleAdd(ev)
		case EventDeviceRemove:
			r.handleRemove(ev)
		default:
			r.handleInput(ev)
		}
	}
}

func (r *Registry) handleAdd(ev Event) {
	key := portKey{backendID: ev.BackendID, kind: ev.Kind, port: ev.Port}
	id, ok := r.ports[key]
	if !ok {
		id = r.nextDeviceID
		r.nextDeviceID++
		r.ports[key] = id
	}
	info := DeviceInfo{
		ID:        id,
		Kind:      ev.Kind,
		Port:      ev.Port,
		Name:      ev.DeviceName,
		Connected: true,
	}
	r.info[id] = info
	// A (re)connected device starts with a pristine state: stale buttons from
	// a previous session must not leak into the new one.
	r.devices[id] = &DeviceState{
		Info:    info,
		Buttons: make(map[ControlID]bool),
		Axes:    make(map[ControlID]float32),
		Touches: make(map[TouchID]Touch),
	}
}

func (r *Registry) handleRemove(ev Event) {
	key := portKey{backendID: ev.BackendID, kind: ev.Kind, port: ev.Port}
	id, ok := r.ports[key]
	if !ok {
		return
	}
	info := r.info[id]
	info.Connected = false
	r.info[id] = info
	if st, ok := r.devices[id]; ok {
		st.Info = info
	}
}

func (r *Registry) handleInput(ev Event) {
	key := portKey{backendID: ev.BackendID, kind: ev.Kind, port: ev.Port}
	id, ok := r.ports[key]
	if !ok {
		r.deadLetters++
		return
	}
	st, ok := r.devices[id]
	if !ok || !st.Info.Connected {
		r.deadLetters++
		return
	}
	switch ev.Type {
	case EventButtonDown:
		st.Buttons[ev.Control] = true
	case EventButtonUp:
		st.Buttons[ev.Control] = false
	case EventAxisMove:
		st.Axes[ev.Control] = ev.Value
	case EventTouchDown:
		st.Touches[ev.TouchID] = Touch{
			ID:         ev.TouchID,
			X:          ev.X,
			Y:          ev.Y,
			StartX:     ev.X,
			StartY:     ev.Y,
			StartTime:  ev.Timestamp,
		}
	case EventTouchMove:
		if t, ok := st.Touches[ev.TouchID]; ok {
			t.X, t.Y = ev.X, ev.Y
			st.Touches[ev.TouchID] = t
		}
	case EventTouchUp:
		delete(st.Touches, ev.TouchID)
	}
}

// Resolve returns the stable device id for a device reference, if a matching
// device is currently connected.
func (r *Registry) Resolve(ref DeviceRef) (DeviceID, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for id, info := range r.info {
		if !info.Connected {
			continue
		}
		if info.Kind != ref.Kind {
			continue
		}
		if ref.Port != 0 && info.Port != ref.Port {
			continue
		}
		if _, ok := r.devices[id]; ok {
			return id, true
		}
	}
	return 0, false
}

// Device returns a deep copy of the device state for the given id.
func (r *Registry) Device(id DeviceID) (*DeviceState, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	st, ok := r.devices[id]
	if !ok {
		return nil, false
	}
	return st.Clone(), true
}

// Info returns the retained device info for an id (works for disconnected
// devices as well).
func (r *Registry) Info(id DeviceID) (DeviceInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.info[id]
	return info, ok
}

// Devices returns deep copies of all currently connected devices, sorted by
// id for deterministic iteration.
func (r *Registry) Devices() []*DeviceState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*DeviceState, 0, len(r.devices))
	for _, st := range r.devices {
		if st.Info.Connected {
			out = append(out, st.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Info.ID < out[j].Info.ID })
	return out
}

// DeadLetters returns the number of events dropped because their target device
// was disconnected or unknown.
func (r *Registry) DeadLetters() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.deadLetters
}

// DeviceCount returns the number of devices ever seen (including disconnected).
func (r *Registry) DeviceCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.devices)
}
