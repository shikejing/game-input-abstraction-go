package input

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Activation 表示绑定触发方式。
type Activation int

const (
	ActivatePress     Activation = iota // 按下即触发
	ActivateLongPress                   // 长按触发
	ActivateDoubleTap                   // 双击触发
)

// Binding 把一个物理控件（可带组合修饰键）映射到一个动作。
type Binding struct {
	Action     string
	Device     DeviceKind  // 适用的设备类别
	Control    ControlID   // 主控件
	Modifiers  []ControlID // 组合键：需同时按住
	Activation Activation
	// 模拟量配置（仅轴类控件有效）
	Deadzone float64 // 死区 [0,1)
	Scale    float64 // 归一化缩放
	Invert   bool
	// 时长参数（秒）
	LongPressDuration float64
	DoubleTapWindow   float64
}

// Conflict 描述一次映射冲突。
type Conflict struct {
	ActionA, ActionB string
	Binding          Binding // 冲突的绑定（归一化后等价）
}

// Error 返回冲突描述。
func (c Conflict) Error() string {
	return fmt.Sprintf("input: binding conflict between action %q and %q on %s",
		c.ActionA, c.ActionB, c.Binding.triggerKey())
}

// ErrInvalidBinding 表示绑定配置非法。
var ErrInvalidBinding = errors.New("input: invalid binding")

// triggerKey 返回绑定的归一化触发键：设备类别 + 主控件 + 排序后的修饰键 + 触发方式。
// 修饰键顺序不同视为同一绑定，保证冲突判定确定。
func (b Binding) triggerKey() string {
	mods := make([]string, 0, len(b.Modifiers))
	seen := map[ControlID]bool{}
	for _, m := range b.Modifiers {
		if !seen[m] {
			seen[m] = true
			mods = append(mods, string(m))
		}
	}
	sort.Strings(mods)
	return fmt.Sprintf("%d|%s|%s|%d", b.Device, b.Control, strings.Join(mods, "+"), b.Activation)
}

// Validate 校验绑定配置是否合法（死区范围、时长非负等）。
func (b Binding) Validate() error {
	if b.Action == "" {
		return fmt.Errorf("%w: empty action", ErrInvalidBinding)
	}
	if b.Control == "" {
		return fmt.Errorf("%w: empty control for action %q", ErrInvalidBinding, b.Action)
	}
	if b.Deadzone < 0 || b.Deadzone >= 1 {
		return fmt.Errorf("%w: deadzone %v out of range [0,1) for action %q", ErrInvalidBinding, b.Deadzone, b.Action)
	}
	if b.LongPressDuration < 0 {
		return fmt.Errorf("%w: negative long-press duration for action %q", ErrInvalidBinding, b.Action)
	}
	if b.DoubleTapWindow < 0 {
		return fmt.Errorf("%w: negative double-tap window for action %q", ErrInvalidBinding, b.Action)
	}
	for _, m := range b.Modifiers {
		if m == "" {
			return fmt.Errorf("%w: empty modifier for action %q", ErrInvalidBinding, b.Action)
		}
		if m == b.Control {
			return fmt.Errorf("%w: modifier duplicates control for action %q", ErrInvalidBinding, b.Action)
		}
	}
	return nil
}

// ActionMap 管理动作到按键的映射与重绑定。
type ActionMap struct {
	bindings []Binding
}

// NewActionMap 创建空映射表。
func NewActionMap() *ActionMap { return &ActionMap{} }

// Bind 添加绑定；若与既有绑定冲突，返回 Conflict 错误且不修改映射。
// 同一动作的重复绑定为幂等空操作。
func (m *ActionMap) Bind(b Binding) error {
	if err := b.Validate(); err != nil {
		return err
	}
	for _, e := range m.bindings {
		if e.triggerKey() == b.triggerKey() {
			if e.Action == b.Action {
				return nil // 幂等
			}
			return Conflict{ActionA: e.Action, ActionB: b.Action, Binding: b}
		}
	}
	m.bindings = append(m.bindings, b)
	return nil
}

// Rebind 原子地替换某动作的全部绑定；任何一条冲突或非法时整体不生效，
// 保证重绑定后的行为确定。
func (m *ActionMap) Rebind(action string, bs []Binding) []Conflict {
	for _, b := range bs {
		if err := b.Validate(); err != nil {
			return []Conflict{{ActionA: action, ActionB: action, Binding: b}}
		}
	}
	// 新绑定内部互相冲突检测
	for i := 0; i < len(bs); i++ {
		for j := i + 1; j < len(bs); j++ {
			if bs[i].triggerKey() == bs[j].triggerKey() {
				return []Conflict{{ActionA: action, ActionB: action, Binding: bs[j]}}
			}
		}
	}
	// 与既有映射（排除本动作）冲突检测
	var kept []Binding
	for _, e := range m.bindings {
		if e.Action != action {
			kept = append(kept, e)
		}
	}
	var conflicts []Conflict
	for _, b := range bs {
		for _, e := range kept {
			if e.triggerKey() == b.triggerKey() {
				conflicts = append(conflicts, Conflict{ActionA: e.Action, ActionB: action, Binding: b})
			}
		}
	}
	if len(conflicts) > 0 {
		return conflicts
	}
	m.bindings = append(kept, bs...)
	return nil
}

// Unbind 移除某动作的全部绑定。
func (m *ActionMap) Unbind(action string) {
	var kept []Binding
	for _, e := range m.bindings {
		if e.Action != action {
			kept = append(kept, e)
		}
	}
	m.bindings = kept
}

// Conflicts 检测给定绑定与现有映射的全部冲突。
func (m *ActionMap) Conflicts(b Binding) []Conflict {
	var out []Conflict
	for _, e := range m.bindings {
		if e.triggerKey() == b.triggerKey() && e.Action != b.Action {
			out = append(out, Conflict{ActionA: e.Action, ActionB: b.Action, Binding: b})
		}
	}
	return out
}

// Bindings 返回某动作的绑定快照。
func (m *ActionMap) Bindings(action string) []Binding {
	var out []Binding
	for _, e := range m.bindings {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

// All 返回全部绑定的确定顺序快照（按动作名、触发键排序）。
func (m *ActionMap) All() []Binding {
	out := make([]Binding, len(m.bindings))
	copy(out, m.bindings)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Action != out[j].Action {
			return out[i].Action < out[j].Action
		}
		return out[i].triggerKey() < out[j].triggerKey()
	})
	return out
}
