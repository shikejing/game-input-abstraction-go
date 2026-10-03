package input

import (
	"errors"
	"fmt"
)

// Platform 表示目标平台。
type Platform string

const (
	PlatformDesktop Platform = "desktop"
	PlatformConsole Platform = "console"
	PlatformMobile  Platform = "mobile"
)

// FallbackPolicy 设备缺失时的降级策略。
type FallbackPolicy int

const (
	FallbackDisable    FallbackPolicy = iota // 直接禁用依赖该设备的动作
	FallbackToKeyboard                       // 降级到键鼠方案（仅保留键鼠绑定）
	FallbackError                            // 视为配置错误
)

// Profile 是一套平台映射方案。
type Profile struct {
	Platform Platform
	Map      *ActionMap
	Gestures GestureConfig
	Fallback FallbackPolicy
}

// ConfigError 表示配置非法，原因可区分。
type ConfigError struct {
	Field  string
	Reason string
}

// Error 返回错误描述。
func (e ConfigError) Error() string {
	return fmt.Sprintf("input: invalid config %s: %s", e.Field, e.Reason)
}

// ErrDeviceMissing 表示按 FallbackError 策略时设备缺失。
var ErrDeviceMissing = errors.New("input: required device missing")

// Validate 校验方案配置；非法时返回 ConfigError，行为确定。
func (p Profile) Validate() error {
	switch p.Platform {
	case PlatformDesktop, PlatformConsole, PlatformMobile:
	default:
		return ConfigError{Field: "Platform", Reason: fmt.Sprintf("unknown platform %q", string(p.Platform))}
	}
	if p.Map == nil {
		return ConfigError{Field: "Map", Reason: "nil action map"}
	}
	for _, b := range p.Map.All() {
		if err := b.Validate(); err != nil {
			return ConfigError{Field: "Map", Reason: err.Error()}
		}
	}
	g := p.Gestures
	if g.SwipeMinDistance < 0 {
		return ConfigError{Field: "Gestures.SwipeMinDistance", Reason: "must be >= 0"}
	}
	if g.SwipeMinFingers < 0 {
		return ConfigError{Field: "Gestures.SwipeMinFingers", Reason: "must be >= 0"}
	}
	if g.PinchMinScale < 0 {
		return ConfigError{Field: "Gestures.PinchMinScale", Reason: "must be >= 0"}
	}
	for i, z := range g.StickZones {
		if z.Name == "" {
			return ConfigError{Field: fmt.Sprintf("Gestures.StickZones[%d].Name", i), Reason: "empty name"}
		}
		if z.Radius <= 0 {
			return ConfigError{Field: fmt.Sprintf("Gestures.StickZones[%d].Radius", i), Reason: "must be > 0"}
		}
		if z.Deadzone < 0 || z.Deadzone >= 1 {
			return ConfigError{Field: fmt.Sprintf("Gestures.StickZones[%d].Deadzone", i), Reason: "must be in [0,1)"}
		}
	}
	return nil
}

// Resolve 按平台与当前设备情况解析出实际生效的映射方案。
// 设备缺失时按 Fallback 策略降级；策略为 FallbackError 时返回 ErrDeviceMissing。
// 返回的映射是独立副本，调用方修改不影响原方案。
func Resolve(p Profile, reg *DeviceRegistry) (*ActionMap, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	// 统计方案依赖的设备类别
	need := map[DeviceKind]bool{}
	for _, b := range p.Map.All() {
		need[b.Device] = true
	}
	// 检测缺失类别（无已连接设备）
	connected := map[DeviceKind]bool{}
	for _, d := range reg.Devices() {
		if d.Connected {
			connected[d.Kind] = true
		}
	}
	var missing []DeviceKind
	for k := range need {
		if !connected[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return cloneMap(p.Map), nil
	}
	switch p.Fallback {
	case FallbackError:
		return nil, fmt.Errorf("%w: %v on platform %s", ErrDeviceMissing, missing, p.Platform)
	case FallbackDisable, FallbackToKeyboard:
		// 移除缺失类别的绑定；FallbackToKeyboard 语义上等价于
		// “只保留键鼠方案”，两者都是确定的部分映射
		out := NewActionMap()
		for _, b := range p.Map.All() {
			if missingKind(missing, b.Device) {
				continue
			}
			if p.Fallback == FallbackToKeyboard &&
				b.Device != DeviceKeyboard && b.Device != DeviceMouse {
				continue
			}
			_ = out.Bind(b) // 子集不会引入新冲突
		}
		return out, nil
	default:
		return nil, ConfigError{Field: "Fallback", Reason: fmt.Sprintf("unknown policy %d", int(p.Fallback))}
	}
}

func missingKind(missing []DeviceKind, k DeviceKind) bool {
	for _, m := range missing {
		if m == k {
			return true
		}
	}
	return false
}

func cloneMap(m *ActionMap) *ActionMap {
	out := NewActionMap()
	for _, b := range m.All() {
		_ = out.Bind(b)
	}
	return out
}
