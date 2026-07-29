# Implement — 前端展示网站（React + Vite + TS）

> 子任务 `07-28-web`。技术方案见同目录 `design.md`；需求与验收见 `prd.md`；上游契约见 `shared/contract.go` 与 `server/internal/api/handlers.go`。
> 每阶段末有验证命令 + review gate，前一个 gate 不过不进下一阶段。

## 阶段 A · 脚手架

- [x] A.1 删除占位 `web/index.html`，用 `npm create vite@latest web -- --template react-ts` 生成项目（bash 非交互，目录须为空）。
- [x] A.2 装 Tailwind v4：`npm i tailwindcss @tailwindcss/vite`，`src/index.css` 用 `@import "tailwindcss";`；`vite.config.ts` 挂 `@tailwindcss/vite`。
- [x] A.3 配 `@` 别名：`tsconfig.json` / `tsconfig.app.json` 加 `baseUrl` + `paths`，`vite.config.ts` 加 `resolve.alias`（需 `@types/node`）。
- [x] A.4 `npx shadcn@latest init`（非交互传参），生成 `components.json` + `src/lib/utils.ts`。
- [x] A.5 `vite.config.ts` 补两项关键配置：dev proxy `/api` → `http://localhost:8080`；`build.outDir = "../server/cmd/server/web"` + `emptyOutDir: true`（design §6）。
- [x] A.6 装 `lucide-react`、`motion`；装 `vitest` 并加 `test` 脚本。
- [x] A.7 确认 ESLint flat config 含 `typescript-eslint` + `eslint-plugin-react-hooks`，且 `react-hooks/exhaustive-deps` 是 **error**（spec quality 要求，Vite 模板默认可能是 warn）。

**验证**：
```bash
cd web && npm run lint && npx tsc --noEmit && npm run build
cd .. && go build ./server/...          # embed 目录被构建产物替换后仍能编译
```
- [x] **Review gate A**：四条命令全绿；`server/cmd/server/web/` 里已是 Vite 产物；目录布局符合 `.trellis/spec/frontend/directory-structure.md`（扁平 `src/`，无 barrel）。

## 阶段 B · 契约镜像与纯逻辑（可独立测试，先于 UI）

- [x] B.1 `src/types/contract.ts`：按 design §3 表镜像 `shared/contract.go`，逐字段对照 JSON tag。`activity` 不带 null，`battery.level` 带 null。
- [x] B.2 同文件写解析守卫 `parseSnapshot(raw: unknown)` / `parseStreamEvent(raw: string)`，坏数据返回 `null`；`as` 断言只允许出现在守卫内部。
- [x] B.3 `src/lib/format.ts`：`timeAgo(iso: string)`（中文相对时间）、`formatIdle(seconds)`、网络/设备类型 → 展示文案的映射。纯函数，不 import React。
- [x] B.4 Vitest 用例：`format.test.ts`（时间边界：<60s、分、时、天、未来时间、非法串）、`contract.test.ts`（合法 update/offline、非法 type、缺 device_id、非 JSON、snapshot 非数组/含坏项）。

**验证**：
```bash
cd web && npm run test -- --run && npx tsc --noEmit && npm run lint
```
- [x] **Review gate B**：测试全绿；`contract.ts` 与 `shared/contract.go` 逐字段人工对照一遍（cross-layer 硬性检查点）；无 `any`、无 `@ts-ignore`、无 `enum`。

## 阶段 C · `useDeviceStream`

- [x] C.1 `src/hooks/useDeviceStream.ts`，返回 `{ devices, connection, error }`（design §4）。
- [x] C.2 mount：`fetch("/api/v1/snapshot")` → `parseSnapshot` → 全量替换并按 `device_id` 排序。
- [x] C.3 `new EventSource("/api/v1/stream")`；`onmessage` → `parseStreamEvent` → 函数式 updater 按 `device_id` upsert（不重复、不丢序）；坏事件 `console.warn` 丢弃。
- [x] C.4 `onopen`：首次 → `connection="live"`；**重连（非首次）→ 重拉一次 snapshot 做全量校正**（`hasConnectedRef` 区分）。`onerror` → `connection="reconnecting"`，不手动重建 EventSource。
- [x] C.5 effect cleanup 里 `es.close()`；依赖数组**不含 `devices`**；snapshot fetch 用 `AbortController` 在 cleanup 中取消。
- [x] C.6 全站只此一处做数据获取；组件层零 fetch / 零 EventSource。

**验证**：`npx tsc --noEmit && npm run lint`（`exhaustive-deps` 必须零告警），并在阶段 E 联调实测重连行为。
- [x] **Review gate C**：人工过 `.trellis/spec/frontend/hook-guidelines.md` 的 3 条 Common Mistakes；确认「重连后重拉 snapshot」代码在位。

## 阶段 D · UI 组件

- [x] D.1 `npx shadcn@latest add card badge skeleton`（生成物落 `src/components/ui/`，不手改结构）。
- [x] D.2 `src/components/DeviceCard.tsx`：设备名 `<h2>` + 类型图标、在线指示灯**配文字**「在线/离线」、活动（`app` + `description`）、空闲徽标、`BatteryIndicator`、网络图标+文案、「最后活跃 X 前」；离线 `cn("...", offline && "opacity-60 grayscale")`。
- [x] D.3 `src/components/BatteryIndicator.tsx`：`battery === null` 不渲染；`level === null` 只显示充电态；进度条宽度用内联 style（唯一允许的内联样式场景）。
- [x] D.4 `src/components/DeviceGrid.tsx`：响应式网格；`AnimatePresence` + `motion.div`（进出场 + `layout`）。
- [x] D.5 `src/App.tsx`：`<main>` 语义骨架、页头 + 连接状态徽标、loading → Skeleton、空列表 → 空态文案、30s `setInterval` tick 刷新相对时间（cleanup 必须有）。
- [x] D.6 a11y：装饰性 lucide 图标 `aria-hidden`；单独承载语义的图标给 `aria-label`；状态不靠颜色单独表达。
- [x] D.7 **主题占位**：`src/index.css` 中把 shadcn CSS 变量圈进带注释的分隔区块（`/* === theme tokens (tweakcn) === */ ... /* === end theme tokens === */`），先用默认 dark token；用户给 tweakcn 主题后整块替换。

**验证**：
```bash
cd web && npm run lint && npx tsc --noEmit && npm run test -- --run && npm run build
```
- [x] **Review gate D**：无手写 UI 基元、无内联 SVG 图标、无手写 keyframes；所有可空字段都有显式 null 分支。

## 阶段 E · 端到端联调与验收

- [x] E.1 起后端：`cd server && go run ./cmd/server`；`register-device` 造两台设备（windows + android）拿 token。
- [x] E.2 `curl` 模拟上报两台设备（一台带 battery、一台 `battery: null` 验降级），确认卡片正确渲染（**AC4**：无痕浏览器直开，无任何认证）。
- [x] E.3 持续上报一台设备并改 `activity`，确认页面**不刷新自动更新**（**AC5**）。
- [x] E.4 停止其中一台上报，等 `OFFLINE_THRESHOLD`(60s)，确认卡片在数秒内置灰 + 显示「离线」+ 最后活跃时间（**AC3 前端部分**）。
- [x] E.5 重连校正实测：上报中途 `Ctrl+C` 杀后端 → 页面进 `reconnecting` → 重启后端并在断线期间补一次上报 → 确认前端重连后经 snapshot 校正拿到断线期间的状态（design §4 关键点）。
- [x] E.6 `npm run build` 出最终产物落 `server/cmd/server/web/`，`go build ./server/...`，跑二进制访问 `http://localhost:8080` 确认 embed 页面就是新前端（不是 Vite dev server）。

- [x] **Review gate E**：`prd.md` 4 条 AC 全部实测通过（含父任务 AC4/AC5）。

## 阶段 F · 收尾（属 Trellis 3.3/3.4）

- [x] F.1 修正 `.trellis/spec/frontend/type-safety.md` 示例中与真实 Go 契约不符的 2 处（`activity` 不可为 null、`battery.level` 可为 null）。
- [x] F.2 在 `.trellis/spec/frontend/quality-guidelines.md`（或 directory-structure）记录构建接入纪律：**前端改动必须 `npm run build` 并连同 `server/cmd/server/web/` 产物一起提交**。
- [x] F.3 `.gitignore` 确认不会误忽略 `server/cmd/server/web/assets/`（当前规则只忽略 `*.exe`/`*.db`/`/server/server`，无冲突，需实跑 `git status` 确认）。
- [x] F.4 主题：用户提供 tweakcn 主题后替换 `index.css` token 区块，重跑 build 与目视验收。

## 实际执行偏差（相对本计划，均已同步进 spec）

- **A.3**：TypeScript 6 已弃用 `baseUrl`（`tsc` 直接报 TS5101 失败），最终只留 `paths`。另发现 Vite 新模板**不再默认开 `strict`**，已手动加进 `tsconfig.app.json`。
- **A.7**：`npm create vite@latest`（create-vite 9.1.1）现在生成的是 **oxlint** 而非 ESLint。实测 oxlint 已实现 `react-hooks(exhaustive-deps)`（默认 warn），故保留 oxlint 并在 `.oxlintrc.json` 提为 `error`，另开 `jsx-a11y` 插件；`src/components/ui/**` 用 `overrides` 关掉 `only-export-components`（shadcn 生成物本就导出 variants）。
- **D.7**：用户在实现中途给出 tweakcn 主题，故跳过"占位 token 再替换"，直接 `npx shadcn@latest add <tweakcn registry url>` 落地。主题字体 Plus Jakarta Sans 改为 `@fontsource-variable/*` **自托管**（避免公网站点依赖 Google Fonts，国内可访问 + 单二进制离线可用），并卸载 preset 默认引入却未使用的 Geist。
- **dev server**：Vite 默认只绑 `[::1]`，本机 IPv6 回环连不通，`vite.config.ts` 显式 `server.host = '127.0.0.1'`。
- **验证命令**：`npx tsc --noEmit` 在 project references 布局下不适用，改用 `npm run typecheck`（`tsc -b`）。

## 交付后遗留观察（不属本子任务）

- 后端 SSE **无心跳帧**。前端已能优雅处理（断线 → `reconnecting` → 重连后重拉 snapshot 校正，实测有效），但若线上反代设了 idle read timeout，会出现规律性重连。若要根治，属后端加 keepalive 注释帧的改动。

## 全局验证命令

```bash
cd web && npm run lint && npm run typecheck && npx vitest run && npm run build
cd ../server && go build ./... && go vet ./...
```

## 风险 / 回滚点

- **阶段 A 的构建接入是唯一会碰后端目录的改动**：`server/cmd/server/web/` 被覆盖。回滚 = `git checkout server/cmd/server/web`，不影响后端代码。
- **契约镜像（阶段 B）是下游一切的地基**：错了后面全错。gate B 要求逐字段人工对照，不靠感觉。
- 阶段 C/D 相互独立可分别回退；阶段 D 的主题替换（F.4）是纯 CSS 变量改动，随时可回滚。
- 若 tweakcn 主题迟迟未到：不阻塞交付，先用默认 dark token 完成全部 AC，主题作为最后一步补。
