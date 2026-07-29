# Implement — 赛博视奸设备状态展示网站

> 复杂多组件任务。**已确定拆为 parent + 子任务**，Android 延后。下面是整体有序执行计划与验证点，对应子任务：`backend` → `web` & `client-windows`（可并行）→ `client-android`（延后）。

## 阶段 0 · 骨架与契约（地基，归属 backend 子任务）
- [ ] 初始化仓库结构：`server/`（Go）、`shared/`（Go module，契约 struct）、`web/`（React+Vite+TS）、`client-windows/`（Go）、`client-android/`（Kotlin，延后）。目录名与 `.trellis/spec/backend/directory-structure.md` 的目录树保持一致。
- [ ] 在 `shared/` 定义 Go struct `DeviceState` / `ReportPayload`，作为契约唯一来源；前端 TS 类型据此镜像。
- [x] 填充 `.trellis/spec/backend/*` 与 `frontend/*` 中与本项目相关的最小约定（已于 2026-07-28 完成并提交，见两个 spec 索引，状态均为 Filled）。
- 验证：`go build ./...` 通过，`shared` 可被 server 与 client 引用。

## 阶段 1 · 后端（R2，Go）
- [ ] Go 服务（`net/http` + chi）+ SQLite（`modernc.org/sqlite`），建 `devices` / `device_state` 表。
- [ ] admin 子命令 `register-device`：生成 token（存哈希），输出客户端配置片段。
- [ ] `POST /api/v1/report`：Bearer 校验 + device_id 一致性 + upsert 最新状态。
- [ ] 在线/离线扫描（`time.Ticker`，阈值可配）+ SSE `GET /api/v1/stream` 广播（订阅者 fan-out）。
- [ ] `GET /api/v1/snapshot` 首屏快照。
- [ ] 静态资源托管 web 构建产物（`embed.FS`）。
- 验证：`curl` 带/不带 token 各测一次（AC6）；两个模拟 report 后 snapshot 正确；停止上报后阈值内变离线（AC3）。

## 阶段 2 · 前端（R3）— **已完成**（子任务 `07-28-web`，2026-07-29）
- [x] React+Vite 项目 + shadcn/ui 初始化（Tailwind、lucide-react、Framer Motion，组件不手写）；`useDeviceStream` hook（snapshot + SSE 合并）。
- [x] 设备卡片组件：活动、在线灯、活跃/空闲、电量、网络、最后活跃时间；离线置灰。
- [x] 构建产物接入后端静态托管（`vite build` 直接输出到 `server/cmd/server/web/`，产物入库）。
- 验证：无痕浏览器直开 URL 看到卡片（AC4）；模拟状态变化页面自动更新（AC5）。**均已实测通过。**

## 阶段 3 · Windows 客户端（R1.1/R1.3/R1.4，Go）
- [ ] Go + Win32 采集：前台窗口+进程名、空闲、电量、网络。
- [ ] `config.yaml` 脱敏映射规则 + 白名单；未命中→通用描述；**只上报映射结果**。
- [ ] 上报循环 + 重试退避；`go build` 单 exe；开机自启说明。
- 验证：真机跑起来，网站显示"应用名·活动"且无原始标题泄露（AC1）。

## 阶段 4 · Android 客户端（R1.2/R1.3/R1.4）— 成本最高，**已延后为独立后续子任务**
- [ ] Kotlin App + 前台 Service；引导授予 PACKAGE_USAGE_STATS。
- [ ] 采集前台 App（UsageStats）、电量、网络；设备端映射规则。
- [ ] 上报同契约；保活/省电白名单引导。
- 验证：真机显示当前 App（映射后）、电量、网络（AC2）。

## 全局验证命令（占位，随实现确定）
- 后端/契约/Windows 客户端：`go build ./...` && `go vet ./...` && `go test ./...`
- 前端：`cd web && npm run build`

## 风险 / 回滚点
- Android 阶段独立，失败不影响 AC1/AC3~AC6；可作为独立子任务回滚而不动其余交付。
- 脱敏为安全关键：阶段 3/4 需专门验证"原始标题绝不出现在上报/页面"。默认从严策略先行。

## 子任务门槛（若拆分）
- 批准后用 `task.py create "<title>" --slug <name> --parent .trellis/tasks/07-28-cyberstalk-me` 创建：`backend`、`web`、`client-windows`、`client-android`。
