# cyberstalk-me · 赛博视奸

把「我现在在干嘛」做成一个公开网页：设备上的客户端定时上报**已在本地脱敏**的活动摘要，
服务端汇总后推给所有访客，页面近实时更新。

原始窗口标题、进程路径这些东西**永远不出设备**——上报的只有映射后的结果
（"VS Code · 在写代码"）。脱敏规则见 [`client-windows/README.md`](client-windows/README.md)。

**架构**：一个 Go 单二进制（内嵌前端 + SQLite）+ 各设备上的上报客户端。
服务端没有配置文件，全部走环境变量；持久化状态只有一个 SQLite 文件。

---

## 快速部署

需要 Docker 与 Docker Compose plugin，别的都不需要（镜像是预构建的，不用装 Go 和 Node）。

```bash
git clone https://github.com/Sallyn0225/cyberstalk-me.git
cd cyberstalk-me
docker compose up -d
```

打开 `http://<你的服务器>:8080` 就能看到页面了——此时还没有任何设备，是空的。

想改端口之类的，复制一份配置再改：

```bash
cp .env.example .env      # 编辑 HOST_PORT / IMAGE_TAG / OFFLINE_THRESHOLD / SCAN_INTERVAL
docker compose up -d
```

不复制 `.env` 也能跑，文件里写的值就是内置默认值。

### 镜像标签

| 标签 | 内容 |
|------|------|
| `latest` | 最新发布版本（推荐） |
| `0.1.0` / `0.1` | 指定版本 |
| `edge` | `main` 分支最新提交，尝鲜用 |

镜像发布在 GHCR：`ghcr.io/sallyn0225/cyberstalk-me`，支持 `linux/amd64` 与 `linux/arm64`。

---

## 注册设备

每台要上报的设备都得先在服务端注册，拿到一个专属 token：

```bash
docker compose exec app cyberstalk-server register-device win-desktop "我的台式机" windows
```

三个参数分别是设备 ID（英文短名，客户端配置要用）、显示名（页面上看到的）、类型（`windows` 或 `android`）。
输出长这样：

```
Device registered. This token is shown ONCE — copy it now:

  token: 3f8a...（64 位十六进制）

Client config (config.yaml):

  server_url: http://localhost:8080
  device_id: win-desktop
  token: 3f8a...
  interval: 10s
```

**token 只打印这一次**，服务端只存它的哈希。弄丢了就用同一个设备 ID 重新
`register-device` 一次，会换一个新 token。

加 `--server-url` 让打印出来的配置片段直接可用：

```bash
docker compose exec app cyberstalk-server register-device win-desktop "我的台式机" windows \
  --server-url https://your.domain.com
```

## 配置客户端

把上面打印的 `server_url` / `device_id` / `token` / `interval` 四行粘进客户端的
`config.yaml`（从 `client-windows/config.example.yaml` 复制一份改），然后跑起来即可。
构建与脱敏规则的完整说明见 [`client-windows/README.md`](client-windows/README.md)。

`config.yaml` 里有 token，当作密码对待——别提交进 git（仓库已经忽略了
`client-windows/config*.yaml`），也别让别人读到。

---

## 放到域名后面（反向代理）

compose 只暴露一个 HTTP 端口，TLS 和域名由你自己的反代处理。**唯一必须注意的是
SSE**：页面靠 `GET /api/v1/stream` 做实时更新，反代如果开着响应缓冲，事件会被攒在
缓冲区里发不出去，表现为「页面能打开但永远不更新」。

后端已经发了 `X-Accel-Buffering: no`，但 nginx 的 `proxy_buffering` 仍需显式关闭：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    # SSE 的三行，别省
    proxy_buffering     off;
    proxy_cache         off;
    proxy_read_timeout  1h;
}
```

Caddy：

```caddyfile
your.domain.com {
    reverse_proxy 127.0.0.1:8080 {
        flush_interval -1     # 不缓冲，SSE 逐条下发
    }
}
```

反代之后建议把 `HOST_PORT` 改成只监听本机的端口，别让 8080 直接暴露在公网。

---

## 运维

**看日志 / 状态**

```bash
docker compose ps          # 含健康检查状态
docker compose logs -f app
```

**升级**

```bash
docker compose pull && docker compose up -d
```

数据在具名卷里，升级不会丢。

**备份**

只有一个 SQLite 文件要备份。为了拿到一致的快照，先停再拷：

```bash
docker compose stop app
docker compose cp app:/data/cyberstalk.db ./cyberstalk-backup.db
docker compose start app
```

**卸载**

```bash
docker compose down        # 停服务，保留数据
docker compose down -v     # 连数据卷一起删（设备和 token 全没了，不可恢复）
```

---

## 本地开发

不用 Docker 也能跑：

```bash
cd server && go run ./cmd/server        # 默认 :8080，数据库落在 ./cyberstalk.db
```

前端开发（dev server 已把 `/api` 代理到 :8080）见 [`web/README.md`](web/README.md)。

自己构建镜像而不是拉预构建的：

```bash
docker compose build && docker compose up -d
```

质量门禁（与 CI 跑的完全一致）：

```bash
gofmt -l shared server client-windows
go vet  ./server/... ./shared/...
go test ./server/... ./shared/...
cd web && npm ci && npm run lint && npm run typecheck && npx vitest run && npm run build
```

### 一条纪律：前端产物必须跟着提交

`vite build` 的输出直接写进 `server/cmd/server/web/`，也就是 Go 二进制
`//go:embed` 的那个目录。所以：

> **改了 `web/` 下的任何东西，都必须 `npm run build` 并把
> `server/cmd/server/web/` 的产物一起提交。**

否则二进制会静默地继续服务旧界面。CI 的 `web` job 会重新构建并 diff 这个目录，
不新鲜就直接失败——不会让你带着旧产物合进去。

---

## 项目结构

| 目录 | 内容 |
|------|------|
| `server/` | Go 后端：HTTP API、SSE、SQLite、内嵌前端 |
| `web/` | React + Vite 前端，构建产物写进 `server/cmd/server/web/` |
| `client-windows/` | Windows 上报客户端（含脱敏规则引擎） |
| `shared/` | 前后端共用的数据契约，唯一真源 |
