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