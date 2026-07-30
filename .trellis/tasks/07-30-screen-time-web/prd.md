# 前端屏幕使用时间视图

父任务：`.trellis/tasks/07-30-screen-time/`。
需求全景见其 `prd.md`；**技术设计在其 `design.md` §8**，本文不重复。

## 前置与顺序

- **前置：`07-30-screen-time-server` 必须先完成**（它交付 `GET /api/v1/usage` 与响应契约）。
  本任务是本组最后一个。
- `07-30-screen-time-contract` 已在更早阶段完成 `Activity.locked` 的 TS 镜像，本任务不重复。

## Goal

在站点上提供一个「使用时间」视图，让访客不登录就能看到每台设备在今日 / 近 7 天 / 近 30 天里
各应用的使用时长排行与时间分布。

## Requirements

- R5.1 顶部提供「此刻」/「使用时间」两个 tab，默认「此刻」。用 React state 切换，
  **不引入 `react-router`**，不改服务端静态兜底（父 D6）。
- R5.2 「使用时间」tab 内提供设备选择与窗口选择（今日 / 近 7 天 / 近 30 天）。
- R5.3 展示三项内容：
  - 三种 state 的总时长（活跃 / 挂机 / 锁屏）；
  - 按 app 的活跃时长排行条，可展开看该 app 内各 description 的时长拆分；
  - 分布图：`today` 按小时（24 槽），`7d`/`30d` 按日。
- R5.4 **不引入新的运行时依赖**。条形与柱状图用 Tailwind / CSS 实现；
  展开交互用已在依赖里的 `radix-ui`（Collapsible）；需要动效时用已在依赖里的 `motion`。
- R5.5 加载中、请求失败、窗口内无数据三种状态都有明确呈现，不得白屏、不得出现 `NaN` 或 `Infinity`
  （注意占比计算的分母为 0 的情形）。
- R5.6 统计**不接入 SSE**（父 D8）。`useUsage` 在窗口变化时重新 fetch，
  用 `AbortController` 取消上一次（对齐 `useDeviceStream` 的既有写法）。
- R5.7 响应体运行时校验风格严格对齐既有 `parseSnapshot`：只校验结构、不校验字符串枚举值；
  `hourly` / `daily` 校验为 `null | Array`；解析失败返回 `null` 并由调用方给出错误态。
- R5.8 无障碍：柱子与条形的信息不得只靠高度/宽度编码，需带文本等价物
  （`title` / `aria-label` 说明「几点 · 时长 · 主要应用」）。

## 约束（来自既有 spec，不是本任务的选择）

- **无状态库、无 Context**（`frontend/state-management.md`）。tab 与窗口/设备选择是 local UI state
  （`useState`），服务端状态放 hook。
- **派生值 render 时算，不进 state**（同上的 Common Mistakes）：条宽百分比、排序、
  格式化后的时长字符串都不许存进 state。
- **目录扁平、无 feature 目录、无 `index.ts` barrel**（`frontend/directory-structure.md`）。
  应用组件平铺在 `components/`，`PascalCase.tsx` + 同名命名导出；纯函数进 `lib/`（不得 import React）。
- **所有契约类型只在 `types/contract.ts` 声明**，别处不得重复声明 API 载荷形状。
- `components/ui/` 下的 shadcn 原语通过 `npx shadcn@latest add <component>` 生成，不手写、不重命名。
- **前端产物必须一起提交**：`vite build` 写入 `server/cmd/server/web/`，
  CI 有 embed freshness 闸，改了前端不重建产物会红。

## Acceptance Criteria

- [ ] AC3.1（父 AC7）未认证的无痕浏览器打开站点，切到「使用时间」tab 即可看到统计，无需登录。
- [ ] AC3.2（父 AC8）窗口在今日 / 近 7 天 / 近 30 天之间切换时，数据与图形态相应变化
      （今日为小时分布，7/30 天为按日趋势）。
- [ ] AC3.3（父 AC9）展开某个 app 后，其下各 description 的时长之和等于该 app 的总时长
      （前端只需正确渲染服务端数据，不做二次求和）。
- [ ] AC3.4 切换 tab 到「此刻」再切回，实时卡片与 SSE 连接状态正常，无重复订阅、无连接泄漏。
- [ ] AC3.5 接口返回 500 / 网络失败时显示错误态而非白屏；返回畸形 JSON 时 `parseUsage` 返回 `null`
      并走同一错误态。
- [ ] AC3.6 窗口内完全无数据的设备显示「无数据」而非空白图表，且不出现 `NaN` / `Infinity`。
- [ ] AC3.7 `npm run lint`、`npm run typecheck`、`npx vitest run` 全绿；
      `parseUsage` 的结构校验有单测（含 `hourly`/`daily` 可空、畸形返回 `null`）。
- [ ] AC3.8 时长格式化函数有单测：0 秒、不足 1 分钟、跨小时、超 100 小时、负数/非有限值的兜底。
- [ ] AC3.9 `npm run build` 后 `git diff --exit-code -- server/cmd/server/web` 干净
      （产物已重建并提交）。
- [ ] AC3.10 深色主题下可读（站点是 dark-only，`index.html` 带 `class="dark"`），
      柱状图与排行条在深色背景下有足够对比度。

## Out of Scope

- 引入前端路由与 `/usage` 深链（父 D6）。
- 引入图表库（父 R5.4）。
- 统计数据的实时推送（父 D8）。
- 访客侧时区切换、自定义时间范围、数据导出（父 Out of Scope）。
- 「此刻」tab 现有卡片的任何视觉或文案改动。
- 移动端专门的图表交互（响应式要能用，但不做手势/长按之类的额外交互）。
