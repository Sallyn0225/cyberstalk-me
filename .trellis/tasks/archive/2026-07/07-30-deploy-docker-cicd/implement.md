# Implement — 后端 Docker 化与 CI/CD 交付

执行顺序遵循一条原则：**能在 GitHub runner 上验证的，绝不占用 VPS 资源**（见 design §7）。因此先把 Dockerfile/compose 写好、交给 CI/CD 验证构建，最后才上 VPS 做运行时验证。每步给出验证命令，未通过不进入下一步。

> **执行位置约定**
> - 文件编辑、Go/前端命令：本地开发机
> - 镜像构建、多架构验证：**GitHub Actions**
> - 运行时链路验证：**VPS，只 `pull` 不 `build`**（唯一例外见 Step 5.2）
> - VPS 连接方式与环境实况：`.trellis/local/vps.md`（不入库，含凭据，勿复制进任何被跟踪的文件）

## Step 0 — 环境探测 ✅ 已完成（2026-07-30）

**结论：容器验证在用户自有 VPS 上做，不在本地。** 本地 Windows 开发机的 Docker Desktop 因 `hypervisorlaunchtype=Off` 起不来，且开启它会影响安卓模拟器（后续 Android 子任务要用），故不改本机启动项。

VPS 已确认：Ubuntu 22.04.5 / x86_64、Docker 29.2.1 + Compose v5.0.2 + buildx v0.31.1、用户在 `docker` 组免 sudo、**8080 空闲**、内存 3.8 Gi（可用约 3.0 Gi）、磁盘余 40G。

**用户明确要求（2026-07-30）**：机器上那三个既有服务（`cliproxyapi` / `cpa-manager-plus` / `grok2api`）**一个都不能被打扰**；若必须在 Linux 上编译，要加内存限制。→ 已据此重排验证策略（design §7）与红线（design §7.2）。

剩余待办：

- [x] 基础镜像标签确认存在（查 Docker Hub tags API：`1.26-alpine` / `1.26.5-alpine` 均在）→ design §8.1 ✅
- [x] 安全组已放行 8080（本机 `curl http://<VPS-IP>:8080/` 返回 200）✅

**回滚点 A**：此步只读，无改动。

## Step 1 — Dockerfile + .dockerignore

- [x] 新增 `.dockerignore`（design §1）
- [x] 新增 `Dockerfile`（design §1），严守：`--platform=$BUILDPLATFORM`、`GOWORK=off`、`CGO_ENABLED=0`、`GOFLAGS=-p=2`、运行阶段**零 `RUN`**
- [x] `.gitignore` 追加 `.env`

验证（**零构建开销**，只做 BuildKit 静态检查，可在 VPS 上安全执行）：
```bash
docker build --check .        # BuildKit lint，只解析不构建
```

真正的构建验证交给 Step 3 的 CI。

**回滚点 B**：`git checkout -- .gitignore && rm Dockerfile .dockerignore`

## Step 2 — compose.yaml + .env.example

- [x] 新增 `compose.yaml`（design §2）：`image` + `build` 并存、`HOST_PORT` 映射、具名卷、healthcheck
- [x] 新增 `.env.example`：`HOST_PORT` / `IMAGE_TAG` / `OFFLINE_THRESHOLD` / `SCAN_INTERVAL`，逐项注释
- [x] **不写死内存/CPU 限制**（design §7.3）——交付物要通用，验证期的限制放 VPS 本地的 `compose.override.yaml`

验证（静态，无需 daemon 干活）：
```bash
docker compose config          # 变量插值与 schema 校验
docker compose -p cyberstalk-verify config --services
```

**回滚点 C**：`rm compose.yaml .env.example`

## Step 3 — CI 工作流（Dockerfile 的第一次真实构建在这里）

- [x] 新增 `.github/workflows/ci.yml`，三个 job：`go` / `web` / `docker`（design §3）
- [x] `go` job：逐模块执行；`client-windows` 用 `GOOS=windows GOARCH=amd64`；`gofmt -l` 输出非空即失败
- [x] `web` job：`npm ci`（非 `npm install`）；`git diff --exit-code -- server/cmd/server/web` 附可操作的失败提示
- [x] `docker` job：`build-push-action`，`push: false`、单架构 amd64、GHA 缓存

本地预演 Go/前端部分（不碰 Docker）：
```bash
gofmt -l .
cd server && go vet ./... && go test ./... && go build ./cmd/server && cd ..
cd shared && go vet ./... && cd ..
cd client-windows && GOOS=windows GOARCH=amd64 go vet ./... && GOOS=windows GOARCH=amd64 go build ./... && cd ..
cd web && npm ci && npm run lint && npm run typecheck && npm run build && cd ..
git diff --stat -- server/cmd/server/web    # 期望为空 → 已提交产物是新鲜的
```
> **Vite 确定性判定（design §8.3）**：连跑两次 `npm run build`，两次之后 `git diff` 都为空 → R3.4 保持硬失败；否则改 `continue-on-error` 并回写 design.md 与 prd.md 的风险条目。

- [x] 推分支跑一次 CI，三个 job 全绿（**AC6 前半**）。`docker` job 绿 = Dockerfile 首次通过真实构建验证
- [x] **AC6 后半**：临时分支分别注入 ① `gofmt` 违规 ② TS 类型错误 ③ 改 `web/src` 但不 build，确认对应 job 各自失败；验证后丢弃该分支

**回滚点 D**：`rm .github/workflows/ci.yml`

## Step 4 — CD 工作流（AC4 多架构在这里验证）

- [x] 新增 `.github/workflows/release.yml`（design §4）：`permissions: {contents: read, packages: write}`、GHCR 登录、metadata-action tag 策略、`platforms: linux/amd64,linux/arm64`、GHA 缓存
- [x] 合入 `main` 触发一次 → 确认推出 `edge` 与 `sha-*`
- [x] **AC4 验证**：CD 的多架构构建成功即达成。**若报 `exec format error`**，说明运行阶段混进了需目标架构执行的指令（design §8.2 假设失败）→ 先回头修 Dockerfile 消除该指令；确实消除不掉才加 `docker/setup-qemu-action`，并回写 design.md 说明为何退让
- [x] 打测试 tag（如 `v0.1.0-rc.1`）→ 确认版本号 tag 正确
- [x] 在 GitHub → Packages 把包设为 **public**（一次性人工步骤，否则别人 `docker pull` 报 `denied`）
- [x] 确认实际镜像路径为全小写 `ghcr.io/sallyn0225/cyberstalk-me`；与 `compose.yaml` 写死的不一致就回改 compose 与 README
```bash
docker manifest inspect ghcr.io/sallyn0225/cyberstalk-me:edge   # amd64 + arm64 都在（本地即可查，无需 VPS）
```

**回滚点 E**：`rm .github/workflows/release.yml`；已推的包版本可在 Packages 页面删除

## Step 5 — VPS 运行时验证（AC1/AC2/AC3/AC5 + R2.2）

> **本步是全流程唯一动用 VPS 的地方。开始前先读 design §7.2 的红线。**

### 5.0 开工前后的邻居体检（每次进出 VPS 都做）

```bash
docker ps --format '{{.Names}} {{.Status}}' | grep -E 'cli-proxy-api|cpa-manager-plus|grok2api'
free -m | awk 'NR==2{print "available:", $7"Mi"}'
```
三个服务状态与开工前不一致 → **立刻停手并告知用户**，不要自行"修好"。

### 5.1 拉镜像跑起来（不构建，AC1/AC2/AC3/AC5）

```bash
# 与 README 教别人做的事完全一致
git clone https://github.com/Sallyn0225/cyberstalk-me.git ~/cyberstalk-me && cd ~/cyberstalk-me
docker compose -p cyberstalk-verify pull
docker compose -p cyberstalk-verify up -d

# AC1
curl -sS http://127.0.0.1:8080/api/v1/snapshot
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/
docker compose -p cyberstalk-verify ps          # healthy

# AC5
docker compose -p cyberstalk-verify exec app id                        # 非 0
docker compose -p cyberstalk-verify exec app sh -c 'which go || echo "no toolchain (expected)"'

# AC2
docker compose -p cyberstalk-verify exec app cyberstalk-server register-device test-dev "测试机" windows
curl -sS -X POST http://127.0.0.1:8080/api/v1/report -H "Authorization: Bearer <token>" -d '<样例 payload>'
curl -sS -X POST http://127.0.0.1:8080/api/v1/report -H "Authorization: Bearer wrong" -d '<样例 payload>'   # 期望 401
curl -sS http://127.0.0.1:8080/api/v1/snapshot                          # 出现该设备

# AC3
docker compose -p cyberstalk-verify down && docker compose -p cyberstalk-verify up -d
curl -sS -X POST http://127.0.0.1:8080/api/v1/report -H "Authorization: Bearer <同一 token>" -d '<样例 payload>'  # 仍成功
```
> 样例 payload 从 `shared/contract.go` 与 `server/internal/api/handlers_test.go` 取真实字段，不要现编。
> 外网访问（AC4 之外的 AC1 公网部分）不通时先查安全组，别改程序。

### 5.2 唯一一次 VPS 构建（R2.2「本地 build 也能用」）

**前置门槛**：`free -m` 可用内存 ≥ 1.5 Gi，否则不开始，改约时间或记为「仅 CI 验证」并如实告知用户。

```bash
docker buildx create --name cyberstalk-builder --driver docker-container \
  --driver-opt memory=1500m --driver-opt cpu-quota=150000 --use
docker buildx build --load -t cyberstalk-me:localbuild .
docker buildx rm cyberstalk-builder            # 用完立刻删
docker buildx use default
```
构建期间另开一个 ssh 会话盯着 5.0 的体检命令。

### 5.3 收摊（只用精确目标，禁止任何 prune）

```bash
docker compose -p cyberstalk-verify down -v
docker rmi cyberstalk-me:localbuild ghcr.io/sallyn0225/cyberstalk-me:edge
docker buildx ls                               # 确认 cyberstalk-builder 已消失
```
再跑一次 5.0 的体检，确认三个服务与开工前一致。是否保留一个常驻实例供长期展示，**问用户**，不要自行决定。

**回滚点 F**：`docker compose -p cyberstalk-verify down -v && rm -rf ~/cyberstalk-me`

## Step 6 — README（仓库根）

- [x] 新增 `README.md`，按 design §5 的结构写
- [x] 必含：compose 三步部署、`register-device` 完整示例（含真实输出样式）、客户端配置指向、nginx/Caddy 反代片段（**SSE 关缓冲**）、数据卷备份、本地开发、前端产物纪律
- [x] **AC8 自检**：README 部署章节的命令序列，就是 Step 5.1 实际跑过的那串 —— 两者必须一字不差地对得上，对不上以实际跑通的为准

**回滚点 G**：`rm README.md`

## Step 7 — 收尾

- [x] 逐条对照 prd.md 的 AC1–AC9 勾选，未通过项写明原因，不含糊带过
- [x] 明确报告：哪些 AC 在 CI 验证、哪些在 VPS 验证、有没有降级或跳过的
- [x] 更新父任务 `07-28-cyberstalk-me` 的相关记录（部署方式已变为 compose）
- [x] `.trellis/spec/backend/` 视情况补一条部署/构建约定（如「运行阶段零 RUN」「前端产物必须提交」）
- [x] 走 Trellis 3.3 / 3.4：spec 更新 + 提交

---

## 执行记录（2026-07-30）

### 已完成

| Step | 结果 |
|------|------|
| 0 | 两项遗留探测均已确认，见上 |
| 1 | `Dockerfile` + `.dockerignore` + `.gitignore` 追加 `.env` / `compose.override.yaml`。`docker build --check`（远端 BuildKit lint）**零告警** |
| 2 | `compose.yaml` + `.env.example`。`docker compose config` 插值与 schema 校验通过 |
| 3 | `.github/workflows/ci.yml`。本地预演 Go 门禁全过、前端 lint/typecheck/22 个用例/build 全过。**Vite 连跑两次产物 diff 为空 → R3.4 保持硬失败**（design §8.3 结论）。PR #1 上 CI 三 job 全绿 |
| 4 | `.github/workflows/release.yml`。合入 main 与打 `v0.1.0` 各触发一次，均成功，标签策略与设计一致（详见下文「合入 main 之后」） |
| 5 | VPS 运行时验证完成，见下 |
| 6 | 根 `README.md`。AC8 两轮实测按 README 逐条照抄，命令与实际跑通的一致 |
| 7 | 收尾：AC 逐条勾选、父任务记录更新、新增 `.trellis/spec/backend/deployment-guidelines.md` |

### AC6 反向验证（三轮，临时分支验完即删）

| 注入的缺陷 | 预期失败的 job / step | 实际 |
|-----------|---------------------|------|
| `server/internal/config/tmp_badfmt.go` 格式违规 | `go` / `gofmt` | ✅ 红在 `gofmt`，报错含 `run: gofmt -w <file>`；`web` `docker` 不受影响 |
| 改 `web/src/App.tsx` 文案但不 `npm run build` | `web` / `embedded frontend is up to date` | ✅ lint·typecheck·test·build 全过后红在新鲜度检查，报错含「Run 'npm run build' in web/ and commit the output」 |
| `web/src/App.tsx` 加一处 `const x: number = "str"` | `web` / `typecheck` | ✅ lint 过、红在 `typecheck`（TS2322）；`go` `docker` 仍绿 |

> 第二轮首次用 `sed` 插入时误匹配多行，oxlint 先在重复声明上挂了，没隔离出 `typecheck`。已重做一轮干净的单处注入，上表是重做后的结果。

### VPS 运行时验证结果

顺序与 implement.md 原定的相反：GHCR 尚无镜像可拉，因此先跑 Step 5.2（唯一一次 VPS 构建）产出本地镜像，再用它跑 5.1 的运行时链路。构建全程可用内存未低于 3.0 Gi，远高于 1.5 Gi 门槛。

- **AC1** ✅ `GET /` 200 且返回内嵌 HTML；`/api/v1/snapshot` 返回合法 JSON；`/api/v1/stream` 是 `text/event-stream` 且带 `X-Accel-Buffering: no`；compose healthcheck 报 `healthy`
- **AC2** ✅ `compose exec app cyberstalk-server register-device` 注册成功并打印 token（写入的正是 `/data` 卷里的库）；该 token 上报 → 204，设备出现在 snapshot；错误 token 与无 token 均 401
- **AC3** ✅ `down` 再 `up` 后同一 token 上报仍 204
- **AC5** ✅ `uid=65532 gid=65532`；容器内无 `go`、无 `/src`；镜像 **29.2 MB**
- **R2.2** ✅ VPS 本地 `docker buildx build` 成功产出可运行镜像
- **AC9** ✅ 开工前 / 构建中 / 收工三次体检，`cli-proxy-api`·`cpa-manager-plus`·`grok2api` 状态与 uptime 均未变化；可用内存 3116→3075 Mi；8080 已释放；容器/卷/builder 无残留。全程未执行任何 `prune`

### 合入 main 之后（PR #1 以 rebase 方式合入，commit `407231e` / `5acbf47` / `84df08c`）

**AC4** ✅ CD 首跑 **2m57s 一次通过**，`platforms: linux/amd64,linux/arm64`，**工作流里没有 `setup-qemu-action`**。design §8.2 的假设（运行阶段零 `RUN` ⇒ 多架构无需模拟）**成立**。镜像名被 metadata-action 自动小写为 `ghcr.io/sallyn0225/cyberstalk-me`，design §4 担心的大写 owner 问题不存在。

**标签策略** ✅ 与设计逐条对上：

| 触发 | 实际产出 |
|------|----------|
| push `main` | `edge`、`sha-84df08c` |
| tag `v0.1.0` | `0.1.0`、`0.1`、`latest`、`sha-84df08c` |

**AC7** ✅ 用户已把 GHCR 包设为 public，匿名取 registry token 即可读 manifest。

- manifest index（`application/vnd.oci.image.index.v1+json`）含 `linux/amd64` 与 `linux/arm64` 两个条目（另有两个 attestation manifest）
- VPS（x86_64）原生 `docker pull ... :latest` → `arch=amd64`
- 按 arm64 digest 精确拉取 → `arch=arm64`；把其中的二进制 copy 出来 `file` 得到
  `ELF 64-bit LSB executable, ARM aarch64, statically linked, stripped`
  —— 同时旁证了 `CGO_ENABLED=0` 静态链接与 `-ldflags="-s -w"` 裁剪都生效
- 注意：`docker pull --platform linux/arm64` 在本地已有同 tag 镜像时**不会**重新拉取，
  inspect 仍显示旧架构。要验架构就按 digest 拉，别信 `--platform` 的表象

**AC8** ✅ 分两轮达成。

第一轮发现 README 第一条 `git clone` 对外人不可用（仓库当时是 private），用 `git archive` 导出的 main 快照顶替 clone，验证了其余全部命令。用户随后把仓库转为 public，**第二轮做了真正的冷启动重跑**：

- 前置确认 VPS 上无任何本项目镜像（真冷启动，不是命中本地缓存）
- 匿名 `git clone https://github.com/Sallyn0225/cyberstalk-me.git` 成功
- `docker compose up -d` 自行从 GHCR 拉 `latest` → `healthy`
- `GET /` 200；`register-device` 输出与 README 示例逐行一致；用该配置上报 204
- `/api/v1/stream` 返回 `text/event-stream` 且带 `X-Accel-Buffering: no`
- 公网访客视角 `GET /api/v1/snapshot` 看到设备卡片

第一轮已验证的部分（下列内容两轮结论一致）：

- `docker compose up -d` → 拉 GHCR `latest` → `healthy`
- `docker compose exec app cyberstalk-server register-device win-desktop '我的台式机' windows --server-url ...`
  → 输出格式与 README 里写的示例**逐行一致**
- 用打印出的 token 上报 → 204，设备出现在 snapshot
- 公网访客视角 `http://<VPS-IP>:8080/` → 200，snapshot 含该设备
- 运维章节：`docker compose logs`、`docker compose pull && up -d`、
  备份三连（`stop` → `cp app:/data/cyberstalk.db` → `start`）逐条实测通过，备份文件 20480 字节，服务恢复正常

**未在 VPS 上实测的一项**：`.env` 的 `HOST_PORT` 改端口路径。design §7.2 红线规定只用 8080，换端口会碰其他端口。插值逻辑已由本地 `docker compose config` 覆盖。

> **两处 VPS 环境限制，与交付物无关**：VPS 直连 GitHub 会 `GnuTLS recv error`，clone 需带
> `https_proxy=http://127.0.0.1:7890`；buildkit 的代理问题见 design §9.1。外部部署者不会遇到。

### 仓库可见性与历史脱敏（已处理）

AC8 第一轮暴露出仓库还是 private，与 PRD「让**任何人**都能部署」及父任务「完全公开」的意图不符。用户选择转 public。

转公开前先处理了一处我自己造成的问题：**验证记录里写进了 VPS 的公网 IP**，而 `.trellis/local/vps.md` 明确要求其内容不得复制进任何被跟踪的文件。处理方式：

```bash
printf '<该 IP>==><VPS-IP>\n' > /tmp/replacements.txt
git filter-repo --replace-text /tmp/replacements.txt --force
```

- 三个受影响的可达 commit 已改写；那个「脱敏」commit 因替换后变成空提交被 filter-repo 自动剪掉
- `filter-repo` 会移除 `origin` remote，需手动 `git remote add` 加回
- tag `v0.1.0` 随之移到改写后的 commit 并 `--force` 重推 → **Release 重跑一次**，使镜像的
  `org.opencontainers.image.revision` 标签指向真实存在的 commit
- force push 前用 `--force-with-lease=refs/heads/main:<锚点 SHA>` 卡住，先核对远端确实在锚点上
- **诚实的边界**：旧 commit 在 GitHub 上变为不可达，但短期内仍可用精确 SHA 访问，GitHub 稍后才回收。内容仅为一个 IP，判断可接受；要彻底清除需开 support ticket 强制 GC

### CD 触发范围收窄

纯文档提交（只改 `.trellis/` 或 `.md`）原本也会重建并推一个新的 `edge` 版本。已给 `release.yml` 的 push 触发加 `paths-ignore: ['.trellis/**', '**.md']`。

**这条与 tag 发布不冲突** —— GitHub 文档明确：*"Path filters are not evaluated for pushes of tags."* 所以 `v*` 发布路径不受影响。CI 未加过滤（它是正确性门禁，多跑无害）。

---

## 全程红线

1. **不改后端业务代码**：本任务是封装与流水线，不新增 API、不动 handler。若发现必须改，先停下来与用户确认。
2. **不打扰 VPS 上的既有服务**：禁止任何 `prune`、禁止重启 daemon、禁止在他人项目目录下执行命令、只用 8080、只用 `-p cyberstalk-verify`。详见 design §7.2。
3. **不把真实 token / `.env` / VPS 凭据提交进仓库**，也不写进任何被 git 跟踪的文档。
4. **运行阶段零 `RUN`** 是多架构不依赖 QEMU 的前提，改 Dockerfile 时守住。
5. **CI 保持精简**：用户明确要求「必要的就行」，不要顺手加 golangci-lint、codecov、dependabot 之类。
