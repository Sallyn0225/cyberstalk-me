# Android 上报客户端（Kotlin，延后）

## Status

**已决定延后交付**：不阻塞 MVP（父任务 AC2 专属于本任务；MVP 验收为 AC1、AC3–AC6）。在 backend / web / client-windows 交付并稳定后，再启动本任务并细化规划（届时补 design/implement 与 jsonl 清单）。

## Goal

Kotlin 原生 App + 前台 Service：采集前台 App/电量/网络，设备端映射后上报后端。

## Scope & 上游文档

- 对应父任务 `.trellis/tasks/07-28-cyberstalk-me/implement.md` 阶段 4；技术方案见父任务 `design.md` §5.2。
- 覆盖父任务需求 R1.2、R1.3、R1.4（Android 侧）。

## Dependencies

- 依赖 `07-28-backend` 的上报契约与 token 机制。

## Requirements（启动时再细化）

- `UsageStatsManager` 取前台 App（引导用户授予 PACKAGE_USAGE_STATS）；`BatteryManager` 电量/充电；`ConnectivityManager` 网络类型。
- `packageName → {app, description}` 映射规则 + 白名单；未命中显示通用类别。
- 前台服务常驻（带通知）+ 省电白名单引导（厂商杀后台是主要风险，见父任务 Risks）。

## Acceptance Criteria

- [ ] 真机运行后，网站卡片显示当前 App（映射后）、电量与充电状态、网络类型（父任务 AC2）。
- [ ] 原始包名细节/标题不出现在上报与页面（同脱敏红线）。
