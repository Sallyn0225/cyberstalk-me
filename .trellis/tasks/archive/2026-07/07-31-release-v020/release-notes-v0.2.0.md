# v0.2.0

v0.1.0 之后共 44 个提交：新增使用时间统计与本地规则可视化配置器两个主要功能，并修复锁屏识别等若干问题。

## ✨ 新功能

- **使用时间统计（screen-time）**：服务端新增按小时聚合的设备使用时间统计，
  新增 `GET /api/v1/usage` 接口；Web 端新增「使用时间」标签页，按应用展示使用时长排行与分布。
  新增 `USAGE_RETENTION` / `USAGE_GAP` / `USAGE_TIMEZONE` 三个配置项（默认值即旧行为）。
- **`agent.exe -setup` 本地规则配置器**：Windows 客户端新增可视化配置界面。
  全程不联网，持续观察前台窗口列出你实际用过的应用与窗口标题，实时预览映射结果，
  点几下即可把规则写回 `config.yaml`——保留原有注释，并自动备份原文件。
- Activity 新增结构化锁屏标记：上报数据中锁屏状态由可选布尔字段 `locked` 表示。

## 🐛 修复

- 锁屏识别：`lockapp.exe` / `logonui.exe` 现在会被正确识别为锁屏状态（此前可能被当作普通应用统计）。
- 服务端：修复 device 列表查询中 LEFT JOIN 字段未按可空扫描的问题。

## 🚀 部署 / CI

- compose 新增 `LOG_MAX_SIZE` / `LOG_MAX_FILE` 变量，限制容器日志大小与轮转文件数。
- compose 新增四个 screen-time 相关变量，把上述三个配置项透传给容器。
- CI：release workflow 对仅文档的推送跳过镜像重建；client-windows 测试改为在 Windows runner 上运行，
  并校验内置 setup WebUI 打包产物与源码一致。

## ⬆️ 升级说明

无迁移要求。服务器上执行：

```bash
docker compose pull && docker compose up -d
```

默认 `IMAGE_TAG=latest` 会自动更新到 v0.2.0；如用 `edge` 尝鲜则无需操作（本就滚动更新）。
