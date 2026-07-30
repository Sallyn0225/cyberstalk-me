# Windows 客户端规则可视化配置 WebUI

## Goal

让不写 YAML 的人也能配好 `client-windows/config.yaml`。

用户运行 `agent.exe -setup`，浏览器自动打开一个本地页面；页面持续观察前台窗口，
把「刚才用过哪些应用、每个应用的窗口标题长什么样」实时列出来；用户点几下就能决定
每个应用公开显示成什么名字、在干什么，以及同一应用在不同场景下显示不同描述
（浏览器在 B 站显示"在看视频"、在 GitHub 显示"在看代码"）。
配完直接写回 `config.yaml`，全程不用手写 YAML、不用写正则、不用查文档。

## Background：现状与约束（已核实）

### 现有配置方式

- 唯一配置点是 `agent.exe` 同目录的 `config.yaml`（`client-windows/internal/config/config.go:141`），
  可用 `-config <path>` 覆盖。模板 `client-windows/config.example.yaml`。
- 配置项：`server_url` / `device_id` / `token` / `interval` / `device_name` / `idle_threshold` /
  `default_app` / `default_description` / `locked_app` / `locked_description` / `rules` / `expose_title`。
- 规则结构（`client-windows/internal/mapping/mapping.go:29`）：
  `rules[].process`（exe 基名，小写归一，不允许重复）、`.app`（必填）、`.description`、
  `.title_patterns[].match`（Go RE2 正则）+ `.description`。
- 匹配优先级（`mapping.go:142`）：无前台窗口 → locked 文案；未命中规则 → default 文案（**不读标题**）；
  命中规则且在 `expose_title` → 原始标题；否则按 `title_patterns` 顺序取第一个命中的 description。
- 校验「大声失败」：未知键（`dec.KnownFields(true)`，`config.go:159`）、非法时长、非法 `server_url`、
  编译不了的正则、重复 `process`、`expose_title` 指向不存在的规则，全部在启动时报错退出
  （`config.go:198` 的 `validate` + `mapping.New`）。
- **只有 `Load`，没有 `Save`** —— 写回 YAML 是全新代码。

### 采集能力现状

- `collect.Collect()`（`client-windows/internal/collect/collect_windows.go:104`）**只读前台窗口**，
  没有任何枚举全部进程/窗口的能力。
- 原始标题不是字符串字段，而是 `Snapshot.title` 惰性 getter（`collect_windows.go:72`）。
- 提权窗口拿不到进程时记为 `?unknown`（含 `?`，永远无法与规则键冲突）。
- 锁屏（`lockapp.exe` / `logonui.exe` 或无前台窗口）走 locked 分支，且不读标题。
- `mapping` 包是纯函数、无 I/O、已单测，**可直接复用做实时预览**。

### 隐私红线（项目的核心安全假设）

- 原始窗口标题与进程完整路径不离开设备：不进上报体、不进日志（含 `-v`）、不进 `-dry-run` 输出、**不落盘**。
- 未命中规则的进程只报通用文案，绝不用 exe 名当应用名。
- `expose_title` 是唯一显式退出口，默认空。
- **本任务新增的攻击面**：一个能读到原始窗口标题的本地 HTTP 端口。此前不存在，必须显式收口（见 R5）。

### 技术栈与发布现状

- `client-windows` 是独立 Go module（`go 1.26.5`），依赖仅 `golang.org/x/sys` + `gopkg.in/yaml.v3`。
  单文件 `agent.exe`，无安装程序、不注册服务、不写注册表。
- **`client-windows` 没有发布产物**：用户自己 `go build -o agent.exe ./cmd/agent`
  （`.github/workflows/release.yml` 只发服务端镜像）。
- 已有前端嵌入模式：`web/`（React 19 + Vite 8 + Tailwind 4 + shadcn/radix + oxlint + vitest）
  构建产物**提交进仓库**（`server/cmd/server/web/`），由 `//go:embed all:web` 编进二进制
  （`server/cmd/server/main.go:50`），CI 有一道 embed freshness 关卡 diff 它是否过期
  （`.github/workflows/ci.yml` 的 `web` job）。现有产物 JS 397K + CSS 46K。

## Key Decisions

- **D1 应用发现机制 = 被动观察前台窗口流。**
  复用 `collect.Collect()` 定时轮询前台窗口，把出现过的 `(exe, 标题样本)` 累积成候选列表；
  用户想配哪个应用就切过去用一下，卡片自己冒出来。不做 `EnumWindows` 全窗口枚举。
  依据：零新增 Win32 代码、隐私面最小，且天然覆盖「同一应用不同状态」。
  取舍：想配的应用必须真的切出来一次，没有「开箱即得的已装应用清单」。

- **D2 交付形态 = `agent.exe -setup` 一次性配置模式。**
  与 `-dry-run` 同一心智：跑起来 → 自动开浏览器 → 以约 1s 节奏观察前台 → 配完写回 → 退出。
  **全程不联网、不上报。** 不做常驻端口，也不拆独立 exe（保住「单文件运行」卖点）。
  取舍：改规则要先停掉常驻 agent，不能边上报边微调。

- **D3 范围 = 全部配置项都进 UI。**
  连接设置 + 应用规则 + 其余全部键，目标是 `config.yaml` 完全不需要手写，无配置文件时能从空白开始。
  取舍：表单面积与测试面变大。

- **D4 `expose_title` = 进 UI，但走高摩擦二次确认。**
  确认弹窗必须**实时展示该应用当前的真实窗口标题**，并要求手动输入确认短语才生效。
  真实预览比文字警告更能唤醒风险感知。已存在的条目写回时原样保留。

- **D5 前端形态 = 在 `client-windows/webui/` 新建 Vite app，复用 `web/` 的栈。**
  构建产物提交进仓库并 `go:embed`。依据：`client-windows` 无发布产物，产物不入库则 clone 后
  `go build` 会拿到空界面；仓库已有同款模式与 CI 关卡。
  代价：exe 增大约 450KB；新增一套 npm workspace、一道 CI freshness 关卡、一份入库构建产物。

- **D6 写回策略 = 全量重写 + 自动生成注释 + 写前备份。**
  每次保存输出格式统一、带教学注释（沿用 `config.example.yaml` 文案，含 `expose_title` 警告）的配置文件，
  写前先备份成 `config.yaml.bak`。取舍：用户手写的注释与排版被抹掉，但可从 `.bak` 找回。

- **D7（工程默认，未占用用户决策）**
  - 本地端口 bind `127.0.0.1` + 一次性随机 token + 空闲超时自动退出。
  - 标题样本**只存内存**，进程退出即消失 —— 落盘会直接违反「原始标题不落盘」红线。
  - 自动生成的正则展示给用户并允许手工修改，同时实时高亮「当前样本里哪几条会命中」。

## Requirements

### R1 配置模式入口

- **R1.1** 新增 `-setup` 标志。与 `-dry-run` 互斥（同时给出时报错退出）。
- **R1.2** `-setup` 模式下**不创建 report client、不发起任何网络请求**，
  除了监听 `127.0.0.1` 的本地配置端口。
- **R1.3** 启动时若 `-config` 指向的文件存在则载入作为初始值；不存在或解析失败时，
  以内置默认值开局并在 UI 顶部提示原因，**不因配置非法而拒绝启动**
  （否则「配置坏了」的用户恰恰进不去修复工具）。
- **R1.4** 自动在默认浏览器打开带一次性 token 的 URL；打开失败时把 URL 打印到 stdout。
- **R1.5** 用户在 UI 里点「完成」后进程退出；同时设置空闲超时（无请求 N 分钟）自动退出。

### R2 应用与标题样本的实时发现

- **R2.1** 以约 1s 节奏轮询 `collect.Collect()`，累积候选列表：
  按 exe 基名聚合，每个应用保留其出现过的**去重标题样本**（含首次/最近出现时间、出现次数）。
- **R2.2** 候选列表仅存内存，任何路径都不写盘、不进日志。
- **R2.3** UI 通过 SSE 或轮询接口近实时看到新应用/新标题样本出现（"切过去就能看见"）。
- **R2.4** `?unknown`（提权窗口）与锁屏状态在 UI 中有明确说明，且不可被添加为规则。
- **R2.5** 单个应用的标题样本数量设上限并淘汰最旧的，避免长时间运行内存无界增长。

### R3 规则编辑与实时预览

- **R3.1** 从候选应用一键创建规则：填 `app`（必填）与 `description`。
- **R3.2** 从某条标题样本一键创建 `title_pattern`：由样本自动生成建议正则
  （默认加 `(?i)`、对元字符做转义），展示给用户且可手工修改。
- **R3.3** 正则输入实时校验（Go RE2 语法，与 `mapping.New` 同一套规则），
  并实时高亮「当前所有标题样本中哪几条会命中这条 pattern」。
- **R3.4** 规则列表支持编辑、删除、调整 `title_patterns` 顺序（顺序有语义：第一条命中即生效）。
- **R3.5** UI 常驻一个「当前会上报什么」的实时预览，其结果由**后端调用 `mapping.Resolve`** 得出，
  不得在前端重新实现一份匹配逻辑。
- **R3.6** `process` 重复、`app` 为空等约束在 UI 中即时提示，不留到保存时才报错。

### R4 全量配置项编辑

- **R4.1** 连接设置：`server_url` / `device_id` / `token` / `interval`，
  支持把 `server register-device` 的输出**整段粘贴自动拆字段**。
- **R4.2** 其余键：`device_name` / `idle_threshold` / `default_app` / `default_description` /
  `locked_app` / `locked_description`，均可视化编辑并显示各自默认值。
- **R4.3** `token` 输入框默认遮蔽；token **不得**出现在日志、错误信息与浏览器地址栏。
- **R4.4** `expose_title`：提供开启入口，但确认弹窗须实时展示该应用当前的真实窗口标题，
  并要求手动输入确认短语；已存在条目原样保留。

### R5 本地端口的安全收口

- **R5.1** 只监听 `127.0.0.1`，端口取系统分配的临时端口。
- **R5.2** 每次启动生成一次性随机 token，所有 API 请求必须携带；缺失或不匹配返回 401。
- **R5.3** 校验 `Origin`/`Host`，拒绝跨站请求，防止本机浏览器里的恶意页面打这个端口。
- **R5.4** 服务在 `-setup` 进程退出时一并关闭，不留后台监听。

### R6 保存与校验

- **R6.1** 保存前用与启动期**完全相同**的校验路径（`config.validate` + `mapping.New`）验一遍，
  不通过则拒绝写入并把错误显示在 UI 上。
- **R6.2** 写入前把原 `config.yaml` 备份为 `config.yaml.bak`（存在则覆盖）。
- **R6.3** 原子写入（临时文件 + rename），且**不得放宽原文件的 ACL**（文件含 token）。
- **R6.4** 输出带教学注释、键序稳定的 YAML；写回结果必须能被 `config.Load` 成功读回
  且语义与保存时一致（round-trip 等价）。

### R7 构建与 CI

- **R7.1** `client-windows/webui/` 的构建产物提交进仓库，供 `go:embed` 使用。
- **R7.2** CI 新增该前端的 lint / typecheck / test / build 与 embed freshness 关卡，
  与现有 `web` job 同构。
- **R7.3** `client-windows` 的 Go 侧交叉编译（`GOOS=windows`）在 CI 中依然通过。

## Acceptance Criteria

- [x] **AC1（红线不破）** 全流程跑一遍后，`config.yaml`、`config.yaml.bak`、stdout 日志（含 `-v`）中
      均**不出现任何原始窗口标题**；用诱饵标题（如把某窗口标题设为 `SETUP-CANARY`）验证。
      唯一例外是用户显式通过 R4.4 开启 `expose_title` 后写入的进程名（进程名本身不是标题）。
- [x] **AC2（零 YAML 路径）** 从**没有** `config.yaml` 的空目录起步，
      仅通过 `-setup` 的 UI 操作即可产出一份能让 `agent.exe` 正常启动并成功上报的配置。
- [x] **AC3（发现机制可用）** `-setup` 运行中切到一个此前未出现过的应用，
      ≤3 秒内该应用出现在候选列表；在该应用内切换到不同页面/文件，新标题样本出现在其样本列表中。
- [x] **AC4（预览一致）** UI 的「当前会上报什么」与同一配置下 `agent.exe -dry-run` 输出的
      `activity.app` / `activity.description` 一致（覆盖：命中规则、命中 title_pattern、未命中默认、锁屏）。
- [x] **AC5（写回等价）** 任取一份含全部键与多条 `title_patterns` 的配置，
      经 UI 保存后再 `config.Load`，得到的 `Config` 与保存前语义等价；`config.yaml.bak` 内容为原文件。
- [x] **AC6（校验一致）** 在 UI 里构造非法配置（重复 `process`、空 `app`、非法正则、
      `expose_title` 指向不存在的规则）时保存被拒绝，且错误信息与 `agent.exe` 启动期报错同源。
- [x] **AC7（端口收口）** 不带 token 请求任一 API 返回 401；带伪造 `Origin` 的请求被拒绝；
      `-setup` 进程退出后端口不再监听。
- [x] **AC8（不联网）** `-setup` 全程抓包/断网验证：除本地回环外无任何出站请求。
- [x] **AC9（危险开关摩擦）** 开启 `expose_title` 必须经过展示真实标题的弹窗并手动输入确认短语，
      任何单次点击路径都无法开启它。
- [x] **AC10（门禁）** `gofmt` / `go vet` / `go test` / `GOOS=windows go build` 全过；
      新前端的 lint / typecheck / test / build 全过；embed freshness 关卡通过。

### 验收记录（2026-07-30）

| AC | 怎么验的 |
|----|---------|
| AC1 | 真实浏览器跑完整流程后 grep `config.yaml` 与 `-v` 日志，5 个真实标题片段零命中；另有 `TestPreviewNeverLeaksAnUnmappedTitle` |
| AC2 | 空目录 → 纯 UI 操作 → 保存 → `agent.exe -dry-run` 成功输出载荷；固化为 `TestLiveSessionSavesAConfigFromScratch` |
| AC3 | 浏览器实测：切到 Chrome 与 Windows Terminal，均在约 1 秒内出现在候选列表并带出标题样本 |
| AC4 | `TestPreviewMatchesMappingResolve` 逐字段比对预览与直接调 `mapping.Resolve`，覆盖命中规则 / 命中 pattern / pattern 未命中 / 无规则 / 锁屏 5 种 |
| AC5 | `TestSaveLoadRoundTrip` + 12 组 YAML 陷阱值；`TestSaveBacksUpPreviousFile` 验 `.bak` 逐字节等于原文件 |
| AC6 | `TestSaveRejectionMatchesStartupRejection` 断言 UI 拒绝文案与启动期**逐字**相同；`TestPutConfigReportsProblemsWithoutLosingTheEdit` 覆盖 6 类非法配置 |
| AC7 | `TestGuardRequiresTheSessionToken`（7 路由 × 5 种坏 token）、`TestGuardRejectsForeignOriginsAndHosts`（6 种伪造）、`TestLiveSessionGuardsItsPort`（真实端口）、退出后 `curl` 连接被拒 |
| AC8 | `netstat -ano` 按 PID：全部 socket 均在 127.0.0.1，配置的 `server_url` 从未被连接；`TestSetupModeCannotReport` 用 `go list -deps` 断言依赖图里没有 `report` |
| AC9 | 浏览器实测：初始禁用 → 输 `chrome` 仍禁用 → 输 `CHROME.EXE` 才启用；弹窗展示该应用的真实当前标题 |
| AC10 | `gofmt` / `go vet` / `go test -race`（三模块）/ `GOOS=windows go build` / 前端 lint+typecheck+32 测试+build 全过 |

## Out of Scope

- 全进程/全窗口枚举（`EnumWindows`）—— 见 D1。
- 常驻配置端口与配置热重载 —— 见 D2。
- 独立 `configurator.exe` —— 见 D2。
- 保留用户手写注释的增量 YAML 编辑（`yaml.Node` round-trip）—— 见 D6，`.bak` 是其替代方案。
- 标题样本跨进程持久化 —— 违反「原始标题不落盘」红线，永久排除。
- 服务端侧的任何改动；Android 客户端（`07-28-client-android`）。
- `agent.exe` 的开机自启配置 —— 与本任务正交。

## Technical Notes

技术方案见 `design.md`，执行清单见 `implement.md`。
