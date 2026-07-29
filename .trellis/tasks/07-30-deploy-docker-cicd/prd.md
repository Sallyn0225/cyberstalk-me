# 后端 Docker 化与 CI/CD 交付

## Goal

把 `server`（Go 单二进制 + 内嵌前端 + SQLite）封装成 Docker 镜像，提供一份 `docker compose` 配置，让**任何人**在自己的服务器上执行两三条命令就能跑起这个网站；同时给仓库配置 GitHub Actions：CI 做必要的质量门禁，CD 产出并发布多架构镜像。

## Background / Confirmed Facts

- 后端 `server/` 是纯 Go 单二进制：SQLite 用 `modernc.org/sqlite`（**纯 Go，无 CGO**），前端构建产物已提交进 `server/cmd/server/web/` 并由 `//go:embed all:web` 打进二进制。→ 镜像构建**不需要 Node 阶段**，`go build` 即出「带最新前端的完整服务」。
- 配置全部走环境变量（`server/internal/config/config.go`）：`ADDR`（默认 `:8080`）、`SQLITE_PATH`（默认 `cyberstalk.db`）、`OFFLINE_THRESHOLD`（60s）、`SCAN_INTERVAL`（5s）。无配置文件。
- 唯一的持久化状态是那一个 SQLite 文件。
- 二进制还带一个管理子命令：`register-device <id> <name> <type> [--server-url URL] [--sqlite-path PATH]`，部署者必须能在容器里跑它来注册设备、拿到 token。
- 仓库是 Go workspace（`go.work` 含 `shared`/`server`/`client-windows`），`server/go.mod` 里有 `replace cyberstalk.me/shared => ../shared`。
- 远端仓库：`github.com/Sallyn0225/cyberstalk-me`，当前**无任何 `.github/` 配置**。
- 仓库根目前**没有 README**，别人 clone 下来看不到任何部署说明。

## Key Decisions（已与用户确认）

| 维度 | 决定 |
|------|------|
| 镜像仓库 | **GHCR**：`ghcr.io/sallyn0225/cyberstalk-me`，CI 用内置 `GITHUB_TOKEN` 推送，部署者无需登录即可拉取公开镜像 |
| compose 组成 | **仅 app 单容器**，映射端口 + 一个数据卷。TLS / 反向代理由部署者自理（README 给 nginx/Caddy 片段提示，尤其是 SSE 不能被缓冲） |
| 目标架构 | **linux/amd64 + linux/arm64** 多架构 manifest |
| CI 范围 | Go（`server` + `shared` + `client-windows` 交叉编译）、Web（lint / typecheck / build）、Docker 镜像可构建性。保持精简，不引入额外 lint 工具链 |
| CD 触发 | 推 tag `v*` 发布正式版本号镜像 + `latest`；推 `main` 发布 `edge` 滚动镜像；支持手动触发 |

## Requirements

### R1 Docker 镜像
- R1.1 仓库根提供 `Dockerfile`，多阶段构建：Go 编译阶段 → 极小运行阶段，最终镜像只含单二进制与数据目录。
- R1.2 通过 Go 交叉编译支持 `linux/amd64` 与 `linux/arm64`，同一份 Dockerfile 两种架构都能构建。
- R1.3 容器以**非 root** 用户运行，监听 `:8080`，SQLite 落在 `/data` 卷内。
- R1.4 提供 `.dockerignore`，构建上下文不包含 `node_modules`、`.trellis/`、`.git/`、本地 `*.db` 等无关内容。
- R1.5 部署者能在容器内执行 `register-device` 子命令注册设备并拿到 token，且该 token 写入的是持久化卷里的同一个数据库。

### R2 docker compose 部署体验
- R2.1 仓库根提供 `compose.yaml`：单服务、命名数据卷、端口映射、`restart: unless-stopped`、健康检查。
- R2.2 默认从 GHCR 拉预构建镜像；同时保留本地 `build:` 配置，`docker compose build` 也能自己构建。
- R2.3 提供 `.env.example`，覆盖可调项（对外端口、离线阈值、扫描间隔等），复制成 `.env` 即可生效；不复制 `.env` 也能用默认值直接启动。
- R2.4 数据卷保证：`docker compose down` 再 `up` 之后，已注册设备与 token 依然有效。

### R3 CI（质量门禁）
- R3.1 `push` 到 `main` 与所有 PR 触发。
- R3.2 Go 检查：`gofmt` 格式一致性、`go vet`、`go test`（`server` + `shared`）、`server` 可构建；`client-windows` 以 `GOOS=windows` 交叉编译 + vet 保证不被破坏。
- R3.3 Web 检查：`npm ci` + `oxlint` + `tsc` typecheck + `vite build`。
- R3.4 内嵌产物新鲜度检查：前端构建后 `server/cmd/server/web/` 若与已提交产物不一致则 CI 失败——直接堵住「改了前端忘记 build 就提交」这个已知风险。
- R3.5 Docker 镜像可构建性检查（单架构、不推送）。
- R3.6 CI 全绿是「可发布」的判据；失败原因在 job 名称上一眼可辨。

### R4 CD（镜像发布）
- R4.1 推 `v*` tag → 构建并推送多架构镜像，打上语义化版本 tag（`v1.2.3` / `1.2` / `latest`）。
- R4.2 推 `main` → 推送 `edge` tag，便于尝鲜与自测。
- R4.3 支持 `workflow_dispatch` 手动触发。
- R4.4 使用 GitHub Actions 缓存加速重复构建；使用最小权限（`contents: read`, `packages: write`）。

### R5 部署文档
- R5.1 仓库根新增 `README.md`：项目简介 + **一键部署章节**（拉起服务 → 注册设备 → 拿 token → 配置客户端 → 打开网页）。
- R5.2 明确写出反向代理注意事项：SSE 必须关闭缓冲（`proxy_buffering off` / `X-Accel-Buffering`）。
- R5.3 明确写出「前端改动必须 `npm run build` 并提交产物」这条纪律，并说明 CI 会强制校验。

## Acceptance Criteria

> 验收执行于 2026-07-30。证据与命令见 `implement.md`「执行记录」。

- [x] **AC1** ✅ 干净检出 `docker compose up -d` 后 healthy；`GET /` 200 返回内嵌 HTML，`/api/v1/snapshot` 返回合法 JSON；公网 `http://<VPS-IP>:8080/` 访客视角同样 200。
- [x] **AC2** ✅ `docker compose exec app cyberstalk-server register-device` 注册成功并打印 token；该 token 上报 → 204 且设备出现在 snapshot；错误 token 与无 token 均 401。
- [x] **AC3** ✅ `down` 再 `up` 后同一 token 上报仍 204，设备数据完整。
- [x] **AC4** ✅ CD 一次通过（2m57s），`platforms: linux/amd64,linux/arm64`，**未使用 QEMU** —— 「运行阶段零 `RUN`」的推论成立。GHCR manifest index 同时含 amd64 与 arm64 条目。
- [x] **AC5** ✅ `uid=65532 gid=65532`；容器内无 `go`、无 `/src`；镜像 29.2 MB。
- [x] **AC6** ✅ 正反都验：main 上 CI 三 job 全绿；三处人为缺陷分别让 `go`/`gofmt`、`web`/`typecheck`、`web`/`embedded frontend is up to date` 各自变红，无关 job 保持绿。
- [x] **AC7** ✅ 包已公开，匿名可拉。amd64 原生拉取 `arch=amd64`；按 arm64 digest 拉取 `arch=arm64`，其二进制经 `file` 确认为 `ELF 64-bit ARM aarch64, statically linked, stripped`。tag 策略产出 `0.1.0` / `0.1` / `latest` / `sha-*`，与设计一致。
- [ ] **AC8** ⚠️ **部分达成 —— 阻塞在仓库可见性**。README 部署章节的**命令序列**已在 VPS 上从干净检出端到端跑通（up → 页面可访问 → register-device → 上报 → 公网访客看到设备卡片），运维章节的看日志 / 升级 / 备份三组命令也逐条实测通过。**但第一条 `git clone` 对外人跑不通：仓库当前是 private**（`gh repo view` 确认 `visibility=PRIVATE`）。验证时用 `git archive` 导出 main 快照代替 clone。**仓库转公开后此项即自动成立，是否转公开由用户决定。**
- [x] **AC9** ✅ 三个既有服务全程状态与 uptime 未变；可用内存 3116 → 2986 Mi；8080 已释放；容器 / 卷 / 镜像 / builder / 工作目录零残留；**全程未执行任何 `prune`**。

## Out of Scope

- Kubernetes / Helm chart。
- compose 内置 TLS 终止或自动签证书（Caddy/Traefik 反代不在本轮，README 给指引即可）。
- 把 Windows / Android 客户端二进制作为 GitHub Release 产物发布。
- 监控、告警、日志聚合、备份策略（README 可提一句数据卷备份，但不做实现）。
- 引入 golangci-lint 等重型 lint 工具链（用户要求 CI 保持精简）。
- 修改后端业务逻辑；本任务不新增 API 端点（健康检查复用现有 `/api/v1/snapshot`）。

## 验证环境约束（用户 2026-07-30 明确要求）

容器验证在用户自有 VPS 上进行（本地开发机无法跑 Docker，原因见 design §7）。该机器 3.8 Gi 内存、同时跑着三个与本项目无关的生产服务，用户要求**一个都不能被打扰**、**必须在 Linux 上编译时要加内存限制**。由此定下：

- 镜像构建与多架构验证放 GitHub Actions，VPS 上**只 `pull` 不 `build`**；仅「本地 build 也能用」这一条在 VPS 上验证一次，且带 builder 内存上限并设可用内存门槛。
- VPS 操作全程禁止任何 `prune`、禁止重启 daemon、只用 8080、只用独立 compose project 名。
- 交付给别人的 `compose.yaml` **不写死资源限制**，验证期的限制只存在于 VPS 本地的 `compose.override.yaml`（不入库）。

细则见 design §7。

## Risks

- **验证环境是别人的生产机**：任何 `docker prune`、端口抢占或内存打满都会伤及三个既有服务。缓解：红线清单（design §7.2）、开工前后邻居体检、构建前内存门槛、精确目标清理。这是本任务**最高优先级的风险**——搞砸它的代价远大于晚交付。
- **多架构构建拖慢 CD**：若运行阶段出现需要在目标架构执行的 `RUN` 指令，就必须挂 QEMU 模拟，构建耗时成倍增长。缓解：运行阶段设计成零 `RUN`（见 design）。
- **内嵌产物一致性检查误报**：Vite 构建若非确定性（依赖浮动、构建时间戳注入），R3.4 会随机红。缓解：`npm ci` 锁死依赖；若实测仍不稳定，降级为「产物落后于源码时告警」而非硬失败，并在 design/implement 记录结论。
- **GHCR 首次推送包默认为 private**：部署者会遇到 `denied`。缓解：README 写明发布后需在 GitHub Packages 把包设为 public，并把该步骤列入交付 checklist。
- **SQLite 卷权限**：非 root 用户对具名卷的写权限依赖镜像内目录属主。缓解：镜像内预建 `/data` 并 `--chown` 给运行用户，AC3 实测覆盖。
