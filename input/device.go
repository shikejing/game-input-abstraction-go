package input

import (
	"fmt"
	"sort"
)

// Device 表示一个已识别的物理设备。
type Device struct {
	ID         DeviceID   // 会话内稳定 ID，重连保持不变
	Kind       DeviceKind // 设备类别
	Name       string     // 人类可读名称
	Persistent string     // 持久身份（序列号/路径哈希），跨重连稳定
	Connected  bool
}

// DeviceRegistry 负责设备热插拔与稳定身份分配。
// 同一 Persistent 身份的设备断开重连后复用同一 DeviceID，
// 其绑定与死区配置不会丢失；重连期间的事件按 Persistent 路由，
// 不会错位到其他设备。
type DeviceRegistry struct {
	byPersistent map[string]DeviceID
	devices      map[DeviceID]*Device
	nextID       int
}

// NewDeviceRegistry 创建空的设备注册表。
func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{
		byPersistent: make(map[string]DeviceID),
		devices:      make(map[DeviceID]*Device),
	}
}

// Connect 注册或重连一个设备，返回其稳定 DeviceID 与是否为新设备。
// 相同 Persistent 的设备重连时复用原 DeviceID。
func (r *DeviceRegistry) Connect(kind DeviceKind, name, persistent string) (DeviceID, bool) {
	if id, ok := r.byPersistent[persistent]; ok {
		d := r.devices[id]
		d.Connected = true
		d.Kind = kind
		d.Name = name
		return id, false
	}
	r.nextID++
	id := DeviceID(fmt.Sprintf("dev-%d", r.nextID))
	r.byPersistent[persistent] = id
	r.devices[id] = &Device{
		ID:         id,
		Kind:       kind,
		Name:       name,
		Persistent: persistent,
		Connected:  true,
	}
	return id, true
}

// Disconnect 标记设备断开，但保留其身份与配置。
func (r *DeviceRegistry) Disconnect(id DeviceID) {
	if d, ok := r.devices[id]; ok {
		d.Connected = false
	}
}

// Lookup 按 DeviceID 查询设备。
func (r *DeviceRegistry) Lookup(id DeviceID) (Device, bool) {
	if d, ok := r.devices[id]; ok {
		return *d, true
	}
	return Device{}, false
}

// Resolve 将后端上报的持久身份解析为稳定 DeviceID；
// 未连接（已断开或未注册）时返回 false，调用方应丢弃或缓存事件，
// 不得将其归到其他设备。
func (r *DeviceRegistry) Resolve(persistent string) (DeviceID, bool) {
	id, ok := r.byPersistent[persistent]
	if !ok {
		return "", false
	}
	if !r.devices[id].Connected {
		return "", false
	}
	return id, true
}

// Devices 返回当前全部设备的快照（含已断开），按 ID 排序保证确定性。
func (r *DeviceRegistry) Devices() []Device {
	out := make([]Device, 0, len(r.devices))
	for _, d := range r.devices {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
