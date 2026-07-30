# 执行计划：前端屏幕使用时间视图

需求见本任务 `prd.md`；技术设计见父任务 `.trellis/tasks/07-30-screen-time/design.md` §8。
顺序按「纯函数 → 契约校验 → hook → 组件 → 装配」，前三步可脱离 UI 单测。

## 有序清单

### 阶段 A：纯逻辑（可先单测）

- [ ] A1. `web/src/lib/format.ts`：新增时长格式化（如 `formatDuration(seconds)` → `4h 32m` / `12m` / `0m`）。
      沿用文件里既有风格：纯函数、无 React、非有限值与负数有兜底（参考 `formatIdle` 的 `!Number.isFinite` 处理）。
- [ ] A2. `web/src/lib/format.test.ts`：AC3.8 的边界用例。
- [ ] A3. `web/src/types/contract.ts`：镜像父 `design.md` §2.2 的类型 +
      `isUsageResponse` / `parseUsage`。
      **校验从宽**：只校验结构，不校验 `window` / `state` 的字符串取值；
      `hourly` / `daily` 校验为 `null | Array`。风格照抄既有 `parseSnapshot`。
- [ ] A4. `web/src/types/contract.test.ts`：合法载荷通过、`hourly` 为 null 通过、
      `daily` 为 null 通过、缺字段返回 `null`、畸形嵌套返回 `null`（AC3.7）。

### 阶段 B：数据获取

- [ ] B1. `web/src/hooks/useUsage.ts`：`useUsage(window)` 返回 `{ data, loading, error }`。
      - `fetch('/api/v1/usage?window=' + window)`，`AbortController` 取消上一次（R5.6）
      - 解析走 `parseUsage`，`null` 时置错误态（AC3.5）
      - **不加 SSE**、不加轮询（父 D8）
      - 组件按 tab 条件渲染即可控制何时请求，hook 里不需要 `enabled` 开关

### 阶段 C：组件（自下而上，每个都能独立看效果）

- [ ] C1. 若要用 Collapsible，先 `cd web && npx shadcn@latest add collapsible`
      —— `components/ui/` 下的原语必须生成，不手写（spec 明文）。
- [ ] C2. `components/UsageTotals.tsx`：活跃 / 挂机 / 锁屏 三个数字。
- [ ] C3. `components/UsageAppList.tsx`：两级排行条。条宽 = `seconds / maxSeconds`，
      **分母为 0 要短路**，否则出 `NaN`（AC3.6）。展开后渲染 `activities`，
      直接用服务端给的数字，不在前端二次求和（AC3.3）。
- [ ] C4. `components/UsageChart.tsx`：一个组件吃两种数据源 —— 传入
      `{ label, seconds, topApp }[]` 的通用形状，由父组件把 `hourly` / `daily` 各自映射进来。
      高度 = `seconds / max`，同样要处理 `max === 0`。每根柱子带 `title`/`aria-label`（R5.8）。
- [ ] C5. `components/UsagePanel.tsx`：设备选择 + 窗口选择 + 组装 C2–C4。
      设备与窗口是 local `useState`。设备列表从 `data.devices` 取；
      设备为空时显示空态。窗口切换触发 `useUsage` 重新 fetch。

### 阶段 D：装配与产物

- [ ] D1. `App.tsx`：header 下加 tab 切换（local `useState`，默认「此刻」），
      按 tab 渲染 `DeviceGrid` 或 `UsagePanel`。
      **`useDeviceStream()` 必须保持在 `App` 顶层无条件调用** —— 不能挪进「此刻」分支里，
      否则切 tab 会反复建/拆 SSE 连接（AC3.4）。tab 只控制渲染什么，不控制 hook 是否调用。
- [ ] D2. 三态检查过一遍：加载中、错误、无数据（AC3.5 / AC3.6）。
      深色主题下看一眼对比度（站点 dark-only，AC3.10）。
- [ ] D3. `npm run build` 并**提交 `server/cmd/server/web/` 产物**（AC3.9）。

## 验证命令

```bash
cd web
npm run lint
npm run typecheck
npx vitest run
npm run build

cd ..
git diff --exit-code -- server/cmd/server/web    # 必须干净（AC3.9）
```

本地联调（前端 dev server 已配 `/api` 代理到 `:8080`，见 `vite.config.ts`）：

```bash
# 终端 1：起后端
cd server && go run ./cmd/server
# 终端 2：起前端
cd web && npm run dev
```

窗口内无数据时 `GET /api/v1/usage` 会返回全 0 的槽 —— AC3.6 就用这个状态验证，
不需要造假数据。要看有数据的形态，让 Windows 客户端跑几分钟即可。

## 风险点

| 项 | 说明 |
|----|------|
| **把 `useDeviceStream` 挪进 tab 分支** | 最可能犯的错。切 tab 会反复建立/断开 SSE，表现为连接状态闪烁、后端订阅者数量上涨。D1 已标注：hook 留在顶层，tab 只控制渲染 |
| **除以 0 得 `NaN`** | 空窗口、全 0 数据时条宽/柱高的分母为 0。C3/C4 都要短路。这是 AC3.6 的主要失败模式 |
| 校验写太严 | 若 `isUsageResponse` 校验 `window` 的字符串取值，后端将来加窗口会让整块数据被丢弃。A3 要求从宽，与既有 `parseSnapshot` 的注释取向一致 |
| 忘记提交构建产物 | CI 会红并提示；本地用 `git diff --exit-code` 自查 |
| 手写 `components/ui/` 原语 | 违反 spec。C1 用 `npx shadcn@latest add` 生成 |
| 把格式化字符串存进 state | 违反 `state-management.md` 的 Common Mistakes。所有格式化在 render 时调 `lib/format.ts` |

## 回滚点

- 阶段 A、B、C 全是**新增文件**，不影响既有页面，可安全先落地。
- **D1 是唯一改动既有文件（`App.tsx`）的一步**，也是唯一的回归风险点，单独提交便于 revert。
- 整体回滚：把 `App.tsx` 改回直接渲染 `DeviceGrid` 并重建产物，新增组件留在树里不影响任何行为。
