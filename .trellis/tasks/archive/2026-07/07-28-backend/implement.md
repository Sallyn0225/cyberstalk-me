# Implement — 共享契约与 Go 后端

> 复杂任务，本子任务交付项目地基。对应父任务 `.trellis/tasks/07-28-cyberstalk-me/implement.md` 阶段 0（骨架+契约）与阶段 1（后端）。技术方案见本任务 `design.md`。每个阶段结束有 review gate + 验证命令；阶段 0 单独可验证，先过再进阶段 1。

## 阶段 0 · 仓库骨架与契约（地基）

- [ ] 0.1 按 `.trellis/spec/backend/directory-structure.md` 目录树建仓：`go.work` + `shared/` + `server/`（含 `cmd/server/main.go` 与 `internal/{api,hub,state,store,config}`）+ `client-windows/`（含 `cmd/agent/main.go`）+ `web/` 目录占位。
- [ ] 0.2 三个 Go module 各自 `go.mod`，`go.work` 把 `shared` / `server` / `client-windows` 串成 workspace。
- [ ] 0.3 在 `shared/contract.go` 定义契约 struct（`ReportPayload` / `Activity` / `Battery` / `Event` / `DeviceState`），字段对齐 `design.md` §2.1，**只含已脱敏字段**。
- [ ] 0.4 `client-windows/cmd/agent/main.go` 占位，`web/index.html` 占位，两者 `go build` / 托管链路可验证即可，不实现业务。
- [ ] 0.5 `server/cmd/server/main.go` 最小可编译入口（可只占位，阶段 1 填充装配）。

**验证**：
```bash
go work sync
go build ./...        # CGO_ENABLED=0
go vet ./...
```
- [ ] Review gate A：`go build ./...` 通过；`shared` 可被 `server` 与 `client-windows` 引用；目录边界符合 spec（无 `pkg/`，无跨 module 引用 `internal/`）。

## 阶段 1 · 后端实现（R2，Go）

### 1a 存储 `store`
- [ ] 1a.1 `store/schema.sql`（`go:embed`）含 `devices` / `device_state` 两表，启动 `CREATE TABLE IF NOT EXISTS`；设 `PRAGMA journal_mode=WAL`、`PRAGMA busy_timeout=5000`。
- [ ] 1a.2 `store.New(db)` 构造，`*sql.DB` 在 `main.go` 创建注入，支持 `:memory:` DSN。
- [ ] 1a.3 方法：`UpsertState`（upsert，见 spec database-guidelines §Canonical upsert example）、`GetState`、`ListStates`、注册/查设备（`RegisterDevice` / `LookupByTokenHash`）；首参 `ctx`，列名显式，无 `SELECT *`。sentinel `ErrDeviceNotFound`。
- [ ] 1a.4 token 仅存 SHA-256 哈希，`last_report_json` 存已脱敏 payload，绝不接收原始标题。

### 1b 鉴权 `api/auth`
- [ ] 1b.1 `ErrBadToken` sentinel；解析 `Authorization: Bearer` → 哈希比对 → `device_id` 一致性校验。
- [ ] 1b.2 红线确认：日志/存储中无明文 token、无 `Authorization` 头、无访客 IP（spec logging §What NOT to log）。

### 1c SSE hub `hub`
- [ ] 1c.1 订阅者 `map[chan shared.Event]struct{}` + `sync.Mutex`；`Subscribe`/`Unsubscribe`/`Broadcast`。
- [ ] 1c.2 fan-out 非阻塞：写满/失败 channel 立即 unsubscribe，不吞 SSE 写错误（spec error-handling）。

### 1d 在线/离线 `state`
- [ ] 1d.1 tracker 构造接收 `now func() time.Time`（默认 `time.Now`，测试可注入假时钟，spec quality §Testing）。
- [ ] 1d.2 report 收到时打 `last_seen_at`（服务端时钟）；`time.Ticker` 每 `SCAN_INTERVAL`（如 5s）扫描，`last_seen_at` 距 `now()` 超 `OFFLINE_THRESHOLD`（如 60s）标离线。
- [ ] 1d.3 状态由在线→离线变化时，`state → hub` 广播 `offline` 事件。

### 1e 端点 `api`（chi 路由）
- [ ] 1e.1 `POST /api/v1/report`：Bearer 校验 → device_id 一致 → upsert state → `hub.Broadcast(update)`。
- [ ] 1e.2 `GET /api/v1/snapshot`：返回全部设备状态数组（含 online 判定）。
- [ ] 1e.3 `GET /api/v1/stream`：SSE，订阅 hub，推 `update`/`offline` 事件。
- [ ] 1e.4 统一错误响应 `{ "error": "<msg>" }`，单 `writeError` helper；状态码映射按 `design.md` §3.5 表；500 固定 `"internal error"`，不回内部错误文本。
- [ ] 1e.5 `/` 静态托管 `web/`（`embed.FS`），占位页验证链路。

### 1f 配置与装配 `config` + `main.go`
- [ ] 1f.1 `config`：env 读取监听地址、`OFFLINE_THRESHOLD`、`SCAN_INTERVAL`、SQLite 路径。
- [ ] 1f.2 `cmd/server/main.go` 装配：建 `*sql.DB` → store/hub/state/router → `slog.SetDefault`（JSON handler）→ `http.Server` + `signal.NotifyContext` + `Shutdown` 优雅关停。无 `init()` 装配。

### 1g admin CLI `register-device`
- [ ] 1g.1 `register-device <id> <name> <type>`：生成随机 token → 落库存哈希 → 打印一次性明文 token + 客户端配置片段（server_url/device_id/token）。可做成 `server` 子命令或独立 `cmd/register`。

### 1h 测试
- [ ] 1h.1 store/handler 测试用 `httptest` + 真实 `:memory:` SQLite，无 store mock（spec quality §Testing）。
- [ ] 1h.2 覆盖：无/错 token 被拒（401，AC6）、device_id 不一致被拒、report upsert 后 snapshot 正确、离线阈值转换（注入假时钟，AC3 后端部分）、token 不入库为明文。
- [ ] 1h.3 `go test -race` 干净（hub 并发）。

**验证**：
```bash
gofmt -l .          # 无输出
go vet ./...
go test -race ./...
CGO_ENABLED=0 GOOS=linux go build ./...   # 交叉编译验证
# 手动冒烟（AC6 / AC3 后端部分）：
#   register-device 生成 token → 无 token / 错 token report 返回 401
#   两个模拟设备 report 后 snapshot 两条正确
#   停止上报超阈值 → SSE 收到 offline，snapshot 显示离线 + last_seen
```
- [ ] Review gate B（对照 prd AC）：
  - `go build ./...` / `go vet ./...` / `go test ./...` 全过。
  - 无/错 token report 被拒（AC6）。
  - 两模拟设备 report 后 snapshot 正确。
  - 停止上报超阈值后 SSE 收 offline + snapshot 离线 + 最后活跃时间（AC3 后端部分）。
  - 抽查：日志与数据库无明文 token、无原始标题、无访客 IP（安全红线）。

## 全局验证命令
```bash
gofmt -l . && go vet ./... && go test -race ./... && CGO_ENABLED=0 go build ./...
```

## Review gate（task.py start 前自查，非必须阻塞，但建议）
- [ ] design.md / implement.md 与父任务 design.md §1–§3、§6 及父 implement.md 阶段 0/1 一致，无冲突。
- [ ] implement.jsonl / check.jsonl 引用的 spec 均为 Filled 状态。

## 风险 / 回滚点
- **阶段 0 是地基**：shared 契约一旦合入，下游 web/client-windows/client-android 全依赖它。阶段 0 应先单独过 review gate A 再进阶段 1，避免契约返工波及下游。
- **契约变更 = cross-layer 变更**：改 `shared` 字段须同步 `web/src/types/contract.ts`（属 `07-28-web`），且过 `.trellis/spec/guides/cross-layer-thinking-guide.md` 清单。
- **脱敏红线**：后端不实现脱敏（脱敏在设备端），但后端契约/存储/日志里不得为原始标题留任何位置——这是安全模型，每次 check 必查（见 check.jsonl backend/index.md、logging-guidelines.md）。
- **交叉编译**：必须 `CGO_ENABLED=0`，SQLite 用 `modernc.org/sqlite` 纯 Go 驱动；若误引入 CGO 依赖会在 Windows→Linux 构建失败，是硬回滚点。
- 回滚：阶段 1 任一子块（store/auth/hub/state/api）可独立回退而不动阶段 0；若整体后端方案需重做，阶段 0 契约可保留。