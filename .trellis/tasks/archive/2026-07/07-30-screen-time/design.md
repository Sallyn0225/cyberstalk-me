# 设计：设备屏幕使用时间统计

需求见 `prd.md`。本文只写技术设计；执行顺序见 `implement.md`。

## 1. 架构与边界

```
client-windows                server                                    web
─────────────                 ──────────────────────────────────        ─────────────
collect (Locked?)             api.Report ──┬─→ usage.Attribute (纯)     App
  └→ mapping.Resolve          （既有语义）  │      ↓ []usage.Bucket       ├ tab: 此刻   → useDeviceStream（既有）
       └→ Activity{Locked}                 └─→ store.AddUsage           └ tab: 使用时间 → useUsage（新）
              ↓ POST /report                                                              ↓ GET /api/v1/usage
                                api.Usage ──→ store.QueryUsage
                                          └─→ usage.Aggregate (纯) ──→ shared.UsageResponse
                                main.go ──→ store.PruneUsage（ticker）
```

### 新增包 `server/internal/usage`

`directory-structure.md` 规定「按职责划分，不加 `service`/`repository`/`dto` 式分层」。
`usage` 是一个真实职责（时长归因与聚合），不是分层仪式：

- **纯逻辑，零 I/O**：`Attribute` 与 `Aggregate` 都是纯函数，不 import `store`，可脱库单测。
  这与 `mapping`（纯映射）、`state`（纯在线判定 + 注入接口）的既有取向一致。
- 依赖方向保持不变：`api` → (`usage`, `store`, `hub`, `state`, `shared`)；`usage` 只 import `shared`。
  `usage` 不 import `store`，`store` 不 import `usage`（`store` 用自己的行结构体，见 §3）。
- 替代方案是把归因塞进 `store`（持久化层混业务逻辑，且无法脱库测）或 `api`（handler 继续膨胀）。都更差。

## 2. 契约变更（`shared/contract.go`）

### 2.1 `Activity` 新增锁屏标记

```go
type Activity struct {
	App         string `json:"app"`
	Description string `json:"description"`
	Idle        bool   `json:"idle"`
	IdleSeconds int    `json:"idle_seconds"`
	// Locked reports that the session is locked or switched. The agent sets it
	// when there is no foreground window at all, OR when the foreground window
	// is the Windows lock/logon screen (lockapp.exe / logonui.exe) — on some
	// Windows builds the lock screen is a real foreground window, not an empty
	// desktop, so the "no foreground window" signal alone misses it (found
	// 2026-07-30 on a Windows 11 Enterprise machine during integration). It is
	// a structured flag rather than something the server infers from App,
	// because App is a user-configured string (locked_app in the agent config)
	// that the server cannot match on.
	Locked bool `json:"locked"`
}
```

**兼容性**：字段是 additive 且零值安全，解码旧 JSON 不会出错；
`device_state.last_report_json` 里的旧 JSON 反序列化同理，无需迁移。

> **降级方向没有原先设计的那么安全（2026-07-30 真机实测更正）。**
> 本节原先写的是「旧客户端不发 `locked` → 锁屏期间 `Idle` 仍为 `true` → 归入 `idle` 而非 `active`」。
> 实测：锁屏期间 `idle_seconds` 恒为 `0`（Windows 锁屏后不再推进 `GetLastInputInfo`），
> 于是旧客户端的锁屏时长被判为 **active**，记到名为 `locked_app` 的「应用」上。
> 服务端没有任何可依据的字段组合来纠正它 —— 这恰恰是引入 `Locked` 的理由。
> 处置：升级客户端，不在服务端猜测。仅影响「服务端已更新、客户端未更新」的过渡窗口。

### 2.2 统计响应类型

新增在 `shared/contract.go`（前端 `web/src/types/contract.ts` 同任务镜像）：

```go
// UsageWindow is the requested aggregation window. Known values: "today",
// "7d", "30d". Plain string for the same reason as DeviceType.
type UsageWindow = string

// UsageState is which bucket a second was attributed to. Known values:
// "active", "idle", "locked".
type UsageState = string

type UsageResponse struct {
	Window   UsageWindow   `json:"window"`
	Timezone string        `json:"timezone"`   // IANA name the window was computed in
	From     time.Time     `json:"from"`       // inclusive, UTC
	To       time.Time     `json:"to"`         // exclusive, UTC
	Devices  []DeviceUsage `json:"devices"`
}

type DeviceUsage struct {
	DeviceID   string      `json:"device_id"`
	DeviceName string      `json:"device_name"`
	DeviceType DeviceType  `json:"device_type"`
	Totals     UsageTotals `json:"totals"`
	// Apps is the active-time ranking, descending. Idle and locked time never
	// appear here — only in Totals.
	Apps []AppUsage `json:"apps"`
	// Hourly is set for window "today" and null otherwise; Daily is set for
	// "7d"/"30d" and null otherwise. Exactly one of them is non-null.
	Hourly []HourUsage `json:"hourly"`
	Daily  []DayUsage  `json:"daily"`
}

type UsageTotals struct {
	ActiveSeconds int `json:"active_seconds"`
	IdleSeconds   int `json:"idle_seconds"`
	LockedSeconds int `json:"locked_seconds"`
}

type AppUsage struct {
	App     string `json:"app"`
	Seconds int    `json:"seconds"` // active only
	// Activities is the per-description breakdown of Seconds, descending.
	// Its Seconds sum equals AppUsage.Seconds (AC9).
	Activities []ActivityUsage `json:"activities"`
}

type ActivityUsage struct {
	Description string `json:"description"`
	Seconds     int    `json:"seconds"`
}

// HourUsage is one hour of the local day. Hours with no usage are still
// present with Seconds 0, so the frontend can render a fixed 24-slot chart
// without filling gaps itself.
type HourUsage struct {
	Hour    int    `json:"hour"`    // 0-23 in Timezone
	Seconds int    `json:"seconds"` // active
	TopApp  string `json:"top_app"` // "" when Seconds is 0
}

// DayUsage is one local day. Days with no usage are present with Seconds 0.
type DayUsage struct {
	Date    string `json:"date"`    // YYYY-MM-DD in Timezone
	Seconds int    `json:"seconds"` // active
	TopApp  string `json:"top_app"` // "" when Seconds is 0
}
```

设计取舍：

- **`Hourly` / `Daily` 二选一而非合并成通用 `buckets`**：两者的键类型不同（`int` 小时 vs `YYYY-MM-DD`），
  合并会逼前端做类型判断。用两个可空字段，前端按 `window` 直接取。
- **空槽也返回**（`Seconds: 0`）：R5.5 要求无数据有明确呈现，让服务端补齐固定 24 槽 / N 天，
  前端就不必自己填洞，也不会因缺槽画出错位的图。
- **`Apps` 只含 active**：D2 的直接体现。挂机与锁屏只出现在 `Totals`，避免读者误以为排行含挂机。

## 3. 存储（`server/internal/store`）

### 3.1 表结构（追加到 `schema.sql`）

```sql
-- Hourly usage buckets. This table intentionally accumulates rows, unlike
-- device_state: it is the aggregate the screen-time view reads. It is NOT raw
-- report history — one row is "device D spent N seconds in state S on app A
-- doing D' during UTC hour H", and individual reports are never retained.
-- Bounded by USAGE_RETENTION_DAYS (see PruneUsage).
CREATE TABLE IF NOT EXISTS usage_bucket (
    device_id   TEXT    NOT NULL REFERENCES devices(device_id),
    hour_start  TEXT    NOT NULL,   -- RFC 3339 UTC, truncated to the hour
    state       TEXT    NOT NULL,   -- 'active' | 'idle' | 'locked'
    app         TEXT    NOT NULL,
    description TEXT    NOT NULL,
    seconds     INTEGER NOT NULL,
    PRIMARY KEY (device_id, hour_start, state, app, description)
);

-- The primary key leads with device_id, so retention pruning (WHERE
-- hour_start < ?) cannot use it. This index is what keeps pruning cheap.
CREATE INDEX IF NOT EXISTS idx_usage_bucket_hour_start
    ON usage_bucket(hour_start);
```

- 时间戳沿用 `database-guidelines.md` 的约定：**RFC 3339 UTC 字符串**。UTC 整小时截断后
  字符串按字典序即时间序，范围查询 `hour_start >= ? AND hour_start < ?` 可直接用索引。
- 不用 `WITHOUT ROWID`：主键含两个可能较长的中文字符串（`app`、`description`），
  收益不确定，而普通表行为更可预期。
- 建表走既有的 `CREATE TABLE IF NOT EXISTS` 路径，**不需要动 `PRAGMA user_version`** ——
  `user_version` 是给 `ALTER TABLE ... ADD COLUMN` 用的，新增表不需要。

### 3.2 新增方法

```go
// UsageDelta is one bucket increment produced by the usage package. store
// keeps its own struct rather than importing usage, preserving the one-way
// dependency (api -> usage, api -> store; store -/-> usage).
type UsageDelta struct {
	HourStart   time.Time
	State       string
	App         string
	Description string
	Seconds     int
}

// AddUsage accumulates deltas in one transaction. Re-running with the same
// deltas adds again — it is additive, not idempotent; the caller must only
// pass each interval once (api.Report does, see §4.1).
func (s *Store) AddUsage(ctx context.Context, deviceID string, deltas []UsageDelta) error

// UsageRow is a stored bucket joined with its device identity.
type UsageRow struct {
	DeviceID    string
	DeviceName  string
	DeviceType  string
	HourStart   time.Time
	State       string
	App         string
	Description string
	Seconds     int
}

// QueryUsage returns every bucket in [fromUTC, toUTC) for all devices.
func (s *Store) QueryUsage(ctx context.Context, fromUTC, toUTC time.Time) ([]UsageRow, error)

// PruneUsage deletes buckets older than beforeUTC and returns the row count.
func (s *Store) PruneUsage(ctx context.Context, beforeUTC time.Time) (int64, error)
```

累加 SQL：

```sql
INSERT INTO usage_bucket (device_id, hour_start, state, app, description, seconds)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(device_id, hour_start, state, app, description)
DO UPDATE SET seconds = seconds + excluded.seconds
```

- 单事务包住整批，避免跨小时拆分出的两三行各占一次写锁（SQLite 单写者，见 §7）。
- `QueryUsage` 一次返回所有设备的行，由 `usage.Aggregate` 在内存分组。行数上界很小
  （30 天窗口 × 每天约 150 行 × 设备数 ≈ 数千行），无需按设备分次查询或下推聚合到 SQL。
  这也让「按 `DISPLAY_TIMEZONE` 换算本地小时/本地日」留在 Go 里做，不必在 SQL 里拼时区偏移。

## 4. 归因（`server/internal/usage`）

### 4.1 触发点与区间语义

在 `api.Report` 中，**`UpsertState` 之前**先读旧状态：

```go
prev, prevErr := h.store.GetState(ctx, dev.DeviceID)   // 既有方法
// ... 既有的校验 / seenAt / re-stamp ...
if prevErr == nil && !prev.LastSeenAt.IsZero() {
	deltas := usage.Attribute(prev.Payload.Activity, prev.LastSeenAt, seenAt, h.maxGap)
	if len(deltas) > 0 {
		if err := h.store.AddUsage(ctx, dev.DeviceID, toStoreDeltas(deltas)); err != nil {
			// R2.7: 归因失败不影响上报语义
			slog.Error("report add usage", "device_id", dev.DeviceID, "err", err)
		}
	}
}
if err := h.store.UpsertState(...); err != nil { ... }   // 既有逻辑不变
```

三个要点：

1. **归因给「上一次上报的状态」**，不是本次。区间 `(prev.LastSeenAt, seenAt]` 内设备处于上次观测到的
   活动，把它算给本次会把刚切换过去的应用记成整个区间。
2. **只用服务端时钟**（`LastSeenAt` / `seenAt`），不碰 `ReportedAt`。客户端时钟可能错、可能跳，
   而 `last_seen_at` 是既有的在线判定基准，口径一致（R2.3）。
3. `GetState` 在「已注册但从未上报」时返回零值 `StateRow` 且 `err == nil`
   （见 `store.go` 的 `if !reportJSON.Valid` 分支），所以用 `LastSeenAt.IsZero()` 判断有无前序区间，
   而不是判断 error。首次上报不产生归因，正确。

### 4.2 `Attribute`

```go
const (
	StateActive = "active"
	StateIdle   = "idle"
	StateLocked = "locked"
)

// Bucket is one hour's increment for a single (state, app, description).
type Bucket struct {
	HourStart   time.Time // UTC, truncated to the hour
	State       string
	App         string
	Description string
	Seconds     int
}

// Attribute splits the interval (from, to] across UTC hour boundaries and
// attributes every second to the activity that was observed at `from`.
//
// It returns nil when:
//   - to is not after from (clock skew / duplicate report), or
//   - to.Sub(from) > maxGap, meaning the device was silent long enough that we
//     do not know what it was doing. A powered-off night must not become eight
//     hours of usage (R2.4).
func Attribute(observed shared.Activity, from, to time.Time, maxGap time.Duration) []Bucket
```

状态判定（R2.6，优先级 锁屏 > 挂机 > 活跃）：

```go
func stateOf(a shared.Activity) string {
	switch {
	case a.Locked:
		// Checked first because Idle cannot be trusted here. On the machine
		// first measured (2026-07-30) Windows stops advancing GetLastInputInfo
		// while locked, so a locked report arrives with Idle == false; on
		// another Windows 11 build (2026-07-30 integration) idle_seconds
		// advances normally during lock. Either way Locked is authoritative
		// because the agent sets it from the foreground window, not from idle.
		return StateLocked
	case a.Idle:
		return StateIdle
	default:
		return StateActive
	}
}
```

跨小时拆分（R2.5）：从 `from` 起，每步取 `min(下一个整点, to)`，产出一个 `Bucket`，
`HourStart` 为该步起点所在的 UTC 整点。区间 `13:59:55 → 14:00:05` 产出 `13:00` 桶 5s + `14:00` 桶 5s。
拆分后秒数按整秒累加，`to - from` 的亚秒部分截断到秒（口径统一，误差上界为每次上报 1s）。

锁屏时 `App` 仍是客户端配置的 `locked_app` 字符串。照样存入桶，让「锁屏」在需要时可展示；
排行只取 `state = active`，所以它不会污染应用榜（D2）。

### 4.3 `Aggregate`

```go
// Aggregate turns stored buckets into the wire response. loc is the site
// timezone (D7): local hour-of-day and local date are derived here, so the
// SQL layer never deals with timezones.
func Aggregate(rows []store.UsageRow, w shared.UsageWindow, from, to time.Time, loc *time.Location) shared.UsageResponse
```

> 注意签名里的 `store.UsageRow` 会让 `usage` import `store`，违反 §1 的依赖方向。
> **实际实现取 `usage` 自己的输入类型**（如 `usage.Row`），由 `api` 层做一次平凡转换 ——
> 与 `state.StateLister` 用接口反转依赖是同一手法。

聚合步骤：

1. 按 `device_id` 分组；`Totals` 按 `state` 求和。
2. `Apps`：只取 `state == active`，先按 `app` 求和排序（降序，秒数相同按 app 名升序保证稳定），
   再在每个 app 内按 `description` 求和排序。
3. `Hourly`（window=today）：把每行的 `HourStart` 转到 `loc` 取 `Hour()`，累加 active 秒数；
   补齐 0–23 全部 24 槽；`TopApp` 取该小时 active 秒数最多的 app。
4. `Daily`（window=7d/30d）：同理按 `loc` 的 `YYYY-MM-DD` 累加，补齐窗口内每一天。
5. 设备顺序按 `device_id` 升序，与 `ListStates` / 前端 `byDeviceId` 的既有稳定排序一致。

**窗口计算（在 `api` 层，用 `loc`）**：

| window | from（含） | to（不含） |
|--------|-----------|-----------|
| `today` | `loc` 当天 00:00 | now |
| `7d` | `loc` 当天 00:00 减 6 天 | now |
| `30d` | `loc` 当天 00:00 减 29 天 | now |

即「近 7 天」含今天在内共 7 个本地日，而非「过去 168 小时」——「按日趋势」图要的是完整的日格子。
`from` 转成 UTC 后传给 `QueryUsage`。

**已知限制（D7）**：桶按 UTC 整小时聚合，因此非整小时偏移的时区（UTC+5:30 等）
本地小时边界不落在桶边界上，小时图会有最多 30 分钟错位，跨日边界的归属也可能偏移最多 30 分钟。
默认的 `Asia/Shanghai`（UTC+8）无此问题。这个限制写进 `.env.example` 的注释。

## 5. API（`server/internal/api`）

```
GET /api/v1/usage?window=today|7d|30d      → 200 shared.UsageResponse
                                             400 非法/缺失 window
```

- `window` 缺失时**默认 `today`**；出现未知值返回 400，不静默回退（R3.3，对齐 `config.getEnvDuration`
  「typo 要炸响不要静默默认」的既有取向）。
- 无鉴权，与 `Snapshot` 同为公开只读（R3.2 / D1）。路由挂在 `/api/v1` 组内，`r.Get("/usage", h.Usage)`。
- 从未上报过的设备不出现在 `Devices` 里（与 `ListStates` 省略无状态设备的行为一致）；
  有设备但窗口内无数据时返回该设备且 `Totals` 全 0、`Apps` 为空数组、图槽全 0。

## 6. 配置（`server/internal/config`）

| 环境变量 | 默认 | 校验 |
|---------|------|------|
| `USAGE_RETENTION_DAYS` | `365` | 必须为正整数，否则启动失败 |
| `USAGE_PRUNE_INTERVAL` | `1h` | 必须为正，复用 `getEnvDuration` |
| `USAGE_MAX_GAP` | 空 → 取 `OFFLINE_THRESHOLD` | 显式给值时必须为正 |
| `DISPLAY_TIMEZONE` | `Asia/Shanghai` | 用 `time.LoadLocation` 校验，失败即启动失败 |

`USAGE_MAX_GAP` 默认跟随 `OFFLINE_THRESHOLD`：设备被判离线的那条线，和「不再相信这段时间在干什么」
的那条线，语义上是同一条，默认不该分叉。留出独立变量是因为有人会把 `OFFLINE_THRESHOLD` 调得很大
（容忍抖动），却不希望归因窗口跟着放宽。

### 6.1 时区数据库必须内嵌 —— 部署阻塞项

运行镜像是 `FROM alpine:3.24`（`Dockerfile`），**alpine 不自带 tzdata**，
`time.LoadLocation("Asia/Shanghai")` 在容器里会直接失败，服务起不来。

Dockerfile 运行阶段有一条硬约束：**"No RUN instructions below this line"** ——
加 `RUN apk add tzdata` 会让多架构构建被迫引入 QEMU（注释里明确写了这会连带要求
`release.yml` 加 `docker/setup-qemu-action`）。因此：

```go
// server/cmd/server/main.go
import _ "time/tzdata" // embed the IANA database: the alpine runtime image has
                       // no tzdata, and the runtime stage must stay RUN-free.
```

代价约 +450 KB 二进制，换掉一整条部署故障路径。**不要**改 Dockerfile 解决这个问题。

### 6.2 `compose.yaml` / `.env.example`

四个新变量按既有风格加入 `compose.yaml` 的 `environment` 与 `.env.example`（含说明注释，
包括 §4.3 的半小时时区限制）。

## 7. 并发与性能

- **写放大可控**：每次上报最多写 2 行（跨小时时），常态 1 行，包在一个事务里。上报间隔 10s、
  设备数个位数，远低于 SQLite 单写者的能力；`busy_timeout=5000` 已在 `store.New` 设好。
- **读不阻塞写**：`journal_mode=WAL` 已启用，`GET /usage` 的范围查询不会挡住 `POST /report`。
- **清理不与上报争锁**：`PruneUsage` 每 `USAGE_PRUNE_INTERVAL`（默认 1h）跑一次，
  单条 `DELETE ... WHERE hour_start < ?` 走 `idx_usage_bucket_hour_start`。一小时的桶量是几十行，
  删除事务极短。清理 goroutine 在 `main.go` 里与 `tracker.Run` 并列启动，随同一个 ctx 取消。
- **体积上界**：每设备每天约 150 行 × ~80 B ≈ 12 KB/天，默认 365 天 ≈ 4.4 MB/设备。

## 8. 前端（`web/`）

### 8.1 结构（遵循 `frontend/directory-structure.md`：扁平、无 feature 目录、无 barrel）

```
web/src/
├── App.tsx                        # 改：header 下加 tab 切换，按 tab 渲染
├── components/
│   ├── UsagePanel.tsx             # 新：设备选择 + 窗口选择 + 组装下面三块
│   ├── UsageTotals.tsx            # 新：活跃 / 挂机 / 锁屏 三个总计
│   ├── UsageAppList.tsx           # 新：两级排行条（app + 可展开 description）
│   └── UsageChart.tsx             # 新：小时 / 按日 柱状图（同一个组件两种数据源）
├── hooks/
│   └── useUsage.ts                # 新：fetch /api/v1/usage，按 window 重新拉
├── types/contract.ts              # 改：镜像 §2 的新类型 + 运行时校验
└── lib/format.ts                  # 改：新增「时长」格式化（4h 32m）
```

### 8.2 状态（遵循 `frontend/state-management.md`：无状态库，hook + props 下传）

- **tab 与窗口选择是 local UI state**，`useState` 放在 `App`（tab）与 `UsagePanel`（设备/窗口）。
  不引入 Context、不引入状态库 —— spec 明确要求，且本次没有跨层共享需求。
- `useUsage(window)` 是第二个服务端状态源，与 `useDeviceStream` 平级、互不干扰。
  它只在「使用时间」tab 挂载时才发请求（组件按 tab 条件渲染即可，无需在 hook 里加 enabled 开关）。
  window 变化时重新 fetch，用 `AbortController` 取消上一次（对齐 `useDeviceStream` 里
  `controller.abort()` 的既有写法）。
- **不接 SSE**（D8）。
- 派生值一律 render 时算，不进 state：条形宽度百分比、排序、格式化后的时长字符串
  （spec 的 Common Mistakes 明确列了「不要把格式化字符串存进 state」）。

### 8.3 契约校验

在 `types/contract.ts` 追加 `isUsageResponse` / `parseUsage`，风格严格对齐现有 `parseSnapshot`：
只校验结构不校验字符串枚举值（后端将来加 window 或 state 时，UI 应降级显示而不是整块丢弃）。
`hourly` / `daily` 校验为 `null | Array`。解析失败返回 `null`，调用方给出错误态，不白屏（R5.5）。

### 8.4 图表不引依赖（R5.4）

小时图 = 24 个 `div`，高度按 `seconds / max`；按日图同理 7 / 30 个。
排行条 = 一个 `div` 宽度百分比。展开交互用已在依赖里的 `radix-ui`（Collapsible），
不新增运行时依赖。`motion` 已在依赖里，若要动效直接用。

无障碍：柱子与条形带 `title` / `aria-label` 说明「几点 · 时长 · 主要应用」，
纯高度编码的信息必须有文本等价物。

## 9. 测试策略

| 层 | 覆盖 |
|----|------|
| `usage.Attribute` | 正常区间；跨整点拆分（AC6）；超 `maxGap` 返回 nil（AC3）；`to <= from`；locked 优先于 idle（AC5/AC6）；亚秒截断 |
| `usage.Aggregate` | app/description 两级求和一致（AC9）；24 槽 / N 天补齐；`TopApp`；空输入；排序稳定性 |
| `store` | `AddUsage` 累加（同键二次写 = 相加）；`QueryUsage` 边界（`from` 含、`to` 不含）；`PruneUsage` 计数 |
| `api` | `window` 非法 → 400（AC11）；缺省 → today；`Report` 在 `AddUsage` 失败时仍 204 + 广播（AC12） |
| `config` | 四个新变量的默认值与非法值失败（AC11） |
| `mapping`（客户端） | 锁屏分支置 `Locked = true`，其他分支为 false（AC1） |
| `web` | `parseUsage` 结构校验（含 `hourly`/`daily` 可空、畸形返回 null）；时长格式化 |

`quality-guidelines.md` 规定：**handler 测试用真实的 `:memory:` store，不许 mock store**。
所以 AC12（`AddUsage` 失败仍 204 + 广播）不能靠注入 stub，而是用真实 store 制造精准故障：

```go
// Only the usage write must fail; UpsertState has to keep working, or the
// handler would 500 for an unrelated reason and the test would prove nothing.
if _, err := db.ExecContext(ctx, `DROP TABLE usage_bucket`); err != nil { ... }
// POST /report  ->  still 204, still broadcasts; the failure is logged only.
```

这样既满足「无 store mock」，又把故障面精确限制在用量写入上。
**`Handlers` 的结构与构造函数不需要改动**，`usage` 的纯函数在自己的包里单测。

`usage.Aggregate` 的输入类型是 `usage` 自己的（见 §4.3 的注记），因此它的单测完全脱库，
不受上面这条规则约束。

## 10. 兼容性与回滚

- **纯增量**：新表、新端点、新前端 tab、契约新增可选字段。既有 `report` / `snapshot` / `stream`
  行为与响应体不变（`DeviceState.Activity` 多一个字段，前端旧校验函数不会因多字段失败）。
- **回滚**：撤掉 `usage` 路由与前端 tab 即可，`usage_bucket` 表留在库里不影响任何既有查询；
  彻底回滚则 `DROP TABLE usage_bucket`。降级到旧二进制不会报错（旧代码不认识这张表）。
- **前向**：新服务端 + 旧客户端可用（见 §2.1 的降级方向）；旧服务端 + 新客户端也可用
  （多出的 `locked` 字段被 `json.Decode` 忽略）。
- **数据不可回填**：功能上线前的时间没有桶，「近 30 天」在上线初期只有部分数据。
  这是预期行为，前端不需要特殊标注。

## 11. 规范更新（Phase 3.3 必做）

- `.trellis/spec/backend/database-guidelines.md`：`device_state` 那条
  「Never accumulate history rows — this is a product decision」现在只对 `device_state` 成立。
  需改写为：`device_state` 仍是 latest-state-only；`usage_bucket` 是**有意累加的聚合表**，
  受 `USAGE_RETENTION_DAYS` 约束；**原始上报明细依然不留存**。并把新表加进 Schema 段。
- `.trellis/tasks/07-28-cyberstalk-me/prd.md`：R2.4 与 Out of Scope 第 1 条按 `prd.md` 的修订表更新。
- `.trellis/spec/backend/deployment-guidelines.md`：补 §6 的四个环境变量与 `time/tzdata` 内嵌的原因。
- `.trellis/spec/frontend/state-management.md`：目前写着「entire state fits in one hook」，
  上线后是两个（`useDeviceStream` + `useUsage`）加 tab 的 local state，需要如实更新。
- `README.md`：功能列表与脱敏说明补一句「统计数据同样只基于已脱敏的映射结果」。
