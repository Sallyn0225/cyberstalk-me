<div align="center">

<img src="../web/public/favicon.svg" width="64" alt="cyberstalk-me">

# client-windows · Windows 采集客户端

**一个 exe，把「我现在在用什么」变成一行脱敏后的文字。**
读取前台窗口、挂机时长、电量、网络类型，全部在本机映射脱敏，只把结果定时 POST 给你自己的服务端。

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/platform-Windows-0078d4?logo=windows&logoColor=white)](https://learn.microsoft.com/windows/win32/)

</div>

---

## 它是什么

[cyberstalk-me](../README.md) 的 Windows 上报端。它在后台常驻，每隔一个间隔采集一次设备状态，
本地跑一遍映射规则，然后把 `{app, description}` 这样的结果发给服务端。

- **单文件运行** — 一个 `agent.exe` 加一个 `config.yaml`，没有安装程序、不注册服务、不写注册表。
- **设备端脱敏** — 原始窗口标题和进程路径不出本机，出网的只有映射后的结果。
- **干跑自检** — `-dry-run` 把「将要发送的载荷」打印出来后退出，不联网，配完规则先跑这个。
- **单点失败不拖垮整轮** — 某个 Win32 调用失败就降级成零值 / `null`，不会让整次上报作废。
- **配错就不启动** — 未知配置键、解析不了的时长、编译不了的正则，全在启动时报错退出，绝不静默用默认值。

## 脱敏红线

整个项目的安全性只建立在一件事上：**原始窗口标题不离开设备**。三条规则保证它成立：

1. **只在规则需要时才读标题** — 标题不是一个字符串字段，而是一个惰性 getter；只有配了
   `title_patterns` 的规则或 `expose_title` 白名单会去调用它。一条标题规则都没配的部署，
   根本不会读取任何标题。
2. **没匹配上规则的进程只报通用描述**（`某个应用 · 使用中`），绝不拿 exe 名当应用名——exe 名
   本身就可能泄露信息（内部工具、项目代号）。想显示？去写规则。
3. **原始标题只在一次映射调用的内存里存在** —— 不进上报体、不进日志（哪怕 `-v`）、不进
   `-dry-run` 输出、不落盘。进程的完整路径同样如此，采集层只取文件名部分。

> [!WARNING]
> `expose_title` 是唯一的显式退出机制：列在里面的进程会把**原始窗口标题原样公开上报**。
> 默认为空，除非你真的清楚自己在做什么，否则别动它。

上面第 3 条有且只有一个例外，就是 `-setup` 配置模式：**原始窗口标题会经由本机回环端口，
展示给正在这台机器前面操作的你自己**。不这样做就没法「看着标题配规则」。这个例外被收得很窄：

| 约束 | 怎么做到的 |
|------|-----------|
| 不出本机 | 监听地址硬编码 `127.0.0.1`，不提供任何可配置的 bind 地址 |
| 本机其它程序读不到 | 每次启动生成一次性随机 token（32 字节），所有 API 都要带 |
| 浏览器里的恶意页面读不到 | 校验 `Origin`/`Host`；token 放 `Authorization` 头而不是 URL，不进历史也不进 `Referer` |
| 不落盘 | 标题只在内存的候选表里，进程退出即消失；**不写日志**（哪怕 `-v`） |
| 活得尽量短 | 点「完成」即退出，另有空闲超时（默认 30 分钟无请求）自动退出 |
| 不影响常驻模式 | `-setup` 与常驻是互斥分支，常驻模式不监听任何端口 |
| 不可能顺手发出去 | `-setup` 这条路径在依赖图上就够不到 `report` 包，有测试盯着（`TestSetupModeCannotReport`） |

## 采集了什么

| 数据 | Win32 来源 | 上报形态 | 取不到时 |
|------|-----------|---------|---------|
| 前台进程 | `GetForegroundWindow` + `QueryFullProcessImageName` | 经规则映射成 `app` / `description` | 读不到进程（典型是管理员权限窗口拒绝 `OpenProcess`）记为 `?unknown`，落到通用描述 |
| 窗口标题 | `GetWindowTextW` | **不上报**，仅在规则需要时读进内存参与匹配 | — |
| 挂机时长 | `GetLastInputInfo` + `GetTickCount64` | `idle_seconds` 与 `idle` 布尔 | `0` |
| 锁屏 | 无前台窗口，或前台是 `lockapp.exe` / `logonui.exe` | `locked: true` + `locked_app`，且不读标题 | — |
| 电量 | `GetSystemPowerStatus` | `level` 0–100 + `charging` | `null`（台式机没电池，页面直接不显示电量块，而不是编一个 0%） |
| 网络类型 | `GetAdaptersAddresses` | `ethernet` / `wifi` / `cellular` / `offline` | `null`（未知，页面不显示，而不是猜一个） |

网络类型按 **有线 > Wi-Fi > 蜂窝** 取优先级（插着扩展坞的笔记本两者都是 up 的）；
没有默认网关的适配器——虚拟交换机、VPN 残留、断线网卡——一律忽略。

## 构建

```bash
cd client-windows
go build -o agent.exe ./cmd/agent
```

> [!NOTE]
> `go.work` 的存在让仓库根目录的裸 `go build ./...` 无法工作（本模块是 Windows-only 的），
> 所以要么先 `cd`，要么显式点名模块路径。
> 在非 Windows 机器上交叉编译：`GOOS=windows GOARCH=amd64 go build ./client-windows/...`，CI 就是这么验的。

不想看到常驻时的黑色控制台窗口，可以加 `-ldflags "-H=windowsgui"` 构建；
代价是日志写在 stdout，也一并看不见了，排查问题时记得换回普通构建。

## 配置

**1. 在服务端注册这台设备，拿一个专属 token。**

```bash
# 已经 docker compose 部署的
docker compose exec app cyberstalk-server register-device win-desktop "我的台式机" windows

# 本地开发直接跑源码的
cd server && go run ./cmd/server register-device win-desktop "我的台式机" windows
```

命令会打印一段可以直接粘贴的配置片段。token **只打印这一次**，服务端只存哈希，找不回来。

> [!NOTE]
> 服务端进程和 `register-device` 共用同一个 SQLite 文件（默认是工作目录下的 `cyberstalk.db`，
> 或 `SQLITE_PATH` 指定的路径），本地开发时两条命令要在同一个目录下跑，否则注册进了另一个库。

**2. 配置。两条路，选一条。**

### 可视化配置：`agent.exe -setup`（推荐）

```bash
./agent.exe -setup
```

浏览器会自动打开一个本地页面（打不开就用它打印在 stdout 的那个地址）。页面会：

- **列出你用过的应用** —— 它不枚举系统里装了什么，只是持续观察前台窗口。
  想配哪个应用，切过去用一下，它就出现在列表里，连同它的窗口标题长什么样。
- **把标题样本变成规则** —— 点一条样本就能生成对应的 `title_patterns` 正则（转义好的），
  并实时告诉你「当前 7 条样本里命中了几条」，省得写出一条永远不生效的正则。
- **常驻显示「此刻会显示成什么」** —— 这一行是 agent 用**同一套映射代码**算出来的，
  不是模拟，所见即将来会公开的内容。
- **整段粘贴** —— 把 `register-device` 打印的那几行直接粘进任意一格，四个字段自动拆好填上。

配完点「完成并退出」，配置写回 `config.yaml`（原文件会先备份成 `config.yaml.bak`），进程退出。
全程不联网，除了本机回环那个端口之外不发起任何请求。

没有 `config.yaml` 也能直接跑 `-setup`，从空白开始配；`config.yaml` 写坏了同样能跑
（坏配置会挡住常驻模式，但不会挡住修它的工具）。

> [!NOTE]
> 保存会重写整个文件：注释和键的顺序按模板重新生成，你手写的注释和排版会没。
> 想找回来就去 `config.yaml.bak`。文件的 ACL 会原样保留，不会因为保存而放宽。

### 手写 YAML

**复制 [`config.example.yaml`](config.example.yaml) 成 `config.yaml`，放在 `agent.exe` 旁边**，
把打印出来的 `server_url` / `device_id` / `token` / `interval` 四行粘进去，再补上你的映射规则。

> [!IMPORTANT]
> `config.yaml` 里有 token，按密码对待：收紧文件 ACL（属性 → 安全）只让自己可读。
> 仓库已忽略 `client-windows/config*.yaml`，只有 `config.example.yaml` 被跟踪。

默认配置路径是 **exe 所在目录**下的 `config.yaml`，而不是工作目录——双击运行和注册表自启的
工作目录都不可靠，用 exe 的位置当锚点，两种启动方式才都能找到配置。

### 配置项

| 键 | 默认值 | 说明 |
|----|--------|------|
| `server_url` | 必填 | 服务端地址，`http(s)://host[:port]`，不带路径 |
| `device_id` | 必填 | 必须与 token 绑定的设备一致，否则服务端返回 400 |
| `token` | 必填 | `register-device` 打印的 64 位 token，**机密** |
| `interval` | `10s` | 上报间隔。接受 Go duration（`10s`、`1m30s`）或纯秒数（`10`） |
| `device_name` | 空 | 只影响本地日志可读性。页面上的名字以服务端注册时为准，改名请重新 `register-device` |
| `idle_threshold` | `5m` | 多久没有键鼠输入算挂机 |
| `default_app` | `某个应用` | 没匹配上任何规则的进程显示成什么 |
| `default_description` | `使用中` | 同上，描述部分 |
| `locked_app` | `已锁屏` | 没有前台窗口（锁屏、切换用户）时显示什么 |
| `locked_description` | `人不在` | 同上，描述部分 |
| `rules` | 空 | 映射规则，见下 |
| `expose_title` | 空 | 原始标题白名单，危险，见下 |

配置校验是「大声失败」的：拼错的键名、解析不了的时长、非法的 `server_url`、编译不了的正则、
重复的 `process`，都会在启动时直接报错退出，不会静默退回默认值继续跑。

## 映射规则

```yaml
rules:
  - process: code.exe
    app: VS Code
    description: 在写代码

  - process: chrome.exe
    app: Chrome
    description: 在上网
    title_patterns:
      - match: "(?i)youtube|bilibili"
        description: 在看视频
      - match: "(?i)github"
        description: 在看代码
```

- `process` 是 exe 的文件名，大小写不敏感（`Code.exe` 与 `code.EXE` 是同一条），不允许重复。
- `app` 必填；`description` 省略时退回 `default_description`。
- `title_patterns` 可选，`match` 是 Go 正则（RE2 语法，`(?i)` 开头即忽略大小写）。按书写顺序匹配，
  第一条命中就用它的 `description`，都没命中就用规则自己的 `description`。
- **标题只参与匹配，上报的是命中项的 `description`**——`在看视频` 会被发出去，视频标题不会。
- 没写规则的进程直接走 `default_app` / `default_description`，这条路径上压根不会去读标题。

### expose_title

```yaml
expose_title: []   # 默认为空，保持这样
```

> [!CAUTION]
> 列在 `expose_title` 里的进程，会把**原始窗口标题原样、公开地**作为描述上报。
> 这是全项目唯一一个脱敏退出口。你正在浏览的网页标题、正在编辑的文件名、聊天窗口对方的昵称，
> 都会直接出现在公开页面上。

列进来的进程必须先有一条对应的 `rule`，否则启动时报错——因为没有规则的进程根本不会读标题，
配了也不会生效，而这显然不是写下这行的人想要的结果，所以直接失败而不是默默无视。

## 运行

```bash
# 可视化配置：开浏览器配规则，配完写回 config.yaml 并退出，不联网
./agent.exe -setup

# 干跑一轮：打印将要发送的脱敏载荷后退出，不联网
./agent.exe -config ./config.yaml -dry-run

# 常驻上报
./agent.exe -config ./config.yaml

# 常驻 + 调试日志
./agent.exe -config ./config.yaml -v
```

| 参数 | 说明 |
|------|------|
| `-config <path>` | 配置文件路径。缺省为 exe 同目录下的 `config.yaml` |
| `-setup` | 开本地配置界面，配完写回 `config.yaml` 后退出。不联网，与 `-dry-run` 互斥 |
| `-dry-run` | 采集并映射一轮，把脱敏后的载荷 JSON 打印到 stdout 后退出，不联网 |
| `-v` | 调试日志。会打印每轮映射后的 `{app, description}`，不会打印原始标题和 token |

`-dry-run` 的输出就是发出去的全部内容：

```json
{
  "device_id": "win-desktop",
  "device_name": "我的台式机",
  "device_type": "windows",
  "activity": {
    "app": "VS Code",
    "description": "在写代码",
    "idle": false,
    "idle_seconds": 3,
    "locked": false
  },
  "battery": null,
  "network": "wifi",
  "reported_at": "2026-07-30T12:34:56.789Z"
}
```

没有标题、没有进程路径、没有 token（token 只在 `Authorization` 头里）。
字段定义见 [`shared/contract.go`](../shared/contract.go)，那是线上格式的唯一真源。

跑起来之后打开服务端页面（默认 <http://localhost:8080>）就能看到设备卡片。

## 验证不泄漏（诱饵测试）

每次改完规则、尤其是动过 `expose_title` 之后，跑一遍这四步：

1. 把某个窗口的标题改成一眼能认出来的诱饵，比如把浏览器标签命名为 `SECRET-TITLE-CANARY`。
2. 跑 `-dry-run`：打印出来的载荷里**不能**出现这个诱饵（除非该进程确实在 `expose_title` 里）。
3. 跑 `-v` 观察几轮：日志里不能出现诱饵，也不能出现 token。
4. 打开网页：卡片上不能出现诱饵。

用过 `-setup` 之后再补两步——那是唯一会把标题读进内存并显示出来的地方：

5. `config.yaml` 和 `config.yaml.bak` 里不能出现诱饵（配置界面显示过它，但绝不写进文件）。
6. `-setup` 自己的日志（含 `-v`）里不能出现诱饵。

## 开机自启

在注册表 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` 下加一个字符串值：

- **名称：** `cyberstalk-agent`（随便起）
- **数据：** `"C:\path\to\agent.exe" -config "C:\path\to\config.yaml"`

或者在 cmd 里一条命令搞定：

```bat
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v cyberstalk-agent /t REG_SZ ^
  /d "\"C:\Tools\agent.exe\" -config \"C:\Tools\config.yaml\"" /f
```

必须用绝对路径——自启进程的工作目录不可靠。删掉这个值就取消自启。
agent 自己不碰注册表，装不装自启是你的决定。

## 断线与重试

服务端挂了、网断了、域名解析不了，agent 都不会崩，只会退避重试：

| 情况 | 判定 | 行为 |
|------|------|------|
| 网络错误 / 超时 / 5xx | 可重试 | 等待时间翻倍：`10s → 20s → 40s → 80s → 120s → 120s…`，上限 2 分钟；成功一次立刻恢复成 `interval` |
| 401（token 不对）/ 400（body 不合法） | 配置错误 | 直接退到 2 分钟上限并保持，等你改配置重启 |
| 其他状态码 | 当作配置错误 | 同上——这个端点不会返回别的状态码，多半是 `server_url` 指错了地方 |

首轮上报是立即发出的（设备马上上线），之后每 `interval` 一轮。单次 HTTP 超时 10s。
`Ctrl+C` / `SIGTERM` 会立即退出，哪怕正卡在退避等待的中途。

失败的那一轮**直接丢弃，不补发**：只有最新状态会被发出去。补发一条过期状态会污染服务端的
`last_seen_at`，把一次真实的掉线掩盖成在线。

## 开发

| 目录 | 内容 |
|------|------|
| `cmd/agent/` | main：解析 flag、装配 collect → mapping → report、处理信号 |
| `internal/collect/` | Win32 采集，`//go:build windows`，全项目唯一碰系统 API 的地方 |
| `internal/mapping/` | 脱敏边界：进程名 → `{app, description}`。纯函数，无 I/O，必须有单测 |
| `internal/config/` | YAML 加载、写回、以及「配错就不启动」的校验 |
| `internal/report/` | HTTP 客户端 + 上报循环 + 退避，只依赖 `shared` 和标准库 |
| `internal/setup/` | `-setup` 的候选表、建议正则、HTTP 接口与守卫。纯 Go，无 Win32，可在任意平台测 |
| `internal/winsetup/` | `-setup` 的接线：把 Win32 采集喂给上面那个包。**不 import `report`**，有测试盯着 |
| `webui/` | 配置界面的前端源码（React + Vite）；产物构建进 `cmd/agent/webui/` 由 exe 嵌入 |

分层是刻意这么切的：`report` 不 import `collect` 和 `mapping`，它只见得到已经脱敏的载荷；
原始标题以 `func() string` 惰性 getter 的形式从 `collect` 交给 `mapping`，从不作为字符串字段流通。
未脱敏的数据在类型层面就无处安放。

同一个道理用在了配置模式上：`winsetup` 及其依赖里没有 `report`，所以 `-setup` 这条路径在
**依赖图层面**就发不出任何请求，这比「代码里没写发送逻辑」强，因为它不会因为后来某次改动而失效。

改前端要重新构建并把产物一起提交（`cd webui && npm run build`）——`client-windows`
没有发布产物，用户是自己 `go build` 的，产物过期意味着他们拿到的是上一版界面。CI 有一道关卡盯着这件事。

质量门禁（在 Windows 上跑）：

```bash
gofmt -l .
go vet ./...
go test ./...
```

CI 有一个 `windows-latest` 的 job 会跑 `go test -race ./client-windows/...`。
Linux 那个 job 只能交叉编译 + `go vet`：`GOOS=linux` 下 `collect` 的文件全被构建约束排除，
整个模块编译都过不了。

编码约定见 [`.trellis/spec/backend/`](../.trellis/spec/backend/)。
