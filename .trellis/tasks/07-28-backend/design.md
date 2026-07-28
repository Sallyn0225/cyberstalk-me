# Design — 共享契约与 Go 后端

> 本子任务的下游技术方案在父任务 `.trellis/tasks/07-28-cyberstalk-me/design.md` §1–§3、§6 已给出大框架；本文档将其下沉为 backend 子任务专属的边界、契约、数据流与取舍细节，并补充 spec 层面的落地约定。执行清单见 `implement.md`。

## 1. 范围与边界

本任务交付整个项目的地基，分两块：

- **仓库骨架 + `shared/` 契约**（对应父任务 implement.md 阶段 0）
- **`server/` Go 后端单二进制**（对应父任务 implement.md 阶段 1）

**不做**：前端 UI（`07-28-web`）、客户端采集（`07-28-client-windows` / `07-28-client-android`）。本任务为它们提供可调用的契约与 API，但客户端/前端代码在本任务中只留空壳或占位。

## 2. 仓库骨架（阶段 0）

布局严格遵循 `.trellis/spec/backend/directory-structure.md` 的目录树，本节重述要点：

```
cyberstalk-me/
├── go.work                 # workspace: shared, server, client-windows
├── shared/                 # 契约 module，仅数据类型，无业务逻辑、无三方依赖
│   └── contract.go
├── server/
│   ├── cmd/server/main.go  # 配置加载、装配、http.ListenAndServe、优雅关停
│   └── internal/
│       ├── api/            # chi 路由、handlers、SSE 端点
│       ├── hub/            # SSE 广播 hub（订阅者集合 + fan-out）
│       ├── state/          # 在线/离线追踪（last_seen + ticker 扫描）
│       ├── store/          # SQLite 持久化（devices, device_state）
│       └── config/         # env 配置
├── client-windows/         # Go module：仅建空壳占位，本任务不实现采集
│   └── cmd/agent/main.go   # 占位（编译通过即可）
├── web/                    # 前端目录占位（阶段 2 子任务填充）
└── client-android/        # Kotlin，延后子任务
```

关键边界（来自 spec directory-structure §Module Organization）：

- `shared/` 只放数据类型，不得有业务逻辑或三方 import（stdlib 之外）。
- `server/internal/` 按职责分包：`api` / `hub` / `state` / `store`，**禁止**再叠 `service/` / `repository/` / `dto/` 这类层级。
- 除 `shared/` 外一律 `internal/`，跨 module 只允许引用 `shared/`。
- 依赖方向：`api` →（`hub`、`state`、`store`、`shared`）；`hub`/`state`/`store` 不互相 import，唯一例外 `state` → `hub`（离线事件需广播）。
- 装配在 `cmd/server/main.go`，禁止 package `init()` 装配。
- 全程 **`CGO_ENABLED=0`**，SQLite 用纯 Go 驱动 `modernc.org/sqlite`，Windows → Linux 交叉编译必须通过。

### 2.1 契约 struct（`shared/contract.go`）

作为客户端 → 后端 → SSE → 前端的**唯一契约源**。新增/修改任何字段前过 `.trellis/spec/guides/cross-layer-thinking-guide.md` 清单，防止漏改一层。MVP 字段（对齐父任务 design.md §2）：

```go
type ReportPayload struct {
    DeviceID   string    `json:"device_id"`
    DeviceName string    `json:"device_name"`
    DeviceType string    `json:"device_type"` // "windows" | "android"
    Activity   Activity  `json:"activity"`
    Battery    *Battery  `json:"battery"`    // 台式机无电池 → null
    Network    *string   `json:"network"`    // wifi|cellular|ethernet|offline|null
    ReportedAt time.Time `json:"reported_at"`
}

type Activity struct {
    App          string `json:"app"`           // 映射后应用名
    Description  string `json:"description"`  // 映射后友好描述
    Idle         bool   `json:"idle"`
    IdleSeconds  int    `json:"idle_seconds"`
}

type Battery struct {
    Level    *int `json:"level"`    // 0-100，可空
    Charging bool `json:"charging"`
}

// SSE 事件
type Event struct {
    Type   string      `json:"type"`    // "update" | "offline"
    Device DeviceState `json:"device"`
}

// 快照/事件里下发到前端的设备状态
type DeviceState struct {
    DeviceID      string    `json:"device_id"`
    DeviceName    string    `json:"device_name"`
    DeviceType    string    `json:"device_type"`
    Activity      Activity  `json:"activity"`
    Battery       *Battery  `json:"battery"`
    Network       *string   `json:"network"`
    Online        bool      `json:"online"`
    ReportedAt    time.Time `json:"reported_at"`
    LastSeenAt    time.Time `json:"last_seen_at"`
}
```

契约**只含已脱敏字段**：原始窗口标题/进程名细节绝不进入 struct，从源头保证不落库、不上报。字段缺失用指针 + `null`。

### 2.2 `client-windows/` 与 `web/` 占位

- `client-windows/cmd/agent/main.go`：最小占位，`go build` 通过即可（如打印 "agent placeholder"），本任务不实现采集/映射/上报循环。
- `web/`：仅建目录，放一个占位 `index.html`（供后端静态托管验证），前端实现属 `07-28-web` 子任务。

## 3. 后端（阶段 1，`server/`）

技术栈（对齐父任务 design.md §3 与 spec）：Go 标准库 `net/http` + 路由 `github.com/go-chi/chi/v5`；SQLite via `modernc.org/sqlite` + `database/sql`，无 ORM，手写 SQL + prepared statement。

### 3.1 存储层 `store`

两张表（schema 见 `.trellis/spec/backend/database-guidelines.md` §Schema，由 `server/internal/store/schema.sql` `go:embed` 启动时 `CREATE TABLE IF NOT EXISTS`）：

- `devices(device_id PK, device_name, device_type, token_hash, created_at)` —— 注册与 token 哈希。
- `device_state(device_id PK→devices, last_report_json, reported_at, last_seen_at)` —— 每设备最新状态，覆盖写。

约定（spec database-guidelines）：

- `device_state` 为**最新态唯一**：每条 report 都是 upsert（`INSERT ... ON CONFLICT(device_id) DO UPDATE`），绝不累积历史行（产品决策：不留活动历史）。
- token **只存 SHA-256 哈希**，永不明文。
- `last_report_json` 存的是已脱敏 payload 原文；store 层永远不接收原始标题。
- 所有 store 方法首参 `context.Context`，用 `QueryRowContext` / `ExecContext`；列名显式写出（不 `SELECT *`）。
- 启动设 `PRAGMA journal_mode=WAL` + `PRAGMA busy_timeout=5000`：SQLite 单写，SSE 读不能阻塞 report。
- `*sql.DB` 在 `main.go` 创建，`store.New(db)` 注入，支持 `:memory:` DSN 供测试。

sentinel 错误（spec error-handling）：`store.ErrDeviceNotFound`。

### 3.2 鉴权 `api/auth`

- 上报校验：`Authorization: Bearer <token>` → 取 SHA-256 哈希比对 `devices.token_hash` → 命中后校验 body 内 `device_id` 与该 token 绑定设备一致，否则 401。
- token 生成：随机串，落库存哈希，明文仅 `register-device` 输出一次。
- sentinel：`api.ErrBadToken`。
- 红线（spec logging §What NOT to log）：**绝不**记录 token（明文或哈希）、`Authorization` 头、访客 IP。

### 3.3 在线/离线追踪 `state`

- 在线判定以**服务端接收时间**为准：收到 report 时由服务端打 `last_seen_at`；客户端 `reported_at` 仅展示/调试，不参与判定（避免设备时钟漂移）。
- `OFFLINE_THRESHOLD`（如 60s）可配。`time.Ticker` 每 5s 扫描：`last_seen_at` 距今超阈值即标离线。
- 状态变化（在线→离线）触发 `state → hub` 广播 `offline` 事件。
- **测试可注入假时钟**：state tracker 构造函数接收 `now func() time.Time`，默认 `time.Now`（spec quality §Testing）。

### 3.4 SSE 广播 hub `hub`

- 内存维护订阅者集合 `map[chan shared.Event]struct{}` + `sync.Mutex`（spec quality §Required Patterns）。
- report upsert 后由 `api → hub` 广播 `update` 事件；离线由 `state → hub` 广播 `offline` 事件。
- fan-out 非阻塞：写满/失败的 channel 视为订阅者已断开，**必须** unsubscribe（spec error-handling §Common Mistakes：SSE 写错误不能吞，要摘掉订阅者）。
- `race` 检测必须干净（`go test -race`）。

### 3.5 端点 `api`

统一错误响应形状 `{ "error": "<msg>" }`，经单个 `writeError` helper 写出（spec error-handling §API Error Responses）：

| 端点 | 方法 | 说明 | 错误状态 |
|------|------|------|----------|
| `/api/v1/report` | POST | Bearer 校验 → device_id 一致 → upsert state → 广播 update | 400 body 坏/缺字段；401 token 错/不一致；404 未知 device；500 内部 |
| `/api/v1/snapshot` | GET | 返回全部设备当前状态数组（含 online 判定） | 500 内部 |
| `/api/v1/stream` | GET | SSE，推 `update`/`offline` 事件 | — |

500 响应体固定 `"internal error"`，**绝不**返回包装后的内部错误文本（防泄露路径/SQL）。

### 3.6 静态托管

`/` 提供 `web/` 构建产物（`embed.FS` 打进二进制）。本任务 web 尚未实现，提供占位页即可验证托管链路。

### 3.7 配置 `config` + admin CLI

- env 配置（`.env`/环境变量，spec 对齐父任务 §6）：监听地址、`OFFLINE_THRESHOLD`、SQLite 路径、`SCAN_INTERVAL`。
- admin 子命令 `register-device <id> <name> <type>`：生成随机 token → 落库存哈希 → 打印**一次性**明文 token + 客户端配置片段（server_url / device_id / token）。实现上可做成 `server` 的另一个子命令入口（`cmd/server/main.go` 按 os.Args 分流）或单独 `cmd/register`，二选一即可。

### 3.8 优雅关停

`signal.NotifyContext` + `http.Server.Shutdown`，确保 SSE 连接在部署/重启时干净关闭（spec quality §Required Patterns）。

## 4. 取舍与回滚

- **无 ORM / 无 migration 工具**：两张表，手写 SQL + `CREATE TABLE IF NOT EXISTS`；后续加列用 `ALTER TABLE ADD COLUMN` + `PRAGMA user_version` 守卫（spec database-guidelines）。
- **不引 golangci-lint**（MVP 门槛为 gofmt + vet + test，spec quality §Tooling）。
- **client-windows 占位**：本任务只保证其 `go build` 通过，不做采集；失败不影响后端 AC，且后续子任务独立推进。
- 回滚点：骨架与契约（阶段 0）一旦合入，下游三个子任务都依赖它——阶段 0 应单独可验证（`go build ./...` 通过、shared 可被 server 引用）后再进阶段 1。
- 安全红线：脱敏即安全模型——原始标题、token、访客数据不得进入日志/存储/契约。每次 check 必抽查（见 prd AC + check.jsonl）。

## 5. 兼容与下游影响

- 契约 struct 一旦定稿即为下游 `07-28-web`（`web/src/types/contract.ts` 镜像）与 `07-28-client-windows`/`client-android`（复用 Go struct）的共同依赖。修改 shared 字段须同步前端 TS 类型（spec quality §Code Review Checklist），属 cross-layer 变更。
- API 路径 `/api/v1/*` 为稳定前缀，下游前端 EventSource 与客户端上报都依赖它。