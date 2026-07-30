# Design — 后端 Docker 化与 CI/CD 交付

## 0. 现状约束（决定了整个方案的形状）

| 事实 | 出处 | 推论 |
|------|------|------|
| SQLite 驱动是 `modernc.org/sqlite`（纯 Go） | `server/go.mod` | `CGO_ENABLED=0` 可静态编译 → **交叉编译零成本**，多架构不需要 QEMU 编译 |
| 前端产物已提交进 `server/cmd/server/web/`，由 `//go:embed all:web` 打包 | `server/cmd/server/main.go:40` | 镜像构建**不需要 Node 阶段**，`go build` 即出完整服务 |
| 配置纯环境变量，无配置文件 | `server/internal/config/config.go` | compose 只需 `environment:` / `env_file:`，无需挂配置文件 |
| 唯一持久状态 = 一个 SQLite 文件 | `server/internal/store/` | 一个具名卷挂 `/data` 即可 |
| `server/go.mod` 有 `replace cyberstalk.me/shared => ../shared` | `server/go.mod` | 构建上下文必须是**仓库根**（要能看到 `shared/`）；不需要 `go.work` |
| `go.work` 含 `client-windows`，其中有 `//go:build windows` 文件 | `client-windows/internal/collect/collect_windows.go:1` | 镜像里若带 `go.work`，Linux 下 workspace 解析会牵扯 windows 模块 → **`GOWORK=off`**；CI 里 client-windows 必须 `GOOS=windows` 才能 vet/build |
| 无健康检查端点 | `server/internal/api/router.go` | 健康检查复用 `GET /api/v1/snapshot`（公开、无需 token，且会读一次 store，能同时验证 DB 可用），不新增端点 |

## 1. Dockerfile

仓库根 `Dockerfile`，两阶段：

```dockerfile
# ---- build（永远跑在 native 架构上）----
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETARCH
WORKDIR /src
# 先拷模块文件，最大化 layer 缓存
COPY shared/go.mod shared/
COPY server/go.mod server/go.sum server/
WORKDIR /src/server
ENV GOWORK=off CGO_ENABLED=0 GOOS=linux
RUN go mod download
WORKDIR /src
COPY shared/ shared/
COPY server/ server/
WORKDIR /src/server
RUN GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/cyberstalk-server ./cmd/server
# 预建数据目录，属主给运行用户（避免运行阶段出现 RUN）
RUN mkdir -p /out/data

# ---- runtime ----
FROM alpine:3.21
COPY --from=build /out/cyberstalk-server /usr/local/bin/cyberstalk-server
COPY --from=build --chown=65532:65532 /out/data /data
ENV ADDR=:8080 SQLITE_PATH=/data/cyberstalk.db
EXPOSE 8080
USER 65532:65532
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/cyberstalk-server"]
```

**关键设计点：**

1. **`--platform=$BUILDPLATFORM` + `GOARCH=$TARGETARCH`**：编译阶段始终在 runner 的原生架构跑，靠 Go 交叉编译产出目标架构二进制。arm64 镜像的构建速度与 amd64 相同。
2. **运行阶段零 `RUN`**：只有 `COPY` / `ENV` / `USER`。BuildKit 处理 `COPY --chown` 无需在目标架构执行任何指令 → **多架构构建不需要 QEMU**。这是「用数字 UID `65532:65532` 而不是 `adduser`」的唯一理由，也是必须守住的约束：**日后若在运行阶段加了 `RUN`，CD 就得补 `docker/setup-qemu-action`**。
3. **底座选 alpine 而非 distroless**：部署者要用 `docker compose exec` 跑 `register-device`、要能 `wget` 做健康检查、出问题要能进容器看 `/data`。alpine 多出的几 MB 换这些运维能力，值。
4. **`GOWORK=off`**：镜像里不拷 `go.work`，只靠 `server/go.mod` 的 `replace ../shared`。构建上下文是仓库根，但只 `COPY shared/ server/` 两个目录。
5. **`-trimpath -ldflags="-s -w"`**：去掉本机路径与调试符号，减小体积、构建可复现。
6. **不打 `HEALTHCHECK` 进镜像**，healthcheck 放 compose 层（部署者可覆盖，且用镜像的人不一定要这套语义）。

`.dockerignore`（白名单思路的反面：只排干净噪音，保持简单）：
```
.git
.github
.trellis
.agents
.claude
.codex
.pi
web/node_modules
web/
client-windows/
*.db
*.db-shm
*.db-wal
*.exe
```
> 注意 `web/` 可以整目录排除——镜像用的是已提交进 `server/cmd/server/web/` 的构建产物，源码不参与镜像构建。`client-windows/` 同理不参与。

## 2. compose.yaml

```yaml
services:
  app:
    image: ghcr.io/sallyn0225/cyberstalk-me:${IMAGE_TAG:-latest}
    build: .                      # 保留本地构建能力：docker compose build
    restart: unless-stopped
    ports:
      - "${HOST_PORT:-8080}:8080"
    environment:
      OFFLINE_THRESHOLD: ${OFFLINE_THRESHOLD:-60s}
      SCAN_INTERVAL: ${SCAN_INTERVAL:-5s}
    volumes:
      - cyberstalk-data:/data
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/api/v1/snapshot"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 5s
volumes:
  cyberstalk-data:
```

**设计点：**

- **`image:` 与 `build:` 并存**：默认 `docker compose up -d` 直接拉 GHCR 预构建镜像（部署者零构建）；`docker compose build` 则本地构建并打成同名 tag。这是「给别人用」与「自己开发」两种诉求的最低成本兼容。
- **容器内监听固定 `:8080`**，对外端口通过 `HOST_PORT` 映射。不让部署者去改 `ADDR`——那会同时打乱 healthcheck 与端口映射。`.env.example` 里也不暴露 `ADDR`。
- **所有变量都有 `:-默认值`**：没有 `.env` 文件也能直接 `up`（对应 R2.3）。
- **具名卷而非 bind mount**：跨平台、免去宿主目录权限问题；README 说明备份方式（`docker run --rm -v cyberstalk-data:/data ...` 或 `docker compose cp`）。
- **healthcheck 用 `wget`**（alpine busybox 自带，无需装 curl），打 `/api/v1/snapshot` 而非 `/`——后者只证明静态文件在，前者顺带证明 DB 可读。

`.env.example`：`HOST_PORT`、`IMAGE_TAG`、`OFFLINE_THRESHOLD`、`SCAN_INTERVAL`，每项带注释与默认值。

## 3. CI：`.github/workflows/ci.yml`

触发：`push: [main]` + `pull_request` + `workflow_dispatch`。三个并行 job，命名让失败一眼可辨：

| job | 步骤 | 覆盖 |
|-----|------|------|
| `go` | `setup-go`（`go-version-file: server/go.mod`，开启 module cache）→ `gofmt -l`（非空即失败）→ `cd server && go vet ./... && go test ./...` → `cd shared && go vet ./...` → `go build ./cmd/server` → `GOOS=windows GOARCH=amd64 go vet/build`（client-windows） | R3.2 |
| `web` | `setup-node`（`cache: npm`）→ `npm ci` → `npm run lint` → `npm run typecheck` → `npm run build` → `git diff --exit-code -- server/cmd/server/web` | R3.3、R3.4 |
| `docker` | `docker/setup-buildx-action` → `docker/build-push-action`（`push: false`、`load: false`、单架构 `linux/amd64`、GHA 缓存） | R3.5 |

**设计点：**

- **Go 命令逐模块执行、不依赖 `go.work`**：`client-windows` 在 Linux runner 上必须 `GOOS=windows`，混在 workspace 的 `./...` 里会直接炸（`build constraints exclude all Go files`）。`shared` 无测试文件时只 vet。
- **`gofmt` 而不是 golangci-lint**：用户明确要求 CI 精简。`gofmt -l .` 输出非空即失败，零依赖、零配置、零误报。
- **R3.4 的实现是 `git diff --exit-code`**：`vite build` 的 `outDir` 就是 `server/cmd/server/web`（`web/vite.config.ts:107`），所以 build 完直接比对工作区脏不脏即可判断「提交的产物是否新鲜」。失败信息里提示「请在本地 `npm run build` 后提交产物」。
  - **风险与回退**：若实测发现 Vite 产物含非确定内容（时间戳、随机 hash），此 step 改为 `continue-on-error: true` + 明确告警，并把结论写回本文件。判定标准：同一 commit 连跑两次 CI，该 step 结果必须一致。
- **docker job 只构 amd64 且不推送**：PR 阶段要的是「Dockerfile 没写坏」的快速反馈，多架构留给 CD。

## 4. CD：`.github/workflows/release.yml`

触发：`push: tags: ['v*']` + `push: branches: [main]` + `workflow_dispatch`。

```yaml
permissions:
  contents: read
  packages: write
```

步骤：`setup-buildx` → `docker/login-action`（`ghcr.io`，`${{ github.actor }}` + `${{ secrets.GITHUB_TOKEN }}`）→ `docker/metadata-action` → `docker/build-push-action`（`platforms: linux/amd64,linux/arm64`、`push: true`、`cache-from/to: type=gha`）。

tag 策略（metadata-action）：

| 触发 | 产出 tag |
|------|----------|
| `v1.2.3` | `1.2.3`、`1.2`、`latest` |
| push `main` | `edge`、`sha-<short>` |
| 手动 | `sha-<short>` |

**设计点：**

- **镜像名用 `${{ github.repository }}` 小写化**：GHCR 拒绝大写路径，而仓库 owner 是 `Sallyn0225`。用 metadata-action 的 `images: ghcr.io/${{ github.repository }}` 并配合小写化（metadata-action 会自动小写化 image 名，无需手工 `tr`）——实现时验证一次实际推出的路径是 `ghcr.io/sallyn0225/cyberstalk-me`。
- **不需要 `setup-qemu-action`**：见 §1 设计点 2。这条依赖必须在 implement 阶段用一次真实多架构构建验证。
- **GHCR 包可见性**：首次推送后包默认 private，需在 GitHub → Packages → Package settings 手动设为 public，否则别人 `docker pull` 报 `denied`。这是一次性人工步骤，写进 README 与交付 checklist。
- **不做 GitHub Release / 客户端二进制附件**（out of scope）。

## 5. README（仓库根）

结构：项目一句话简介 → 架构图（一句话：Go 单二进制内嵌前端 + SQLite + 各设备上报客户端）→ **快速部署**（compose 三步）→ **注册设备**（`docker compose exec`）→ **配置客户端** → **反向代理注意事项** → **本地开发** → **数据备份** → **开发纪律**。

两处必须写清、否则部署者一定踩坑：

1. **反向代理与 SSE**：nginx 需 `proxy_buffering off; proxy_read_timeout 1h;`；后端已发 `X-Accel-Buffering: no`，但 `proxy_buffering` 仍需显式关闭。给可直接粘贴的 nginx location 片段与 Caddy 单行反代。
2. **前端产物纪律**：`web/` 改动后必须 `npm run build` 并把 `server/cmd/server/web/` 的产物一起提交，CI 会硬校验。

## 6. 兼容性与回滚

- **纯增量**：本任务只新增文件（`Dockerfile`、`.dockerignore`、`compose.yaml`、`.env.example`、`.github/workflows/*`、`README.md`），**不改动任何现有 Go / TS 源码**，因此对已部署的裸二进制部署方式零影响。
- `.gitignore` 需追加 `.env`（真实环境变量不入库），这是唯一对既有文件的改动。
- **回滚**：删除新增文件即可回到当前状态；已推送的 GHCR 镜像可在 Packages 页面删除版本。

## 7. 验证环境策略：构建放 CI，VPS 只跑

验证环境是用户自有 VPS（连接方式见 `.trellis/local/vps.md`，不入库）。这台机器上**同时跑着三个与本项目无关的生产服务**（`cliproxyapi` / `cpa-manager-plus` / `grok2api`），用户明确要求**一个都不能被打扰**。机器只有 3.8 Gi 内存、约 3.0 Gi 可用。

由此定下验证分工 —— **能在 GitHub runner 上做的，绝不在 VPS 上做**：

| 验收项 | 在哪验 | 理由 |
|--------|--------|------|
| AC4 多架构构建 | **GitHub Actions（CD）** | runner 有 7G 内存且是镜像真正的产出地，比 VPS 上手搓 builder 更权威 |
| Dockerfile 可构建性 | **GitHub Actions（CI docker job）** | 每次改动自动验证，零 VPS 开销 |
| AC1/AC2/AC3 运行时链路 | VPS | 必须在真实 Linux 上跑，但**只 `pull` 不 `build`**，运行期常驻内存只有 Go 服务的几十 MB |
| AC5 非 root / 镜像内容 | VPS（对 pull 下来的镜像做 inspect） | 只读检查，无开销 |
| R2.2「本地 build 也能用」 | VPS，**仅一次**，带内存限制 | 这条路要交付给别人，必须真验过一次，不能只靠 CI 的 build 推断 |

### 7.1 那唯一一次 VPS 构建的资源约束

`modernc.org/sqlite` 是编译大户（C 转译产物），Go 并行编译的内存峰值可能上 GB。三重限制：

```bash
# builder 容器本身限内存（buildx docker-container driver 支持 memory driver-opt）
docker buildx create --name cyberstalk-builder --driver docker-container \
  --driver-opt memory=1500m --driver-opt cpu-quota=150000

# Dockerfile 内再压一层 Go 编译并发（-p 限制并行编译包数）
ENV GOFLAGS=-p=2
```

构建**前后**都要确认三个既有服务健在：

```bash
docker ps --format '{{.Names}} {{.Status}}' | grep -E 'cli-proxy-api|cpa-manager-plus|grok2api'
free -m | awk 'NR==2{print "available:", $7"Mi"}'
```

可用内存低于 1.5 Gi 就**不要开始构建**，改约时间或直接放弃这条验证（记为「仅 CI 验证」并如实告知用户）。

### 7.2 不干扰既有服务的硬性红线

| 禁止 | 原因 |
|------|------|
| `docker system prune` / `image prune` / `builder prune`（任何形式） | 会连带删掉那三个项目的镜像层与构建缓存 |
| `docker network prune` / `volume prune` | 同上，会误删他人网络与数据卷 |
| 重启或重配 Docker daemon | 会一并重启他人容器 |
| 占用 8080 以外的端口 | 8080 已确认空闲；其余端口一律视为他人占用 |
| 在他人目录（`~/CLIProxyAPI`、`~/cpa-manager-plus`、`~/grok2api`）下执行任何命令 | 避免 compose 误作用到他人项目 |

隔离手段：

- 独立工作目录 `~/cyberstalk-me`（VPS 上 `git clone`，与 README 里让部署者做的事完全一致）
- 独立 compose project 名：所有命令带 `-p cyberstalk-verify`
- 独立卷名（compose 具名卷会自动带 project 前缀，天然隔离）
- 清理只用**精确目标**：`docker compose -p cyberstalk-verify down -v`、`docker rmi <明确的镜像ID>`、`docker buildx rm cyberstalk-builder`

### 7.3 交付物不因验证环境而变形

给别人用的 `compose.yaml` **不写死内存限制**（别人的机器可能大得多）。VPS 验证期若需要限制，用 VPS 本地的 `compose.override.yaml`（不提交），Compose 会自动叠加。交付物保持通用，验证约束留在验证环境里。

## 8. 待实现阶段验证的假设

1. ✅ **`golang:1.26-alpine` 存在**（2026-07-30 查 Docker Hub tags API 确认，同时有 `1.26.5-alpine`）。无需退化。
2. 多架构构建在无 QEMU 情况下成功（验证运行阶段零 `RUN` 的推论）—— 待 CD 验证。
3. ✅ **`vite build` 产物确定性成立**：本地连跑两次 `npm run build`，`server/cmd/server/web/` 的 `git status --porcelain` 两次均为空（资源文件名是内容 hash，无时间戳注入）。→ **R3.4 保持硬失败**，不降级为告警。
4. `65532` 用户对具名卷 `/data` 有写权限 —— 待 VPS 运行时验证。

## 9. 实现阶段的设计偏离（2026-07-30，均已在此定稿）

| 项 | 原设计 | 实际 | 理由 |
|----|--------|------|------|
| 运行阶段底座 | `alpine:3.21` | **`alpine:3.24`** | 3.24 是当前稳定线，3.21 已接近生命周期尾声。二进制静态链接，换底座无兼容风险 |
| Go 编译并发限制 | Dockerfile 里写死 `ENV GOFLAGS=-p=2` | **`ARG GOFLAGS=""`**，VPS 构建时用 `--build-arg GOFLAGS=-p=2` 传入 | 与 §7.3「交付物不因验证环境而变形」一致。写死会让所有人的构建永久变慢，只为服务一次性的 VPS 验证 |
| CI 的 Go 命令形态 | 逐模块 `cd` 执行 | **仓库根 + 显式模块路径**（`go vet ./server/... ./shared/...`） | 与 `.trellis/spec/backend/quality-guidelines.md` 已确立的门禁形态一字对齐，避免 CI 与本地规范各说各话。`client-windows` 仍单独用 `GOOS=windows` 跑 |
| CI web job | lint / typecheck / build | 追加 **`npx vitest run`** | `web/src/` 下有 2 个测试文件（22 个用例），且 `.trellis/spec/frontend/quality-guidelines.md` 已把 `vitest run` 列为必过门禁。这是复用既有门禁，不是新增工具链，不违反「CI 精简」 |
| Actions 版本 | 未定 | checkout@v7 / setup-go@v7 / setup-node@v7 / setup-buildx@v4 / build-push@v7 / login@v4 / metadata@v6 | 实现时查 GitHub releases 取当前大版本 |

### 9.1 VPS 网络的构建适配（仅验证环境，不入交付物）

VPS 在国内，且 Docker daemon 的代理配置（`~/.docker/config.json` 的 `proxies.default` 指向 `172.17.0.1:7890`，mihomo）**不会被 `docker-container` driver 的 buildkit 容器继承**。踩到两层：

1. buildkit 自身拉基础镜像 → 需建 builder 时注入：`--driver-opt env.http_proxy=... --driver-opt env.https_proxy=...`
   （注意 `--driver-opt` 按逗号切分，`no_proxy=localhost,127.0.0.1` 会报 `invalid value ... expecting k=v`，只能省略或单值）
2. `RUN` 步骤里 `go mod download` 打 `proxy.golang.org` → 需构建时传 BuildKit **预置** build-arg：
   `--build-arg HTTP_PROXY=... --build-arg HTTPS_PROXY=...`（预置 arg 无需在 Dockerfile 里 `ARG` 声明，也不进构建缓存 key）

**这是 VPS 的网络环境问题，不是 Dockerfile 缺陷** —— GitHub runner 直连，CI/CD 里不需要任何代理配置。交付物因此保持零代理痕迹。
