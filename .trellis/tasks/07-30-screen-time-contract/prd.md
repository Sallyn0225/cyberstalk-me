# 契约新增锁屏标记

父任务：`.trellis/tasks/07-30-screen-time/`（需求全景见其 `prd.md`，技术设计见其 `design.md` §2.1）。

## 前置与顺序

- **无前置**，这是本组三个子任务的第一个。
- 后续 `07-30-screen-time-server` 依赖本任务交付的 `Locked` 字段才能正确区分锁屏与挂机。

## Goal

让服务端能**可靠地**判断一台设备是否处于锁屏 / 无前台窗口状态。

## 为什么需要它（代码实证）

锁屏时客户端 `collect.Collect` 拿不到前台窗口，`Snapshot.Process` 为空字符串；
`mapping.Resolve` 走 `key == ""` 分支，输出 `app = m.lockedApp`，即用户在 `config.yaml` 里
自定义的 `locked_app`（示例为「已锁屏」，见 `client-windows/config.example.yaml`）。

服务端收到的只是这个**用户自定义字符串**，因此：

- 无法区分「真的锁屏」和「一个恰好叫这个名字的应用」；
- 不同部署的字符串不同，服务端无法硬编码匹配；
- 靠字符串匹配判断锁屏是不可靠的，而屏幕使用时间统计必须把锁屏排除在应用时长之外。

结论：需要一个**结构化布尔字段**，由客户端在锁屏分支显式置位。

## Requirements

- R1.1 `shared.Activity` 新增布尔字段 `Locked`（JSON `locked`），语义为「完全没有前台窗口（锁屏 / 会话切换）」。
- R1.2 `mapping.Resolve` 在无前台窗口分支置 `Locked = true`；其余分支保持 `false`。
- R1.3 `web/src/types/contract.ts` 的 `Activity` 接口与 `isActivity` 运行时校验同步新增该字段
  （跨层约定要求契约改动同任务内镜像）。
- R1.4 字段必须零值安全：旧客户端不发 `locked` 时按 `false` 处理，解码不得出错。
  **新客户端**（本任务交付）锁屏时 `Locked = true`，服务端据此归入「锁屏」，不会算成活跃。
- R1.5 不得因此字段泄露任何新信息：`Locked` 是纯布尔量，不携带标题、进程名或路径。

### R1.4 的原始假设已被真机证伪（2026-07-30 实测）

规划时 R1.4 写的是「旧客户端锁屏期间 `Idle` 仍为 `true`，因此归入挂机，降级方向安全」。
`-dry-run` 循环采样（锁屏 10s）实测**推翻**了这个前提：

```json
{ "app": "已锁屏", "description": "人不在", "idle": false, "idle_seconds": 0, "locked": true }
```

锁屏期间 `idle_seconds` 恒为 `0` —— Windows 在锁屏后不再推进 `GetLastInputInfo`
（进程在 Default 桌面，输入焦点在 Winlogon 桌面），所以**锁屏的机器看起来完全不空闲**。

因此，旧客户端 + 新服务端的真实降级是：`locked` 缺失 → `false`，`idle` 也是 `false`
→ 服务端判为 **active**，整段锁屏时间被记成一个名为 `locked_app`（示例「已锁屏」）的应用的活跃时长。
这正是原 R1.4 声称「绝不允许」的组合。

**处置：接受，靠升级客户端解决。** 服务端无法从 `app` 字符串区分锁屏（那正是本任务存在的理由），
也没有别的字段组合可依据。客户端是手动部署在作者自己的机器上的，升级是必然且唯一可行的路径。
本任务交付的新客户端行为完全正确，此条只影响「服务端已更新但客户端尚未更新」的过渡窗口。

这一事实同时影响父任务 `design.md` §4.2 的 `stateOf` 注释（「locked implies no input」不成立），
以及子任务 2 对 `Idle` 的信任程度，已在那两处同步更正。

## Acceptance Criteria

- [ ] AC1.1（父 AC1）设备锁屏时上报载荷中 `activity.locked` 为 `true`；`-dry-run` 输出可见该字段，
      且输出中**不出现**原始窗口标题。
- [ ] AC1.2 有前台窗口时（无论是否命中映射规则、是否挂机）`activity.locked` 均为 `false`。
- [ ] AC1.3 反序列化一段不含 `locked` 键的旧上报 JSON，得到 `Locked == false`，不报错。
- [ ] AC1.4 `mapping` 包单测覆盖：锁屏分支、无规则进程分支、命中规则分支、`expose_title` 分支的 `Locked` 取值。
- [ ] AC1.5 前端 `isActivity` 对缺少 `locked` 的对象的处置与下方决定一致，并有单测覆盖。

### 关于 AC1.5 的一个决定

前端 `isActivity` 现在逐字段校验类型。若要求 `typeof value.locked === 'boolean'`，
则一台**旧客户端**上报的设备会因缺字段被整卡丢弃 —— 这违背 `contract.ts` 里写明的取向
（「结构校验从宽，让 UI 降级而不是丢掉设备」）。

因此：`isActivity` 对 `locked` 采用**可选校验** ——
`value.locked === undefined || typeof value.locked === 'boolean'`，
TS 接口声明为 `locked?: boolean`。渲染侧按 `activity.locked === true` 判断，`undefined` 与 `false` 等价。
这与 `network`、`battery` 可空字段的既有处理风格一致。

Go 侧不需要对应处理：`Locked bool` 遇到缺失键时天然为 `false`（AC1.3）。

## Out of Scope

- Android 客户端填充该字段（Android 侧尚未实现，属 `07-28-client-android`）。
- 前端对锁屏状态的任何**展示**改动（「此刻」tab 的卡片文案不变；使用时间 tab 属子任务 3）。
- 服务端对该字段的消费（属子任务 2）。
