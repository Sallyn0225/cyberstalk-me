# Design — 赛博视奸设备状态展示网站

## 1. 架构总览

```
┌─────────────┐  HTTPS POST /api/v1/report (Bearer token)   ┌──────────────┐
│ Windows 客户端│ ───────────────────────────────────────────▶ │              │
│ (Go, 单 exe) │                                              │              │
└─────────────┘                                              │   Backend    │
┌─────────────┐  HTTPS POST /api/v1/report (Bearer token)   │ (Go, 单二进制)│
│ Android 客户端│ ───────────────────────────────────────────▶ │  net/http    │
│ (Kotlin 前台)│  ※ 延后交付                                   │  + SQLite    │
└─────────────┘                                              │              │
                                                             │              │
┌─────────────┐  GET /api/v1/snapshot (一次性)              │              │
│  浏览器访客   │ ◀──────────────────────────────────────────▶ │              │
│ (React SPA) │  GET /api/v1/stream (SSE 实时推送)          │              │
└─────────────┘ ◀────────────────────────────────────────── └──────────────┘
```

- 单台 VPS 同时托管后端 API + SSE + 静态前端资源；后端为单个 Go 静态二进制，部署=传一个文件。TLS 由反代终止（推荐 Caddy，自动 HTTPS；用 nginx 则需对 `/api/v1/stream` 关闭 `proxy_buffering`，否则 SSE 被缓冲），Go 服务只监听 `127.0.0.1`。
- 数据流单向：客户端 → 后端（写）；后端 → 浏览器（读，SSE 广播）。
- **后端与 Windows 客户端同为 Go**，共享同一份契约 struct（`shared/` Go module）。

## 2. 数据契约（客户端 → 后端）

`POST /api/v1/report`，Header `Authorization: Bearer <device_token>`，Body：

```jsonc
{
  "device_id": "win-desktop",        // 与 token 绑定，服务端校验一致
  "device_name": "我的台式机",
  "device_type": "windows",          // "windows" | "android"
  "activity": {
    "app": "VS Code",                // 映射后的应用名，已脱敏
    "description": "在写代码",         // 友好活动描述，已脱敏
    "idle": false,                    // 是否空闲
    "idle_seconds": 0
  },
  "battery": { "level": 82, "charging": true },   // 可选（PC 台式机可能无电池 → null）
  "network": "wifi",                  // "wifi" | "cellular" | "ethernet" | "offline" | null
  "reported_at": "2026-07-28T10:30:00Z"
}
```

- 服务端**只接收已脱敏字段**。原始窗口标题/包名细节不在契约内，保证不落库。
- 字段缺失（如台式机无电池）用 `null`，前端优雅降级。

## 3. 后端

- 语言/框架：**Go**（标准库 `net/http`，路由用 `chi` 或标准库 mux；SSE 手写）。
- 存储：**SQLite**（纯 Go 驱动 `modernc.org/sqlite`，免 CGO，交叉编译最省心），两张表：
  - `devices(device_id PK, device_name, device_type, token_hash, created_at)` — 设备注册与 token（存哈希）。
  - `device_state(device_id PK, last_report_json, reported_at, last_seen_at)` — 每设备最新状态（覆盖写，不留历史）。
- 在线判定：以**服务端接收时间**为准——收到上报时由服务端打点 `last_seen_at`；客户端 `reported_at` 仅作展示/调试，不参与判定（避免设备时钟漂移误判）。`last_seen_at` 距今超过 `OFFLINE_THRESHOLD`（如 60s）判为离线；`time.Ticker` 每 5s 扫描，状态变化即通过 SSE 广播。
- SSE 广播：内存维护订阅者 channel 集合（`map[chan Event]struct{}` + `sync.Mutex`），report/离线事件 fan-out 给所有连接。
- 端点：
  - `POST /api/v1/report` — 校验 Bearer token → 匹配 device_id → upsert `device_state` → 广播增量。
  - `GET /api/v1/snapshot` — 返回全部设备当前状态数组（前端首屏）。
  - `GET /api/v1/stream` — SSE，推送 `{type:"update"|"offline", device}` 事件。
  - 静态资源：`/` 提供前端构建产物（`embed.FS` 打进二进制，或从磁盘目录托管）。
- 鉴权：设备 token 为随 CLI 生成的随机串，落库存哈希（如 SHA-256）；上报比对哈希。访客侧无鉴权。

## 4. 前端

- **React + Vite + TypeScript**。
- **UI 栈**：组件优先用 **shadcn/ui**（含 Tailwind CSS，生成物在 `components/ui/`），先不自己手写；图标一律 **lucide-react**，不自绘 SVG；动效用 **Framer Motion**（`motion` 包），不手写 keyframes（简单 hover 过渡用 Tailwind transition 即可）。
- 首屏 `GET /snapshot` 拿全量，之后订阅 `/stream` SSE 增量合并到状态。EventSource 断线自动重连后**必须重拉一次 snapshot** 做全量校正（重连间隙的事件会丢失），该逻辑封装在 `useDeviceStream` 内。
- UI：设备卡片网格。每卡：设备名/类型图标、在线指示灯、当前活动（app + description）、活跃/空闲徽标、电量条+充电图标、网络图标、"最后活跃 X 前"。离线卡片置灰。
- 遵循 `.trellis/spec/frontend/` 规范（组件/hook/类型安全），SSE 封装为 `useDeviceStream` hook。

## 5. 上报客户端

### 5.1 Windows（Go，单 exe）
- 采集（Win32 via `golang.org/x/sys/windows` + `syscall`）：
  - 前台窗口：`GetForegroundWindow` + `GetWindowTextW`；进程名经 `GetWindowThreadProcessId` → `QueryFullProcessImageNameW`。
  - 空闲：`GetLastInputInfo`（算 idle 秒数）。
  - 电量/充电：`GetSystemPowerStatus`（台式机无电池 → battery=null）。
  - 网络：`wifi`/`ethernet` 简单判定（可查活动网卡类型）。
- **脱敏映射（设备端）**：读取 `config.yaml`（`gopkg.in/yaml.v3`）——`process_name → {app, description}` 规则表 + 白名单（放开显示原始标题的应用，可选）。未命中规则的进程只输出通用描述（如"使用中"），绝不外泄原始标题。
- 循环每 `INTERVAL`（如 10s）采集→映射→POST。失败重试/退避。
- 契约复用后端 `shared/` Go struct。
- 分发：`go build` 出**单 exe**，双击即用；开机自启用注册表 Run 键或计划任务（后续）。

### 5.2 Android（Kotlin 原生 App）
- 前台 Service 常驻（带通知），定时：
  - 前台 App：`UsageStatsManager.queryEvents`（需用户在系统设置授予 `PACKAGE_USAGE_STATS`）。
  - 电量/充电：`BatteryManager` / sticky `ACTION_BATTERY_CHANGED`。
  - 网络：`ConnectivityManager`（wifi/cellular）。
- 设备端映射：`packageName → {app, description}` 规则 + 白名单，内置资源文件，未命中显示通用类别。
- 上报同契约 POST。保活：前台服务 + 引导用户加省电白名单。
- 是本项目最大工作量项（见 PRD Risks）。

## 6. 配置与密钥

- 后端 `.env`：端口、`OFFLINE_THRESHOLD`、SQLite 路径。
- 设备注册用一个 admin CLI/脚本：`register-device <id> <name> <type>` → 打印一次性 token，写入客户端配置。
- 客户端配置：`server_url`、`device_id`、`token`、`interval`、映射规则表。

## 7. 复用与规范

- 后端遵循 `.trellis/spec/backend/`（目录结构、错误处理、日志、质量）。
- 前端遵循 `.trellis/spec/frontend/`。
- 契约源为 **Go struct `DeviceState` / `ReportPayload`**（`shared/` Go module，后端与 Windows 客户端共享）；前端 TS 类型据此手写或用工具生成保持一致。

## 8. 任务分解建议（parent/child）

本任务含多个可独立验证的交付物，建议在批准后拆为子任务：
1. **共享契约 + 后端**（API/SSE/SQLite/鉴权/在线判定）—— 其他所有部分的地基。
2. **前端展示网站**（依赖后端契约）。
3. **Windows 客户端**（依赖后端契约）。
4. **Android 客户端**（依赖后端契约，成本最高，可最后做或延后）。

顺序：契约+后端 → 前端 & Windows 客户端（可并行验证）→ Android 客户端。
