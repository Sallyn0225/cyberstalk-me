# web — 赛博视奸展示前端

公开只读 SPA：设备卡片网格，首屏 `GET /api/v1/snapshot` + `GET /api/v1/stream`
(SSE) 近实时更新。React + Vite + TypeScript，UI 用 shadcn/ui + Tailwind v4。

约定见 `.trellis/spec/frontend/`。

## 本地开发

```bash
# 1. 另开一个终端跑后端（默认 :8080）
cd ../server && go run ./cmd/server

# 2. 前端 dev server，/api 已配代理到 :8080
npm run dev        # http://127.0.0.1:5173
```

造测试数据：

```bash
cd ../server
go run ./cmd/server register-device win-desktop "我的台式机" windows   # 打印一次性 token
curl -X POST http://localhost:8080/api/v1/report \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  --data-binary @payload.json
```

> Windows 下用 `--data-binary @文件` 传含中文的 body，直接 `-d '...'` 会被
> 控制台代码页转码搞坏。

## 质量门禁（提交前必须全过）

```bash
npm run lint        # oxlint
npm run typecheck   # tsc -b
npx vitest run      # 纯逻辑单测（format / contract 解析守卫）
npm run build       # tsc -b && vite build
```

## 构建与部署（重要）

`vite build` 的产物**直接写入 `../server/cmd/server/web/`**，也就是 Go 服务
`//go:embed all:web` 嵌入的目录。因此：

> **前端一有改动，必须 `npm run build` 并把产物一起提交**，否则编出来的二进制
> 里仍是旧页面。

发布 = `npm run build && go build ./cmd/server` → 单个可执行文件。

## 主题

配色取自 tweakcn 主题，通过 `npx shadcn@latest add <theme-registry-url>` 写入
`src/index.css`。换主题重跑该命令，不要手改色值。页面固定深色模式
（`index.html` 上的 `class="dark"`）。
