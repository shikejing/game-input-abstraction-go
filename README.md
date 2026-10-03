# game-input-abstraction-go

面向游戏引擎的跨平台输入抽象与动作映射库。键盘鼠标、手柄与触屏的原始事件粒度各不相同，本库将其统一为一致的设备与事件模型，上层逻辑按固定步长拿到统一且确定的动作状态。

## 输入模型

```
后端原始事件 ──► Event（统一事件）──► Engine.Tick(dt) ──► Snapshot（一致快照）
                     │                      │
              DeviceRegistry          ActionMap 求值
              （稳定身份/热插拔）       （绑定/组合键/死区）
                     │
              GestureRecognizer（触屏手势 → 合成轴）
```

### 统一事件（event.go）

所有后端的事件统一为 `Event`：

| 字段 | 含义 |
|---|---|
| `Kind` | `EventButtonDown/Up`、`EventAxisMove`、`EventTouchBegin/Move/End`、`EventDeviceConnected/Disconnected` |
| `Device` | 稳定设备 ID（见下文） |
| `Control` | 控件标识，如 `"space"`、`"lx"`、`"stick:move/x"` |
| `Value` | 轴值（按钮为 1/0） |
| `X, Y, TouchIdx` | 触点坐标与手指序号 |
| `Seq` | 全局单调序号；帧内事件按 Seq 排序，与到达顺序无关 |

同一物理操作映射到同一动作时，各后端求值结果一致（见 `TestCrossBackendConsistency`）。

### 设备与热插拔（device.go）

- `DeviceRegistry.Connect(kind, name, persistent)`：以持久身份（序列号/路径哈希）注册设备；同一 `persistent` 断开重连后**复用同一 DeviceID**，绑定与死区配置不丢失。
- `Resolve(persistent)`：只解析已连接设备；断开期间到达的事件返回 `false`，调用方丢弃或缓存，**不会错位到其他设备**。
- 设备断开时引擎自动释放其全部控件状态（按键不会卡住），触屏断开同时复位手势识别器。

### 动作映射（mapping.go）

`Binding` 把物理控件映射到动作：

| 配置项 | 说明 |
|---|---|
| `Action` / `Device` / `Control` | 动作名 / 设备类别 / 控件 |
| `Modifiers` | 组合键：需同时按住（顺序无关，归一化比较） |
| `Activation` | `ActivatePress` / `ActivateLongPress`（配 `LongPressDuration`）/ `ActivateDoubleTap`（配 `DoubleTapWindow`） |
| `Deadzone` | 模拟量死区 `[0,1)`，死区外重新归一化到 [-1,1] |
| `Scale` / `Invert` | 缩放 / 反向 |

- `Bind`：单条添加；与既有绑定（归一化触发键相同而动作不同）冲突时返回 `Conflict` 错误，映射不变。
- `Rebind`：**原子替换**某动作的全部绑定；任何一条非法或冲突则整体不生效，行为确定。
- `Validate`：非法配置（死区越界、负时长、空控件等）返回 `ErrInvalidBinding`，原因可区分。

### 触屏手势（gesture.go）

`GestureConfig` 配置判定阈值：

| 配置项 | 说明 |
|---|---|
| `SwipeMinDistance` / `SwipeMinFingers` | 多指滑动的最小位移 / 手指数 |
| `PinchMinScale` | 捏合的最小缩放变化（增量式基线） |
| `StickZones` | 虚拟摇杆区域：`X,Y,W,H` 矩形 + `Radius` 最大偏移 + `Deadzone` |

手势结果转为控件事件进入统一求值：摇杆 → `stick:<zone>/x|y`（持续轴），滑动 → `gesture:swipe/x|y`，捏合 → `gesture:pinch`（瞬时轴）。多手势共存，且**触点事件不被消费**——手势动作与按键动作并发时互不吞并。

### 固定步长求值与回放（engine.go）

- 采集线程调用 `Submit`（只经短暂临界区，不阻塞），逻辑线程调用 `Tick(dt)` 推进固定步长。
- 帧内事件按 `Seq` 排序：同一输入序列在任意帧率、任意到达顺序下，逐帧求值结果一致。
- `Snapshot()` 通过原子指针发布，任意时刻读到的都是**一致快照**，绝无部分更新；慢消费者不阻塞采集。
- `Recorder` 录制每帧事件，`Replay` 在全新引擎上确定性回放，逐帧结果可逐位一致。

### 平台方案（profile.go）

`Profile{Platform, Map, Gestures, Fallback}` 描述一套平台映射方案：

- `Validate` 校验全部配置，非法时返回 `ConfigError{Field, Reason}`，原因可区分。
- `Resolve(profile, registry)` 按当前已连接设备解析生效映射；设备缺失时按策略降级：
  - `FallbackDisable`：移除缺失设备类别的绑定；
  - `FallbackToKeyboard`：只保留键鼠绑定；
  - `FallbackError`：返回 `ErrDeviceMissing`。

## 测试

```sh
go test ./... -race
```

覆盖：热插拔稳定身份与事件不错位、重绑定冲突与原子性、组合键/长按/双击、死区归一化、滑动/捏合/虚拟摇杆、手势与按键并发、乱序到达确定性、录制回放确定性、并发读取一致性（`-race` 通过）。
