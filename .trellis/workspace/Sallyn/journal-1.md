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

---

## 2026-07-29 · client-windows 新环境重装工具链 + 门禁复验 + e2e 复跑

**背景**：本会话开始时本机**无任何 Go/gcc 工具链**（环境重置，上次 journal 记的 `D:\go`、`D:\mingw` 均已不存在）。用户授权装 D 盘、推荐 `D:\Program Files`。任务代码此前已完成，目标是重建环境、复验全门禁、复跑 e2e 非交互项。

**Go 安装**：`go1.26.5.windows-amd64.zip`（go.dev/dl，74MB）→ 7-Zip 解压 → `D:\Program Files\Go`。用户级环境变量 `GOROOT=D:\Program Files\Go`、`GOPATH=D:\gopath`、`GOPROXY=https://goproxy.cn,direct`（国内拉 modernc/x-sys 依赖必须，默认 proxy.golang.org 太慢），PATH 幂等追加 `Go\bin` + `gopath\bin`。**Go 本身能处理带空格的安装路径**。
- git bash 当前会话 PATH 用 **POSIX 形式** `/d/Program Files/Go/bin`（`D:/...` 混合路径 git bash 的 PATH 查找不认，但直接全路径执行 `.exe` 可以）。

**mingw（-race 需要 cgo）**：WinLibs GCC 16.1.0 UCRT POSIX+SEH r3（github brechtsanders/winlibs_mingw，109MB 7z）。
- **关键坑**：初装到 `D:\Program Files\mingw64`，`go test -race` 链接失败——`ld.exe: cannot find D:/Program: No such file or directory`。**mingw 的 ld 无法处理带空格的安装路径**（default-manifest.o 等库搜索路径按空格被拆断）。移到无空格的 `D:\mingw64` 后 race 全过。**结论：Go 可装带空格路径，mingw 必须无空格路径。**

**行尾根治（gofmt 门禁阻塞）**：本机 `core.autocrlf=true`，新 checkout 把所有 `.go` 转成 CRLF，`gofmt -l` 因此把**每个文件**都标为需格式化（仓库 blob 其实是 LF，`git ls-files --eol` 证实 `i/lf w/crlf`）。这是标准 Windows git 配置下的通病，非代码问题。
- 修复：`.gitattributes` 顶部加 `* text=auto eol=lf`（通用规则放最前，下方已有的 `web/assets/** -text` 特例因"后者覆盖前者"仍生效）。再 `git config core.autocrlf false`（仅本仓库）+ `git ls-files -z | xargs -0 rm -f && git checkout-index -f -a` 重刷工作区为 LF。
- **副作用与消解**：rm+checkout 改了所有文件 mtime → `git status` 一度把上百文件标 ` M`，但 `git diff --numstat`/`git diff HEAD --stat` 证明**唯一真实内容变更只有 `.gitattributes`（+8 行）**，其余是 stat 缓存脏。`git add -u`（不碰未追踪的 `00-join-sallyn/`）写回 stat 后 status 干净。
- **唯一改动的追踪文件：`.gitattributes`。** 这是让 gofmt 门禁跨平台/跨 autocrlf 稳定的基础设施修复。

**代码门禁全绿**（本机实测）：`gofmt -l` 无输出 / `go vet` 三 module / `go test` 全 ok / `CGO_ENABLED=1 go test -race ./client-windows/...`（config/mapping/report 全过 race 干净）/ `CGO_ENABLED=0 go build` 三 module / `CGO_ENABLED=0 GOOS=linux go build`（server+shared）。

**e2e 非交互项复跑**（临时 `D:\tmp-godl\e2e`，测试 token，跑完已连同 db/config 一并删除）：
- 这台机器**有电池**（`battery.level=93 charging=true`），与上次那台台式机（battery=null）不同——契约两种都正确渲染。`network=wifi`，`reported_at` 带 Z（UTC）。
- **F.2 隐私红线四处全过**：`-dry-run` payload、`-v` 日志、`/api/v1/snapshot`、DB `last_report_json`（python 查 activity 键只有 `app/description/idle/idle_seconds`）均无 title 字段、无 token。前台是终端（未命中规则）→ `某个应用 · 使用中`（F.3：不暴露 exe 名）。
- **F.6 韧性**：杀 server → 不崩、退避 `3s→6s→12s→24s→48s`（interval 起指数 ×2，err 只含 URL 无 token）→ 重启 server → 退避到期自动续报 `report accepted`、snapshot `online:true`、退避复位回 3s。
- **F.7 坏 token**：401 → `permanent report error: ... server status 401` → 退避直跳 `2m0s`、仅 1 条 WARN（不刷屏）、不崩、无 token。
- **F.8 单 exe**：agent.exe + config.yaml 拷到异目录，从任意 CWD 不带 `-config` 跑 → 正确解析 exe 同目录 config（`os.Executable()` 同目录，非 CWD）。

**遗留（交互项，仍待真机桌面人工确认）**：F.1 切前台 app 看卡片跟随、F.4 静置 5min 看空闲标记、F.5 拔网线/切 wifi 看网络字段——逻辑均有单测覆盖（mapping 规则命中、idle 阈值边界、network wifi 采集），数据通路本会话已端到端验证。未 commit（`.gitattributes` 改动待用户决定是否随任务一起提交）。


## Session 1: 后端 Docker 化与 CI/CD 交付

**Date**: 2026-07-30
**Task**: 后端 Docker 化与 CI/CD 交付
**Branch**: `main`

### Summary

把 server 封装成多架构容器镜像并配好 GitHub Actions 流水线，部署方式从「自己编译裸二进制」变为「clone + docker compose up -d」。AC1-AC9 全部通过，其中运行时链路在自有 VPS 上实测、多架构构建由 CD 验证。纯增量，未改任何业务代码。

### Main Changes

- Dockerfile 两阶段：build 阶段钉 $BUILDPLATFORM 靠 Go 交叉编译够到目标架构，runtime 阶段零 RUN —— 因此多架构构建完全不需要 QEMU（实测 amd64+arm64 一次 2m57s）。镜像 29.2MB，非 root uid 65532
- compose.yaml 让 image 与 build 并存：up -d 拉 GHCR 预构建镜像，build 则本地构建。所有变量带内联默认值，无 .env 也能起。刻意不写死资源限制——那属于部署者的机器
- CI 三个 job（go/web/docker）复用 .trellis/spec 里已有的门禁，没另起一套标准。唯一新增的门禁是内嵌产物新鲜度检查：CI 重建前端并 diff server/cmd/server/web，堵住「改了前端忘记 build」导致静默发布旧界面
- CD 推 GHCR：v* 出 semver+latest，main 出 edge，都带 GHA 缓存与最小权限。后加 paths-ignore 让纯文档提交不再触发无意义的 edge 重建（tag 推送不受路径过滤影响，已查文档确认）
- 新增 .trellis/spec/backend/deployment-guidelines.md，把「运行阶段零 RUN」这条代码里看不出来的承重约束固化下来

### Git Commits

| Hash | Message |
|------|---------|
| `407231e` | (see git log) |
| `5acbf47` | (see git log) |
| `46f5e42` | (see git log) |
| `f5307e7` | (see git log) |
| `5c4be2b` | (see git log) |
| `0d4831e` | (see git log) |
| `6b19d14` | (see git log) |

### Testing

- [OK] VPS 实测 AC1/2/3/5：页面 200、snapshot 合法 JSON、SSE 带 X-Accel-Buffering: no、register-device 在容器内注册成功、token 上报 204 而错误/缺失 token 401、down 再 up 后 token 仍有效、容器内 uid=65532 且无 Go 工具链无源码
- [OK] AC6 正反都验：三处人为缺陷分别让 go/gofmt、web/typecheck、web/embed-freshness 各自变红，无关 job 保持绿
- [OK] AC7 按 digest 拉 arm64 镜像，其二进制经 file 确认为 ELF ARM aarch64 statically linked stripped —— 顺带旁证 CGO_ENABLED=0 与 -s -w 生效
- [OK] AC8 仓库转公开后从真正冷启动重跑：无本地镜像 → 匿名 clone → compose 自行拉 latest → 公网访客看到设备卡片，全程只用 README 里的命令
- [OK] AC9 三次邻居体检：VPS 上三个既有生产服务 uptime 全程未断，容器/卷/镜像/builder 零残留，8080 已释放，全程未执行任何 prune

### Status

[OK] **Completed**

### Next Steps

- 只剩 Android 客户端子任务（07-28-client-android，仍在 planning），父任务 4/5
- vps.md 里那个弱密码仍开着 SSH 密码登录且已出现在会话记录中，建议轮换并关掉 PasswordAuthentication
- 验证期发现 docker pull --platform 在本地已有同 tag 时不重新拉、inspect 仍报旧架构；验架构须按 digest 拉。已写进 spec
