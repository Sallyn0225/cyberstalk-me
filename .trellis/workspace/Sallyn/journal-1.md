# Journal - Sallyn (Part 1)

> AI development session journal
> Started: 2026-07-28

---

## 2026-07-28 · backend 子任务实现完成

**任务**：`07-28-backend`（共享契约 + Go 后端）。父任务 `07-28-cyberstalk-me`。

**前期规划**：
- 补齐 backend 子任务的 `design.md` + `implement.md`（此前只有 prd.md + jsonl）。内容对齐父任务 design.md §1–§3/§6 与已 Filled 的 `.trellis/spec/backend/*`。
- `task.py start` 激活 backend，planning → in_progress。

**环境**：
- 机器原无 Go。用户授权安装且不要装 C 盘。下载 `go1.26.5.windows-amd64.zip` 解压到 `D:\go\go`，设置用户级 `GOROOT=D:\go\go`、`GOPATH=D:\go\gopath`、PATH 追加 `D:\go\go\bin`。`CGO_ENABLED` 默认 0，符合项目硬约束。
- git bash 当前会话 PATH 未含 go，用 `/d/go/go/bin` 临时追加或全路径调用；新进程/重启终端后直接可用。
- 注意：`Expand-Archive` 在本机 PowerShell 不可用（模块加载失败），用 .NET `ZipFileExtensions.ExtractToFile` 解压。

**实现**（trellis-implement 子代理，已亲验）：
- 阶段 0：`go.work` + `shared/`(contract.go 契约 struct) + `server/`(cmd/server/main.go + internal/{api,hub,state,store,config}) + `client-windows/`(占位 main) + `web/`(占位 index.html)。
- 阶段 1：store（两表 upsert + WAL + token SHA-256 哈希）、auth（Bearer 校验，不记 token/IP）、hub（SSE fan-out + mutex）、state（服务端时钟在线判定，`now func() time.Time` 可注入假时钟，离线广播）、api（report/snapshot/stream + 统一 writeError + chi 路由 + 静态托管 embed）、config、main 装配 + 优雅关停、`register-device` admin 子命令。
- 测试：httptest + `:memory:` SQLite，覆盖 AC6/AC3 后端部分/安全红线。

**亲验结果**：
- `gofmt -l` 无输出；三个 module `go vet ./...` 干净；`go test ./server/...` 全过（api/hub/state/store）。
- `CGO_ENABLED=0 GOOS=linux go build ./...` 三 module 全过。
- 实跑冒烟：无 token/错 token → 401；snapshot → 200（空数组）；静态 `/` → 200；`register-device` 打印一次性 token + config.yaml 片段。
- **未跑 `go test -race`**：本机无 gcc（race 需 CGO+C 编译器，与 `CGO_ENABLED=0` 约束冲突）。并发安全靠 hub mutex + 缓冲 channel，并有逻辑测试 `TestBroadcastConcurrent`/`TestBroadcastDropsSlowSubscriber` 兜底。后续在有 gcc 的 Linux 环境补 `-race`。

**遗留 / 注意**：
- config env 变量名为 `ADDR`/`SQLITE_PATH`/`OFFLINE_THRESHOLD`/`SCAN_INTERVAL`（design.md §3.7 未钉死具体名，功能等价，非 bug）。
- server 的 `go.mod` 用了 `replace cyberstalk.me/shared => ../shared`（Go 1.26 workspace 在本环境未自动解析本地 shared，replace 兜底）。
- `client-windows/` 仅占位；`web/` 占位（embed 指向 `server/cmd/server/web/index.html`）；`client-android/` 延后未建目录。
- 未 git commit（按规约等待 finish 阶段 3.4）。

**下一步**：进入 Trellis finish 阶段（spec 更新 3.3 → commit 3.4），或先跑 trellis-check 做完整质量验证。

---

## 2026-07-28 · backend check + gcc 工具链 + state 解耦

**check**：dispatch trellis-check 子代理对照 check.jsonl 6 个 spec 审查。安全红线/错误处理/目录结构/数据库/quality 主体合规，prd 5 AC 达标。修复 2 处：
- （重要）`state.Tracker.known` 共享 map 缺锁——HTTP 协程 `MarkOnline` 与后台 `scanOnce` 并发读写会 `concurrent map writes` fatal。加 `sync.Mutex`，广播移到锁外避免锁序死锁；新增 `TestTrackerConcurrentMarkOnlineAndScan` 运行时兜底。
- （轻微）`config.getEnvDuration` 对垃圾值静默回退默认，与「invalid returns error」契约不符。改 fail loud。

**state→store 解耦**（用户选重构）：`state` import `store` 违反 directory-structure「hub/state/store 互不 import，唯豁免 state→hub」。在 `state` 定义 `StateLister interface{ ListDeviceStates(ctx)([]shared.DeviceState,error) }`，`store` 新增 `ListDeviceStates` 方法，`*store.Store` 自动满足接口。`state.go` 不再 import store，main/test 无需改。

**gcc 工具链**：本机无 gcc 导致 `go test -race` 跑不了（race 需 cgo+C 编译器）。用户授权装 D 盘。下载 WinLibs x86_64 UCRT POSIX+SEH GCC 16.1.0 便携 7z（105MB）解压到 `D:\mingw\mingw64`，gcc 在 `D:\mingw\mingw64\bin`。持久化到用户 PATH（重启终端生效）。当前会话临时加载跑通 `CGO_ENABLED=1 go test -race ./...`，api/hub/state/store 全过、race 干净。

**最终门禁**：gofmt / vet / build / test / `CGO_ENABLED=0 GOOS=linux` build / `go test -race` 全绿。

**spec 更新**：`backend/quality-guidelines.md` Tooling 节补「Windows `-race` 需 mingw-w64 gcc（cgo），生产构建仍 `CGO_ENABLED=0`」。

**遗留**：check 轻微观察未做（store_test panic→t.Fatal、单函数文件内联、register-device 补测试、web embed 路径约定属 web 子任务）。未 commit。

---

## 2026-07-28 · backend 轻微观察清理

处理 check 遗留的 3 项轻微观察（web embed 路径属 web 子任务，404 不可达可接受，不动）：
- register-device：新增 `cmd/server/main_test.go`，覆盖正常路径（stdout 一次性 token + config 片段、设备落库 + sha256 哈希、明文 ≠ 哈希且 HashToken(明文)=哈希）+ 参数校验错误路径。
- 单函数文件：内联 `store/json.go`(decodeJSON) 回 `store.go`、`store/json_test.go`+`state/json_test.go`(jsonMarshal helpers) 回各自 `_test.go`，删 3 个单函数文件（spec：不为单函数建文件）。
- panic：`store_test` 的 jsonMarshal2 panic → 合并的 `jsonMarshal(t,p)` 用 `t.Fatal`（spec：main.go 外无 panic）。

全门禁绿：gofmt / vet / build / test / `-race` / `CGO_ENABLED=0 GOOS=linux` build。已 commit。

---

## 2026-07-29 · web 子任务（前端展示网站）

**规划**：`07-28-web` 原本只有 prd + 两个 jsonl，补齐 `design.md` / `implement.md` 后 `task.py start`。design 里把后端**实测行为**钉死（不是照抄设计稿）：snapshot 返回**裸数组**非包装对象；SSE 首帧是**命名事件** `event: ready`（不进 `onmessage`，所以连接就绪只能靠 `onopen`）。

**用户决策**：构建产物落地方式选「vite build 直接输出到 `server/cmd/server/web/` 并把产物提交进 git」——任何 commit 上 `go build` 都能出带最新前端的单二进制，代价是前端改动必须连产物一起提交。主题用户从 tweakcn 挑的（mint/emerald + 纯黑深色）。

**契约漂移（重要）**：spec `type-safety.md` 的示例与真实 Go 契约不符 2 处，以 Go 为准并已改 spec——`activity` 是 Go 值类型**永不为 null**（示例写成了 `Activity | null`）；`Battery.Level` 是 `*int` **可为 null**（示例写成 `number`）。另定规矩：解析守卫只校验结构、**不校验字符串联合取值**（后端对 `network` 根本没做校验），未知取值降级展示而不是丢设备。

**工具链现实（与 spec 原文不符，已同步 spec）**：
- create-vite 9.1.1 现在生成 **oxlint 不是 ESLint**。实测 oxlint 已实现 `react-hooks(exhaustive-deps)`（默认 warn），保留 oxlint 并提为 error + 开 jsx-a11y；`components/ui/**` 用 overrides 关 `only-export-components`。
- **TS 6 弃用 `baseUrl`**（直接 TS5101 报错），只留 `paths`；且新模板**不再默认开 `strict`**，需手动加进 `tsconfig.app.json`。
- Vite dev server 默认只绑 `[::1]`，本机 IPv6 回环连不通 → 显式 `server.host='127.0.0.1'`。
- 字体：tweakcn 主题要 Plus Jakarta Sans，改 `@fontsource-variable/*` 自托管（公网站点不依赖 Google Fonts，国内可访问 + 单二进制离线可用），卸载 preset 带的未用 Geist。

**亲验结果**（起真后端 + 两台模拟设备 curl 上报 + Playwright 实开页面）：
- AC4 无认证直开见卡片；AC5 改 activity 页面不刷新自动更新（VS Code→Chrome 实截图确认）。
- AC3 停止上报超阈值 → 卡片置灰 + 文字「离线」+「最后活跃 X 前」。
- **重连校正实测**：杀后端 → 页面进「重连中」→ 重启后端并在断线期间补一次上报 → 前端重连后拿到断线期间的状态。网络面板确认重连后**多打了一次 `/api/v1/snapshot`**，证明是 snapshot 校正而非 SSE 补发。
- `battery: null` 台式机整块不渲染；`battery.level: null` 只显示充电态。控制台除故意杀服务器期间的网络重试外无任何应用级报错。
- E.6：跑 `go build` 出的二进制访问 `:8080`，页面就是新前端（embed 生效）。

**坑**：Windows 下 `curl -d '中文'` 会被控制台代码页转码搞坏（后端收到乱码），必须 `--data-binary @file`。已写进 `web/README.md`。

**遗留观察**：后端 SSE 无心跳帧。前端能优雅处理（重连+校正），但线上反代若有 idle read timeout 会规律性重连；要根治属后端加 keepalive 注释帧，不在本子任务范围。

---

## 2026-07-29 · client-windows 子任务实现（report + main 装配）

**任务**：`07-28-client-windows`（Windows 上报客户端）。父任务 `07-28-cyberstalk-me`。本会话接续已完成的阶段 A（config）/B（mapping）/C（collect），补齐缺失的阶段 D（report）+ E（main 装配 / README / spec 同步）。

**模型插曲**：派发的 trellis-implement 子代理因 `claude-opus-5[1m]` 不可用而失败（无产出），改为本会话直接实现。

**实现**：
- `internal/report/client.go`：`Client` + `Send`——204=成功、400/401=`ErrPermanent`、5xx/网络/超时=可重试；token 只在 Authorization 头，错误信息只含状态码/URL，无 token。
- `internal/report/loop.go`：`Loop.Run`——首次立即上报；失败退避 interval→2×→4×→…→cap 2min，成功复位 interval，永久错误直跳 cap；失败轮丢弃不补发；ctx 可打断退避。`Next` 回调由 main 装配 collect+mapping，**report 包只收 `shared.ReportPayload`，不 import collect/mapping/config，全平台可测**。
- `cmd/agent/main.go`：替换占位——flag（`-config`/`-dry-run`/`-v`）+ slog Text→stdout + `signal.NotifyContext` 优雅退出；dry-run 只打脱敏 payload JSON（启动日志移到 dry-run 分支之后，保证 stdout 纯 JSON 可 pipe）；启动 Info（server_url/interval/规则数/expose_title 数）+ 非空 expose_title Warn。
- `README.md`（E.4）、spec 同步（E.5：directory-structure 补 `internal/config` + client-windows 包隔离说明；quality-guidelines 注明根 `./...` 不可用须显式列 module、`GOOS=linux` 门禁不含 client-windows）、`.gitignore` 加 `config*.yaml`（E.6，防真 token 泄漏）。
- A.1 修正：`go mod tidy` 把 `golang.org/x/sys` 补为 client-windows 直接依赖（之前靠 workspace 从 server 经 modernc 传递解析，脱离 workspace 单编会失败）。
- 小 refinement：loop 的 Warn 错误值键名 `reason`→`err`，与 spec logging「err (use "err", err)」及 server 端一致。

**门禁**（全绿）：gofmt / vet / test / `CGO_ENABLED=1 -race`（config/mapping/report 全过，race 干净）/ `CGO_ENABLED=0` build / `GOOS=linux` build（server+shared）。注意 collect_windows.go 的 const 块列对齐被 gofmt 修过（既有文件，纯格式）。

**真机 e2e**（本机 Windows，起真后端 + `register-device` + 跑 agent.exe）：
- F.2 隐私 canary 四处：dry-run 输出无 canary 无 title 字段；`-v` 日志无 canary 无 token；snapshot 无标题；DB `last_report_json` 无 title 键。前台未命中规则 → 通用「某个应用·使用中」（F.3：不显示 exe 名）。
- F.6 韧性：杀后端 → 退避 1s→2s→4s（无崩溃、warn err 无 token）→ 重启后端 → 自动续报 `report accepted`、snapshot online。
- F.7 坏 token：401 → `ErrPermanent` → 退避直跳 2m0s、4s 内仅 1 条 warn（不刷屏）、不崩溃、无 token/Bearer。
- F.8 单 exe：拷到临时目录、不带 `-config`，`DefaultPath` 正确解析同目录 config.yaml。
- battery=null（台式机）、network=wifi、reported_at 带 Z（UTC）均符合契约。

**遗留（交互项，待真机桌面）**：F.1 切到 VS Code/Chrome/微信看卡片变对应应用名（逻辑已由 mapping_test 覆盖，数据通路已验证）；F.4 静置 5min 看空闲标记（`TestResolveIdle` 覆盖阈值边界）；F.5 拔网线/切 wifi 看网络字段变化（network 采集已验证 wifi）。

**注意（操作）**：git bash 的 `$!` 是 MSYS PID，`taskkill //PID` 找不到；按映像名 `taskkill //IM agent.exe //F` 才可靠。Windows 控制台代码页会搞坏中文（python 打印 DB 内容乱码，但 curl snapshot 正常 UTF-8；校验用字节匹配）。
