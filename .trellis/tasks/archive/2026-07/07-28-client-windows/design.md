# Design — Windows 上报客户端（Go 单 exe）

> 父任务 `.trellis/tasks/07-28-cyberstalk-me/design.md` §5.1 给了大框架；本文档下沉为 client-windows 子任务专属的包边界、采集/映射/上报数据流与取舍，并把**后端实测行为**（而非设计稿）钉死。执行清单见 `implement.md`。

## 1. 范围与边界

交付 `client-windows/` 一个 Go module，`go build` 出单 exe，常驻 Windows：**采集 → 设备端脱敏映射 → 定时上报**。对应父任务 implement.md 阶段 3、需求 R1.1 / R1.3 / R1.4。

**不做**：后端与前端改动（已交付）、Android 客户端（`07-28-client-android`）、图形安装器/托盘 UI、开机自启的自动注册（只交付文档说明）。

**已有地基**（本任务只消费，不改）：
- `shared/contract.go` — `ReportPayload` / `Activity` / `Battery` 契约 struct，通过 `go.work` 直接引用。
- `POST /api/v1/report` — 已上线，Bearer 鉴权，成功返回 **204 No Content**。
- `server register-device <id> <name> windows` — 打印一次性 token 与 config.yaml 片段。

## 2. 与后端的实测契约（照实现，不照设计稿）

来源：`server/internal/api/handlers.go`、`server/cmd/server/main.go`。

| 事项 | 实测行为 | 对客户端的含义 |
|------|----------|----------------|
| 端点 | `POST {server_url}/api/v1/report` | `server_url` 不含路径，客户端负责拼接 |
| 鉴权 | `Authorization: Bearer <token>` | 缺失/错误 → 401 |
| 成功响应 | **204 No Content，无 body** | 只认 204 为成功，不解析 body |
| `device_id` | body 内必须与 token 绑定设备一致，否则 401 | 从 config 读，不自行推导 |
| `device_type` | 必须是 `"windows"`（否则 400） | 客户端硬编码 `"windows"` |
| `device_id` / `device_name` / `device_type` | 服务端落库前用注册信息**重新盖章**（handlers.go 中三行 re-stamp） | 客户端 `device_name` 仅占位，页面显示的是注册时的名字；改名走 `register-device`，不是改 config |
| `reported_at` | 仅存来展示/调试，**不参与在线判定** | 客户端时钟漂移不会误判离线；但仍应送 UTC |
| 在线判定 | 服务端收到报文时打 `last_seen_at`，超 `OFFLINE_THRESHOLD`（默认 60s）判离线 | **必须每个 interval 都发**，即使状态没变；否则会被判离线 |
| 电池缺失 | `battery: null` 前端整块不渲染；`battery.level: null` 只显示充电态 | 台式机送 `null`，不要伪造 0 |

契约 struct 一律从 `shared/` import，**禁止**在 client 内重新声明 payload 结构（spec quality §Required Patterns）。

## 3. 包结构

遵循 `.trellis/spec/backend/directory-structure.md` 的 client-windows 子树，新增一个 `config` 包（与 `server/internal/config` 对称）：

```
client-windows/
├── go.mod                      # + shared(replace) / gopkg.in/yaml.v3 / golang.org/x/sys
├── config.example.yaml         # 可直接改名使用的样例（含中文映射规则示例）
├── README.md                   # 部署、开机自启（注册表 Run 键）、隐私说明
├── cmd/agent/main.go           # flag 解析、slog 装配、依赖装配、Ctrl+C 优雅退出
└── internal/
    ├── config/                 # config.yaml 载入 + 校验 + 默认值（纯逻辑，必测）
    ├── collect/                # Win32 采集，windows-only，豁免单测
    ├── mapping/                # 脱敏映射：进程名 → {app, description}（纯函数，必测）
    └── report/                 # HTTP 上报 + 重试退避（httptest 可测）
```

依赖方向单向：`cmd/agent` →（`config`、`collect`、`mapping`、`report`、`shared`）；四个 internal 包**互不 import**（`report` 只收 `shared.ReportPayload`，`mapping` 只收进程名与取标题的回调）。装配在 `main.go`，无 `init()`（spec directory-structure §Module Organization）。

> spec 触碰：`directory-structure.md` 的 client-windows 子树未列 `internal/config`，本任务落地后需同步该文件（spec 自述"real layout diverges 就更新"）。

## 4. 隐私模型（本任务的安全红线）

整个产品的安全性建立在"原始标题不出设备"上。落地成三道闸：

1. **默认不取标题**。`GetWindowTextW` **只在**当前前台进程命中了「需要标题」的规则（`title_patterns`）或在 `expose_title` 白名单里时才调用。未配置任何标题规则的部署，进程里根本不存在原始标题。
2. **未命中一律通用化，绝不回落 exe 名**。未匹配任何规则的进程输出 `default_app`（默认「某个应用」）+ `default_description`（默认「使用中」）。**不允许**把 `notepad.exe` → `Notepad` 这类"自动派生"作为兜底——exe 名本身就可能是泄密面（内部工具名、项目代号）。想显示就显式写规则。
3. **标题只在内存里活一次**。`collect` 返回的快照里，原始标题字段**不导出、无 json tag**，只传给 `mapping`；`mapping` 的返回值是已脱敏 `shared.Activity`。原始标题不进日志（含 Debug）、不进 `-dry-run` 输出、不落磁盘（spec logging §What NOT to Log）。
4. `expose_title` 白名单是唯一的显式豁免口子，默认**空**。命中白名单时，原始标题作为 `description` 上报——这是用户显式授权的行为，README 里写明风险。

## 5. 数据流（一个 tick）

```
ticker(interval)
  └─ collect.Snapshot()                     // Win32，失败字段降级为零值/nil，不中断
       ├─ foreground: pid → exe base name（小写，如 "code.exe"）(+ rawTitle 懒取)
       ├─ idle:       GetLastInputInfo → idleSeconds
       ├─ battery:    GetSystemPowerStatus → *shared.Battery（无电池 → nil）
       └─ network:    GetAdaptersAddresses → "wifi"|"ethernet"|"cellular"|"offline"
  └─ mapping.Resolve(exeName, titleFunc, idleSeconds)   // 纯函数，隐私边界
       └─ shared.Activity{App, Description, Idle, IdleSeconds}
  └─ 组装 shared.ReportPayload{DeviceID, DeviceName, "windows", Activity, Battery, Network, ReportedAt: now().UTC()}
  └─ report.Client.Send(ctx, payload)       // 失败不致命：warn + 退避，循环继续
```

## 6. 采集层 `collect`（Win32）

全部文件加 `//go:build windows`，包内**只有** windows 实现，不写跨平台 stub（见 §9 构建策略）。优先用 `golang.org/x/sys/windows` 已有封装，缺失的用 `windows.NewLazySystemDLL(...).NewProc(...)` 自行声明（实现时逐个确认该版本 x/sys 是否已导出，不预设）。

| 字段 | API | 降级与坑 |
|------|-----|----------|
| 前台进程 | `GetForegroundWindow` → `GetWindowThreadProcessId` → `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` → `QueryFullProcessImageName` | hwnd 为 0（锁屏/切换中）→ 视为无前台，输出锁屏/空活动；提权进程 `OpenProcess` 可能 Access Denied → 降级为未知进程（走通用描述），不报错退出 |
| 原始标题 | `GetWindowTextW` | **懒调用**，仅规则需要时（§4） |
| 空闲 | `GetLastInputInfo` + `GetTickCount64` | `LASTINPUTINFO.dwTime` 是 **32 位** tick：必须用 `uint32(GetTickCount64()) - dwTime` 做无符号回绕减法，直接跟 64 位相减会在开机 49.7 天后炸出天文数字 |
| 电池 | `GetSystemPowerStatus` | `BatteryFlag & 128`（NO_SYSTEM_BATTERY）或 `BatteryLifePercent == 255`（未知）→ `Battery = nil`；`ACLineStatus == 1` → `Charging = true` |
| 网络 | `GetAdaptersAddresses(AF_UNSPEC, GAA_FLAG_INCLUDE_GATEWAYS, ...)` | 取 `OperStatus == IfOperStatusUp` 且有默认网关的网卡：`IF_TYPE_IEEE80211`→wifi，`IF_TYPE_ETHERNET_CSMACD`→ethernet，`IF_TYPE_WWANPP*`→cellular，跳过 loopback；一个都没有 → `"offline"`；多个 up 时**有线优先**（笔记本插网线同时开 wifi 的常见场景） |

采集失败的统一策略（spec error-handling §Client Errors）：**降级该字段为零值/nil，照常上报**，不因为一个 Win32 调用失败就跳过整轮。

## 7. 映射层 `mapping`（纯函数，必测）

```go
type Rule struct {
    Process       string          // 匹配 exe 名，大小写不敏感，如 "code.exe"
    App           string          // 脱敏后应用名
    Description   string          // 脱敏后默认描述
    TitlePatterns []TitlePattern  // 可选：按原始标题细化描述（标题只在内存中参与匹配）
}
type TitlePattern struct {
    Match       string  // Go regexp
    Description string  // 命中后替换 description
}
```

`Resolve(process string, title func() string, idleSeconds int) shared.Activity`：

1. 进程名小写归一后查规则表（`map[string]Rule`，`config` 载入时预编译正则）。
2. 未命中 → `{App: defaultApp, Description: defaultDescription}`，**且不调用 `title()`**。
3. 命中且有 `TitlePatterns` → 调 `title()`，按顺序首个匹配的 pattern 覆盖 description；都不中则用规则默认 description。
4. 命中且进程在 `ExposeTitle` 白名单 → description 直接用 `title()` 原文（唯一的显式豁免）。
5. 前台不可用（锁屏/hwnd=0）→ `{App: lockedApp, Description: lockedDescription}`（默认「已锁屏」）。
6. `Idle = idleSeconds >= idleThreshold`；`IdleSeconds` 原样带上。空闲时 App/Description **仍按前台真实值输出**（前端已有"空闲"徽标做区分），不做二次隐藏。

正则在 `config.Load` 阶段 `regexp.Compile`，编译失败 = 配置错误，启动即失败（fail loud），不在 tick 里静默吞。

## 8. 上报层 `report`

- `Client{httpClient *http.Client, url, token string}`；`http.Client` **复用**，`Timeout: 10s`；`http.Transport` 默认即可（长连接复用）。
- `Send(ctx, payload) error`：JSON 编码 → POST → 判定：
  - `204` → nil。
  - `401` / `400` → **永久性配置错误**：`ErrPermanent` sentinel，调用方 warn 一次并直接退到最大退避（token 写错时不刷屏、也不放弃——用户可能正在改配置后重启服务端）。
  - `5xx` / 网络错误 / 超时 → 可重试错误。
- 退避：初始 `interval`，失败后指数翻倍（×2），**上限 2 分钟**（spec error-handling 明确写了 cap ~2 minutes）；成功后立即复位到 `interval`。
- **不缓冲历史**：失败的那一轮直接丢弃，下一轮送最新采集结果。产品决策就是"只有最新状态"，补发旧状态既没意义又会污染 `last_seen`。
- 退避期间也要响应 ctx 取消（`select { case <-ctx.Done(): case <-timer.C: }`），Ctrl+C 秒退。

## 9. 构建与跨平台策略

- `collect` 是 windows-only，包内所有文件 `//go:build windows`，**不写空壳 stub**：写了也永远跑不了，只是给自己一层假的编译绿灯。
- 后果：`GOOS=linux go build` 覆盖不到 `client-windows`（"build constraints exclude all Go files"）。因此**门禁命令分域**：
  - Linux 交叉编译门禁（VPS 部署验证）：`CGO_ENABLED=0 GOOS=linux go build ./server/... ./shared/...`
  - Windows 本机门禁：`./server/... ./shared/... ./client-windows/...` 全量 build / vet / test
- 已知环境事实（journal 2026-07-28）：workspace 根目录直接 `go build ./...` 在本机报 "directory prefix . does not contain modules listed in go.work"，**必须显式列三个 module 路径**。implement.md 的验证命令按此写。
- `go test -race` 需 `CGO_ENABLED=1` + mingw-w64 gcc（已装在 `D:\mingw\mingw64\bin`，spec quality §Tooling 有记录）。生产构建仍 `CGO_ENABLED=0`。
- `client-windows/go.mod` 沿用 server 的做法加 `replace cyberstalk.me/shared => ../shared`（Go 1.26 workspace 在本机未自动解析本地 module，journal 已记）。
- 新增第三方依赖：`gopkg.in/yaml.v3`、`golang.org/x/sys` —— 两者都在 spec quality §Forbidden Patterns 的 MVP 批准清单内，无需额外审批。

## 10. 配置 `config`

`config.yaml` 的前四个键**与 `register-device` 的输出片段逐字对齐**（`server_url` / `device_id` / `token` / `interval`），保证注册输出可以原样粘贴：

```yaml
server_url: http://localhost:8080
device_id: win-desktop
token: <64 hex>
interval: 10s

device_name: 我的台式机        # 可选，服务端会用注册名覆盖，仅本地日志可读性
idle_threshold: 5m            # 超过这个时长没有键鼠输入判为空闲
default_app: 某个应用          # 未命中规则时的兜底（隐私默认从严）
default_description: 使用中
locked_app: 已锁屏
locked_description: 人不在

rules:
  - process: code.exe
    app: VS Code
    description: 在写代码
  - process: chrome.exe
    app: Chrome
    description: 在上网
    title_patterns:
      - match: "(?i)youtube"
        description: 在看视频
  - process: wechat.exe
    app: 微信
    description: 在聊天

expose_title: []              # 危险：列在这里的进程会把原始窗口标题原样公开
```

- `-config` flag 指定路径，默认取 **exe 同目录**的 `config.yaml`（`os.Executable()`），不是 CWD——双击启动和注册表自启的工作目录都不可靠。
- 校验（缺失即启动失败，fail loud）：`server_url` / `device_id` / `token` 非空，`interval > 0`，正则可编译。默认值：`interval=10s`、`idle_threshold=5m`、三组兜底文案如上。
- token 是密钥：日志/`-dry-run` 输出中**绝不**出现（spec logging）；README 提示给 config.yaml 收紧 ACL。

## 11. 入口与可观测性 `cmd/agent`

- flag：`-config <path>`、`-dry-run`（采集+映射一次，把**已脱敏**的 payload 打到 stdout 后退出，不发网络——这是 AC1 的取证手段）、`-v`（slog level Debug）。
- 日志：`slog` + **Text** handler → stdout（spec logging 允许客户端用 text）。Info 记启动摘要（server_url、interval、规则条数）、Warn 记上报失败与退避时长、Debug 记每轮**映射后**的 `{app, description}`。**永不**记 token、原始标题、完整 payload（Info 及以上）。
- `signal.NotifyContext(SIGINT, SIGTERM)` → ctx 取消 → 退出循环。
- 开机自启：README 说明注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 加一项指向 exe 绝对路径（含 `-config` 绝对路径）。不写自动安装代码——改注册表是用户该自己拍板的动作。

## 12. 取舍与回滚

- **不做托盘/GUI**：MVP 只要能跑；有 GUI 需求另开子任务。
- **不做规则热重载**：改规则重启 exe。热重载要处理文件监听 + 正则重编译失败的中间态，收益不抵复杂度。
- **不做本地日志文件**：stdout 即可，需要留存时用自启命令重定向（README 写法）。
- **不做多显示器/多窗口聚合**：只取前台窗口，符合"视奸"语义。
- 回滚点：`collect` / `mapping` / `report` 三层互不 import，任一层可单独回退重写；`mapping` 是隐私边界，改动它必须重跑其单测。
- 契约不变更：本任务**不改** `shared/`，因此不触发 cross-layer 同步（若实现中发现必须加字段，按 `.trellis/spec/guides/cross-layer-thinking-guide.md` 走，需同步后端与 `web/src/types/`，并回头改父任务 design.md §2）。

## 13. 下游影响

- `client-android` 将复用同一份 config 语义（`packageName → {app, description}` + 未命中通用化 + 白名单），本任务定下的三条隐私规则（§4）是它的模板。
- README 里的注册/部署流程是最终 VPS 上线文档的一部分。
