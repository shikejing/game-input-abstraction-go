package input_test

import (
	"fmt"
	"time"

	input "github.com/shikejing/game-input-abstraction-go/input"
)

// Example demonstrates building an input map, sampling fixed ticks and
// reading the resulting action state.
func Example() {
	mp := input.NewInputMap()
	jump, _ := mp.AddAction("jump")
	mp.Bind(jump.ID, &input.ButtonBinding{
		Ref:     input.DeviceRef{Kind: input.DeviceGamepad},
		Control: input.GamepadButtonControl(input.GamepadSouth),
	})

	s := input.NewSampler(16*time.Millisecond, mp)
	s.AddBackend(newDemoBackend())

	s.Update(16 * time.Millisecond)
	frame, _ := s.Snapshot()
	if a := frame.Actions[jump.ID]; a.JustPressed {
		fmt.Println("jump pressed")
	}
	// Output: jump pressed
}

// newDemoBackend returns a backend that announces a gamepad and presses its
// south button on the first tick.
func newDemoBackend() input.Backend {
	return &demoBackend{}
}

type demoBackend struct {
	ticked bool
}

func (b *demoBackend) Name() string { return "demo" }

func (b *demoBackend) Poll(now time.Duration) []input.Event {
	if b.ticked {
		return nil
	}
	b.ticked = true
	return []input.Event{
		{Kind: input.DeviceGamepad, Port: 0, DeviceName: "demo pad", Type: input.EventDeviceAdd, Timestamp: 0},
		{Kind: input.DeviceGamepad, Port: 0, Type: input.EventButtonDown,
			Control: input.GamepadButtonControl(input.GamepadSouth), Timestamp: 5 * time.Millisecond},
	}
}
