# 正式发布 v0.2.0

## Goal

把当前 `main` 的代码正式发一版：打 `v0.2.0` tag 并推送，触发 `release.yml` 构建
多架构镜像到 GHCR（更新 `latest` / `0.2.0` / `0.2` 标签），并在 GitHub 创建带
changelog 的 Release。这样服务器按默认 `IMAGE_TAG=latest` 拉到的就是最新代码。

## Background（已核实）

- 仓库当前只有 `v0.1.0` 一个 tag（commit `46f5e42`），之后有 40+ 个提交未发版。
- 镜像发布机制（`.github/workflows/release.yml`）：
  - 触发：push 到 `main`（paths-ignore `.trellis/**`、`**.md`）、push `v*` tag、手动。
  - 标签生成（`docker/metadata-action` + flavor 默认 `latest=auto`）：
    - `type=semver` → tag push 时生成 `0.2.0`、`0.2`，并触发 auto-latest 更新 `latest`；
    - `type=raw,value=edge,enable={{is_default_branch}}` → main push 时滚动更新 `edge`；
    - `type=sha,format=short` → 每次构建都有短 SHA 标签。
  - 因此 `latest` **只在打 tag 时更新**，`edge` 跟随 main 滚动。
- GHCR 实测（2026-07-31）：当前标签为 `edge`（= commit `fc557a0`）、`0.1.0`、`0.1`、
  `latest`（仍指向 v0.1.0 构建）及若干 `sha-*`。
- 最近两个提交（`381dd4b` docs、`857d52d` archive）只改文档/.trellis，被 paths-ignore
  跳过，不影响镜像内容。

## v0.1.0 → v0.2.0 主要变化（changelog 素材）

- **使用时间统计（screen-time）**：服务端按小时聚合设备使用时间（`usage` 包），
  新增 `GET /api/v1/usage` 接口；Web 端新增「使用时间」tab，按应用排行与分布展示；
  新增 `USAGE_RETENTION` / `USAGE_GAP` / `USAGE_TIMEZONE` 配置项及 compose 变量。
- **agent.exe -setup**：Windows 客户端新增本地规则可视化配置器（内置 WebUI），
  持续观察前台窗口、实时预览映射结果、写回 `config.yaml`（带注释和备份）。
- **锁屏识别增强**：把 `lockapp.exe` / `logonui.exe` 识别为锁屏（此前可能被当普通应用）。
- **配置健壮性**：config 错误与归一化逻辑重构为可复用；写回 YAML 带注释与备份。
- **部署**：新增 `LOG_MAX_SIZE` / `LOG_MAX_FILE` 等 compose 编排变量。

## 验收标准

1. 发布前验证通过：`shared` / `server` 的 `go test` 全绿（client-windows 测试在
   Windows 本地跑 `go test -race ./client-windows/...` 全绿）。
2. `git tag v0.2.0` 推送到 origin 成功；tag 指向当前 `main` 的有效代码提交。
3. `release.yml` workflow 运行成功；GHCR 上 `0.2.0`、`0.2`、`latest` 三个标签
   均指向新构建（digest 与 `edge` 一致）。
4. GitHub Release `v0.2.0` 创建成功，正文为整理好的 changelog（上述主要变化）。

## 非目标

- 不改代码、不引入新功能；本次只做发布动作。
- 不迁移/不修改现有部署配置（服务器侧 `IMAGE_TAG` 是否切换由用户决定）。
