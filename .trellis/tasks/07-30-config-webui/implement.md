# Implement — `agent.exe -setup` 规则可视化配置 WebUI

> 需求见 `prd.md`，技术方案见 `design.md`。
> 分阶段推进，每阶段末尾都有可验证的门禁；未过门禁不进下一阶段。

## 阶段 0 · 地基改造（不引入新功能）

- [x] 0.1 `config.validate` → 导出 `Validate(source string) error`，新增结构化错误类型（字段路径 + 消息）。
      `Load` 内部改为调用它并传文件路径；**对外错误文案保持不变**。
      落地：`mapping/errors.go` 的 `RuleError`（带 `FieldPath()`）+ `config/errors.go` 的 `ValidationError`
      （`Source` / `Fields` / `Message`），`Load` 的解析错误**不**包成 `ValidationError`（没有字段可指）。
- [x] 0.2 把 `main.go` 里 `collect.Collect() → mapper.Resolve(...)` 那段抽成可复用函数
      （供常驻 / `-dry-run` / `-setup` 预览三处共用），`-dry-run` 输出必须逐字节不变。
      落地：`cmd/agent/main.go` 的 `resolve(m, snap)`。
- [x] 0.3 补 `config_test.go`：确认 `Load` 的既有错误文案未回归。
      落地：`config/errors_test.go` + `mapping/errors_test.go`，8 + 5 条**逐字**文案断言（原测试只做 Contains）。

**门禁**：`gofmt` / `go vet ./client-windows/...` / `go test ./client-windows/...`（Windows 上跑）
/ `GOOS=windows go build ./client-windows/...` 全过；`-dry-run` 输出与改造前一致。

> ✅ 2026-07-30 通过。`-dry-run` 一致性用 `git archive HEAD` 构建改造前的 exe，
> 与新 exe 跑同一份 config 对比输出（除 `reported_at` 外逐字节相同）。

## 阶段 1 · YAML 写回（纯 Go，先于任何 UI）

- [x] 1.1 `internal/config/save.go`：`Config` → `*yaml.Node` 树，节点带 `HeadComment`，
      注释文案沿用 `config.example.yaml`（`expose_title` 的 DANGER 块原样保留）。
      `Save` 先跑 `Validate` 再写盘；`Marshal` 导出，便于后续做「将写入什么」的预览。
- [x] 1.2 `replaceFile(dst, tmp, bak string) error` 抽象；
      `save_windows.go` 用 `ReplaceFileW`（`NewLazySystemDLL` 声明，照 `collect_windows.go:50` 的模式），
      非 Windows 侧给 `os.Rename` 实现仅供编译。
- [x] 1.3 目标文件不存在时走 `os.Rename` 分支（首次创建，无 ACL 可保留、无备份可做）。
- [x] 1.4 **round-trip 单测（AC5）**：构造覆盖全部键 + 多条 `title_patterns` 的 `Config`
      → `Save` → `Load` → 逐字段比对；含中文、含正则元字符、含 `:` 的值。
      另加 12 组 YAML 陷阱值（全数字 token、`true`/`null`/`~`、`#` 开头、`1:30`、emoji、反斜杠）。
- [x] 1.5 备份单测：已有文件时 `.bak` 内容等于原文件；连存两次时 `.bak` 覆盖为上一版。

**门禁**：阶段 0 的全套 + 新增单测过；在真实 Windows 上手工验一次 ACL 未被放宽
（属性 → 安全，保存前后对比）。

> ✅ 2026-07-30 通过。ACL 用 `icacls` 验：把 `config.yaml` 收紧成
> `ARISA\z1921:(F)`（`/inheritance:r`）后跑 `Save`，ACL 原样保留，`.bak` 同样保持收紧。
> 反例同时确认：普通 `mv` 覆盖会让该文件继承目录 ACL（多出数个 SID 的 `(M,DC)` 权限），
> 即 design §6.2 预测的静默安全回归——`ReplaceFileW` 不是可选项。
> 纯包（`config` / `mapping`）已确认能在 `GOOS=linux` 下编译，为 6.2 的 CI 覆盖留好了路。

⚠️ **风险点**：`ReplaceFileW` 是本任务最容易搞砸配置文件的地方。先在临时目录里验证，
不要拿真实 `config.yaml` 当第一个试验品。

## 阶段 2 · catalog 与建议正则（纯 Go）

- [x] 2.1 `internal/setup/catalog.go`：按 exe 聚合、标题去重、记录首次/最近出现时间与次数、
      样本数上限 + 最旧淘汰（R2.5）。并发安全。另加应用数上限（默认 256，淘汰最久未见），
      防止长时间 `-setup` 会话内存无界。
- [x] 2.2 `?unknown` 与锁屏状态在 catalog 里单独标记，不可被当成普通应用（R2.4）。
      判据不是硬编码 `"?unknown"`（那要 import `collect`），而是「进程名含 Windows 文件名非法字符」
      —— 与 `collect` 选这个占位符的理由同源。
- [x] 2.3 `internal/setup/suggest.go`：`regexp.QuoteMeta` + `(?i)` 生成建议正则；
      给定 pattern 计算在样本集中的命中列表。
- [x] 2.4 两者的单测（含 Unicode 标题、含正则元字符的标题、上限淘汰行为）。

**门禁**：单测过；`go vet` 干净。

> ✅ 2026-07-30 通过，另跑了 `CGO_ENABLED=1 go test -race`（catalog 有 8 goroutine 并发读写用例）。
>
> ⚠️ **对 design §3 的偏离**：`setup` 包做成了**完全平台无关、无 build tag**。
> 原计划的 `observe_windows.go` 不放在 `setup` 里，改由 `cmd/agent` 注入一个前台读取函数。
> 理由是 design §3 自己的目标——「让 handlers 与 catalog 能在 Linux CI 上被覆盖」——这样能 100% 达成，
> 且 `setup` 一行 Win32 都不碰。design.md 待同步。

## 阶段 3 · HTTP 服务与守卫（纯 Go，`httptest` 可测）

- [x] 3.1 `internal/setup/server.go`：`Source` 注入（取代 design 的 `Observer` 接口）+ draft 状态机
      （`sync.Mutex`，draft 变更时重编译并缓存 `*mapping.Mapper`）。
      半成品正则**不**替换已缓存的 mapper——用户打字打一半时实时预览不能崩。
- [x] 3.2 守卫中间件：一次性 token（`crypto/rand` 32 字节，放 `Authorization` 头，
      `subtle.ConstantTimeCompare` 比较）+ `Origin`/`Host` 校验，失败 401/403。访问日志**不记录 body**。
- [x] 3.3 实现 §4 的七个端点；`/api/catalog` 用 SSE。
- [x] 3.4 空闲超时（默认 30min 无请求）自动退出；`/api/quit` 优雅关闭。
      SSE 推送也算活动，否则用户盯着页面时会被踢掉。
- [x] 3.5 单测：假 `Source` + `httptest`。覆盖 **AC7**（7 条路由 × 5 种坏 token 全 401、
      6 种伪造 Origin/Host 全 403）、**AC6**（保存被拒且错误与启动期**逐字**相同）、
      **AC4**（预览结果与直接调 `mapping.Resolve` 逐字段相等，覆盖 5 种场景）。

**附带的既有代码改动**（超出「阶段 0 是唯一改动既有代码的阶段」的预期，均为消除重复）：
- `config.ParseDuration` / `FormatDuration` 导出——UI 与 YAML 接受同一套时长写法。
- `config.Normalize()` 抽出——原先内联在 `Load` 里的 trim + 默认值逻辑，现在 `Draft.Config()` 复用它。
  若不做这一步，UI 就会有第二套归一化规则，AC6 迟早破。
- `mapping.Rule` / `TitlePattern` 加 json tag（与 yaml tag 同名），避免再定义一套线上结构。

**门禁**：单测过；`go test -race` 干净（draft 与 catalog 都有并发访问）。

> ✅ 2026-07-30 通过，`CGO_ENABLED=1 go test -race ./client-windows/...` 全绿。

## 阶段 4 · Windows 观察层与 CLI 接线

- [x] 4.1 `internal/winsetup/winsetup.go`（**不是** design 说的 `setup/observe_windows.go`）：
      1s 轮询 `collect.Collect()` 喂 catalog。**这是唯一会把原始标题读进 catalog 的地方**，注释写明。
- [x] 4.2 `main.go` 加 `-setup`；与 `-dry-run` 互斥（同时给出直接报错退出，已用真实 exe 验证）。
- [x] 4.3 配置不存在 / 解析失败时仍进 UI，用默认值开局并把原因传给前端（R1.3）。
      4 种坏文件（YAML 语法错、未知键、正则编译错、必填项空）都验了能进 UI。
- [x] 4.4 自动开浏览器（`rundll32 url.dll,FileProtocolHandler`）；URL 无条件打印到 stdout（R1.4）。
- [x] 4.5 确认 `-setup` 分支不 import `report` 包（AC8 的结构性保证）。
      **做成了可执行断言**：`TestSetupModeCannotReport` 跑 `go list -deps` 检查依赖图里没有 `report`。
      这正是把 setup 接线拆成独立包 `winsetup` 的理由——同包内无法做这个保证。

**门禁**：`GOOS=windows go build` 过；在真实 Windows 上跑起来，用 `curl` 验证 token 守卫。

> ✅ 2026-07-30 通过。除 `curl` 外还留下了 `session_test.go`：起一个**真实**会话
> （真端口、真 Win32 采集、真写盘），验证无 token 401 / 伪造 Origin 403 / 正确 token 200、
> 只绑 127.0.0.1、以及**空目录起步 → 纯 API 操作 → 存盘 → `config.Load` 成功**（AC2 的可回归版本）。

## 阶段 5 · 前端

- [x] 5.1 `client-windows/webui/` 初始化 Vite app，栈与 `web/` 对齐；
      `vite.config.ts` 的 `outDir` 指向 `client-windows/cmd/agent/webui/`。
      不引入字体包（省约 57KB，中文本来也用系统字体）；产物 448KB，与 design §10 预估一致。
- [x] 5.2 发现面板（候选应用流 + 标题样本展开）、规则编辑器
      （`title_patterns` **用上下按钮排序，不是拖动** —— 见下方偏离说明）。
- [x] 5.3 常驻实时预览条（`/api/preview`）。
- [x] 5.4 连接设置：整段粘贴 `register-device` 输出自动拆字段（R4.1）；token 输入框默认遮蔽。
- [x] 5.5 高级设置：其余全部键 + 各自默认值提示（R4.2）。若报错字段落在这个折叠区里会自动展开
      —— 否则用户被告知"某处错了"却看不见它。
- [x] 5.6 危险区：`expose_title` 二次确认弹窗 —— 实时真实标题 + 手动输入确认短语（R4.4 / AC9）。
      确认短语是**进程名本身**而非固定句子：固定句子第二次就变成肌肉记忆，进程名强迫用户看清在公开哪个应用。
- [x] 5.7 vitest 覆盖：粘贴解析（9 例）、规则/顺序操作（12 例）、确认短语（4 例）、相对时间（5 例），共 32 例。

**门禁**：`npm run lint` / `typecheck` / `vitest run` / `build` 全过。

> ✅ 2026-07-30 通过，并在**真实浏览器**里跑了一遍完整流程（Playwright + chrome-devtools）：
> 粘贴 `register-device` 片段一次填好 4 个字段 → 从发现列表加规则 → 预览条立刻变成「Chrome 在上网」
> → 保存 → `agent.exe -dry-run` 能加载该文件。诱饵检查：`config.yaml` 与 `-v` 日志中
> 5 个真实标题片段全部零命中（AC1）。确认门禁实测：初始禁用 / 输 `chrome` 仍禁用 / 输 `CHROME.EXE` 才启用（AC9）。
>
> **对 design §7 的偏离**：`title_patterns` 排序改为上下按钮。理由：拖放对键盘用户不可达，
> 且要引入 dnd 依赖；顺序语义（第一条命中即生效）用按钮表达同样清楚。
>
> **新增第 8 个端点** `POST /api/regex/suggest`（design §4 原定 7 个）：建议正则必须由 Go 的
> `regexp.QuoteMeta` 生成——JS 的转义规则与 RE2 不同，前端自己转义可能产出语义不一致或编译不了的正则。
>
> 按用户要求跑了 `/design-taste-frontend` 与 `/web-design-guidelines`。前者自述适用于 landing page，
> 明确把配置工具列为 out of scope，故只取其通用部分（零 em-dash、无装饰状态点、无 scroll cue、
> 配色/圆角一致性、表单规范）。后者查出 6 处并已修：
> `color-scheme`（暗色下滚动条是真 bug）、`theme-color`、机器标识符加 `translate="no"`
> （浏览器自动翻译会改掉进程名和正则）、输入框默认 `autocomplete="off"`、弹窗 `overscroll-contain`、
> 以及**未保存改动提示 + `beforeunload` 守卫**——改动实时同步给 agent 但只有点保存才写盘，此前没有任何地方说明这件事。

## 阶段 6 · CI、文档与端到端验收

- [x] 6.1 CI 新增 `webui` job（lint / typecheck / test / build + embed freshness），
      照搬现有 `web` job 结构。
- [x] 6.2 CI 新增 `windows-latest` job 跑 `go test -race ./client-windows/...`
      —— 补上 client-windows 单测从不在 CI 运行的现存缺口（design §3）。
- [x] 6.3 `client-windows/README.md`：新增 `-setup` 章节；**在脱敏红线章节补上那条精确例外**
      （原始标题可经本地回环展示给本机用户，及其**七**条约束——比 design 多一条：
      依赖图层面够不到 `report`）。诱饵测试章节也补了 `-setup` 专属的两步。
- [x] 6.4 根 `README.md` 提一句 `-setup`（含脱敏例外），`config.example.yaml` 顶部注明可视化配置。
- [x] 6.5 **AC1 诱饵验证**：用真实浏览器跑完整流程后，`grep` `config.yaml` 与 `-v` 日志，
      5 个真实标题片段（`Google Chrome` / `Sallyn0225` / `cyberstalk-me 客户端` / `about:blank` / `LAPLACE`）零命中。
- [x] 6.6 **AC2 空目录验证**：空目录起步，纯 UI 操作产出配置，`agent.exe -dry-run` 能加载并输出载荷。
      另有 `TestLiveSessionSavesAConfigFromScratch` 把这条路径固化成了可回归的测试。
- [x] 6.7 **AC8 零出站验证**：`netstat -ano` 按 PID 过滤，`-setup` 进程的**全部** socket 都在
      `127.0.0.1`（1 监听 + 3 条浏览器回环连接）；配置里那个不可路由的 `server_url`（`192.0.2.77`）
      从未被连接。加上 `TestSetupModeCannotReport` 的依赖图断言，运行时与结构两侧都封住了。

**门禁**：AC1–AC10 全部勾掉。

> ✅ 2026-07-30 全部通过。最终门禁：`gofmt` 干净、`go vet` 三模块干净、
> `CGO_ENABLED=1 go test -race` 三模块全绿、`GOOS=linux` 建服务端、`GOOS=windows` 建客户端、
> 前端 lint/typecheck/32 项测试/build 全过。

## 验证命令速查

```bash
# Go（在 Windows 上跑才能执行 client-windows 的测试）
gofmt -l shared server client-windows
go vet ./client-windows/...
go test -race ./client-windows/...
GOOS=windows GOARCH=amd64 go build ./client-windows/...

# 前端
cd client-windows/webui && npm run lint && npm run typecheck && npx vitest run && npm run build

# 嵌入产物是否过期
git diff --exit-code -- client-windows/cmd/agent/webui
```

## 回滚点

- 阶段 0 是唯一改动既有代码的阶段，独立成一次提交，出问题可单独 revert。
- 阶段 1–5 均为新增文件，回滚 = 删目录 + 摘掉 `-setup` 分支。
- 用户侧：`config.yaml.bak` 改名回去。
