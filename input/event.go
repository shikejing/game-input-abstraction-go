// Package input 提供面向游戏引擎的跨平台输入抽象与动作映射能力。
package input

// DeviceKind 表示设备类别。
type DeviceKind int

const (
	DeviceKeyboard DeviceKind = iota
	DeviceMouse
	DeviceGamepad
	DeviceTouch
)

// String 返回设备类别名称。
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

// EventKind 表示统一后的输入事件类别。
type EventKind int

const (
	EventButtonDown         EventKind = iota // 按键/按钮按下
	EventButtonUp                            // 按键/按钮抬起
	EventAxisMove                            // 模拟量轴变化（摇杆、扳机、鼠标滚轮等）
	EventPointerMove                         // 指针移动（鼠标、触点拖动）
	EventTouchBegin                          // 触点开始
	EventTouchMove                           // 触点移动
	EventTouchEnd                            // 触点结束
	EventDeviceConnected                     // 设备接入
	EventDeviceDisconnected                  // 设备断开
)

// DeviceID 是设备在会话内的稳定标识。
type DeviceID string

// ControlID 标识设备上的一个具体控件（键、按钮、轴、触点槽位）。
type ControlID string

// Event 是各后端统一后的输入事件。
type Event struct {
	Kind     EventKind
	Device   DeviceID
	Control  ControlID // 按钮/轴/触点标识
	Value    float64   // 轴值或压感值，按钮事件为 1/0
	X, Y     float64   // 指针/触点坐标（归一化或像素，由后端约定）
	TouchIdx int       // 触点时有效：第几根手指
	Seq      uint64    // 全局单调序号，由采集层分配，保证确定排序
}
