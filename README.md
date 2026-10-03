# game-input-abstraction-go

面向游戏引擎的跨平台输入抽象与动作映射库。统一键盘、鼠标、手柄与触屏的
原始事件，提供稳定的设备热插拔身份、可重绑定的动作映射（组合键、长按、
双击、模拟量死区）、触屏手势（滑动、捏合、虚拟摇杆）、固定步长采样与
确定性回放、并发一致快照读取，以及平台映射方案与降级策略。

## 安装

```bash
go get github.com/shikejing/game-input-abstraction-go
```

## 快速开始

```go
mp := input.NewInputMap()
jump, _ := mp.AddAction("jump")
mp.Bind(jump.ID, &input.ButtonBinding{
    Ref:     input.DeviceRef{Kind: input.DeviceGamepad},
    Control: input.GamepadButtonControl(input.GamepadSouth),
})

s := input.NewSampler(16*time.Millisecond, mp)
s.AddBackend(yourBackend)

// 游戏循环：
s.Update(frameDt)
frame, _ := s.Snapshot()
if frame.Actions[jump.ID].JustPressed { /* ... */ }
```

## 文档

- [输入模型与配置说明](docs/input-model.md) — 事件模型、设备身份、绑定类型、
  手势、确定性采样、录制回放、并发模型、平台方案与配置项。

## 测试

```bash
go test -race ./...
```

覆盖：热插拔与稳定身份、重绑定冲突、组合键/长按/双击/死区、手势共存、
跨后端一致性、确定性回放（不同帧率与事件顺序）、并发快照读取、配置校验
与平台降级。
