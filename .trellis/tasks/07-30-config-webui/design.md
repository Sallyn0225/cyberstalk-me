# Design — `agent.exe -setup` 规则可视化配置 WebUI

> 需求与验收见 `prd.md`，执行清单见 `implement.md`。
> 本文档钉死包边界、隐私边界、API 契约与几个非显然的 Windows 实现细节。

## 1. 总体形态

```
agent.exe -setup
   ├─ observer goroutine（windows-only）
   │     每 1s collect.Collect() → 读 exe 名 + 惰性标题 → 写入内存 catalog
   ├─ draft（内存中的 config.Config，服务端持有，唯一真源）
   └─ http.Server on 127.0.0.1:<临时端口>
         ├─ go:embed 的前端静态资源
         └─ /api/*（token + Origin 双重守卫）
   → 用户点「完成」→ 校验 → 备份 → 原子写回 config.yaml → 进程退出
```

**不联网**：`-setup` 分支根本不构造 `report.Client`，`report` 包不被这条路径 import。

## 2. 隐私边界（本设计最关键的部分）

现有红线：原始窗口标题不进上报体、不进日志、不进 `-dry-run` 输出、不落盘。

本任务**必然**要突破的一点：为了"直观"，原始标题必须显示给本机用户看。
因此新增一条精确的、收窄的例外，并把它写进 `client-windows/README.md` 的脱敏红线章节：

> 原始窗口标题可以经由 `-setup` 模式的本地回环 HTTP 接口，展示给正在本机操作的用户。
> 除此之外的所有既有约束不变。

对应的硬约束（实现必须逐条满足）：

| 约束 | 实现手段 |
|------|----------|
| 不出本机 | 监听地址硬编码 `127.0.0.1`，不接受任何可配置的 bind 地址 |
| 不被本机其他程序读走 | 一次性随机 token（32 字节，`crypto/rand`）+ `Origin`/`Host` 白名单校验 |
| 不被浏览器里的恶意页面读走 | 同上的 `Origin` 校验；token 放 `Authorization` 头而非 URL 查询串（避免进浏览器历史/`Referer`） |
| 不落盘 | catalog 只在内存；**不写日志**（即使 `-v`）；写回 YAML 时标题不参与序列化 |
| 生命周期最短 | 进程退出即关闭；空闲超时（默认 30min 无请求）自动退出 |
| 常驻上报模式不受影响 | `-setup` 与常驻是互斥分支，常驻模式不监听任何端口 |

**token 的处理**：`config.yaml` 里的设备 token 会经 `/api/config` 下发给前端（要显示在表单里）。
它与窗口标题受同一套守卫保护，且 `Config.String()` 已做 redact（`config.go:74`），
日志侧无需额外处理，但新增的 HTTP 访问日志必须**不记录 body**。

## 3. 包边界与构建约束

现状纪律：`mapping` 是纯包且必须单测；`collect` 是 Windows-only 且豁免单测
（`.trellis/spec/backend/quality-guidelines.md:84`）。新代码沿用同一分层：

| 包/文件 | build tag | 职责 | 可测性 |
|---------|-----------|------|--------|
| `internal/config/save.go` | 无 | `Config` → 带注释的 YAML，备份 + 原子写 | 纯逻辑，必须单测（AC5） |
| `internal/setup/catalog.go` | 无 | 内存候选表：按 exe 聚合、标题去重、样本上限与淘汰 | 纯逻辑，必须单测 |
| `internal/setup/suggest.go` | 无 | 由标题样本生成建议正则（转义 + `(?i)`）、命中高亮计算 | 纯逻辑，必须单测 |
| `internal/setup/draft.go` | 无 | 线上 `Draft` ↔ `config.Config` 互转 | 纯逻辑，必须单测 |
| `internal/setup/server.go` | 无 | HTTP handlers、token/Origin 守卫、draft 状态机 | 用 `httptest` + 假 `Source` 单测 |
| `internal/winsetup/` | `windows` | `-setup` 的接线：轮询 `collect.Collect()` 喂 catalog、开浏览器、生命周期 | 薄适配层 + 真实会话端到端测试 |
| `webui/` | — | Vite app（React 19 + Tailwind 4 + shadcn） | vitest |

**关键约束**：`internal/setup` **完全不碰 Win32，整个包没有 build tag**。前台读取由调用方注入：

```go
// setup 包定义，winsetup 提供实现
type Foreground struct {
    Process     string
    Title       func() string // 惰性，与 collect.Snapshot.Title 同一个约定
    IdleSeconds int
}
type Source func() Foreground
```

> 📝 **2026-07-30 实现修订**：原计划是 `setup` 包内放一个 `observe_windows.go`，并用 `Observer` 接口。
> 改成上面这样有两个实打实的好处：
> 1. `setup` 包 100% 平台无关 —— design 本节的目标（「让 handlers 与 catalog 能在 Linux CI 上被覆盖」）
>    从「大部分文件」变成「整个包」。
> 2. 接线独立成 `internal/winsetup` 包后，**AC8 从「承诺」变成「可断言」**：
>    `TestSetupModeCannotReport` 直接跑 `go list -deps` 检查 `report` 不在依赖图里。
>    若接线留在 `cmd/agent`（package main 已 import `report`）或留在 `setup` 包内，这个断言都写不出来。

> ⚠️ **现存 CI 缺口**：CI 目前对 `client-windows` 只做 `GOOS=windows` 的 `go vet` + `go build`，
> **从不运行它的单测**（`.github/workflows/ci.yml` 的 `go` job）。因为 `collect` 是 Windows-only，
> `GOOS=linux` 下整个 module 连编译都过不了。
> 本任务新增的 YAML 写回与校验逻辑正是 AC5/AC6 的命门，必须有 CI 覆盖。
> **方案**：新增一个 `windows-latest` 的 CI job 跑 `go test ./client-windows/...`。
> 这比"把纯包拆成独立 module 以便 linux 测"更简单，且顺带把现有的 `mapping`/`config` 单测纳入 CI。

## 4. HTTP API 契约

全部前缀 `/api`，全部要求 `Authorization: Bearer <一次性 token>`，全部只接受同源请求。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/catalog` | SSE 流：候选应用列表与标题样本的增量推送（复用项目已有的 SSE 心智，前端不轮询） |
| GET | `/api/config` | 当前 draft（含 token 明文，供表单回填） |
| PUT | `/api/config` | 整体替换 draft（前端本地编辑 + 防抖提交），返回校验结果 |
| GET | `/api/preview` | 用当前 draft 对**当前前台窗口**跑一次 `mapping.Resolve`，返回将要上报的 `Activity` |
| POST | `/api/regex/test` | 给定 pattern，返回它在当前标题样本集中的命中情况（R3.3 的高亮） |
| POST | `/api/save` | 校验 → 备份 → 原子写回；失败返回结构化错误 |
| POST | `/api/quit` | 关闭服务并退出进程 |

**draft 由服务端持有**（`sync.Mutex` 保护），理由：
`/api/preview` 与 `/api/save` 都需要一份编译好的 `*mapping.Mapper`；由服务端持有可以在
draft 变更时编译一次并缓存，而不是每次预览重编译全部正则。

**预览一致性（AC4）的实现方式**：`/api/preview` 调的就是 `mapping.Resolve`，
用的是 draft 编译出的 mapper。`mapping` 包里只有这一个实现，前端不重算匹配（R3.5）。

> 📝 **2026-07-30 实现修订**：`collect` 与 `mapping` 互不 import 是既有纪律，
> 所以「把三元组喂给 `Resolve`」的展开点无法压缩成一处：常驻/`-dry-run` 在 `cmd/agent/main.go` 的
> `resolve()`，预览在 `winsetup.foreground()` → `setup.Server.handlePreview`。
> 两处共用同一个 `mapping.Resolve`，漂移风险由两个测试兜住：
> `TestPreviewMatchesMappingResolve`（预览 == 直接调 Resolve，5 种场景）与
> `TestForegroundCarriesTheWholeSnapshot`（注入的 Source 没漏字段，尤其 `IdleSeconds`）。

## 5. 校验：单一实现，两处调用

`PUT /api/config` 与 `POST /api/save` 都调用现有 `config.validate` + `mapping.New`（`config.go:198`）。
`validate` 目前是私有方法且签名带 `path`（用于错误信息）。改造为：

```go
func (c *Config) Validate(source string) error   // 导出，source 是错误信息里的来源描述
```

`Load` 内部继续调用它并传文件路径；`-setup` 传 `"draft"`。
**不新建第二套校验规则** —— AC6 要求 UI 报错与启动期报错同源。

错误需要能定位到具体字段才能在 UI 上标红，因此新增一个结构化错误类型
（字段路径 + 消息），`Validate` 返回它，`Load` 侧仍按 `error` 处理，文案不变。

## 6. YAML 写回

### 6.1 生成带注释的 YAML

`yaml.v3` 的 `Marshal(struct)` 不产出注释。方案：**手工构造 `*yaml.Node` 树**，
在节点上设置 `HeadComment`，再 `yaml.Marshal(node)`。
这样既拿到正确的引号/转义处理（中文、含元字符的正则、含 `:` 的标题），又能带注释。

注释文案直接沿用 `client-windows/config.example.yaml` 的中文说明，
其中 `expose_title` 的 DANGER 警告块必须原样保留。

**round-trip 测试（AC5）**：构造覆盖全部键与多条 `title_patterns` 的 `Config` →
`Save` → `Load` → 与原对象逐字段比对。这是 `save.go` 的核心单测。

### 6.2 备份 + 原子写 + ACL 保持

三个要求会互相打架，必须用对 API：

- **不能**用 `os.Rename` 覆盖已有的 `config.yaml`。Windows 上它走 `MoveFileEx(MOVEFILE_REPLACE_EXISTING)`，
  目标文件会被**源文件的 ACL 取代** —— 而 README 明确教用户收紧 `config.yaml` 的 ACL（里面有 token）。
  一次保存就把权限放开回默认，是个静默的安全回归。
- **正确解法**：`ReplaceFileW`。它专为此设计：原子替换、**保留目标文件原有的 ACL 与属性**，
  并且第三个参数 `lpBackupFileName` 会顺手把原文件存成备份 —— R6.2 与 R6.3 一次满足。

```
写临时文件 config.yaml.tmp（同目录，保证同卷）
  └─ 目标已存在 → ReplaceFileW(config.yaml, config.yaml.tmp, config.yaml.bak, ...)
  └─ 目标不存在 → os.Rename（首次创建，没有 ACL 要保留，也没有备份可做）
```

`ReplaceFileW` 未被 `golang.org/x/sys/windows` 导出，按 `collect_windows.go:50` 已有的
`NewLazySystemDLL` + `NewProc` 模式声明即可。
这部分放 `internal/config/save_windows.go`（Windows-only）；
`save.go` 里平台无关的"生成字节流"部分保持可测，替换动作藏在一个 `replaceFile(dst, tmp, bak string) error`
接口后面，非 Windows 侧给一个 `os.Rename` 实现供 CI 编译（实际不会被执行）。

## 7. 前端

- 位置 `client-windows/webui/`，产物输出到 `client-windows/cmd/agent/webui/`（`go:embed` 目标），
  与 `web/` → `server/cmd/server/web/` 完全同构。
- 栈与 `web/` 对齐：React 19 + Vite 8 + Tailwind 4 + shadcn/radix + oxlint + vitest。
  **不复制 `web/src` 的业务组件**，只共享设计语言；两者数据契约完全不同。
- 主要界面：
  1. **发现面板**（左）—— 候选应用卡片流，新出现的高亮；每张卡片可展开看标题样本列表。
  2. **规则编辑器**（右）—— 当前 `rules`，可拖动 `title_patterns` 排序（顺序有语义）。
  3. **实时预览条**（常驻底部）—— "现在会显示成：VS Code · 在写代码"，直接来自 `/api/preview`。
  4. **连接设置 / 高级设置** —— 折叠区，含整段粘贴解析。
  5. **危险区** —— `expose_title`，红色分区 + 二次确认弹窗（实时真实标题 + 输入确认短语）。
- 建议正则的生成（`suggest.go`）：对样本做 `regexp.QuoteMeta` 转义后加 `(?i)` 前缀；
  UI 里展示为可编辑输入框，并即时显示"命中 3/7 条样本"与命中项高亮。

## 8. CLI 行为

```
agent.exe -setup                    # 用默认 config 路径
agent.exe -setup -config ./a.yaml   # 指定路径
agent.exe -setup -dry-run           # 报错退出：两者互斥
```

- 配置文件不存在 / 解析失败 → **仍然进入 UI**，用内置默认值开局，顶部横幅说明原因（R1.3）。
  这一条与"配错就不启动"的常驻模式纪律不同，是有意为之：修复工具不能被坏配置挡在门外。
- 启动后 `exec.Command("rundll32", "url.dll,FileProtocolHandler", url)` 或
  `cmd /c start` 打开浏览器；失败则把 URL 打到 stdout（R1.4）。

## 9. 兼容性与回滚

- **纯增量**：不改上报契约、不改 `shared/`、不改服务端、不改常驻模式的任何行为。
  `mapping` 与 `collect` 只被读取与复用，不修改语义。
- 唯一的既有代码改动是 `config.validate` → 导出的 `Validate` 并返回结构化错误
  （`Load` 的对外行为与错误文案不变，由现有 `config_test.go` 兜底）。
- 回滚 = 删掉 `-setup` 分支与 `internal/setup`、`webui/` 及嵌入产物；`config/save.go` 可留可删。
- 用户侧回滚：`config.yaml.bak` 改名回去即可。

## 10. 主要风险

| 风险 | 缓解 |
|------|------|
| 本地端口成为标题泄露通道 | §2 的六条硬约束 + AC7/AC8 显式验收 |
| `ReplaceFileW` 用法出错导致配置丢失 | 先写 tmp 再替换；`.bak` 兜底；AC5 round-trip 测试 |
| 前端产物忘记重建 → 用户 `go build` 得到旧界面 | 照搬 `web` job 的 embed freshness 关卡（R7.2） |
| client-windows 单测在 CI 里不跑 | 新增 `windows-latest` job（§3） |
| exe 体积增长 | 约 +450KB，可接受；不引入字体文件（用系统字体栈）可再省约 57KB |
