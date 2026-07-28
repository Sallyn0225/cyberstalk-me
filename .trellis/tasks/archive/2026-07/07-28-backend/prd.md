# 共享契约与 Go 后端（API/SSE/SQLite/鉴权/在线判定）

## Goal

交付整个项目的地基：仓库骨架、`shared/` 契约 Go module，以及可部署的 Go 后端单二进制（接收上报、鉴权、在线判定、SSE 广播、快照、静态托管）。

## Scope & 上游文档

- 对应父任务 `.trellis/tasks/07-28-cyberstalk-me/implement.md` 阶段 0 + 阶段 1；技术方案见父任务 `design.md` §1–§3、§6。
- 覆盖父任务需求 R2.1–R2.4，以及 R1.4 的服务端侧（token 校验）。

## Dependencies

- 无上游依赖（本任务是其他所有子任务的地基）。
- 下游：`07-28-web`、`07-28-client-windows`、`07-28-client-android` 均依赖本任务产出的契约（`shared/` struct）与 API。

## Requirements

- 仓库骨架按 `.trellis/spec/backend/directory-structure.md`：`go.work` + `shared/` + `server/` + `client-windows/`（空壳占位）+ `web/`（阶段 2 填充）。
- `shared/`：定义 `ReportPayload` / `DeviceState` 契约 struct，作为唯一契约源。
- `server/`（chi + `modernc.org/sqlite`，`devices` / `device_state` 两表）：
  - `POST /api/v1/report`：Bearer token 哈希校验 + device_id 一致性校验 + upsert 最新状态 + SSE 广播。
  - `GET /api/v1/snapshot`：返回全部设备当前状态。
  - `GET /api/v1/stream`：SSE，`update` / `offline` 事件 fan-out 给所有订阅者。
  - 静态托管 `web/dist`（`embed.FS`；web 未就绪前提供占位页）。
- 在线判定以服务端接收时间 `last_seen_at` 为准，距今超过 `OFFLINE_THRESHOLD` 判离线（`time.Ticker` 扫描，状态变化即广播）。
- admin 子命令 `register-device <id> <name> <type>`：生成一次性 token（落库存哈希），打印客户端配置片段。

## Acceptance Criteria

- [ ] `go build ./...`、`go vet ./...`、`go test ./...` 全部通过。
- [ ] 无 token / 错误 token 的 report 请求被拒绝（401/403）（父任务 AC6）。
- [ ] 两个模拟设备 report 后，snapshot 返回两条正确状态。
- [ ] 停止上报超过阈值后，SSE 收到 offline 事件且 snapshot 显示离线 + 最后活跃时间（父任务 AC3 的后端部分）。
- [ ] 日志与数据库中不出现明文 token（安全红线抽查）。

## Out of Scope

- 前端 UI（`07-28-web`）、客户端采集（`07-28-client-windows` / `07-28-client-android`）。
