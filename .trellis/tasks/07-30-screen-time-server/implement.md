# 执行计划：服务端时长归因与聚合 API

需求见本任务 `prd.md`；技术设计见父任务 `.trellis/tasks/07-30-screen-time/design.md` §1、§3–§7。
这是本组最大的一个子任务。顺序按「纯逻辑 → 存储 → 装配 → 接口 → 部署」，每步都能独立跑测试。

## 有序清单

### 阶段 A：契约与纯逻辑（无 I/O，先把口径钉死）

- [x] A1. `shared/contract.go`：加 §2.2 的响应类型（`UsageResponse` / `DeviceUsage` / `UsageTotals` /
      `AppUsage` / `ActivityUsage` / `HourUsage` / `DayUsage`，以及 `UsageWindow`、`UsageState` 别名）。
      `shared` 只放数据类型，不放逻辑。
- [x] A2. 新建 `server/internal/usage/usage.go`：`StateActive/StateIdle/StateLocked` 常量、
      `Bucket` 结构、`stateOf`、`Attribute`。
      **`Attribute` 的三条早退**：`to <= from`、`to.Sub(from) > maxGap`、拆分后秒数为 0。
- [x] A3. `server/internal/usage/usage_test.go`：表驱动覆盖 —— 正常区间、跨整点拆分（AC2.5）、
      超 `maxGap` 返回 nil（AC2.2）、`to <= from`、locked 优先于 idle（AC2.4）、亚秒截断。
      **先写这个测试再往下走**：归因口径是整个功能的地基，错了后面全错且很难从 UI 看出来。
- [x] A4. `usage.Aggregate` + 它自己的输入类型 `usage.Row`（**不要**用 `store.UsageRow` 做参数，
      那会让 `usage` import `store`，破坏依赖方向 —— 见父 `design.md` §4.3 的注记）。
- [x] A5. `Aggregate` 的测试：两级求和一致（AC2.11）、24 槽 / N 天补齐（R3.5）、`TopApp`、
      空输入、排序稳定性（秒数相同按 app 名升序）。

### 阶段 B：存储

- [x] B1. `server/internal/store/schema.sql`：追加 `usage_bucket` 表 + `idx_usage_bucket_hour_start` 索引。
      **不要动 `PRAGMA user_version`** —— 那是给 `ALTER TABLE ADD COLUMN` 用的，新增表走
      `CREATE TABLE IF NOT EXISTS` 即可。索引不可省：主键以 `device_id` 开头，
      清理用的 `WHERE hour_start < ?` 走不到主键。
- [x] B2. `store.go`：`UsageDelta`、`UsageRow`、`AddUsage`（单事务批量 upsert 累加）、
      `QueryUsage`、`PruneUsage`。所有方法 `ctx` 第一参、用 `*Context` 变体、
      **不用 `SELECT *`**（`database-guidelines.md` 明令禁止）。
- [x] B3. `store_test.go`：`AddUsage` 累加语义（AC2.9）、`QueryUsage` 的 `[from, to)` 边界（AC2.10）、
      `PruneUsage` 返回计数、跨设备隔离。用既有的 `:memory:` 测试夹具风格。

### 阶段 C：配置与装配

- [x] C1. `server/internal/config/config.go`：四个新变量 + 校验。
      `USAGE_RETENTION_DAYS` 是整数（新写一个 `getEnvInt`，或复用现有风格），
      `DISPLAY_TIMEZONE` 用 `time.LoadLocation` 校验并把 `*time.Location` 放进 `Config`。
      `USAGE_MAX_GAP` 未设置时取 `OfflineThreshold` —— 注意这个默认值依赖另一个字段，
      要在两者都解析完之后再定。
- [x] C2. `config_test.go`（若无则新建）：默认值、非法值报错（AC2.7）、
      `USAGE_MAX_GAP` 跟随 `OFFLINE_THRESHOLD` 的默认行为。
- [x] C3. `server/cmd/server/main.go`：
      - `import _ "time/tzdata"`（**R5.1，最容易漏且只在容器里炸**，注释写明原因）
      - 起清理 goroutine，与 `tracker.Run` 并列，共用同一个 ctx
      - 把 `cfg.MaxGap` / `cfg.Location` 传给 `api.New`

### 阶段 D：接口

- [x] D1. `api/handlers.go` 的 `Report`：在 **`UpsertState` 之前**插入归因（父 `design.md` §4.1 的代码骨架）。
      判据用 `prev.LastSeenAt.IsZero()`，**不是** error（R2.8）。
      `AddUsage` 出错只 `slog.Error`，绝不 `return`（R2.7）。
- [x] D2. `api/handlers.go` 新增 `Usage` handler：解析 `window`（缺省 `today`，未知值 400）、
      按 `cfg.Location` 算窗口边界（父 `design.md` §4.3 的表）、查库、`usage.Aggregate`、写 JSON。
- [x] D3. `api/router.go`：`r.Get("/usage", h.Usage)` 加入 `/api/v1` 组。
- [x] D4. `api/handlers_test.go`：
      - 非法 `window` → 400，缺省 → today（AC2.7）
      - **AC2.8 用 `DROP TABLE usage_bucket` 制造精准故障**，断言仍 204 且 SSE 有事件。
        真实 `:memory:` store，不许 mock（`quality-guidelines.md`）
      - 一条完整链路：连续两次 `POST /report`（第二次晚 N 秒）→ `GET /usage` 能看到 N 秒活跃时长
      - 锁屏 / 挂机 / 超 gap 三种情形各一条（AC2.2–AC2.4 的 handler 层验证）

### 阶段 E：部署配置

- [x] E1. `compose.yaml` 的 `environment` 加四个变量（沿用 `${VAR:-default}` 写法）。
- [x] E2. `.env.example` 加四个变量 + 注释，**包含半小时偏移时区的已知限制**
      （父 `design.md` §4.3）。
- [x] E3. `docker compose build && docker compose up -d`，确认启动正常（AC2.13）；
      再把 `DISPLAY_TIMEZONE` 设为 `Not/AZone` 确认启动失败且错误可读。

## 验证命令

```bash
gofmt -l shared server client-windows
go vet ./server/... ./shared/...
go test ./server/... ./shared/...
# AC2.12。-race 需要 cgo；本机 mingw-w64 的 gcc 已在 PATH，可直接跑。生产构建仍是 CGO_ENABLED=0
CGO_ENABLED=1 go test -race ./server/... ./shared/...
CGO_ENABLED=0 GOOS=linux go build ./server/... ./shared/...

docker compose build                                        # AC2.13
```

本任务不改前端源码，因此不需要 `npm run build`，也不会触发 CI 的 embed freshness 闸。

## 风险点

| 项 | 说明 |
|----|------|
| **漏掉 `import _ "time/tzdata"`** | 本地全绿、CI 全绿、一进容器就起不来。alpine 无 tzdata。清单 C3 已标注；AC2.13 是它的闸 |
| **归因给了本次而非上一次的状态** | 症状隐蔽：总时长对得上，但归属全部偏移一个上报间隔，切换应用越频繁越明显。A3 的测试是防线 |
| **`usage` import 了 `store`** | 破坏依赖方向且让纯逻辑无法脱库测。A4 明确要求 `usage` 用自己的输入类型 |
| 忘记建 `hour_start` 索引 | 功能全对，但保留期清理随数据增长逐渐变慢，上线数月后才显形。B1 已标注 |
| 归因错误导致上报失败 | R2.7 是硬要求；AC2.8 用 `DROP TABLE` 精准验证 |
| `USAGE_MAX_GAP` 默认值的解析顺序 | 它依赖 `OfflineThreshold`，必须在两者都解析后再定，否则默认值恒为 0 并让所有归因被丢弃 |

## 实施记录：与父 `design.md` 的三处偏差（2026-07-30）

都是实现细节，不改对外契约；`web` 子任务照 §2.2 镜像类型即可，不受影响。

| 偏差 | 设计原文 | 实际做法与理由 |
|------|---------|--------------|
| `store.UsageRow` 不 join 设备身份 | §3.2「a stored bucket **joined with its device identity**」 | `UsageRow` 只有 `device_id` + 桶字段，SQL 无 join。为满足 R3.6（窗口内无数据的设备也要出现），`api` 本来就要另查 `ListStates`，那份列表已经是设备身份的唯一来源；再 join 一遍等于同一信息两个出处，会让「以哪个为准」变成隐患 |
| `usage.Aggregate` 多一个 `devices` 参数 | §4.3 签名为 `Aggregate(rows, w, from, to, loc)` | 实为 `Aggregate(devices []Device, rows []Row, ...)`。承接上一条：设备集合由 `devices` 决定，`rows` 里 `device_id` 不在其中的行被忽略 —— 「从未上报过的设备不出现」因此是结构上的必然，而不是一句要记得写的过滤 |
| 清理在启动时先跑一次 | §7「每 `USAGE_PRUNE_INTERVAL`（默认 1h）跑一次」 | ticker 之外，`pruneUsage` 入口先 `prune()` 一次。否则重启间隔短于 1h 的部署（改配置、发版）永远不会真正清理 |

另外 `api.New` 增加了 `maxGap` / `loc` 两个参数（§6 已预期），`usage` 包拆成
`usage.go`（归因）+ `aggregate.go`（聚合）两个文件。

`design.md` §11 清单中，属本任务的两项已完成（`database-guidelines.md` 的累加表口径、
`deployment-guidelines.md` 的四个变量与 tzdata），并顺带修订了
`07-28-cyberstalk-me/prd.md` 的 R2.4 与 Out of Scope 第 1 条。
**剩下 `frontend/state-management.md` 与 `README.md` 属 `web` 子任务**：它们描述的是
用户可见面，而这一面此刻还不存在。

## 回滚点

- 阶段 A、B 是纯新增（新包、新表、新契约类型），不影响任何既有行为，可安全先落地。
- 阶段 D1 是**唯一触碰既有热路径**（`Report` handler）的改动，也是唯一的回归风险点。
  单独提交，便于独立 revert。
- 整体回滚：撤掉 D3 的路由注册即可让功能对外消失；表留库无害；彻底清理 `DROP TABLE usage_bucket`。
