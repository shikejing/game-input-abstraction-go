package input

import (
	"math"
	"sort"
)

// GestureKind 表示手势类别。
type GestureKind int

const (
	GestureSwipe        GestureKind = iota // 多指滑动
	GesturePinch                           // 捏合
	GestureVirtualStick                    // 虚拟摇杆区域
)

// GestureEvent 是手势识别结果，作为普通输入事件进入动作求值，
// 与按键动作并发时互不吞并。
type GestureEvent struct {
	Kind    GestureKind
	Zone    string  // 虚拟摇杆区域名（仅 GestureVirtualStick）
	Fingers int     // 参与手指数
	DX, DY  float64 // 位移增量（滑动/摇杆，摇杆为归一化偏移）
	Scale   float64 // 捏合缩放比（>1 放大）
	X, Y    float64 // 手势中心
	Active  bool    // 手势是否仍在进行
}

// GestureConfig 手势判定阈值配置。
type GestureConfig struct {
	SwipeMinDistance float64 // 判定滑动的最小位移
	SwipeMinFingers  int     // 判定滑动的最少手指数
	PinchMinScale    float64 // 判定捏合的最小缩放变化
	StickZones       []StickZone
}

// StickZone 虚拟摇杆区域（矩形，归一化坐标）。
type StickZone struct {
	Name       string
	X, Y, W, H float64 // 区域
	Radius     float64 // 摇杆最大偏移
	Deadzone   float64
}

// Contains 判断点是否落在区域内。
func (z StickZone) Contains(x, y float64) bool {
	return x >= z.X && x <= z.X+z.W && y >= z.Y && y <= z.Y+z.H
}

// touch 跟踪单个触点。
type touch struct {
	startX, startY float64
	x, y           float64
}

// stickState 跟踪一个虚拟摇杆的捕获状态。
type stickState struct {
	captured bool
	touchIdx int
	origX    float64
	origY    float64
}

// GestureRecognizer 从触点事件流中识别手势。
// 多手势共存：同一触点序列可同时驱动滑动与捏合判定。
type GestureRecognizer struct {
	cfg       GestureConfig
	touches   map[int]*touch
	sticks    []stickState // 与 cfg.StickZones 一一对应
	pinchBase float64      // 双指基线距离，<=0 表示未建立
}

// NewGestureRecognizer 创建手势识别器。
func NewGestureRecognizer(cfg GestureConfig) *GestureRecognizer {
	if cfg.SwipeMinFingers < 1 {
		cfg.SwipeMinFingers = 1
	}
	return &GestureRecognizer{
		cfg:     cfg,
		touches: make(map[int]*touch),
		sticks:  make([]stickState, len(cfg.StickZones)),
	}
}

// Handle 处理一个触点事件，返回本事件触发的手势结果（可为空）。
// 触点事件本身不被消费，调用方仍可将其用于按键类动作。
func (g *GestureRecognizer) Handle(ev Event) []GestureEvent {
	switch ev.Kind {
	case EventTouchBegin:
		return g.onBegin(ev)
	case EventTouchMove:
		return g.onMove(ev)
	case EventTouchEnd:
		return g.onEnd(ev)
	default:
		return nil
	}
}

func (g *GestureRecognizer) onBegin(ev Event) []GestureEvent {
	g.touches[ev.TouchIdx] = &touch{startX: ev.X, startY: ev.Y, x: ev.X, y: ev.Y}
	var out []GestureEvent
	// 按配置顺序捕获虚拟摇杆，保证确定性
	for i, z := range g.cfg.StickZones {
		if g.sticks[i].captured || !z.Contains(ev.X, ev.Y) {
			continue
		}
		g.sticks[i] = stickState{captured: true, touchIdx: ev.TouchIdx, origX: ev.X, origY: ev.Y}
		out = append(out, GestureEvent{Kind: GestureVirtualStick, Zone: z.Name, X: ev.X, Y: ev.Y, Active: true})
		break // 一个触点只捕获一个摇杆
	}
	g.updatePinchBase()
	return out
}

func (g *GestureRecognizer) onMove(ev Event) []GestureEvent {
	t, ok := g.touches[ev.TouchIdx]
	if !ok {
		return nil // 未知触点，忽略（不错位）
	}
	t.x, t.y = ev.X, ev.Y
	var out []GestureEvent

	// 虚拟摇杆：被该触点捕获的摇杆更新偏移
	for i, z := range g.cfg.StickZones {
		s := &g.sticks[i]
		if !s.captured || s.touchIdx != ev.TouchIdx {
			continue
		}
		dx, dy := t.x-s.origX, t.y-s.origY
		if z.Radius > 0 {
			d := math.Hypot(dx, dy)
			if d > z.Radius {
				dx, dy = dx/d*z.Radius, dy/d*z.Radius
			}
			dx, dy = dx/z.Radius, dy/z.Radius // 归一化到 [-1,1]
		}
		dx = applyDeadzone1D(dx, z.Deadzone)
		dy = applyDeadzone1D(dy, z.Deadzone)
		out = append(out, GestureEvent{Kind: GestureVirtualStick, Zone: z.Name, DX: dx, DY: dy, X: t.x, Y: t.y, Active: true})
	}

	// 多指滑动：位移超过阈值后持续上报增量
	if len(g.touches) >= g.cfg.SwipeMinFingers {
		cx, cy := g.centroid()
		var sumDX, sumDY float64
		for _, tt := range g.touches {
			sumDX += tt.x - tt.startX
			sumDY += tt.y - tt.startY
		}
		n := float64(len(g.touches))
		if math.Hypot(sumDX/n, sumDY/n) >= g.cfg.SwipeMinDistance {
			out = append(out, GestureEvent{
				Kind: GestureSwipe, Fingers: len(g.touches),
				DX: sumDX / n, DY: sumDY / n, X: cx, Y: cy, Active: true,
			})
		}
	}

	// 捏合：双指距离相对基线的缩放变化
	if len(g.touches) == 2 && g.pinchBase > 0 {
		d := g.twoFingerDistance()
		scale := d / g.pinchBase
		if math.Abs(scale-1) >= g.cfg.PinchMinScale {
			cx, cy := g.centroid()
			out = append(out, GestureEvent{Kind: GesturePinch, Fingers: 2, Scale: scale, X: cx, Y: cy, Active: true})
			g.pinchBase = d // 增量式更新基线
		}
	}
	return out
}

func (g *GestureRecognizer) onEnd(ev Event) []GestureEvent {
	var out []GestureEvent
	for i := range g.cfg.StickZones {
		s := &g.sticks[i]
		if s.captured && s.touchIdx == ev.TouchIdx {
			out = append(out, GestureEvent{Kind: GestureVirtualStick, Zone: g.cfg.StickZones[i].Name, X: ev.X, Y: ev.Y, Active: false})
			g.sticks[i] = stickState{}
		}
	}
	delete(g.touches, ev.TouchIdx)
	g.updatePinchBase()
	return out
}

// centroid 返回当前触点中心，按触点序号排序保证确定性。
func (g *GestureRecognizer) centroid() (float64, float64) {
	idxs := make([]int, 0, len(g.touches))
	for i := range g.touches {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	var sx, sy float64
	for _, i := range idxs {
		sx += g.touches[i].x
		sy += g.touches[i].y
	}
	n := float64(len(idxs))
	return sx / n, sy / n
}

// twoFingerDistance 返回两个触点的距离（触点序号为序，确定）。
func (g *GestureRecognizer) twoFingerDistance() float64 {
	idxs := make([]int, 0, len(g.touches))
	for i := range g.touches {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	if len(idxs) != 2 {
		return 0
	}
	a, b := g.touches[idxs[0]], g.touches[idxs[1]]
	return math.Hypot(a.x-b.x, a.y-b.y)
}

// updatePinchBase 在触点集合变化时重建捏合基线。
func (g *GestureRecognizer) updatePinchBase() {
	if len(g.touches) == 2 {
		g.pinchBase = g.twoFingerDistance()
	} else {
		g.pinchBase = 0
	}
}

// applyDeadzone1D 对归一化轴值应用死区并重新归一化到 [-1,1]。
func applyDeadzone1D(v, dz float64) float64 {
	if dz <= 0 {
		return v
	}
	a := math.Abs(v)
	if a <= dz {
		return 0
	}
	out := (a - dz) / (1 - dz)
	if v < 0 {
		out = -out
	}
	return out
}

// Reset 清空手势状态（如设备断开时）。
func (g *GestureRecognizer) Reset() {
	g.touches = make(map[int]*touch)
	g.sticks = make([]stickState, len(g.cfg.StickZones))
	g.pinchBase = 0
}
