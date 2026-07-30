# Implement — 设备卡片 ASCII 动画装饰

> 需求见 `prd.md`，技术方案见 `design.md`。
> 分阶段推进，每阶段末尾都有可验证的门禁；未过门禁不进下一阶段。

## 前置：素材准备（用户侧，阻塞阶段 0）

**这是唯一一个我做不了的环节。** 需要你先准备源素材：

- 最少 1 份用于阶段 0 打通管线（建议先做 `coding`）。
- 完整落地需要 5–6 份，对应 `scenes.json` 里的 `locked` / `idle` / `fallback` 与几个应用场景。
- 格式 GIF 或短视频均可，循环流畅的最好（首尾能接上）。
- 建议先在 asciistudio.space 上传试一下，肉眼确认这段素材在 24×12 这种小尺寸下还认得出是什么
  —— 细节太多的素材缩到 24 列后会糊成一团噪点，这是最常见的失败原因。

素材不入库（D3），放在仓库外任意路径，跑脚本时用 `--input` 指过去即可。

## 阶段 0 · 素材管线（Python，先于任何前端代码）

- [ ] 0.1 `web/scripts/gif2ascii.py`：`cv2.VideoCapture` 逐帧 → 灰度均值 → 字符表映射，
      算法照搬 ASCII-generator 的 `img2txt.py`，宽高比补偿 `cell_height = aspect × cell_width`（R1.2）。
- [ ] 0.2 参数：`--input` / `--output` / `--cols` / `--fps` / `--aspect`（默认 2.0）/
      `--charset`（simple|complex）/ `--invert`；`--help` 可用（R1.1 / R1.4）。
- [ ] 0.3 按目标 fps 抽帧（R1.3）；默认字符表**剔除 `\` 与 `"`**（design §2.1）。
- [ ] 0.4 写文件前自校验：每帧 `rows` 行、每行 `cols` 列，不满足非零退出（R1.5）。
- [ ] 0.5 文件头 docstring 写明：依赖（Python 3 + opencv-python + numpy）、
      「离线工具，CI 不运行」、调参建议、源素材出处。
- [ ] 0.6 用真实素材跑出第一份 `web/src/assets/ascii/coding.json`，
      **在终端 `cat` 出来肉眼确认动画认得出是什么**。

**门禁**：脚本能跑出合法 JSON；坏输入会报错退出（AC7）；单个文件 ≤ 32 KB（R5.1）；
肉眼确认 24×12 下素材可辨识。

> ⚠️ 这一阶段的真正风险不是代码，是**素材在小尺寸下不可辨识**。
> 先用一份素材把这件事验证掉，再去转其余五份——否则可能六份全部返工。

## 阶段 1 · 纯逻辑（TypeScript，可完全单测）

- [ ] 1.1 `web/src/types/ascii.ts`：`AsciiClip`（§2 的格式）与 `SceneConfig` / `SceneRule` 类型。
- [ ] 1.2 `web/src/config/scenes.json`：按 design §3.1 的结构写好，
      先只填阶段 0 产出的那一个动画 + 三个结构化状态槽位。
- [ ] 1.3 `web/src/lib/scene.ts`：`resolveScene(device, config): string | null` 纯函数，
      严格实现 design §3.2 的五级判定顺序与 §3.3 的匹配语义。不 import React、不发请求（R2.4）。
- [ ] 1.4 `import.meta.glob` 枚举可用动画名，导出 `AVAILABLE`（design §3.4）。
- [ ] 1.5 **`lib/scene.test.ts`（AC3）**，逐条覆盖：
      离线 → `null`；locked / idle 压过 app 匹配；app 全等；description 子串；
      同规则内 AND；数组内 OR；规则顺序优先级；trim + 大小写折叠；未命中 → fallback。
- [ ] 1.6 **配置自洽性测试（R2.5）**：`scenes.json` 引用的每个动画名都在 `AVAILABLE` 里；
      每条规则至少给了 `app` 或 `description` 之一（两者都缺会无条件命中，让后续规则全失效）。

**门禁**：`npm run lint` / `typecheck` / `vitest run` 全过；1.5 的九类用例全绿。

> 这一阶段一行 UI 代码都不写。匹配逻辑是本任务唯一有真实分支复杂度的部分，
> 先把它在纯函数里钉死，后面的组件就只剩渲染。

## 阶段 2 · 播放组件

- [ ] 2.1 `web/src/hooks/useAsciiFrames.ts`：动态 `import()` 加载帧数据，
      `cancelled` 标志防竞态，加载失败静默返回 `null`（R4.3）；依赖数组是 `name` 而非 `device`（design §4.3）。
- [ ] 2.2 `web/src/components/AsciiAnimation.tsx`：渲染 `<pre>`，
      rAF + 时间戳累加驱动，**直写 `textContent`**（design §4.1/§4.2）。
- [ ] 2.3 cleanup 必须 `cancelAnimationFrame`（`quality-guidelines.md:46`）。
- [ ] 2.4 `document.hidden` 暂停；恢复时 `acc % step` 丢弃积压，不补播（R3.3）。
- [ ] 2.5 `prefers-reduced-motion: reduce` → 只写首帧、不启动循环，并监听该 media query 的变化（R4.2）。
- [ ] 2.6 容器 `aria-hidden="true"`（R4.1）；尺寸由 `cols`/`rows` 算出，
      加载中 / 失败 / 播放中**三态同尺寸**（R3.4 / R3.5）。
- [ ] 2.7 `font-mono` + `leading-[1.2]` **只加在 `<pre>` 上**（design §6 / R4.4）。

**门禁**：`lint` / `typecheck` / `vitest run` / `build` 全过；
`npm run dev` 下单独渲染该组件，动画能播、切标签页会停、开系统「减少动态效果」后静止。

## 阶段 3 · 接入卡片与排版校准

- [ ] 3.1 `DeviceCard` 里 render 期调 `resolveScene`（**不进 `useEffect`**，`quality-guidelines.md:71`），
      结果为 `null` 时不渲染动画（R3.6）。
- [ ] 3.2 定位与尺寸：在 `CardContent` 内，与 app/description 文案并排或其上方；
      字号取到 `{cols}ch` 恰好适配卡片宽度（24 列约 10–11px），**不硬编码进帧数据**。
- [ ] 3.3 **排版校准（design §6）**：肉眼确认动画没有被纵向拉长或压扁。
      拉长 → 检查是否漏了 `leading-[1.2]`（Tailwind 默认 1.5 会拉长 25%）；
      仍不对 → 调脚本的 `--aspect` 重转，**不要**去改行高凑。
- [ ] 3.4 离线卡片验证：`opacity-60 grayscale`（`DeviceCard.tsx:63`）不与动画打架，
      且离线时本就不渲染动画。
- [ ] 3.5 响应式检查：`sm:grid-cols-2` / `lg:grid-cols-3`（`App.tsx:35` 的网格）三档宽度下
      动画都不溢出、不换行、不撑破卡片。

**门禁**：AC5（三态不跳高度）用 DOM 尺寸断言或肉眼 + 开发者工具确认；
三档断点均正常；`build` 过。

## 阶段 4 · 素材补齐

- [ ] 4.1 用阶段 0 校准好的参数转出其余场景：`locked` / `idle`(dozing) / `fallback`(active)
      + 各应用场景。
- [ ] 4.2 补全 `scenes.json` 的 `scenes[]`，**`app` 值必须与你 `config.yaml` 里的映射规则实际产出一致**
      （对着客户端配置抄，别凭记忆写）。
- [ ] 4.3 阶段 1.6 的自洽性测试自动覆盖新增项，跑一遍确认全绿。
- [ ] 4.4 体积核对（AC6）：单文件 ≤ 32 KB；全部动画 gzip 合计 ≤ 60 KB；
      `build` 后确认帧数据在独立 chunk 里、不在首屏 JS 内（design §5）。

**门禁**：AC1 / AC2 在真机上走一遍——切应用、挂机、锁屏、断客户端等离线，四种状态动画正确切换。

## 阶段 5 · 规范同步、文档与验收

- [ ] 5.1 **更新 `.trellis/spec/frontend/component-guidelines.md`（R6.1）**：
      在 Animation 一条下补充「逐帧文本动画（ASCII）用 rAF 手写帧循环，Framer Motion 不适用」
      及 design §8 的判据表。**英文**（`frontend/index.md:35`）。
- [ ] 5.2 更新 `.trellis/spec/frontend/quality-guidelines.md`（R6.2）：
      依赖基线一节注明本任务零新增依赖，帧数据是数据资产非依赖。**英文**。
- [ ] 5.3 `web/README.md`：新增一节说明 ASCII 动画的素材管线、
      `scenes.json` 与客户端 `config.yaml` 映射规则的对应关系（design §11 的失配风险缓解）。
- [ ] 5.4 根 `README.md` 的特性列表提一句（可选，别喧宾夺主——这是装饰层）。
- [ ] 5.5 **`npm run build` 并提交 `server/cmd/server/web/` 产物**（R5.5，
      `quality-guidelines.md:29` 的硬规矩）。
- [ ] 5.6 确认 `package.json` 的 `dependencies` 一字未改（R5.4 / AC8）。

**门禁**：AC1–AC8 全部勾掉；CI 全绿（尤其 embed 新鲜度关卡）。

## 验证命令速查

```bash
# 前端全套门禁（与 CI 一致）
cd web && npm run lint && npm run typecheck && npx vitest run && npm run build

# 嵌入产物是否过期（CI 的 web job 就是这么判的）
git diff --exit-code -- server/cmd/server/web

# 帧数据体积
ls -l web/src/assets/ascii/
gzip -c web/src/assets/ascii/*.json | wc -c

# 帧数据是否被切成独立 chunk（不应出现在主 chunk 里）
ls -l server/cmd/server/web/assets/

# 素材转换（离线，按需）
python web/scripts/gif2ascii.py --input ~/art/coding.gif \
  --output web/src/assets/ascii/coding.json --cols 24 --fps 12
```

## 回滚点

- 阶段 0–1 纯新增文件且不接入任何界面，随时可弃。
- 阶段 3.1 是唯一改动既有组件的地方（`DeviceCard` 加一行），
  **独立成一次提交**，出问题可单独 revert。
- 完整回滚：摘掉 `DeviceCard` 那一行，删 `assets/ascii/`、`config/scenes.json`、`lib/scene.ts`、
  `components/AsciiAnimation.tsx`、`hooks/useAsciiFrames.ts`、`scripts/gif2ascii.py`、
  `types/ascii.ts`，重新 `build` 并提交产物。无数据迁移、无用户侧影响。
