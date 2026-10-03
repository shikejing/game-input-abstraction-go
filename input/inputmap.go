package input

import (
	"reflect"
	"sort"
	"sync"
)

// BindConflict describes why a bind request was not applied.
type BindConflict uint8

const (
	// ConflictNone means the binding was added successfully.
	ConflictNone BindConflict = iota
	// ConflictDuplicate means an identical binding already exists on the
	// same action (the request was a no-op).
	ConflictDuplicate
	// ConflictControlTaken means the same physical control is already bound
	// to a different action.
	ConflictControlTaken
)

// String returns a human readable conflict name.
func (c BindConflict) String() string {
	switch c {
	case ConflictNone:
		return "none"
	case ConflictDuplicate:
		return "duplicate"
	case ConflictControlTaken:
		return "control_taken"
	default:
		return "unknown"
	}
}

// BindResult describes the outcome of a bind request.
type BindResult struct {
	// Added reports whether the binding was added.
	Added bool
	// Conflict is the conflict kind, if any.
	Conflict BindConflict
	// ConflictingAction is the other action that already uses the control
	// (set for ConflictControlTaken).
	ConflictingAction ActionID
}

// InputMap maps logical actions to bindings. It is safe for concurrent use:
// rebinding can happen from game logic while the sampler evaluates.
type InputMap struct {
	mu sync.RWMutex

	nextActionID ActionID
	actions      map[ActionID]*Action
	nameIndex    map[string]ActionID
	bindings     map[ActionID][]Binding
}

// NewInputMap creates an empty input map.
func NewInputMap() *InputMap {
	return &InputMap{
		nextActionID: 1,
		actions:      make(map[ActionID]*Action),
		nameIndex:    make(map[string]ActionID),
		bindings:     make(map[ActionID][]Binding),
	}
}

// AddAction registers a new named action. It returns an error if the name is
// already taken.
func (m *InputMap) AddAction(name string) (Action, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.nameIndex[name]; ok {
		return Action{}, &ConfigError{
			Code:    ErrConfigDuplicateAction,
			Field:   "action",
			Message: "action " + name + " already exists",
		}
	}
	a := &Action{ID: m.nextActionID, Name: name}
	m.nextActionID++
	m.actions[a.ID] = a
	m.nameIndex[name] = a.ID
	return *a, nil
}

// Action looks up an action by name.
func (m *InputMap) Action(name string) (Action, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.nameIndex[name]
	if !ok {
		return Action{}, false
	}
	return *m.actions[id], true
}

// ActionByID looks up an action by id.
func (m *InputMap) ActionByID(id ActionID) (Action, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.actions[id]
	if !ok {
		return Action{}, false
	}
	return *a, true
}

// Actions returns all actions sorted by id.
func (m *InputMap) Actions() []Action {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Action, 0, len(m.actions))
	for _, a := range m.actions {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// BindingsOf returns the bindings of an action.
func (m *InputMap) BindingsOf(id ActionID) []Binding {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Binding, len(m.bindings[id]))
	copy(out, m.bindings[id])
	return out
}

// Bind adds a binding to an action. Conflicts are reported via BindResult and
// the binding is not added unless it is conflict-free.
func (m *InputMap) Bind(actionID ActionID, b Binding) BindResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bindLocked(actionID, b, false)
}

// ForceBind adds a binding, removing any conflicting bindings from other
// actions first.
func (m *InputMap) ForceBind(actionID ActionID, b Binding) BindResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bindLocked(actionID, b, true)
}

func (m *InputMap) bindLocked(actionID ActionID, b Binding, force bool) BindResult {
	if _, ok := m.actions[actionID]; !ok {
		return BindResult{Conflict: ConflictNone}
	}
	// Duplicate check on the same action.
	for _, existing := range m.bindings[actionID] {
		if reflect.DeepEqual(existing, b) {
			return BindResult{Added: false, Conflict: ConflictDuplicate}
		}
	}
	// Control conflict check against other actions.
	for _, ref := range b.Controls() {
		for otherID, otherBindings := range m.bindings {
			if otherID == actionID {
				continue
			}
			for _, ob := range otherBindings {
				for _, or := range ob.Controls() {
					if or == ref {
						if force {
							m.removeBindingLocked(otherID, ob)
							break
						}
						return BindResult{
							Added:             false,
							Conflict:          ConflictControlTaken,
							ConflictingAction: otherID,
						}
					}
				}
			}
		}
	}
	m.bindings[actionID] = append(m.bindings[actionID], b)
	return BindResult{Added: true, Conflict: ConflictNone}
}

func (m *InputMap) removeBindingLocked(actionID ActionID, b Binding) {
	list := m.bindings[actionID]
	for i, ob := range list {
		if reflect.DeepEqual(ob, b) {
			m.bindings[actionID] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

// UnbindControl removes all bindings of an action that use the given control.
func (m *InputMap) UnbindControl(actionID ActionID, ref ControlRef) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.bindings[actionID][:0]
	removed := 0
	for _, b := range m.bindings[actionID] {
		match := false
		for _, r := range b.Controls() {
			if r == ref {
				match = true
				break
			}
		}
		if match {
			removed++
			continue
		}
		kept = append(kept, b)
	}
	m.bindings[actionID] = kept
	return removed
}

// ClearBindings removes all bindings of an action.
func (m *InputMap) ClearBindings(actionID ActionID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bindings, actionID)
}

// ResetBindings clears timing state of all bindings (e.g. after a device
// reconnect or a replay restart).
func (m *InputMap) ResetBindings() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, list := range m.bindings {
		for _, b := range list {
			b.Reset()
		}
	}
}

// Evaluate evaluates all actions for one tick. prev holds the previous frame's
// action states for edge detection. The returned map is keyed by action id.
func (m *InputMap) Evaluate(ctx *EvalContext, prev map[ActionID]ActionState) map[ActionID]ActionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[ActionID]ActionState, len(m.actions))
	for id, a := range m.actions {
		var value float32
		pressed := false
		for _, b := range m.bindings[id] {
			v := b.Value(ctx)
			if v != 0 {
				pressed = true
			}
			if abs32(v) > abs32(value) {
				value = v
			}
		}
		p := prev[id]
		out[id] = ActionState{
			Name:         a.Name,
			Value:        value,
			Pressed:      pressed,
			JustPressed:  pressed && !p.Pressed,
			JustReleased: !pressed && p.Pressed,
		}
	}
	return out
}
