# Windows 上报客户端（Go 单 exe）

## Goal

常驻 Windows 的 Go 单 exe：采集前台窗口/空闲/电量/网络，在设备端完成脱敏映射后按固定间隔上报后端。

## Scope & 上游文档

- 对应父任务 `.trellis/tasks/07-28-cyberstalk-me/implement.md` 阶段 3；技术方案见父任务 `design.md` §5.1，契约见 §2。
- 覆盖父任务需求 R1.1、R1.3、R1.4（客户端侧）。

## Dependencies

- 依赖 `07-28-backend`：`shared/` 契约 struct（同 go.work 直接引用）、`/api/v1/report` 端点、`register-device` 生成的设备 token。

## Requirements

- Win32 采集（`golang.org/x/sys/windows`）：前台窗口 + 进程名、`GetLastInputInfo` 空闲秒数、`GetSystemPowerStatus` 电量（台式机无电池 → null）、活动网卡类型判定。
- `config.yaml`：`server_url`、`device_id`、`token`、`interval`、`process_name → {app, description}` 映射规则 + 白名单；未命中规则一律输出通用描述（如"使用中"）。
- **安全红线：原始窗口标题只在进程内存中用于映射，绝不进入上报 payload、日志或磁盘。**
- 上报循环 + 失败重试退避；`go build` 产出单 exe；开机自启以文档说明（注册表 Run 键）交付。

## Acceptance Criteria

- [ ] `go build ./...`、`go vet ./...`、`go test ./...` 全部通过（Win32 采集层按 spec 豁免单测）。
- [ ] 真机运行后，网站卡片显示"应用名 · 活动描述"，且抓包/日志/页面中均无原始窗口标题内容（父任务 AC1）。
- [ ] 断网或后端不可用时客户端不崩溃，恢复后自动续报。

## Out of Scope

- 图形化安装器/托盘 UI；Android 客户端。
