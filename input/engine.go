package input

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

// ActionState 是一个动作在某一帧的求值结果。
type ActionState struct {
	Active    bool    // 是否激活
	Value     float64 // 模拟量值（死区处理+归一化后），数字键为 0/1
	Started   bool    // 本帧刚激活
	Completed bool    // 本帧刚结束
}

// Snapshot 是某一帧全部动作状态的一致快照，对并发读取安全。
// 快照发布后不可变，读取者永远不会看到部分更新。
type Snapshot struct {
	Frame  uint64
	states map[string]ActionState
}

// Action 读取某动作状态；未映射的动作返回零值。
func (s Snapshot) Action(name string) ActionState {
	if s.states == nil {
		return ActionState{}
	}
	return s.states[name]
}

// gestureDeviceID 是手势合成事件使用的虚拟设备。
const gestureDeviceID = DeviceID("touch-gestures")

// Engine 是输入系统核心：采集线程投递事件，逻辑线程按固定步长求值。
// 同一事件序列（按 Seq 排序）在任意帧率与任意事件到达顺序下求值结果一致。
type Engine struct {
	m   *ActionMap
	reg *DeviceRegistry
	g   *GestureRecognizer

	// 采集侧（多生产者安全）
	mu      sync.Mutex
	pending []Event
	seq     uint64

	// 求值状态（仅 Tick 访问）
	frame     uint64
	time      float64
	buttons   map[DeviceID]map[ControlID]bool
	axes      map[DeviceID]map[ControlID]float64
	impulses  map[ControlID]float64 // 手势等瞬时轴，仅当前帧有效
	pressTime map[DeviceID]map[ControlID]float64
	lastTap   map[DeviceID]map[ControlID]float64
	doubleTap map[DeviceID]map[ControlID]float64 // 本帧触发双击：值为两次按下间隔
	prev      map[string]ActionState

	snap atomic.Pointer[Snapshot]
	rec  *Recorder
}

// NewEngine 创建输入引擎。
func NewEngine(m *ActionMap, reg *DeviceRegistry, g *GestureRecognizer) *Engine {
	if m == nil {
		m = NewActionMap()
	}
	if reg == nil {
		reg = NewDeviceRegistry()
	}
	e := &Engine{
		m:         m,
		reg:       reg,
		g:         g,
		buttons:   make(map[DeviceID]map[ControlID]bool),
		axes:      make(map[DeviceID]map[ControlID]float64),
		impulses:  make(map[ControlID]float64),
		pressTime: make(map[DeviceID]map[ControlID]float64),
		lastTap:   make(map[DeviceID]map[ControlID]float64),
		doubleTap: make(map[DeviceID]map[ControlID]float64),
		prev:      make(map[string]ActionState),
	}
	e.snap.Store(&Snapshot{states: map[string]ActionState{}})
	return e
}

// NextSeq 分配全局单调事件序号。
func (e *Engine) NextSeq() uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seq++
	return e.seq
}

// Submit 从采集线程投递事件；只经历短暂临界区，不得阻塞采集。
// Seq 为 0 时由引擎按到达顺序分配序号。
func (e *Engine) Submit(ev Event) {
	e.mu.Lock()
	if ev.Seq == 0 {
		e.seq++
		ev.Seq = e.seq
	}
	e.pending = append(e.pending, ev)
	e.mu.Unlock()
}

// Tick 推进一个固定步长（dt 秒），求值并发布新快照。
func (e *Engine) Tick(dt float64) {
	// 取出本帧事件：交换切片，临界区极短
	e.mu.Lock()
	events := e.pending
	e.pending = nil
	e.mu.Unlock()

	// 按 Seq 排序：与到达顺序无关，保证确定性
	sort.Slice(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })

	// 手势瞬时轴与双击标记仅存活一帧
	e.impulses = make(map[ControlID]float64)
	e.doubleTap = make(map[DeviceID]map[ControlID]float64)

	for _, ev := range events {
		e.apply(ev)
	}
	e.time += dt
	e.frame++

	states := e.evaluate()
	snap := &Snapshot{Frame: e.frame, states: states}
	e.snap.Store(snap)

	if e.rec != nil {
		e.rec.append(events)
	}
}

// apply 把单个事件应用到求值状态。
func (e *Engine) apply(ev Event) {
	switch ev.Kind {
	case EventDeviceDisconnected:
		// 设备断开：释放其全部控件状态，事件不得错位到其他设备
		delete(e.buttons, ev.Device)
		delete(e.axes, ev.Device)
		delete(e.pressTime, ev.Device)
		delete(e.lastTap, ev.Device)
		if e.g != nil {
			if d, ok := e.reg.Lookup(ev.Device); ok && d.Kind == DeviceTouch {
				e.g.Reset()
				delete(e.axes, gestureDeviceID) // 清空手势合成轴
			}
		}
	case EventButtonDown:
		setNested(e.buttons, ev.Device, ev.Control, true)
		setNested(e.pressTime, ev.Device, ev.Control, e.time)
		// 双击判定：距上次按下不超过窗口（窗口在求值时按绑定读取，这里记录原始时间）
		if last, ok := e.lastTap[ev.Device][ev.Control]; ok && e.time-last <= maxDoubleTapWindow {
			setNested(e.doubleTap, ev.Device, ev.Control, e.time-last)
		}
		setNested(e.lastTap, ev.Device, ev.Control, e.time)
	case EventButtonUp:
		setNested(e.buttons, ev.Device, ev.Control, false)
	case EventAxisMove:
		setNested(e.axes, ev.Device, ev.Control, ev.Value)
	case EventTouchBegin, EventTouchMove, EventTouchEnd:
		// 触点事件先驱动手势识别；事件本身不被消费
		if e.g != nil {
			for _, ge := range e.g.Handle(ev) {
				e.applyGesture(ge)
			}
		}
	}
}

// maxDoubleTapWindow 是双击判定记录的最大窗口上界（秒）；
// 具体绑定可用更小的窗口在求值时二次过滤。
const maxDoubleTapWindow = 1.0

// applyGesture 把手势结果转换为瞬时轴。
func (e *Engine) applyGesture(ge GestureEvent) {
	switch ge.Kind {
	case GestureVirtualStick:
		// 摇杆偏移是持续状态：Active 时写入，结束时清零
		// 通过 axes 持久化，用合成设备承载
		setNested(e.axes, gestureDeviceID, ControlID("stick:"+ge.Zone+"/x"), ge.DX)
		setNested(e.axes, gestureDeviceID, ControlID("stick:"+ge.Zone+"/y"), ge.DY)
	case GestureSwipe:
		e.impulses["gesture:swipe/x"] = ge.DX
		e.impulses["gesture:swipe/y"] = ge.DY
	case GesturePinch:
		e.impulses["gesture:pinch"] = ge.Scale
	}
}

// evaluate 求值全部动作，返回新状态表。
func (e *Engine) evaluate() map[string]ActionState {
	bindings := e.m.All()
	cur := make(map[string]ActionState, len(bindings))
	for _, b := range bindings {
		v, active := e.evalBinding(b)
		st := cur[b.Action]
		if active {
			if !st.Active || math.Abs(v) > math.Abs(st.Value) {
				st.Value = v
			}
			st.Active = true
		}
		cur[b.Action] = st
	}
	// 计算 Started/Completed：与上一帧对比
	for name, st := range cur {
		p := e.prev[name]
		st.Started = st.Active && !p.Active
		st.Completed = !st.Active && p.Active
		cur[name] = st
	}
	for name, p := range e.prev {
		if p.Active {
			if _, ok := cur[name]; !ok {
				cur[name] = ActionState{Completed: true}
			}
		}
	}
	e.prev = cur
	return cur
}

// evalBinding 求值单个绑定，返回（值，是否激活）。
func (e *Engine) evalBinding(b Binding) (float64, bool) {
	if !e.modifiersDown(b) {
		return 0, false
	}
	// 轴路径：该控件存在轴状态
	if raw, ok := e.axisValue(b.Device, b.Control); ok {
		v := applyDeadzone1D(raw, b.Deadzone)
		if b.Scale != 0 {
			v *= b.Scale
		}
		if b.Invert {
			v = -v
		}
		return v, v != 0
	}
	// 瞬时轴（手势）路径
	if v, ok := e.impulses[b.Control]; ok && b.Device == DeviceTouch {
		v = applyDeadzone1D(v, b.Deadzone)
		if b.Invert {
			v = -v
		}
		return v, v != 0
	}
	// 按钮路径
	switch b.Activation {
	case ActivatePress:
		if e.buttonDown(b.Device, b.Control) {
			return 1, true
		}
	case ActivateLongPress:
		if dev, ok := e.firstDeviceWithButton(b.Device, b.Control); ok {
			// epsilon 吸收固定步长累加的浮点误差，保证判定确定
			if pt, ok := e.pressTime[dev][b.Control]; ok && e.time-pt >= b.LongPressDuration-1e-9 {
				return 1, true
			}
		}
	case ActivateDoubleTap:
		if interval, ok := e.firstDoubleTap(b.Device, b.Control); ok {
			// 用绑定自己的窗口二次过滤
			if interval <= b.DoubleTapWindow {
				return 1, true
			}
		}
	}
	return 0, false
}

// modifiersDown 检查全部修饰键是否按住（同设备类别）。
func (e *Engine) modifiersDown(b Binding) bool {
	for _, mod := range b.Modifiers {
		if !e.buttonDown(b.Device, mod) {
			return false
		}
	}
	return true
}

// deviceIDsOfKind 返回某类别已连接设备的确定顺序 ID 列表。
func (e *Engine) deviceIDsOfKind(kind DeviceKind) []DeviceID {
	var ids []DeviceID
	for _, d := range e.reg.Devices() {
		if d.Kind == kind && d.Connected {
			ids = append(ids, d.ID)
		}
	}
	if kind == DeviceTouch {
		ids = append(ids, gestureDeviceID) // 手势合成设备最后
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// buttonDown 检查某类别下是否有设备按住该控件。
func (e *Engine) buttonDown(kind DeviceKind, c ControlID) bool {
	for _, id := range e.deviceIDsOfKind(kind) {
		if e.buttons[id][c] {
			return true
		}
	}
	return false
}

// axisValue 取某类别下该控件的轴值；按设备 ID 顺序，后写覆盖（确定）。
func (e *Engine) axisValue(kind DeviceKind, c ControlID) (float64, bool) {
	var v float64
	found := false
	for _, id := range e.deviceIDsOfKind(kind) {
		if val, ok := e.axes[id][c]; ok {
			v, found = val, true
		}
	}
	return v, found
}

// firstDeviceWithButton 返回按住该控件的第一个设备（确定顺序）。
func (e *Engine) firstDeviceWithButton(kind DeviceKind, c ControlID) (DeviceID, bool) {
	for _, id := range e.deviceIDsOfKind(kind) {
		if e.buttons[id][c] {
			return id, true
		}
	}
	return "", false
}

// firstDoubleTap 返回本帧该控件触发双击的间隔（按设备确定顺序取第一个）。
func (e *Engine) firstDoubleTap(kind DeviceKind, c ControlID) (float64, bool) {
	for _, id := range e.deviceIDsOfKind(kind) {
		if iv, ok := e.doubleTap[id][c]; ok {
			return iv, true
		}
	}
	return 0, false
}

// Snapshot 返回当前一致快照；任意时刻调用都安全，绝不返回部分更新。
func (e *Engine) Snapshot() Snapshot {
	return *e.snap.Load()
}

// Frame 返回当前帧号。
func (e *Engine) Frame() uint64 {
	return e.frame
}

// Recording 是一次输入录制，可用于确定性回放。
type Recording struct {
	DT     float64   // 固定步长
	Frames [][]Event // 每帧投递的事件（按 Seq 排序）
}

// Recorder 在引擎上录制输入。
type Recorder struct {
	mu     sync.Mutex
	dt     float64
	frames [][]Event
}

// NewRecorder 创建录制器，绑定到引擎。
func NewRecorder(e *Engine, dt float64) *Recorder {
	r := &Recorder{dt: dt}
	e.rec = r
	return r
}

// append 记录一帧的事件（引擎内部调用）。
func (r *Recorder) append(events []Event) {
	r.mu.Lock()
	cp := make([]Event, len(events))
	copy(cp, events)
	r.frames = append(r.frames, cp)
	r.mu.Unlock()
}

// Recording 导出录制结果。
func (r *Recorder) Recording() Recording {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := Recording{DT: r.dt, Frames: make([][]Event, len(r.frames))}
	for i, f := range r.frames {
		cp := make([]Event, len(f))
		copy(cp, f)
		out.Frames[i] = cp
	}
	return out
}

// Replay 用录制结果在新引擎上确定性回放，返回逐帧快照。
func Replay(rec Recording, m *ActionMap, reg *DeviceRegistry, g *GestureRecognizer) []Snapshot {
	e := NewEngine(m, reg, g)
	out := make([]Snapshot, 0, len(rec.Frames))
	for _, events := range rec.Frames {
		for _, ev := range events {
			e.Submit(ev)
		}
		e.Tick(rec.DT)
		out = append(out, e.Snapshot())
	}
	return out
}

func setNested[K comparable, V any](m map[DeviceID]map[K]V, d DeviceID, k K, v V) {
	inner, ok := m[d]
	if !ok {
		inner = make(map[K]V)
		m[d] = inner
	}
	inner[k] = v
}
