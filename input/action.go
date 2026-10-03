package input

// ActionID is the stable identifier of a logical action within an InputMap.
type ActionID uint32

// Action is a named logical action.
type Action struct {
	ID   ActionID
	Name string
}

// ActionState is the evaluated state of an action for a single frame.
type ActionState struct {
	// Name is the action name (for debugging/UI).
	Name string
	// Pressed is true while the action is active.
	Pressed bool
	// JustPressed is true only on the frame the action transitioned to active.
	JustPressed bool
	// JustReleased is true only on the frame the action transitioned to inactive.
	JustReleased bool
	// Value is the aggregated analog value of the action, in [-1,1] or [0,1]
	// depending on the binding. Digital bindings report 0 or 1.
	Value float32
}
