# 前端展示网站（React+Vite+TS）

## Goal

完全公开的只读 SPA：设备卡片网格，snapshot 首屏 + SSE 近实时更新，无需登录即可"视奸"。

## Scope & 上游文档

- 对应父任务 `.trellis/tasks/07-28-cyberstalk-me/implement.md` 阶段 2；技术方案见父任务 `design.md` §4，契约见 §2。
- 覆盖父任务需求 R3.1–R3.3。

## Dependencies

- 依赖 `07-28-backend` 已交付的 `/api/v1/snapshot`、`/api/v1/stream` 契约与 `shared/` Go struct；`web/src/types/contract.ts` 必须镜像之（规则见 `.trellis/spec/frontend/type-safety.md`）。
- 后端未部署公网时，用本地运行的后端验证即可。

## Requirements

- React+Vite+TS 初始化于 `web/`，遵循 frontend spec 的目录结构与质量门禁。
- UI 技术选型（硬性约定）：组件优先用 shadcn/ui（`npx shadcn add`），先不自己手写；图标一律 lucide-react，不自绘 SVG；动效用 Framer Motion（`motion` 包），不手写 keyframes。
- `useDeviceStream` hook：snapshot 全量 + SSE 增量合并；EventSource 断线重连后重拉一次 snapshot 做全量校正。
- 设备卡片：设备名/类型图标、在线指示灯、当前活动（app + description）、活跃/空闲徽标、电量条+充电图标、网络图标、"最后活跃 X 前"；离线卡片置灰。
- 字段缺失（如台式机 battery=null）优雅降级。
- 构建产物可被后端 `embed.FS` 托管。

## Acceptance Criteria

- [x] lint / type-check / `npm run build` 全部通过。
- [x] 无痕浏览器直开 URL 即见全部设备卡片，无需任何认证（父任务 AC4）。
- [x] 模拟状态变化时页面自动更新，无需手动刷新（父任务 AC5）。
- [x] 离线设备有明确视觉区分并显示最后活跃时间（父任务 R3.3 / AC3 的前端部分）。

## Out of Scope

- 历史时间线、访客登录、评论等（见父任务 Out of Scope）。
