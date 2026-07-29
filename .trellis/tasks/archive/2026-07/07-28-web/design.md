# Design — 前端展示网站（React + Vite + TS）

> 子任务 `07-28-web`，对应父任务 `.trellis/tasks/07-28-cyberstalk-me/implement.md` 阶段 2、`design.md` §4。
> 上游 `07-28-backend` 已交付并归档，本文档中的后端行为均**读自已交付代码**（`server/internal/api/*.go`、`shared/contract.go`），不是推测。

## 1. 范围

一个完全公开的只读 SPA：首屏 `GET /api/v1/snapshot` 拉全量，随后 `GET /api/v1/stream`（SSE）增量合并，渲染设备卡片网格。无路由、无鉴权、无写操作。

## 2. 已交付后端的实测行为（契约事实）

| 端点 | 事实 |
|------|------|
| `GET /api/v1/snapshot` | 200，返回**裸数组** `DeviceState[]`（`api/handlers.go` 直接 `Encode(out)`，**不是** `{devices:[...]}` 包装），无设备时为 `[]` |
| `GET /api/v1/stream` | SSE。**首帧是命名事件** `event: ready\ndata: {}`；后续数据帧无 event 名，`data: {"type":"update"\|"offline","device":{...}}` |
| 静态托管 | chi `r.Get("/*")` → `http.FileServer(http.FS(embedFS))`，同源，前端用相对路径 `/api/v1/...` |
| 监听地址 | `ADDR` 默认 `:8080`（`internal/config/config.go`） |
| 离线阈值 | `OFFLINE_THRESHOLD` 默认 60s，`SCAN_INTERVAL` 默认 5s |

**由此推出两条实现约束**：

1. 命名事件 `ready` 不会进入 `EventSource.onmessage`（onmessage 只收无名/`message` 事件），因此**不能**用 onmessage 判断连接就绪 —— 连接状态用 `onopen` / `onerror`。`ready` 帧无需监听（监听了也无害）。
2. snapshot 是数组，解析守卫要按数组处理，不能找 `.devices`。

## 3. 契约镜像（`web/src/types/contract.ts`）

以 `shared/contract.go` 的 JSON tag 为唯一真源。**发现 spec 示例与真实 Go 契约有 2 处漂移，以 Go 代码为准，并在本任务内同步修正 spec**（`.trellis/spec/frontend/index.md` 明确要求「real code diverges → update the spec in the same task」）：

| 字段 | Go 真实类型 | spec `type-safety.md` 示例 | 本任务采用 | 说明 |
|------|------------|---------------------------|-----------|------|
| `activity` | `Activity`（值类型） | `Activity \| null` | **`Activity`** | Go 非指针 → JSON 永不为 null |
| `battery.level` | `*int` | `number` | **`number \| null`** | Go 指针 → 可为 null（部分设备取不到百分比） |
| `battery` | `*Battery` | `Battery \| null` | `Battery \| null` | 一致 |
| `network` | `*NetworkType` | 联合含 null | 一致 | 台式机/未知为 null |

其余字段（`device_id` / `device_name` / `device_type` / `online` / `reported_at` / `last_seen_at`）与 spec 示例一致。时间为 RFC 3339 字符串。

`device_type` / `network` 用字符串字面量联合（spec 禁 `enum`）。**解析守卫不校验枚举取值**——后端未来加设备类型时前端应降级显示（未知类型给通用图标），而不是丢事件。

### 解析守卫（无 Zod，手写 type guard，仅在信任边界）

```ts
export function parseSnapshot(raw: unknown): DeviceState[] | null   // 必须是数组，逐项过 isDeviceState
export function parseStreamEvent(raw: string): StreamEvent | null   // JSON.parse + type/device 检查
```
坏数据返回 `null`，调用方 `console.warn` 后丢弃，绝不崩页面。

## 4. 数据流与 `useDeviceStream`

```
mount ──▶ fetch /api/v1/snapshot ──▶ devices[]（全量替换，按 device_id 排序）
      └─▶ new EventSource("/api/v1/stream")
             ├─ onopen  ─▶ connection="live"；若是【重连】则重拉一次 snapshot 做全量校正
             ├─ onmessage ─▶ parseStreamEvent ─▶ 按 device_id upsert（update/offline 都是整条替换）
             └─ onerror ─▶ connection="reconnecting"（EventSource 自带重连，不手动重建）
unmount ─▶ es.close()（effect cleanup，硬性要求）
```

返回值（对齐 `.trellis/spec/frontend/hook-guidelines.md`）：

```ts
export interface DeviceStreamState {
  devices: DeviceState[];
  connection: "connecting" | "live" | "reconnecting";
  error: string | null;   // snapshot 首次失败时展示用；SSE 断线不算 error（有重连态）
}
```

关键点：

- **重连必须重拉 snapshot**：SSE 自动重连不会补发断线期间的事件（spec 明示，也是最容易漏的 bug）。用一个 `hasConnectedRef` 区分首次 open 与重连 open。
- effect 依赖数组**不放 `devices`**，state 更新一律用函数式 updater，否则 EventSource 会被反复重建。
- 只有一个 EventSource 实例，由 `App` 持有；卡片组件不做任何数据获取。
- `devices` 按 `device_id` 稳定排序，避免事件到达时卡片跳位。

## 5. 组件树与 UI

```
App                      ← useDeviceStream + 30s tick（刷新「最后活跃 X 前」相对时间）
├── 页头（站名 + 连接状态徽标）
└── DeviceGrid           ← devices[]
    └── DeviceCard       ← 单设备（AnimatePresence + motion.div 进出场/布局动画）
        └── BatteryIndicator  ← battery: Battery | null，null 时不渲染
```

- 基元一律 `npx shadcn@latest add`：`card`、`badge`、`skeleton`。不手写。
- 图标一律 `lucide-react`：`Monitor`/`Smartphone`（设备类型）、`Wifi`/`Signal`/`Cable`/`WifiOff`（网络）、`BatteryCharging`/`Battery`、`MoonStar`（空闲）、`Radio`（连接态）。不内联 SVG。
- 动效用 `motion/react`（Framer Motion）：卡片进出场 + `layout`；hover 之类用 Tailwind `transition-*`。
- **主题（待定）**：shadcn 的 CSS 变量 token 集中在 `src/index.css` 的一个明确分隔区块内。用户将从 tweakcn 提供主题，届时**整块替换该区块即可**，组件代码零改动。在此之前先用 shadcn 默认 token（dark 优先）落地，不阻塞实现。

### 各状态的渲染约定

| 情况 | 渲染 |
|------|------|
| 加载中（snapshot 未回） | shadcn `Skeleton` 卡片占位 |
| 设备数为 0 | 空态文案，不是白屏 |
| 离线设备 | 保留卡片，`opacity-60 grayscale` + **文字**「离线」+「最后活跃 X 前」（颜色不可作为唯一信号，a11y 要求） |
| `battery === null` | 整块不渲染（台式机），不显示 0% |
| `battery.level === null` | 只显示充电图标/文案，不画进度条 |
| `network === null` | 不渲染网络块 |
| `activity.idle === true` | 「空闲 X 分钟」徽标（`MoonStar`），活动描述仍展示 |

## 6. 构建接入后端（已确认方案 A）

```ts
// web/vite.config.ts
build: { outDir: "../server/cmd/server/web", emptyOutDir: true }
```

- Vite 直接把产物写进后端 `//go:embed all:web` 指向的目录，**构建产物提交进 git**。任何 commit 上 `go build ./...` 都能出「带最新前端的单二进制」，部署 = 传一个文件。
- 代价与纪律：**前端改动必须 `npm run build` 后连同产物一起提交**，否则二进制里是旧页面。写进本任务 checklist 与 README/spec。
- `outDir` 在 Vite root 之外，必须显式 `emptyOutDir: true` 否则 Vite 会拒绝清空并告警。
- 该目录当前的占位 `index.html` 会被首次构建覆盖，属预期（`main.go` 注释已写明由本子任务替换）。

## 7. 本地开发

- `npm run dev`（Vite :5173）+ dev proxy：`/api` → `http://localhost:8080`。SSE 走 Vite proxy 正常（Vite 默认不缓冲 EventSource 响应）。
- 后端本地跑：`cd server && go run ./cmd/server`（SQLite 落 `server/cyberstalk.db`）。
- 造数据：`go run ./cmd/server register-device <id> <name> windows` 拿 token，再 `curl` POST `/api/v1/report` 模拟上报（验收 AC5 用两台设备 + 停发触发离线）。

## 8. 依赖基线

运行时（spec `quality-guidelines.md` 已批准的基线，不额外引入）：`react`、`react-dom`、shadcn 栈（`tailwindcss` + `@tailwindcss/vite`、Radix 基元、`class-variance-authority`、`clsx`、`tailwind-merge`）、`lucide-react`、`motion`。

开发时：`vite`、`@vitejs/plugin-react`、`typescript`、ESLint（`@eslint/js` + `typescript-eslint` + `eslint-plugin-react-hooks`）、`vitest`、`@types/node`（vite.config 里 `path.resolve` 需要）。

**不引入**：状态库、React Query/SWR、axios、dayjs/moment、Zod（均被 spec 明确禁止，`Intl` + 自写 `timeAgo` 足够）。

## 9. 测试策略

Vitest 只测纯逻辑（spec：组件/E2E 测试 MVP 不要求，靠手工对照 AC 验收）：

- `lib/format.ts`：`timeAgo` 边界（刚刚 / 秒 / 分 / 时 / 天 / 未来时间 / 非法时间串）、`formatIdle`。
- `types/contract.ts`：`parseStreamEvent` 对合法 update/offline、非法 type、缺 `device.device_id`、非 JSON 串的行为；`parseSnapshot` 对非数组、数组含坏项的行为。

## 10. 风险

- **契约漂移**：`shared/contract.go` 与 `types/contract.ts` 必须同任务同步（spec 核心不变量）。本任务已发现 spec 示例漂移 2 处并会修正 spec 本身。
- **忘记 build 就提交**：方案 A 的固有风险 —— 二进制里前端变旧。缓解：checklist 明写、commit 前跑一次 `npm run build`。
- **SSE 经反代被缓冲**：后端已发 `X-Accel-Buffering: no`，nginx 还需关 `proxy_buffering`（父任务 design §1 已记）；本地开发走 Vite proxy 无此问题。
- **重连丢事件**：靠 onopen 重拉 snapshot 兜底，这是必做项不是优化项。
