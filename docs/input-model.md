# 输入模型与配置说明

本库为游戏引擎提供跨平台输入抽象与动作映射能力。本文档说明核心数据模型、
各模块的行为语义与配置项。

## 1. 设计总览

```
Backend(键盘/鼠标/手柄/触屏)
   │ 原始 Event（带时间戳、设备端口）
   ▼
Registry ── 设备生命周期与稳定身份（热插拔、断线重连）
   │ DeviceState（按键、轴、触点）
   ▼
GestureSystem ── 多指滑动 / 捏合 / 虚拟摇杆
   │ GestureFrame
   ▼
Sampler（固定步长）── 每 Tick 求值 Binding → ActionState
   │ Frame（一致快照）
   ▼
SnapshotBus ── 并发读取（最新帧，慢消费者不阻塞采集）
```

## 2. 统一事件模型

所有后端产生统一的 `Event`：

| 字段 | 含义 |
|------|------|
| `Seq` | 后端内单调递增序号 |
| `Timestamp` | 事件发生时间（相对后端启动），决定固定步长归属 |
| `BackendID` | 后端标识，采样器轮询后回填 |
| `Kind` / `Port` | 设备类别与后端本地端口，构成稳定身份键 |
| `Type` | Add / Remove / ButtonDown / ButtonUp / AxisMove / TouchDown / Move / Up |
| `Control` | 按键/轴控件 ID（高字节编码设备类别，避免跨类冲突） |
| `Value` | 模拟量值 |
| `X,Y` / `TouchID` | 归一化触点坐标与触点 id |

控件 ID 由构造函数生成：`KeyControlID`、`MouseButtonControl`、
`AxisControl`、`GamepadButtonControl`、`GamepadAxisControl`。

## 3. 设备身份与热插拔

- 身份键为 `(BackendID, DeviceKind, Port)`。
- 首次见到某端口时分配 `DeviceID`；同一端口断开重连复用同一 `DeviceID`，
  绑定与配置无需重绑。
- 重连期间到达的事件因设备未连接被丢弃（计入 `DeadLetters`），绝不会错位到
  其他设备。
- 设备断开后状态冻结；重连时状态重置（避免上一次会话的按键残留）。

## 4. 动作与绑定

`InputMap` 维护 `ActionID → []Binding`。每帧求值：

- `Pressed`：当前是否激活
- `JustPressed` / `JustReleased`：沿变化（上一帧未激活/已激活）
- `Value`：聚合模拟值（数字绑定为 0/1）

绑定类型：

| 绑定 | 说明 |
|------|------|
| `ButtonBinding` | 单按键 |
| `AxisBinding` | 单模拟轴，支持轴向死区 |
| `StickBinding` | 双轴摇杆，支持径向/缩放射向死区 |
| `ComboBinding` | 多键同时按下 |
| `LongPressBinding` | 持续按住超过 `Duration` 激活 |
| `DoubleTapBinding` | `MaxInterval` 内双击触发（边沿语义） |
| `SwipeBinding` | 多指滑动，可配方向/距离/指数量 |
| `PinchBinding` | 双指捏合，输出每帧缩放增量 |
| `JoystickBinding` | 虚拟摇杆区域，浮动摇杆中心 |

死区模式 `DeadzoneMode`：`None` / `Axial` / `Radial` / `ScaledRadial`
（把 `[Radius,1]` 重映射到 `[0,1]`，消除死区跳跃）。

### 冲突检测

`Bind` 返回 `BindResult`：

- `ConflictNone`：添加成功
- `ConflictDuplicate`：同一动作上的重复绑定（幂等，非错误）
- `ConflictControlTaken`：物理控件已被另一动作占用（可区分结果）

`ForceBind` 会先移除其他动作上的冲突绑定再添加。

## 5. 触屏手势

手势识别器每 Tick 基于触点历史输出 `GestureFrame`：

- **滑动** `SwipeRecognizer`：跟踪触点起点与位移，超过 `MinDistance` 且
  耗时低于 `MaxTime` 时识别；同方向、同时窗的多指合并为一个 `Fingers=N`
  的滑动事件。
- **捏合** `PinchRecognizer`：双指距离相对初始距离的缩放与每帧增量。
- **虚拟摇杆** `JoystickBinding`：触点落入 `Region` 即设为摇杆中心，
  拖动输出 `-1..1`。

手势可共存：识别器互不抢占，动作绑定各自过滤。阈值可配
（`GestureConfig.SwipeMinDistance/MaxTime`）。

## 6. 固定步长采样与确定性

`Sampler.Update(realDt)` 累加真实时间，每累计一个 `TickRate` 执行一个
固定 Tick：

1. 非阻塞轮询所有后端；
2. 按 `(Timestamp, Seq)` 稳定排序（确定性事件顺序）；
3. 记录到 `Recorder`；
4. 应用到注册表；
5. 手势识别；
6. 求值动作（沿检测依赖上一帧状态）；
7. 发布 `Frame` 快照。

确定性保证：

- 事件按时间戳归属 Tick，与帧率无关（8ms/16ms/任意帧率结果一致）；
- 同一 Tick 内事件按时间戳+序号排序，与到达顺序无关；
- 绑定求值为纯函数（给定 `EvalContext` 结果唯一）。

### 录制与回放

`Recorder` 录制原始事件流；`ReplayBackend` 按时间戳回放。录制的事件可
`Save`/`Load`（gob）。回放时用相同 `Update` 节奏即可逐帧复现。

## 7. 并发模型

- `Frame` 是不可变快照；`Snapshot()`/`Bus.Latest()` 返回深拷贝。
- `SnapshotBus.Publish` 永不阻塞（互斥锁 + 条件变量广播），慢消费者通过
  `Latest` 始终拿到最新帧，采集推进不被阻塞。
- `Wait(afterSeq)` 可等待下一帧（消费者不丢帧场景）。

## 8. 平台方案与降级

`PlatformScheme` 声明平台、所需设备与降级策略：

- 所需设备在线 → 使用主映射；
- 缺失且 `Fallback != None` → 使用 `FallbackMap`，标记 `Degraded=true`
  并列出 `Missing`；
- 缺失且无降级 → 返回 `ErrConfigMissingDevice`。

平台：`PlatformPC` / `PlatformConsole` / `PlatformMobile`。

## 9. 配置与校验

`Config`（可序列化）→ `Build()` 构建 `InputMap`。非法配置返回可区分的
`ConfigError`（`Code` + `Field` + `Message`），不会返回半成品映射：

| Code | 触发条件 |
|------|----------|
| `unknown_action` | 绑定引用了不存在的动作 |
| `duplicate_action` | 动作名重复 |
| `invalid_binding` | 绑定定义不完整/越界 |
| `invalid_deadzone` | 死区半径越界 |
| `invalid_threshold` | 时长/距离/指数阈值越界 |
| `invalid_platform` | 平台值非法 |
| `missing_device` | 必需设备缺失且无降级 |

配置项：

```go
Config{
    Actions: []ActionDef{{Name: "jump"}},
    Bindings: []BindingDef{{
        Action: "jump", Type: BindingButton,
        Device: DeviceRef{Kind: DeviceGamepad},
        Control: GamepadButtonControl(GamepadSouth),
    }},
    Gestures: GestureConfig{SwipeMinDistance: 0.15, SwipeMaxTime: 500*time.Millisecond},
}
```

## 10. 测试覆盖

- 热插拔与稳定身份（含断线期间事件丢弃、不错位）
- 重绑定冲突（占用/重复/强制）
- 组合键、长按、双击、死区
- 滑动、捏合、虚拟摇杆、手势共存
- 跨后端一致性（键盘/手柄同操作同结果）
- 确定性回放（录制/回放、不同帧率、不同事件顺序）
- 并发快照读取（`-race`）、慢消费者不阻塞
- 配置校验与平台降级
