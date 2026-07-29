# Implement — Windows 上报客户端（Go 单 exe）

> 复杂任务。对应父任务 `.trellis/tasks/07-28-cyberstalk-me/implement.md` 阶段 3，技术方案见本任务 `design.md`。阶段间有 review gate；**阶段 B（映射）是隐私边界，必须先于阶段 C/D 定稿并测透**。

## 前置事实（开工前确认，勿重复踩坑）

- 基线已绿：`gofmt` 无输出、`go vet` / `go test ./server/...` 全过（2026-07-29 实测）。
- **根目录 `go build ./...` 在本机不可用**（报 "directory prefix . does not contain modules listed in go.work"），一律显式列 module 路径。
- `go test -race` 需 `CGO_ENABLED=1` + mingw-w64（`D:\mingw\mingw64\bin`，已装）。
- Windows 下 `curl -d '中文'` 会被控制台代码页搞坏编码，冒烟必须 `--data-binary @file`（journal 2026-07-29）。

## 阶段 A · 骨架、依赖与配置

- [ ] A.1 `client-windows/go.mod`：加 `cyberstalk.me/shared v0.0.0` + `replace ... => ../shared`、`gopkg.in/yaml.v3`、`golang.org/x/sys`（三者均在 spec 批准清单内）。
- [ ] A.2 `internal/config/config.go`：`Config` struct + `Load(path string) (*Config, error)`。键名与 `register-device` 输出逐字对齐（`server_url`/`device_id`/`token`/`interval`），其余键与默认值见 `design.md` §10。
- [ ] A.3 校验 fail loud：`server_url`/`device_id`/`token` 空、`interval <= 0`、正则编译失败 → 返回带上下文的 error（`fmt.Errorf("...: %w", err)`），**不静默回退默认**（backend 子任务踩过这个，见 journal 2026-07-28）。
- [ ] A.4 默认路径解析：`-config` 未给时取 `os.Executable()` 同目录的 `config.yaml`，不用 CWD。
- [ ] A.5 `config.example.yaml`：可直接改名使用，含中文规则示例 + `expose_title` 的风险注释。
- [ ] A.6 `internal/config/config_test.go`：表驱动覆盖 —— 完整配置解析、默认值填充、必填缺失报错、坏正则报错、`interval` 时长解析。

**验证**：`go build ./client-windows/... && go test ./client-windows/...`

- [ ] **Review gate A**：config 包无 Win32 依赖（可在任意平台编译测试）；token 不出现在任何 String()/日志路径。

## 阶段 B · 映射层（隐私边界 · 必测）

- [ ] B.1 `internal/mapping/mapping.go`：`New(cfg)` 预编译正则建 `map[string]Rule`（key 为小写 exe 名）；`Resolve(process string, title func() string, idleSeconds int) shared.Activity`。
- [ ] B.2 实现 `design.md` §7 的 6 条判定顺序，重点：
  - 未命中 → `default_app` / `default_description`，**且不调用 `title()`**（用桩函数断言"没被调用"）。
  - **禁止**任何"从 exe 名派生 app 名"的兜底逻辑。
  - 命中 + `title_patterns` → 按序首个匹配覆盖 description。
  - 命中 + `expose_title` 白名单 → description 用原始标题（唯一豁免）。
  - 前台不可用 → `locked_app` / `locked_description`。
  - `Idle = idleSeconds >= idle_threshold`；空闲不隐藏 App/Description。
- [ ] B.3 `mapping_test.go` 表驱动（spec quality 明确要求本包必测）：
  - 命中规则 → 映射结果正确；进程名**大小写不敏感**（`Code.exe` / `code.EXE`）。
  - **未知进程 → 通用描述，且 `title()` 未被调用**（隐私边界的核心用例）。
  - `title_patterns` 命中 / 全不中回落规则默认。
  - `expose_title` 白名单生效。
  - 锁屏态、idle 阈值边界（`==` 阈值判空闲）。
  - 反向断言：遍历所有用例的输出结构体，断言**不包含**桩标题字符串（防止将来加逻辑时漏出）。

**验证**：`go test ./client-windows/internal/mapping/... -v`

- [ ] **Review gate B（隐私红线）**：mapping 输出的 `shared.Activity` 在未白名单场景下永不含原始标题；`title` 是懒回调而非预取的字符串参数（接口层面就堵死误用）。

## 阶段 C · 采集层（Win32 · 豁免单测）

全部文件 `//go:build windows`，文件名 `*_windows.go`。优先用 `golang.org/x/sys/windows` 已导出的封装，缺失的用 `windows.NewLazySystemDLL(...).NewProc(...)`——**实现时逐个确认**，不预设某个符号一定存在。

- [ ] C.1 `collect/collect_windows.go`：`Snapshot` 结构（前台 exe 名、`rawTitle func() string` 懒取回调、idle 秒、`*shared.Battery`、network 字符串）。**原始标题字段不导出、无 json tag。**
- [ ] C.2 前台窗口：`GetForegroundWindow` → `GetWindowThreadProcessId` → `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` → `QueryFullProcessImageName` → `filepath.Base` 小写。hwnd=0 → 标记"无前台"；`OpenProcess` Access Denied（提权进程）→ 降级为未知进程，不返回致命错误。
- [ ] C.3 标题懒取：`GetWindowTextW` 封装成闭包，只有 mapping 调用才触发。
- [ ] C.4 空闲：`GetLastInputInfo` + `GetTickCount64`，**用 `uint32(GetTickCount64()) - dwTime` 做无符号回绕减法**（32 位 tick，开机 49.7 天后会绕回）。
- [ ] C.5 电池：`GetSystemPowerStatus` → `BatteryFlag&128`（无电池）或 `BatteryLifePercent==255`（未知）→ `nil`；`ACLineStatus==1` → `Charging=true`。
- [ ] C.6 网络：`GetAdaptersAddresses(AF_UNSPEC, GAA_FLAG_INCLUDE_GATEWAYS)` → up 且有默认网关的网卡按 `IfType` 映射 wifi/ethernet/cellular，跳 loopback，**有线优先**，全无 → `"offline"`。
- [ ] C.7 统一降级：单个 Win32 调用失败 → 该字段零值/nil + Debug 日志，**照常上报**（spec error-handling §Client Errors）。

**验证**（无单测，靠真机手动）：`go build ./client-windows/...`，然后阶段 E 的 `-dry-run` 实跑观察。

- [ ] **Review gate C**：collect 包不 import mapping/report/config；原始标题不以字符串字段形式跨包传递。

## 阶段 D · 上报层

- [ ] D.1 `report/client.go`：`Client{httpClient, url, token}`，复用 `*http.Client`（`Timeout: 10s`）；`Send(ctx, payload)` → POST `{server_url}/api/v1/report`，`Authorization: Bearer`、`Content-Type: application/json`。
- [ ] D.2 响应判定：**204 = 成功**；`400`/`401` → `ErrPermanent` sentinel；`5xx`/网络错误/超时 → 可重试错误。错误一律 `fmt.Errorf` 带上下文，**由调用方（loop）记一次日志**，下层不记。
- [ ] D.3 `report/loop.go`：ticker(interval) → 采集回调 → 映射 → 组装 `shared.ReportPayload`（`device_type` 硬编码 `"windows"`，`reported_at` = `now().UTC()`）→ Send。失败：Warn（带 `reason` 与下次退避时长，**不带 token/标题**）+ 指数退避 ×2，**上限 2 分钟**；成功即复位到 interval。`ErrPermanent` 直接跳到最大退避。
- [ ] D.4 退避等待必须可被 ctx 打断（`select { case <-ctx.Done(): case <-timer.C: }`）。
- [ ] D.5 失败轮次**丢弃不补发**（只报最新状态，补发会污染 `last_seen`）。
- [ ] D.6 `report/client_test.go` + `loop_test.go`（`httptest`，注入假时钟/假 sleep）：
  - 204 → 成功；401 → `ErrPermanent`；500 → 可重试；网络不可达 → 可重试且不 panic。
  - 请求头带 Bearer、body 是合法契约 JSON、`device_type == "windows"`。
  - 退避序列正确（interval → 2× → 4× → … → cap 2min），成功后复位。
  - ctx 取消时退避中能立即返回。

**验证**：`go test ./client-windows/... && CGO_ENABLED=1 go test -race ./client-windows/...`

- [ ] **Review gate D**：断网/服务端 500/坏 token 三种场景均不崩溃、不无限刷日志（PRD AC3）。

## 阶段 E · 装配与交付物

- [ ] E.1 `cmd/agent/main.go` 替换占位实现：flag（`-config` / `-dry-run` / `-v`）→ `slog` Text handler + `SetDefault` → 载入 config → 装配 collect/mapping/report → `signal.NotifyContext` → 跑 loop → 优雅退出。`panic`/`log.Fatal` 只允许出现在这里（spec quality §Forbidden Patterns）。
- [ ] E.2 `-dry-run`：采集+映射一轮，把**已脱敏的 payload** JSON 打到 stdout 后退出，不发网络。这是 AC1 的取证手段。
- [ ] E.3 启动日志（Info）：server_url、interval、规则条数、`expose_title` 条数（**非空时额外 Warn 提示会公开原始标题**）。
- [ ] E.4 `client-windows/README.md`：注册设备（`server register-device <id> <name> windows`）→ 粘贴 config 片段 → 补规则 → `go build -o agent.exe ./cmd/agent` → 注册表 `HKCU\...\CurrentVersion\Run` 自启（含 `-config` 绝对路径）→ 隐私说明（默认不取标题、`expose_title` 的风险、config.yaml 含 token 需收紧 ACL）。
- [ ] E.5 spec 同步（spec 自述"real layout diverges 就更新"）：
  - `.trellis/spec/backend/directory-structure.md`：client-windows 子树补 `internal/config/`。
  - `.trellis/spec/backend/quality-guidelines.md`：Tooling 节注明 **`GOOS=linux` 交叉编译门禁只覆盖 `./server/... ./shared/...`**（client-windows 是 windows-only，构建约束排除全部文件），以及根目录 `./...` 在本 workspace 不可用、须显式列 module。
- [ ] E.6 确认 `.gitignore` 已覆盖 `agent.exe` / `/client-windows/agent`（已有，勿提交二进制）；`config.yaml`（含真 token）**必须加进 `.gitignore`**，只提交 `config.example.yaml`。

## 阶段 F · 门禁与验收

**代码门禁**（全绿才算完）：
```bash
gofmt -l shared server client-windows            # 无输出
go vet ./server/... ./shared/... ./client-windows/...
go test ./server/... ./shared/... ./client-windows/...
CGO_ENABLED=1 go test -race ./client-windows/... # 需 D:\mingw\mingw64\bin 在 PATH
CGO_ENABLED=0 go build ./server/... ./shared/... ./client-windows/...
CGO_ENABLED=0 GOOS=linux go build ./server/... ./shared/...   # 部署门禁，不含 client-windows
```

**真机端到端**（父任务 AC1）：
```bash
# 1) 起后端
go run ./server/cmd/server
# 2) 注册本机设备，拿一次性 token
go run ./server/cmd/server register-device win-desktop 我的台式机 windows
# 3) 填 config.yaml，先 dry-run 看脱敏结果
go run ./client-windows/cmd/agent -config ./client-windows/config.yaml -dry-run
# 4) 常驻跑，打开 http://localhost:8080 看卡片
go run ./client-windows/cmd/agent -config ./client-windows/config.yaml -v
```

- [ ] F.1 AC1：切到 VS Code / Chrome / 微信，页面卡片在 `interval + 阈值` 内跟着变成对应「应用名 · 活动描述」。
- [ ] F.2 AC1 隐私红线四处抽查（**逐一留证据**）：
  - `-dry-run` 输出无原始标题（故意把窗口标题改成 `SECRET-TITLE-CANARY` 再验一次）。
  - agent stdout 日志（含 `-v` Debug）无原始标题、无 token。
  - 页面渲染文本无原始标题。
  - 数据库 `device_state.last_report_json` 无原始标题（`sqlite` 查一次）。
- [ ] F.3 未命中规则的进程（随便开个没写规则的程序）→ 页面显示「某个应用 · 使用中」，**不显示 exe 名**。
- [ ] F.4 空闲：静置超过 `idle_threshold` → 卡片出现空闲标记。
- [ ] F.5 电池/网络：笔记本显示电量与充电态、台式机 `battery` 块不渲染；拔网线/切 wifi → 网络字段跟随变化。
- [ ] F.6 韧性（PRD AC3）：杀掉后端 → agent 不崩、Warn 退避日志间隔递增至 2 分钟封顶 → 重启后端 → **自动续报**，页面恢复在线。
- [ ] F.7 坏 token：把 config 里 token 改错 → 401 → agent 不崩、日志说明原因且不刷屏。
- [ ] F.8 单 exe 交付：`go build -o agent.exe ./cmd/agent` 产物在**另一个目录**双击/命令行可跑（验证 exe 同目录 config 解析）。

- [ ] **Review gate F（对照本任务 prd.md AC）**：
  - build / vet / test / -race 全过（Win32 采集层按 spec 豁免单测）。
  - 真机页面显示「应用名 · 活动描述」，四处抽查均无原始标题（AC1）。
  - 断网/后端不可用不崩溃，恢复后自动续报。

## 风险 / 回滚点

- **隐私红线是本任务的存在意义**：F.2 的四处抽查必须真跑，不能"看代码觉得没问题"。canary 标题法是最省事的取证方式。
- **Win32 封装的不确定性**：`golang.org/x/sys/windows` 各版本导出的符号不同，C.2/C.5/C.6 可能需要退回 `LazyDLL` 手写声明。这是**实现细节层面的返工**，不影响 mapping/report 的边界，回滚只动 `collect`。
- **`GetLastInputInfo` 的 32 位回绕**是最容易漏的静默 bug（开机 49 天后才发作），C.4 必须按无符号减法写。
- **提权进程采不到**：以管理员身份运行的窗口在非管理员 agent 下 `OpenProcess` 会失败 → 显示为通用描述。这是可接受降级，不为此要求 agent 提权（提权常驻是更大的安全面）。
- **不改 `shared/`**：本任务不触发 cross-layer 同步。若实现中发现必须加字段，停下来走 `.trellis/spec/guides/cross-layer-thinking-guide.md`，同步后端 + `web/src/types/` + 父任务 design.md §2 后再继续。
- 回滚粒度：阶段 B/C/D 三层互不 import，任一层可独立重写；阶段 E 的 spec 更新与代码解耦，可单独回退。
