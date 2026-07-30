# 网页 ASCII 动画装饰

## Goal

让访客一眼看出「他此刻在干嘛」，而不只是读到一行字。

在每张设备卡片里嵌一小块 ASCII 动画：在写代码时是敲键盘的动画，在看视频时是播放器的动画，
挂机时是打盹的动画。动画由本人准备的 GIF/视频离线转换而来，产物是纯文本帧序列，
运行时不做任何视频解码，也不新增任何运行时依赖。

**这是纯装饰层**：动画不承载任何靠它才能获得的信息，去掉它页面功能不减。

## Background：现状与约束（已核实）

### 数据契约

- `Activity`（`shared/contract.go:38`）只有四个可用字段：
  `App`、`Description`（设备端映射规则产出）、`Idle` / `IdleSeconds`、`Locked`；
  `DeviceState.Online` 由服务端时钟判定（`contract.go:155`）。
- **`App` 与 `Description` 是用户在 `config.yaml` 里自定义的任意字符串。**
  `contract.go:46` 的注释已经为此立过判例：`Locked` 之所以做成结构化 flag 而不是从 `App` 推断，
  正是因为「`App` 是用户配置的字符串，服务端无法据此匹配」。本任务撞上同一个问题。
- 契约里刻意没有原始窗口标题——动画选型只能基于已脱敏的 `App` / `Description`，
  这不是限制，是这个项目的前提。

### 前端现状

- `web/` = React 19 + Vite 8 + Tailwind 4 + shadcn/radix，单页只读。
- `DeviceCard`（`web/src/components/DeviceCard.tsx:49`）是纯展示组件，数据全部来自 props；
  离线态是 `opacity-60 grayscale`（`DeviceCard.tsx:63`）。
- 数据获取集中在 `useDeviceStream`，由 `App` 无条件调用（`App.tsx:53`）；卡片层不取数。
- `src/index.css:52` 已声明 `--font-mono: IBM Plex Mono, monospace`，
  但 `index.css` 只 `@import` 了 Plus Jakarta Sans——**`font-mono` 目前实际 fallback 到系统等宽字体**。
- 构建产物写进 `server/cmd/server/web/` 并 `//go:embed`，CI 有 embed 新鲜度关卡
  （`.github/workflows/ci.yml` 的 `web` job）。现有产物：JS 405,964 B + CSS 46,465 B + 字体 57,428 B。

### 相关规范（`.trellis/spec/frontend/`）

| 规范 | 出处 | 对本任务的影响 |
|------|------|----------------|
| 动画用 Framer Motion，禁手写 CSS keyframes | `component-guidelines.md:18` | **本任务构成例外**，见 D6 与 design §8 |
| CJK 混排用 `tabular-nums` 而非 `font-mono` | `component-guidelines.md:111` | 等宽只能作用于纯 ASCII 块，不得波及卡片文案 |
| 新增运行时依赖须在 PRD 记录理由 | `quality-guidelines.md:60` | 本任务零新增运行时依赖 |
| 装饰性元素 `aria-hidden`，不重复朗读 | `component-guidelines.md:121` | 动画必须对屏幕阅读器隐藏 |
| `useEffect` 不做派生计算，在 render 里派生 | `quality-guidelines.md:71` | 场景匹配是纯派生，不进 effect |
| 纯逻辑必须有 vitest 覆盖 | `quality-guidelines.md:78` | 匹配函数与帧数据校验必须单测 |
| 构建产物随源码一起提交 | `quality-guidelines.md:29` | 动画 JSON 与 rebuild 后的产物同批提交 |

### 素材转换方案（本轮调研结论）

- `vietnh1009/ASCII-generator` 的 `img2txt.py` 是唯一直接产出**纯文本**的实现，
  算法四十行（灰度均值 → 字符表），且已处理等宽字体 1:2 宽高比（`cell_height = 2 * cell_width`）。
  它缺的只是「视频/GIF → 帧序列」入口（`video2video.py` 输出的是渲染好的 mp4，不是文本）。
- `joelibaceta/video-to-ascii` 是终端播放器：核心 `render()` 直接 `sys.stdout.write`，
  耦合 `stty size` 与 `time.sleep`，只能导出 ANSI 转义的 `.sh`。不适合。
- `vansh-nagar/ascii-studio` 是 Next.js 产品站（monorepo 只有 `apps/landing`，未发 npm 包），
  浏览器内实时转换。不能作依赖，但可作为**手工调参的可视化预览工具**。

## Key Decisions

- **D1 绑定方式 = 前端配置表匹配 `app` / `description`。**
  `web/` 内一份 `scenes.json`，把已脱敏的 `App` / `Description` 映射到动画名。
  依据：改动面收敛在一个目录，不动 `shared/` 契约、不动客户端、不动服务端与 SQLite，
  纯装饰功能不值得一次破坏性契约变更。
  取舍：改了客户端映射规则后，要同步改前端配置并重新 `npm run build` 提交产物；
  两者不同步的表现是「掉回 fallback 动画」，属于优雅降级，不是故障。

- **D2 版面 = 每张设备卡片内嵌小动画（约 24×12 字符）。**
  与现有卡片网格布局一致；每台设备各自反映自己的状态；离线时随卡片一起 grayscale。
  取舍：尺寸小，素材细节有限，转换时必须按最终尺寸调参而不是缩放。

- **D3 素材管线 = 离线手工跑，产物入库。**
  仓库内放 `web/scripts/gif2ascii.py`（参考 ASCII-generator 的 `img2txt` 算法），
  本人在本地跑一次，把帧 JSON 提交进仓库；源 GIF 不入库。
  依据：与 `client-windows/webui` 产物入库同一心智；CI 不需要 Python 与 OpenCV。
  取舍：产物与源素材之间没有自动关卡，同步靠人。素材变更频率极低，可接受。

- **D4 视觉 = 单色，跟随主题色。**
  帧数据只含字符不含颜色，颜色由 CSS 用主题语义类（`text-muted-foreground` 等）给。
  依据：三条——① 自动适配亮/暗主题，② 不引入第二个色相（`component-guidelines.md:102` 的既有纪律），
  ③ 彩色要么带 ANSI 转义要么带 per-cell 颜色数组，体积翻数倍。

- **D5 结构化状态优先于应用匹配。**
  判定顺序：`!online` → 不渲染；`locked` → 锁屏动画；`idle` → 挂机动画；
  再按 `scenes` 顺序匹配 `app` / `description`；都不中则 fallback。
  依据：人不在电脑前时，前台窗口仍是 VS Code，但此时播「在写代码」的动画是**错误信号**。
  `Idle` / `Locked` 是契约里可信的结构化事实，必须压过用户自定义字符串的匹配。

- **D6 播放实现 = `requestAnimationFrame` 手写帧循环，不用 Framer Motion。**
  这是对 `component-guidelines.md:18` 的显式偏离：Framer Motion 做的是属性补间，
  而这里要做的是「按固定 fps 替换 `<pre>` 的文本内容」，两者不是同一类问题。
  按 spec 自己的规定（`frontend/index.md:14`「real code diverges for a good reason → update the spec
  in the same task」），本任务必须同步更新该规范。
  不用 `setInterval`：它在后台标签页被浏览器节流，且误差会累积。

- **D7 等宽字体 = 系统等宽栈，不引入字体包。**
  `--font-mono` 已声明但无对应字体文件。ASCII 动画只要求「等宽」，不要求「哪一款等宽」；
  引入 IBM Plex Mono 要多付约 50-100 KB 而收益仅是跨平台字形一致。
  代价：不同系统字形略有差异，同一份帧数据在各平台的观感不完全相同。

- **D8（工程默认，未占用决策）**
  - 帧数据按场景拆分文件、动态 `import()` 懒加载，Vite 自动产出独立 chunk，不进首屏 bundle。
  - `prefers-reduced-motion: reduce` 时只渲染首帧，不播放。
  - `document.hidden` 时暂停帧循环。

## Requirements

### R1 素材转换管线

- **R1.1** 提供 `web/scripts/gif2ascii.py`：输入 GIF/视频，输出符合 R2.1 格式的帧 JSON，
  参数至少含 `--input` / `--output` / `--cols` / `--fps` / `--charset`。
- **R1.2** 必须做等宽字体宽高比补偿（`cell_height ≈ 2 × cell_width`），否则输出纵向拉伸。
- **R1.3** 支持抽帧到目标 fps（源素材 30fps 不应原样产出 30 帧/秒的数据）。
- **R1.4** 脚本自带 `--help` 与文件头注释，说明依赖（Python 3 + opencv-python + numpy）
  与「这是离线工具，CI 不运行它」。
- **R1.5** 输出前自校验：每帧行数 == `rows`、每行列数 == `cols`，不满足直接报错退出。

### R2 帧数据与场景配置

- **R2.1** 帧数据格式固定为 `{ name, cols, rows, fps, frames: string[] }`，
  每帧是以 `\n` 分隔的纯 ASCII 文本；文件位置 `web/src/assets/ascii/<name>.json`。
- **R2.2** 场景配置 `web/src/config/scenes.json`：有序规则数组，每条含 `anim` 与
  可选的 `app[]`（全等匹配）/ `description[]`（子串匹配），外加一个 `fallback`。
- **R2.3** 匹配语义钉死：数组内部 OR，同条规则内 `app` 与 `description` 同时给出时 AND，
  规则数组顺序即优先级、第一条命中即生效（与客户端 `title_patterns` 同一心智）。
  比较前统一 trim 并做大小写折叠。
- **R2.4** 匹配实现为 `web/src/lib/scene.ts` 里的**纯函数**，输入 `DeviceState` 输出动画名或 `null`，
  不碰 React、不发请求。
- **R2.5** 配置里引用了不存在的动画名时，构建期或测试期必须报错，不能在运行时静默变成空白。

### R3 播放与渲染

- **R3.1** 新增 `AsciiAnimation` 组件：接收动画名，渲染一个 `<pre>`，按 `fps` 逐帧替换内容。
- **R3.2** 帧循环用 `requestAnimationFrame` + 时间戳累加驱动；组件卸载时必须取消（cleanup）。
- **R3.3** `document.hidden` 为真时暂停，恢复可见时继续，不补播积压的帧。
- **R3.4** 帧数据按需 `import()` 加载；加载完成前渲染等尺寸占位，**不得引起卡片高度跳动**。
- **R3.5** 动画区域尺寸由 `cols`/`rows` 决定并固定，任何加载/切换状态都不改变卡片布局。
- **R3.6** 设备离线时不渲染动画（D5）。

### R4 无障碍与降级

- **R4.1** 动画容器 `aria-hidden`，不向屏幕阅读器暴露任何字符——它读到的仍是既有的
  `VS Code · 在写代码`，不得出现一大段乱码。
- **R4.2** 尊重 `prefers-reduced-motion: reduce`：只渲染首帧，不启动帧循环。
- **R4.3** 帧数据加载失败时，卡片其余部分完全正常，不显示错误、不留空洞。
- **R4.4** 等宽只作用于 ASCII 块本身（`font-mono` 加在 `<pre>` 上），
  绝不波及卡片内的中文文案（`component-guidelines.md:111` 的既有教训）。

### R5 体积与构建

- **R5.1** 单个动画 JSON 原始体积 ≤ 32 KB。
- **R5.2** 全部动画数据 gzip 后合计 ≤ 60 KB。
- **R5.3** 首屏 JS chunk 不包含任何帧数据（懒加载生效的可验证判据）。
- **R5.4** 零新增运行时依赖（`package.json` 的 `dependencies` 不变）。
- **R5.5** 前端产物重新构建并与源码同批提交，CI embed 新鲜度关卡通过。

### R6 规范同步

- **R6.1** 更新 `.trellis/spec/frontend/component-guidelines.md`：
  在动画一节写明「逐帧文本动画（ASCII）用 rAF 手写帧循环，Framer Motion 不适用」及其判据。
- **R6.2** 更新 `.trellis/spec/frontend/quality-guidelines.md` 的依赖基线一节，
  记录本任务零新增依赖，并说明帧数据是数据资产而非依赖。
- **R6.3** spec 文档用英文（`frontend/index.md:35`），任务文档用中文。

## Acceptance Criteria

- [ ] **AC1（能看见）** 在某设备上切到已配场景的应用，页面卡片里在 SSE 推送到达后
      开始播对应动画；切到未配置的应用，掉回 fallback 动画且不报错。
- [ ] **AC2（状态优先级）** 同一台设备前台停在 VS Code 时：正常使用 → 写代码动画；
      触发挂机 → 挂机动画；锁屏 → 锁屏动画；断开客户端等到判离线 → 无动画且卡片 grayscale。
      四种状态切换均即时生效（下一次 SSE 推送内）。
- [ ] **AC3（纯函数可测）** `lib/scene.ts` 有 vitest 覆盖：全等/子串/AND/OR/顺序优先级/
      大小写与空白折叠/结构化状态压过匹配/fallback/未知动画名，全部有用例。
- [ ] **AC4（无障碍）** 屏幕阅读器（或 `aria-hidden` 的 DOM 断言）确认动画字符不被朗读；
      系统开启「减少动态效果」后动画静止在首帧；两者均不影响卡片其余信息的可读性。
- [ ] **AC5（不跳动）** 首次加载、场景切换、加载失败三种情况下卡片高度不变化
      （固定尺寸容器，肉眼 + DOM 尺寸断言）。
- [ ] **AC6（体积）** `npm run build` 后：帧数据不在首屏 chunk 内（产物文件清单可查）；
      单个动画 JSON ≤ 32 KB；全部动画 gzip 合计 ≤ 60 KB。
- [ ] **AC7（管线可重跑）** 从一个源 GIF 出发跑 `gif2ascii.py`，产出的 JSON 能被前端直接使用；
      故意给一份尺寸不合法的输入，脚本报错退出而不是产出坏数据。
- [ ] **AC8（门禁）** `npm run lint` / `typecheck` / `vitest run` / `build` 全过；
      `server/cmd/server/web/` 产物已重建并提交；CI embed 新鲜度关卡通过；
      `dependencies` 无新增；`.trellis/spec/frontend/` 的两处规范已更新。

## Out of Scope

- **浏览器内实时视频转 ASCII** —— 素材固定且有限，运行时转换是纯浪费（见调研结论）。
- **彩色 ASCII** —— 见 D4。若日后要做，帧格式需要扩展，属另一个任务。
- **页面顶部大场景 / 悬停放大** —— 本轮只做卡片内嵌（D2）。
- **`shared/contract.go` 加 `scene` 字段** —— 见 D1，本任务不动契约。
  日后若映射规则复杂到前端配置表维护不动，再单独立任务。
- **构建期自动生成帧数据** —— 见 D3，CI 不引入 Python 工具链。
- **动画编辑器 / 在线预览页** —— 调参用外部工具（asciistudio.space）即可。
- **Android 客户端与服务端的任何改动**；`07-30-config-webui` 的 WebUI 不涉及。

## Technical Notes

技术方案见 `design.md`，执行清单见 `implement.md`。
