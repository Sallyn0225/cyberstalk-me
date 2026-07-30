# 服务端时长归因与聚合 API

父任务：`.trellis/tasks/07-30-screen-time/`。
需求全景见其 `prd.md`；**技术设计全部在其 `design.md` §1、§3–§7**，本文不重复。

## 前置与顺序

- **前置：`07-30-screen-time-contract` 必须先完成。** 本任务依赖 `shared.Activity.Locked`
  才能正确区分锁屏与挂机（R2.6 / AC5）。若在前置完成前动工，锁屏时间会被错误归入「挂机」。
- 后续 `07-30-screen-time-web` 依赖本任务交付的 `GET /api/v1/usage` 及其响应契约。

## Goal

把每台设备的活动时长按小时累加落库，并提供一个公开只读的聚合查询接口，
使前端能展示「今日 / 近 7 天 / 近 30 天」的应用时长排行与时间分布。

## Requirements

### R2 归因与存储

- R2.1 新增 `usage_bucket` 表，按 `(device_id, UTC 整小时, state, app, description)` 累加秒数；
  `state ∈ {active, idle, locked}`。
- R2.2 归因在收到上报时进行，把「上一次上报到本次上报之间的时间差」计入**上一次上报所描述的状态**。
  上一次状态从既有的 `store.GetState` 读取（`device_state.last_report_json`），不引入新的内存状态。
- R2.3 时间差只用**服务端时钟**（`last_seen_at` / 本次 `seenAt`）计算，不使用客户端 `reported_at`。
- R2.4 时间差超过 `USAGE_MAX_GAP` 时判定为断线空洞，**不归因任何时长**，仅更新最新状态。
- R2.5 跨整点的时间差按小时边界拆分，分别累加到相应的桶。
- R2.6 `state` 判定优先级：锁屏 > 挂机 > 活跃（锁屏必然无键鼠输入，不得同时计入挂机）。
- R2.7 归因失败不得改变 `POST /report` 的既有语义：仍返回 204 并广播 SSE，错误只记日志。
- R2.8 首次上报（无前序区间）不产生归因。判据是 `GetState` 返回的 `LastSeenAt.IsZero()`，
  **不是** error —— `GetState` 对「已注册但从未上报」返回零值且 `err == nil`（见 `store.go` 的
  `if !reportJSON.Valid` 分支）。

### R3 聚合查询 API

- R3.1 新增 `GET /api/v1/usage?window=today|7d|30d`，返回每台设备的三种 state 总时长、
  按 `app` 的活跃时长排行（含各 `description` 明细）、以及 `today` 的按小时分布或 `7d`/`30d` 的按日趋势。
- R3.2 无鉴权，与 `GET /api/v1/snapshot` 同为公开只读。
- R3.3 `window` 缺省为 `today`；出现未知值返回 400，**不静默回退**。
- R3.4 响应类型声明在 `shared/contract.go`（见父 `design.md` §2.2）。
- R3.5 空槽也返回：`today` 固定 24 个小时槽、`7d`/`30d` 固定覆盖窗口内每一天，无数据的槽 `seconds` 为 0。
  让服务端补齐，前端不必填洞。
- R3.6 从未上报过的设备不出现在结果中（与 `ListStates` 一致）；有状态但窗口内无数据的设备出现，
  各项为 0。

### R4 保留期与磁盘上限

- R4.1 定期删除早于 `USAGE_RETENTION_DAYS` 的桶，清理 goroutine 在 `main.go` 中与 `tracker.Run`
  并列启动，随同一个 ctx 取消。
- R4.2 四个新环境变量的非法值在启动时失败，不静默回退（对齐 `config.Load` 既有风格）：
  `USAGE_RETENTION_DAYS`（默认 365）、`USAGE_PRUNE_INTERVAL`（默认 1h）、
  `USAGE_MAX_GAP`（默认取 `OFFLINE_THRESHOLD`）、`DISPLAY_TIMEZONE`（默认 `Asia/Shanghai`）。

### R5 部署

- R5.1 `main.go` 必须 `import _ "time/tzdata"`。运行镜像是 `alpine:3.24`，**不带 tzdata**，
  否则 `time.LoadLocation("Asia/Shanghai")` 在容器里失败、服务起不来。
  **不得**改 Dockerfile 运行阶段加 `RUN apk add tzdata` —— 那会迫使多架构构建引入 QEMU
  （Dockerfile 注释里已明确这条约束）。
- R5.2 四个新变量按既有风格加入 `compose.yaml` 的 `environment` 与 `.env.example`（含说明注释）。

## 约束

- 不新增 Go 第三方依赖。`quality-guidelines.md` 的 MVP 许可清单是
  `chi/v5`、`modernc.org/sqlite`、`yaml.v3`、`golang.org/x/sys`，新增需要理由 —— 本任务不需要。
- 新增包 `server/internal/usage` 必须是**纯逻辑、零 I/O**，不 import `store`
  （依赖方向见父 `design.md` §1）。
- handler 测试用真实 `:memory:` store，**不许 mock store**（`quality-guidelines.md` 明文要求）。
- 时间戳一律 RFC 3339 UTC 字符串（`database-guidelines.md`）。
- 不得 `panic` / `log.Fatal` 于 `main.go` 之外；不得引入包级可变状态。

## Acceptance Criteria

- [ ] AC2.1（父 AC2）设备正常心跳并切换前台应用，各应用活跃时长之和与实际经过时间一致
      （允许一个上报间隔的误差）。
- [ ] AC2.2（父 AC3）停止上报超过 `USAGE_MAX_GAP` 后恢复，中断的那段时间不计入任何应用时长。
- [ ] AC2.3（父 AC4）前台应用不变但超过 `idle_threshold` 无输入，该段计入 `idle` 总计，
      不计入该应用的活跃时长。
- [ ] AC2.4（父 AC5）锁屏期间计入 `locked` 总计，不计入任何应用，也不计入 `idle`。
- [ ] AC2.5（父 AC6）跨整点的上报间隔被拆分到前后两个小时桶。
- [ ] AC2.6（父 AC10）把 `USAGE_RETENTION_DAYS` 设为极小值并触发清理后，旧桶从库中消失，
      接口不因此报错。
- [ ] AC2.7（父 AC11）非法 `window` 返回 400；非法 `USAGE_RETENTION_DAYS` / `DISPLAY_TIMEZONE`
      导致启动失败并给出可读错误。
- [ ] AC2.8（父 AC12）`usage_bucket` 表不可写时（测试中 `DROP TABLE`），
      `POST /report` 仍返回 204 并广播 SSE。
- [ ] AC2.9 `AddUsage` 对同一键二次写入表现为**相加**而非覆盖。
- [ ] AC2.10 `QueryUsage` 的边界语义为 `[from, to)`：等于 `from` 的桶包含，等于 `to` 的桶排除。
- [ ] AC2.11 某 app 的 `activities` 各项秒数之和等于该 app 的 `seconds`（父 AC9 的服务端一半）。
- [ ] AC2.12 `go test -race` 干净（新增的清理 goroutine 与上报写入并发，必须无数据竞争）。
- [ ] AC2.13 `docker compose build && up` 后服务正常启动（验证 `time/tzdata` 生效）。

## Out of Scope

- 前端任何改动（属子任务 3）。
- 客户端改动（属子任务 1）。
- 统计数据的 SSE 推送（父 D8：统计走 GET 拉取）。
- 为统计接口引入鉴权（父 D1：全公开）。
- 逐段时间轴的存储或接口（父 D3）。
- 非整小时偏移时区的精确小时对齐。已知限制：UTC+5:30 之类的时区小时图会错位最多 30 分钟，
  写进 `.env.example` 注释即可（父 `design.md` §4.3）。
