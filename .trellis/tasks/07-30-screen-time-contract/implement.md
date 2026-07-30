# 执行计划：契约新增锁屏标记

需求见本任务 `prd.md`；技术设计见父任务 `.trellis/tasks/07-30-screen-time/design.md` §2.1。
体量小但**跨三个模块**（`shared` / `client-windows` / `web`），顺序不能乱。

## 有序清单

- [ ] 1. `shared/contract.go`：`Activity` 加 `Locked bool \`json:"locked"\``，
      注释写明「为什么是结构化字段而不是从 `App` 推断」（`App` 是用户自定义字符串，服务端无法匹配）。
- [ ] 2. `client-windows/internal/mapping/mapping.go`：`Resolve` 的 `key == ""` 分支置 `act.Locked = true`。
      **只改这一处**；其他分支不显式赋 `false`（零值已是 `false`，显式赋值反而暗示这里有逻辑）。
- [ ] 3. `client-windows/internal/mapping/mapping_test.go`：补 `Locked` 的表驱动断言，
      覆盖 prd AC1.4 的四个分支。注意现有测试是表驱动 + stdlib `testing`，不要引入断言库。
- [ ] 4. `shared` 或 `client-windows` 侧补一个反序列化测试（AC1.3）：
      `json.Unmarshal` 一段不含 `locked` 键的 `ReportPayload`，断言 `Locked == false` 且无 error。
      放在 `shared/` 下更合适（它是契约的所有者），但 `shared` 目前无测试文件 ——
      新建 `shared/contract_test.go` 是可以的，`quality-guidelines.md` 要求测试与代码同放。
- [ ] 5. `web/src/types/contract.ts`：`Activity` 加 `locked?: boolean`；
      `isActivity` 加 `(value.locked === undefined || typeof value.locked === 'boolean')`。
      **务必用可选校验**，理由见 prd 的「关于 AC1.5 的一个决定」——写成必需会让旧客户端的设备卡整块消失。
- [ ] 6. `web/src/types/contract.test.ts`：补两个用例 —— 带 `locked` 通过、缺 `locked` 也通过、
      `locked` 为字符串时不通过。
- [ ] 7. 跑通验证命令（见下）。**前端有改动，必须 `npm run build` 并提交 `server/cmd/server/web/` 产物**，
      否则 CI 的 embed freshness 闸会红。

## 验证命令

```bash
gofmt -l shared server client-windows
go vet ./server/... ./shared/... ./client-windows/...
go test ./server/... ./shared/... ./client-windows/...
GOOS=windows GOARCH=amd64 go build ./client-windows/...

cd web && npm run lint && npm run typecheck && npx vitest run && npm run build
cd .. && git diff --exit-code -- server/cmd/server/web   # 必须干净（产物已重建并 add）
```

真机验证 AC1.1（需要 Windows 桌面，无法在 CI 覆盖）：

```bash
cd client-windows
go build -o agent.exe ./cmd/agent
# 先把某个浏览器标签命名为 SECRET-TITLE-CANARY 作为诱饵
./agent.exe -dry-run          # 有前台窗口：locked 应为 false，且输出无诱饵字符串
# 锁屏（Win+L）后立刻再跑一次，或用一个循环 dry-run 观察锁屏期间的输出
```

> `-dry-run` 直接用 `json.Encoder` 编码整个 `ReportPayload`（`client-windows/cmd/agent/main.go:89`），
> 所以新字段**自动**出现在输出里，无需改动输出路径。
>
> 但它采集一轮就退出，锁屏时不方便手动执行。用一个循环，锁屏后观察输出：
> `for /l %i in (1,1,20) do @agent.exe -dry-run`（cmd）。
> 注意跑这个循环时控制台代码页要能显示中文，否则 `app` 字段会是乱码而不是「已锁屏」。

## 风险点与回滚

| 项 | 说明 |
|----|------|
| **前端校验写成必需** | 最可能犯的错。会导致旧客户端设备卡整块消失，且症状是「设备莫名不见了」而非报错，很难查。清单第 5 步已标注 |
| 忘记提交前端构建产物 | CI 会红并给出明确提示；本地用上面的 `git diff --exit-code` 自查 |
| `client-windows` 只能在 Windows 上原生测 | CI 用 `GOOS=windows` 交叉构建 + vet 兜底；`mapping` 是纯逻辑包，可在任意平台跑单测 |

回滚：三处改动互不耦合，`git revert` 单个提交即可。字段是 additive，撤掉后新旧两侧都不受影响。
