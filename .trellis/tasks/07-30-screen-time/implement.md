# 执行计划（父任务）：设备屏幕使用时间统计

父任务本身**不承担实现**。它持有任务地图、跨子任务的集成验收，以及最后的规范同步。
需求见 `prd.md`，技术设计见 `design.md`。

## 任务地图

| 顺序 | 子任务 | 交付物 | 依赖 |
|------|--------|--------|------|
| 1 | `07-30-screen-time-contract` | `shared.Activity.Locked` + Windows 客户端填充 + TS 镜像 | 无 |
| 2 | `07-30-screen-time-server` | `usage` 包、`usage_bucket` 表、`GET /api/v1/usage`、保留期清理、内嵌 tzdata、四个环境变量 | 需要 1 的 `Locked` 字段才能正确区分锁屏（R2.6 / AC5） |
| 3 | `07-30-screen-time-web` | 「使用时间」tab、设备/窗口选择、两级排行、小时/按日图 | 需要 2 的接口与响应契约 |

这是**顺序依赖，不是并行**。父/子结构不提供依赖系统，因此每个子任务的 `prd.md` 里都写明了它的前置。

各子任务的验收标准分配：

| 父任务 AC | 归属 |
|-----------|------|
| AC1 | 子任务 1 |
| AC2–AC6、AC10–AC13 | 子任务 2 |
| AC7–AC9 | 子任务 3（AC7 需要子任务 2 已上线） |
| **AC14（脱敏红线复验）、AC15（无回归）** | **父任务集成验收**，见下节第 2、5 步 |

## 集成验收（三个子任务都完成后，在父任务里做）

> **状态（2026-07-30）**：5 步全部实测通过，15 条 AC 全勾（见 `prd.md`）。步骤 1 的锁屏项在本机
> 发现 `lockapp.exe` 作为前台窗口使 `process == ""` 检测失效（AC5 失败），已修复 `collect` 识别
> `lockapp.exe`/`logonui.exe` 为锁屏并复验通过，见 `prd.md` R1.2 与 `design.md` §2.1。

1. **端到端真机链路** ✓：本机跑一台 Windows 客户端，正常上报若干分钟，期间人为制造四种状态 ——
   切换应用、静置超过 `idle_threshold`、锁屏、杀掉客户端超过 `USAGE_MAX_GAP` 后重启。
   打开站点「使用时间」tab 逐条核对 AC2–AC6。
2. **脱敏红线复验**（本任务的最高风险项） ✓：用诱饵标题（把浏览器标签改成 `SECRET-TITLE-CANARY`）
   跑一轮，然后确认：
   - `GET /api/v1/usage` 的响应体里不出现诱饵字符串；
   - `usage_bucket` 表的 `app` / `description` 列里不出现诱饵字符串
     （`docker compose exec app` 或直接查本地 `.db`）；
   - 服务端日志（含 `-v`）里不出现诱饵字符串。

   这一步必须做：本任务第一次把活动描述**持久化**了，此前它只在内存和一次响应里过一遍。
   映射结果本身是脱敏的，但 `expose_title` 白名单的存在意味着一旦有人配了它，
   原始标题会被**长期落盘**并公开 —— 这是风险等级的实质变化，需在 README 明确写出来。
3. **保留期与体积** ✓：把 `USAGE_RETENTION_DAYS` 设成 1、`USAGE_PRUNE_INTERVAL` 设成 10s 起服务，
   确认旧桶被删且接口不报错（AC10）。
4. **容器内时区** ✓：`docker compose build && up`，确认服务正常启动（验证 `time/tzdata` 内嵌生效，
   见 `design.md` §6.1 —— alpine 无 tzdata，这是最容易在本地全绿、一进容器就挂的一项）。
   再把 `DISPLAY_TIMEZONE` 设成非法值，确认启动失败并给出可读错误（AC11）。
5. **既有功能不回归** ✓：「此刻」tab 的实时卡片、离线判定、SSE 重连后校正全部照常（对应父项目 AC3–AC6）。

## 全量验证命令

```bash
# Go：三个模块显式列出（go.work 使得裸 ./... 会失败）
gofmt -l shared server client-windows
go vet ./server/... ./shared/... ./client-windows/...
go test ./server/... ./shared/... ./client-windows/...
# -race 需要 cgo。本机 mingw-w64 的 gcc 已在 PATH（/d/mingw/mingw64/bin），可直接跑；
# 换机器若报找不到 gcc，把该目录加进 PATH 即可。生产构建仍是 CGO_ENABLED=0。
CGO_ENABLED=1 go test -race ./server/... ./shared/...
CGO_ENABLED=0 GOOS=linux go build ./server/... ./shared/...
GOOS=windows GOARCH=amd64 go build ./client-windows/...

# Web
cd web && npm run lint && npm run typecheck && npx vitest run && npm run build

# 前端产物必须一起提交，否则 CI 的 embed freshness 闸会红
git diff --exit-code -- server/cmd/server/web

# 镜像
docker compose build
```

## 规范与文档同步（Phase 3.3，父任务收口时做）

清单见 `design.md` §11。逐项确认，不要漏：

- [x] `.trellis/spec/backend/database-guidelines.md` —— 「Never accumulate history rows」需要限定到 `device_state`，
      并把 `usage_bucket` 加入 Schema 段（子任务 2 已做）
- [x] `.trellis/tasks/07-28-cyberstalk-me/prd.md` —— R2.4 与 Out of Scope 第 1 条（子任务 2 已做）
- [x] `.trellis/spec/backend/deployment-guidelines.md` —— 四个新环境变量 + `time/tzdata` 的原因（子任务 2 已做）
- [x] `.trellis/spec/frontend/state-management.md` —— 「entire state fits in one hook」已不成立（子任务 3 已做）
- [x] `README.md` —— 功能列表；以及上面集成验收第 2 步得出的结论：
      `expose_title` 的后果从「实时公开」升级为「长期落盘且公开」（子任务 3 已做）
- [x] `.trellis/spec/guides/cross-layer-thinking-guide.md` —— `Locked` 的 Real-world example 补第二台机器
      `lockapp.exe` 锁屏检测失效案例（父任务集成验收发现，本次新增）
- [x] `design.md` §2.1 / §4.2 —— `Locked` 语义拓宽到「无前台窗口或前台是锁屏进程」、`stateOf` 注释更正
      `idle_seconds=0` 为机型相关（本次新增）
- [x] `prd.md` R1.2 —— 记录 `lockapp.exe` 集成验收发现与修复（本次新增）

## 风险与回滚点

| 风险 | 缓解 |
|------|------|
| **容器内 `LoadLocation` 失败**（alpine 无 tzdata） | `import _ "time/tzdata"`；集成验收第 4 步专门验证。**不要**改 Dockerfile 运行阶段加 `RUN`，那会迫使多架构构建引入 QEMU |
| 归因把断线时间算成使用时间 | `USAGE_MAX_GAP` 默认跟随 `OFFLINE_THRESHOLD`；AC3 覆盖 |
| 统计功能拖累实时上报 | 归因错误只记日志不影响 204（R2.7 / AC12）；写入包在单事务，WAL 下读不阻塞写 |
| `expose_title` 用户的原始标题被长期落盘 | 集成验收第 2 步 + README 明确警告。**不在代码里为它加特殊处理** —— 那会造成「配了 expose_title 却统计不到」的隐性行为 |
| 前端产物未重建导致线上仍是旧 UI | CI 的 embed freshness 闸已覆盖；`implement.md` 的验证命令里显式带上 `git diff --exit-code` |

回滚点：三个子任务都是纯增量。任一环节需要回退，撤掉 `usage` 路由 + 前端 tab 即可，
`usage_bucket` 表留存不影响既有查询；彻底回滚 `DROP TABLE usage_bucket`（见 `design.md` §10）。
