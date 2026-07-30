# 设备屏幕使用时间统计

> 父任务。持有整体需求、跨子任务验收与最终集成审查；实现落在子任务上。

## Goal

在赛博视奸站点上增加一个「屏幕使用时间」视图：展示每台设备在最近一段时间里，
各应用 / 活动的**累计使用时长与排行**，形态参考手机 OS 的「屏幕使用时间」。

用户价值：现在的页面只回答「他此刻在干嘛」，看一眼就没了。加上时长统计后，
访客能看出「他今天主要在干什么」「哪个应用最费时间」，把一次性的窥视变成有信息量的画像；
作者本人也获得一份自我量化数据。

## Background / Confirmed Facts

### 这是对既有产品约束的一次显式修订

父项目任务 `07-28-cyberstalk-me` 曾明确排除历史留存，本任务**推翻**下列三处，必须同步更新：

| 位置 | 原文 | 本任务的处置 |
|------|------|--------------|
| `.trellis/tasks/07-28-cyberstalk-me/prd.md` R2.4 | 「上报数据仅保存"最新状态"，不长期留存历史活动明细」 | 修订为：保存**聚合后的时长数据**，保留期有上限 |
| 同上 Out of Scope 第 1 条 | 「历史时间线 / 过去 N 小时活动回放」 | 部分解除（见本 PRD In / Out of Scope） |
| `.trellis/spec/backend/database-guidelines.md` Conventions | 「Never accumulate history rows — this is a product decision (no activity history is retained)」 | Phase 3.3 改为限定到 `device_state`：`device_state` 仍是 latest-state-only，新增的小时桶表是**有意累加的聚合表**且受保留期约束，**原始上报明细依然不留存** |

### 现状（代码实证）

- **存储只有最新状态，零历史**。`device_state` 以 `device_id` 为主键，每次上报 upsert 覆盖
  （`server/internal/store/store.go:161`，`server/internal/store/schema.sql`）。全库仅 `devices` + `device_state` 两表。
- **API 只有三个端点**，无任何查询/聚合接口：`POST /api/v1/report`、`GET /api/v1/snapshot`、
  `GET /api/v1/stream`（`server/internal/api/router.go:20`）。
- **上报原材料基本足够**。客户端默认 `interval: 10s` 等间隔心跳
  （`client-windows/internal/config/config.go:31`），每条 `ReportPayload` 已带
  `activity.app`、`activity.description`、`activity.idle`、`activity.idle_seconds`
  （`shared/contract.go`）；服务端另有自己时钟的 `last_seen_at` 可用于断线检测。
- **但锁屏在服务端不可识别 —— 契约需要补一个结构化标记**。锁屏时客户端 `process == ""`，
  `mapping.Resolve` 输出 `app = locked_app`，即用户在 config 里自定义的字符串（示例为「已锁屏」，
  `client-windows/internal/mapping/mapping.go`）。服务端只收到该字符串，无法可靠区分「锁屏」与
  「一个恰好同名的应用」，靠字符串匹配判断锁屏不可靠。因此需给 `shared.Activity` 增加结构化的
  锁屏标记，并由客户端填充。
- **`idle` 只反映键鼠输入**，来自 `GetLastInputInfo`（`client-windows/internal/collect/collect_windows.go`
  的 `idleSeconds`）。看视频 / 读长文不动键鼠，超过 `idle_threshold`（默认 5m）即被判 `idle=true`。
  这是统计口径失真的已知来源，处置见 Key Decisions 的 D2。
- **未知进程都落进同一个 `default_app` 桶**（示例「某个应用」）。`mapping.Resolve` 对无规则进程一律返回默认值，
  `UnknownProcess`（`?unknown`）也走这条路。因此排行里必然存在一个混合了多种应用的大杂烩桶，
  只能靠补映射规则收窄，不在本任务范围内。
- **离线阈值默认 60s，扫描间隔 5s**（`server/internal/config/config.go` 的 `OFFLINE_THRESHOLD` / `SCAN_INTERVAL`）。
  上报间隔（10s）远小于离线阈值，因此「相邻上报间隔异常大」是可靠的断线信号。
- **前端是无路由单页**。`web/package.json` 无 `react-router`、无图表库；`web/src/App.tsx` 直接渲染
  `DeviceGrid`；唯一的服务端数据源是 `useDeviceStream`（一次 snapshot + 一条 SSE）。
- **SQLite 单写者**，`journal_mode=WAL` + `busy_timeout=5000`，SSE 读不能被上报写阻塞
  （`server/internal/store/store.go` 的 `New`）。
- 部署形态为 `docker compose` + 单个 `.db` 文件，磁盘增长必须有界（同 `50e5090` 给容器日志加上限的思路）。

## Key Decisions

| # | 决策 | 结论 | 取舍 |
|---|------|------|------|
| D1 | 公开粒度 | **全粒度公开，含按小时的时间分布**。API 保持纯公开只读，不引入 admin 鉴权。 | 与站点既有立场一致。已知代价：访客可反推作息（睡眠、上班、通宵、上班时间在玩什么）。用户在知情下选择此项。 |
| D2 | 统计口径 | **应用排行只统计活跃时长**，排除挂机（`idle == true`）与锁屏；挂机、锁屏各自作为独立总计展示。 | 避免「睡前忘锁屏把 8h 记到 VS Code」。已知代价：看视频 / 读长文因不动键鼠被判挂机，这类应用时长被低估；可由用户调大 `idle_threshold` 缓解。 |
| D3 | 不做逐段时间轴回放 | 只需「总时长 + 排行 + 按小时分布」，因此**不存会话段，改存小时桶累加**。 | 会话段需在内存维护未闭合的段，服务重启/崩溃要处理恢复；小时桶是纯 upsert 累加，无状态、幂等、体积小一个量级。代价：无法回放「14:03–14:47 在干什么」。 |
| D4 | 归因维度 | 存储层按 `app + description` 存；前端**两级展示**：主排行按 `app` 聚合，可展开看该 app 内各 `description` 的时长拆分。 | 存 `description` 才能事后降级到只按 `app`，反之不行，故存储层无可选项。两级展示保住「Chrome 3h 里 2h10m 在看视频」这个信息，代价是多一个展开交互与一个明细字段。 |
| D5 | 时间窗口与保留期 | 前端提供**今日 / 近 7 天 / 近 30 天**三个窗口；保留期由 `USAGE_RETENTION_DAYS` 控制，**默认 365**。 | 体积不是约束：每设备每天约 150 行 × ~80 B ≈ 12 KB/天，一年约 4.4 MB/设备。代价：30 天窗口的小时分布无意义，需改为按日趋势，前端多一种图形态。 |
| D6 | 前端入口 | **页内 tab 切换**（「此刻」/「使用时间」），用 React state，不引入 `react-router`、不改服务端静态兜底。 | 首页保持以实时状态为主，且避免为 SPA 深链改 `router.go` 的 `r.Get("/*", ...)` 兜底（现在对 `/usage` 返回 404）。代价：统计视图无法作为链接分享，刷新回到默认 tab。 |
| D7 | 统计时区 | 「今日」「按小时」「按日」一律以**站点固定时区**计算，由 `DISPLAY_TIMEZONE` 控制，默认 `Asia/Shanghai`；不使用访客浏览器时区。 | 这份数据描述的是作者的作息，用访客时区渲染会让「凌晨 3 点在打游戏」在另一个时区变成「下午 3 点」，语义错误。存储仍按 UTC 整小时分桶（遵守 `database-guidelines.md` 的 UTC 约定），聚合时在 Go 层换算。**已知限制**：非整小时偏移的时区（如 UTC+5:30）小时桶会错位 30 分钟；`Asia/Shanghai` 无此问题。 |
| D8 | 统计不走实时推送 | 统计数据通过 `GET` 拉取，切换 tab / 窗口时重新拉一次；不接入 SSE。 | 时长统计的分钟级新鲜度足够，为它扩展 SSE 事件类型会污染现有 `Event` 契约。代价：停在统计 tab 时数字不会自己动。 |

## Requirements

### R1 契约：结构化锁屏标记

- R1.1 `shared.Activity` 增加布尔字段标记「无前台窗口（锁屏 / 会话切换）」，因为 `app` 是用户自定义字符串，服务端无法据此判断锁屏。
- R1.2 Windows 客户端在 `mapping.Resolve` 走锁屏分支时填充该字段。
- R1.3 `web/src/types/contract.ts` 同步镜像该字段并纳入运行时校验（与 `shared/contract.go` 同任务内改动，遵守跨层约定）。
- R1.4 旧版本客户端不发该字段时按 `false` 处理，解码不得出错。
  **注意（2026-07-30 真机实测更正）**：规划时以为「锁屏必然无键鼠输入，故旧客户端的锁屏时长会归入挂机」，
  实测证伪 —— 锁屏期间 `idle_seconds` 恒为 `0`（Windows 锁屏后不再推进 `GetLastInputInfo`），
  所以旧客户端的锁屏时长会被判为**活跃**，记到名为 `locked_app` 的「应用」上。
  服务端无从区分（这正是本字段存在的理由），处置是升级客户端；详见
  `07-30-screen-time-contract/prd.md` 的「R1.4 的原始假设已被真机证伪」。

### R2 服务端：时长归因与存储

- R2.1 新增小时桶表，按 `(device_id, UTC 整小时, state, app, description)` 累加秒数；`state` 取 `active` / `idle` / `locked` 三值之一。
- R2.2 归因发生在收到上报时：把「上一次上报到本次上报之间的时间差」计入**上一次上报所描述的状态**（那段时间设备处于上次观测到的状态）。上一次的状态从 `device_state.last_report_json` 读取，无需新增内存状态。
- R2.3 时间差以**服务端时钟**（`last_seen_at`）计算，不信任客户端时钟。
- R2.4 时间差超过归因上限（默认取 `OFFLINE_THRESHOLD`）时判定为断线空洞，**不归因任何时长**，仅更新最新状态。这防止关机 8 小时被算成使用了 8 小时。
- R2.5 跨小时的时间差必须按小时边界拆分，分别累加到相应的桶。
- R2.6 `state` 判定优先级：锁屏 > 挂机 > 活跃。锁屏必须先判，因为锁屏时 `idle` **不可信** ——
  实测锁屏期间 `idle_seconds` 恒为 `0`、`idle` 为 `false`（见 R1.4 的更正），
  若先判 `idle` 会把锁屏漏成活跃。
- R2.7 归因失败不得导致上报失败：`POST /report` 的既有语义（204 + SSE 广播）优先，归因错误记日志。
- R2.8 首次上报（无前序区间）不产生归因。判据是「已注册但从未上报」这一状态，而非读取错误 ——
  `store.GetState` 对该情形返回零值且 `err == nil`。

### R3 服务端：聚合查询 API

- R3.1 新增公开只读端点，按窗口参数（今日 / 近 7 天 / 近 30 天）返回每台设备的统计：三种 state 的总时长、按 `app` 的排行（含各 `description` 明细）、以及今日窗口的按小时分布或多日窗口的按日趋势。
- R3.2 无鉴权，与 `snapshot` 一致的公开只读语义。
- R3.3 非法窗口参数返回 400，不静默回退到默认值。
- R3.4 响应体需在契约层声明并被前端运行时校验，风格对齐现有 `parseSnapshot`（只校验结构，不校验字符串枚举值）。

### R4 服务端：保留期与磁盘上限

- R4.1 定期删除早于 `USAGE_RETENTION_DAYS` 的桶，保证 `.db` 体积有界。
- R4.2 清理周期与保留期均可配置，非法值在启动时失败而非静默回退（对齐 `config.Load` 的既有风格）。

### R5 前端：使用时间视图

- R5.1 顶部提供「此刻」/「使用时间」两个 tab，默认「此刻」。
- R5.2 使用时间 tab 内提供设备选择与窗口选择（今日 / 近 7 天 / 近 30 天）。
- R5.3 展示三种 state 的总时长、按 app 的排行条（可展开看 description 拆分）、以及对应窗口的分布图（今日按小时，7/30 天按日）。
- R5.4 图表不引入新的图表库依赖，用 Tailwind / CSS 实现条形与柱状图（对齐现有「UI 组件优先复用」的取向，避免为两种简单图形引入运行时依赖）。
- R5.5 加载中、请求失败、无数据三种状态都有明确呈现，不得白屏或显示 `NaN`。
- R5.6 柱状图与排行条的信息不得只靠高度/宽度编码，需带文本等价物（无障碍）。

### R6 配置与部署

- R6.1 新增四个环境变量，非法值一律在启动时失败而非静默回退：保留期天数、清理周期、
  归因间隔上限、站点时区。归因间隔上限默认跟随 `OFFLINE_THRESHOLD`（两者语义上是同一条线），
  但保留独立变量，以便有人把离线阈值调大却不希望归因窗口跟着放宽。
- R6.2 服务端必须**内嵌 IANA 时区数据库**（`import _ "time/tzdata"`）。运行镜像 `alpine:3.24`
  不带 tzdata，否则加载时区会失败、服务起不来。**不得**改 Dockerfile 运行阶段加 `RUN apk add tzdata`
  —— 那会迫使多架构构建引入 QEMU（Dockerfile 注释已明确此约束）。
- R6.3 四个新变量按既有风格写入 `compose.yaml` 与 `.env.example`，注释需包含
  D7 的半小时偏移时区限制。

## Acceptance Criteria

- [ ] AC1：客户端锁屏后，上报载荷中的锁屏标记为真；`-dry-run` 输出中能看到该字段，且**不出现**原始窗口标题。
- [ ] AC2：让一台设备以正常心跳连续上报，期间切换前台应用；使用时间 tab 中该设备各应用的活跃时长之和与实际经过时间一致（允许一个上报间隔的误差）。
- [ ] AC3：设备停止上报超过归因上限后再恢复上报，中断的那段时间**不计入**任何应用时长（关机 8 小时不产生 8 小时使用记录）。
- [ ] AC4：设备保持前台应用不变但超过 `idle_threshold` 无输入，这段时间计入「挂机」总计，**不计入**该应用的活跃时长。
- [ ] AC5：设备锁屏期间的时间计入「锁屏」总计，不计入任何应用，也不计入「挂机」。
- [ ] AC6：跨越整点的上报间隔被正确拆分到前后两个小时桶（今日按小时分布图在整点前后都有对应时长）。
- [ ] AC7：未认证浏览器直接打开站点，切到「使用时间」tab 即可看到统计，无需任何登录。
- [ ] AC8：窗口在今日 / 近 7 天 / 近 30 天之间切换时，数据与图形态相应变化（今日为小时分布，7/30 天为按日趋势）。
- [ ] AC9：某个 app 的排行条展开后，其下各 description 的时长之和等于该 app 的总时长。
- [ ] AC10：把保留期配成一个很小的值并触发清理后，早于保留期的桶从库中消失，且接口不因此报错。
- [ ] AC11：非法窗口参数返回 400；非法 `USAGE_RETENTION_DAYS` / `DISPLAY_TIMEZONE` 导致启动失败并给出可读错误。
- [ ] AC12：`POST /report` 在归因逻辑出错时仍返回 204 并广播 SSE（既有实时功能不被统计功能拖累）。
- [ ] AC13：`docker compose build && up` 后服务正常启动（证明内嵌 tzdata 生效 —— 这一项本地全绿也可能在容器里失败）。
- [ ] AC14：脱敏红线复验通过。用诱饵标题跑一轮后，`GET /api/v1/usage` 响应体、小时桶表的
      `app` / `description` 列、以及服务端日志（含 `-v`）中均不出现诱饵字符串。
- [ ] AC15：既有功能无回归 —— 实时卡片、离线判定、SSE 断线重连后的快照校正全部照常。

## In Scope

- 契约新增锁屏标记，以及 Windows 客户端填充它。
- 服务端小时桶存储、归因逻辑、聚合查询接口、保留期清理、四个新环境变量、内嵌 tzdata。
- 前端使用时间 tab：总计、两级应用排行、小时/按日分布图。
- 规范与文档同步（Phase 3.3，完整清单见 `design.md` §11）：
  `database-guidelines.md`、父项目任务 `07-28-cyberstalk-me` 的 R2.4 与 Out of Scope、
  `deployment-guidelines.md`、`frontend/state-management.md`（「entire state fits in one hook」将不再成立）、
  以及 `README.md`（功能列表 + `expose_title` 后果升级的警告）。

## Risks

| 风险 | 说明与处置 |
|------|-----------|
| **`expose_title` 的后果被本任务实质放大** | 这是本任务最大的风险，且不在原立项范围内。此前活动描述只在内存与一次响应里过一遍；本任务第一次把它**持久化**。对于配置了 `expose_title` 的进程，其描述就是**原始窗口标题**——上线后它会被长期落盘（默认 365 天）并持续公开。风险等级从「实时公开」升为「长期落盘且公开」。处置：集成验收做诱饵标题复验（`implement.md` 第 2 步），并在 `README.md` 的警告里明确写出这一升级。**不为它加代码特殊处理**——那会造成「配了 `expose_title` 却统计不到」的隐性行为。 |
| 访客可反推作息 | D1 的已知代价，用户在知情下选择。无技术缓解（这正是所选形态）。 |
| 看视频类应用时长被低估 | D2 的已知代价。缓解手段是用户调大 `idle_threshold`，属配置问题。 |
| 容器内时区数据库缺失导致启动失败 | 运行镜像 `alpine:3.24` 不带 tzdata。处置：Go 内嵌 `time/tzdata`，见 `design.md` §6.1。这是最容易本地全绿、进容器就挂的一项。 |
| 统计功能拖累实时上报 | 归因失败只记日志、不影响 `POST /report` 的 204 与 SSE 广播（R2.7 / AC12）。 |
| `.db` 无界增长 | 保留期清理（R4）+ `USAGE_RETENTION_DAYS` 上限。体积上界已估算：约 4.4 MB/设备/年。 |

## Out of Scope

- **逐段时间轴回放**（「14:03–14:47 在写代码」这种明细列表）。见 D3。
- **原始上报明细留存**。只留聚合桶，不留每条上报。
- Android 客户端填充锁屏标记（Android 侧尚未实现，属 `07-28-client-android`）。
- **收窄 `default_app` 大杂烩桶**。这要靠用户补映射规则，不是代码问题。
- 访客侧时区切换、自定义时间范围、导出数据。
- 统计数据的实时推送（见 D8）。
- 引入前端路由与 SPA 深链（见 D6）。
- 为统计接口引入任何鉴权（见 D1）。

