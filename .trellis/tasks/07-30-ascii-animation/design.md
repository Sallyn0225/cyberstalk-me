# Design — 设备卡片 ASCII 动画装饰

> 需求与验收见 `prd.md`，执行清单见 `implement.md`。
> 本文档钉死帧数据格式、匹配语义、播放器的几个非显然实现细节，以及对既有前端规范的一处偏离。

## 1. 总体形态

改动全部落在 `web/`，一条单向数据流，没有新的网络请求：

```
[ 离线 · 手工，仅素材变更时 ]
  coding.gif ──► web/scripts/gif2ascii.py ──► web/src/assets/ascii/coding.json
                  (Python + cv2 + numpy)          { name, cols, rows, fps, frames[] }

[ 运行时 · 浏览器 ]
  useDeviceStream (既有，不改)
        │  DeviceState
        ▼
  DeviceCard ──► resolveScene(device, SCENES)  ← web/src/config/scenes.json
        │              纯函数，render 期派生
        │              返回 animation name | null
        ▼
  <AsciiAnimation name={...} />
        ├─ useAsciiFrames(name)  → 动态 import() 帧数据（Vite 独立 chunk）
        └─ rAF 帧循环 → 直写 <pre> 的 textContent
```

**不新增运行时依赖，不新增网络端点，不动 `shared/` 契约。**
`DeviceCard` 仍是纯展示组件——它拿到 `DeviceState` 后在 render 里派生动画名，
不取数、不进 `useEffect`（`quality-guidelines.md:71`）。

## 2. 帧数据格式

```jsonc
{
  "name": "coding",     // 与文件名一致，便于出错时定位
  "cols": 24,           // 每行字符数，所有行必须相等
  "rows": 12,           // 行数，所有帧必须相等
  "fps": 12,            // 播放帧率
  "frames": [
    "  ,--.  ,--.  ...\n /    \\/    \\ ...\n...",   // rows 行，以 \n 分隔
    "..."
  ]
}
```

**尺寸是数据的一部分，不是渲染参数。** 渲染侧从数据里读 `cols`/`rows` 去定容器尺寸（§6），
所以换一份不同尺寸的素材不需要改任何代码。

### 2.1 字符集必须排除 `\` 与 `"`

JSON 里这两个字符要转义成两字节，而 ASCII art 的字符表通常包含它们
（ASCII-generator 的 complex 表里就有 `\` 和 `"`）。在 288 字符的帧里若各出现十几次，
体积平白涨几个百分点，且让 JSON 难以肉眼校对。

脚本默认字符表在 ASCII-generator 的基础上剔除这两个字符：

```
$@B%8&WM#*oahkbdpqwmZO0QLCJUYXzcvunxrjft/|()1{}[]?-_+~<>i!lI;:,^`'.
```

灰度分级从 70 级降到 68 级，肉眼无差别。

### 2.2 体积预算的来源

| 项 | 计算 |
|----|------|
| 单帧 | 24×12 = 288 字符 + 11 个 `\n`（转义后 22 字节）≈ 313 B |
| 单动画（60 帧） | ≈ 18.8 KB —— R5.1 的 32 KB 上限留了约 70% 余量 |
| 6 个场景合计 | ≈ 113 KB 原始；ASCII art 重复度极高，gzip 后估 17–23 KB —— R5.2 的 60 KB 上限 |

超预算时的第一手段是降 fps 或减帧数，**不是**降尺寸——尺寸掉下去动画就看不清了。

## 3. 场景匹配

### 3.1 配置格式

```jsonc
// web/src/config/scenes.json
{
  "version": 1,
  "locked": "locked",       // 结构化状态专用，见 3.2
  "idle": "dozing",
  "fallback": "active",
  "scenes": [
    { "anim": "coding", "app": ["VS Code", "Visual Studio Code"] },
    { "anim": "video",  "description": ["在看视频", "在看番"] },
    { "anim": "gaming", "app": ["Steam"], "description": ["游戏中"] }
  ]
}
```

### 3.2 判定顺序（这是本设计最容易搞错的地方）

`resolveScene(device, config)` 严格按此顺序，命中即返回：

| 序 | 条件 | 结果 | 为什么排在这里 |
|----|------|------|----------------|
| 1 | `!device.online` | `null`（不渲染） | 卡片已 grayscale；离线设备上「他在写代码」是谎话 |
| 2 | `activity.locked` | `config.locked` | 锁屏是契约里的结构化事实（`contract.go:54`），最可信 |
| 3 | `activity.idle` | `config.idle` | 人不在，前台窗口仍是 VS Code，此时播写代码动画是**错误信号** |
| 4 | `scenes[]` 顺序匹配 | 首个命中的 `anim` | 用户自定义字符串，可信度最低，排最后 |
| 5 | 都不中 | `config.fallback` | 未配置的应用优雅降级，不留空洞 |

第 2、3 步压过第 4 步是 D5 的核心。反过来写（先匹配 app 再看 idle）会产生
「挂机两小时，页面上他还在热火朝天地敲代码」这种最尴尬的失真。

### 3.3 匹配语义

```ts
// 一条规则内部
match(rule, activity) =
  (rule.app         === undefined || rule.app.some(a => eq(a, activity.app))) &&
  (rule.description === undefined || rule.description.some(d => includes(activity.description, d)))
```

- `app` 走**全等**，`description` 走**子串包含**。
  依据：`app` 是映射规则里的固定标识（`VS Code`），值域小且稳定；
  `description` 是给人看的句子（`在写代码 · main.go`），只能子串匹配。
- 数组内部 OR，同规则内 `app` 与 `description` 同时给出时 AND。
- 两个字段都不给的规则**视为配置错误**（它会无条件命中，让后续规则全部失效），
  由 §3.4 的校验拦下。
- 比较前统一 `trim()` + `toLowerCase()`。中文不受大小写折叠影响，英文应用名更宽容。
- 数组顺序即优先级，第一条命中即生效——与客户端 `title_patterns` 的语义
  （`client-windows/internal/mapping/mapping.go`，第一条命中即生效）刻意保持一致，
  一套心智管两处。

### 3.4 未知动画名的拦截（R2.5）

`import.meta.glob` 在构建期就能枚举出 `assets/ascii/` 下的全部文件名：

```ts
const LOADERS = import.meta.glob<AsciiClip>('../assets/ascii/*.json')
export const AVAILABLE = new Set(Object.keys(LOADERS).map(basename))
```

据此写一个 vitest 用例：遍历 `scenes.json` 引用到的每个动画名（含 `locked`/`idle`/`fallback`），
断言它在 `AVAILABLE` 里，且每条规则至少给了 `app` 或 `description` 之一。
**配错的表现是测试红，不是线上空白。**

## 4. 播放器

### 4.1 为什么直写 `textContent` 而不是 setState

12 fps × 页面上 N 张卡片 = 每秒数十次状态更新，每次只为把一段文本换掉。
走 React 状态会让每帧都过一遍 reconciliation，纯粹是浪费。

因此 `AsciiAnimation` 里 React 只负责挂载一个稳定的 `<pre>`，帧内容由 rAF 回调
通过 ref 直写 `textContent`。这是命令式动画的标准做法，不是绕过 React。

React 状态只在两处变化：动画名变了、帧数据加载完成了。

### 4.2 帧循环

```ts
// 时间戳驱动，不是「每 N 次 rAF 换一帧」——后者的实际帧率随显示器刷新率漂移
let last = performance.now()
let acc = 0
const step = 1000 / clip.fps

function tick(now: number) {
  raf = requestAnimationFrame(tick)
  if (document.hidden) { last = now; return }   // 暂停，且不累积欠账
  acc += now - last
  last = now
  if (acc < step) return
  acc = acc % step                              // 丢弃积压，不追帧
  index = (index + 1) % clip.frames.length
  el.textContent = clip.frames[index]
}
```

三个要点：

- **不用 `setInterval`**：后台标签页被节流到 ≥1s，且误差累积。
- **`acc % step` 而不是 `acc -= step`**：标签页切回来时不补播积压的几十帧（R3.3）。
- **cleanup 必须 `cancelAnimationFrame`**（`quality-guidelines.md:46` 对创建资源的 effect 的硬要求）。

### 4.3 加载与竞态

`useAsciiFrames(name)` 负责动态 import：

```ts
useEffect(() => {
  let cancelled = false
  LOADERS[path(name)]?.().then(m => { if (!cancelled) setClip(m.default) })
                        .catch(() => { if (!cancelled) setClip(null) })  // R4.3 静默降级
  return () => { cancelled = true }
}, [name])
```

设备状态变化频繁（每次上报都可能换 SSE 事件），动画名却很少变——依赖数组是 `name` 而非 `device`，
所以状态抖动不会触发重新加载。

同一动画被多张卡片同时使用时，ES module 语义保证只请求一次，天然去重。

**不做多卡片播放同步**：让两张卡片的同一动画对齐到同一帧需要一个共享时钟，
收益是几乎注意不到的视觉整齐，成本是一个全局单例。不值得。

## 5. 懒加载与产物切分

`import.meta.glob` 不带 `{ eager: true }` 时，Vite 为每个匹配文件产出独立 chunk，
只在 loader 被调用时请求。**帧数据因此不进首屏 bundle**（R5.3 / AC6）。

验证方式：`npm run build` 后检查 `server/cmd/server/web/assets/` 下出现独立的
`coding-<hash>.js` 一类的文件，且主 chunk 体积增量远小于帧数据总量。

## 6. 排版：`ch` 单位与行高（跨字体的正确性）

转换脚本按 `cell_height = aspect × cell_width`（默认 `aspect = 2.0`）采样，
**渲染侧的字符单元格宽高比必须与之匹配**，否则动画会被压扁或拉长。

```
容器宽 = cols 个字符宽 → width: {cols}ch      （ch 在等宽字体里就是一个字符宽）
容器高 = rows 行        → height: {rows * lineHeight}em
实际宽高比 = lineHeight / (字符advance宽 / 字号)
```

等宽字体的 advance width 普遍在 0.6em 附近（Consolas ≈ 0.55，Menlo ≈ 0.60，DejaVu Sans Mono = 0.60）。
取 `line-height: 1.2` 时，单元格宽高比 = 1.2 / 0.6 = **2.0**，与脚本默认值吻合。

因此：

- `<pre>` 固定 `leading-[1.2]`，不用 Tailwind 的默认行高（1.5 会让动画纵向拉长 25%）。
- 尺寸用 `ch` / `em` 而非 px，字号变化时比例自动保持。
- **D7 选了系统等宽栈，所以 advance width 有 ±8% 的跨平台浮动，动画会略胖或略瘦。**
  这是 D7 明确接受的代价。若日后觉得不可接受，解法是引入一款自托管等宽字体，
  而不是去调 `aspect`。
- 字号取到让 `{cols}ch` 适配卡片宽度即可（24 列在约 300px 宽的卡片内约需 10–11px），
  实现时按实际卡片宽度定，不硬编码在数据里。

`font-mono` **只加在 `<pre>` 上**。`component-guidelines.md:111` 已经记过一次教训：
等宽字体的 CJK fallback 会让「小时 / 分」字距散开。卡片里的中文文案绝不能被波及（R4.4）。

## 7. 无障碍

| 要求 | 实现 |
|------|------|
| 不被朗读（R4.1） | 容器 `aria-hidden="true"`。屏幕阅读器读到的仍是既有的 `VS Code · 在写代码` |
| 减少动态效果（R4.2） | `window.matchMedia('(prefers-reduced-motion: reduce)')`，为真则只写首帧、不启动 rAF；并监听其变化 |
| 不因加载失败而破相（R4.3） | `clip === null` 时渲染空的等尺寸容器，无错误提示 |
| 布局不跳（R3.4/R3.5/AC5） | 容器尺寸来自 `cols`/`rows`，加载中／失败／播放中三态**同尺寸** |

装饰性元素 `aria-hidden` 是这个项目的既有纪律（`component-guidelines.md:121`，
总计条带的装饰性 bar 就是这么处理的），本任务照做。

## 8. 对既有前端规范的偏离（必须同步 spec）

`component-guidelines.md:18` 写的是：

> **Animation: Framer Motion** ... Don't hand-write CSS keyframes.

本任务用手写 rAF 循环，构成偏离。判据是**这不是同一类动画**：

| | Framer Motion 适用 | 本任务 |
|---|---|---|
| 动的是什么 | CSS 属性（位移、透明度、布局） | `<pre>` 的**文本内容** |
| 插值 | 在两个值之间补间 | 帧是离散的，不存在「两帧之间」 |
| 帧率 | 跟随显示器刷新率 | 固定 12 fps，由素材决定 |

用 Framer Motion 表达「每 83ms 换一段字符串」只能退化成一个计时器，
既没用上它的补间能力，还多绕一层。

`frontend/index.md:14` 明确规定「real code diverges for a good reason → update the spec in the
same task」，所以 R6.1 不是可选项：要在动画一节补一条「逐帧文本动画走 rAF」的例外及其判据，
否则下一个人会照着规范把它改回去。

## 9. 转换脚本

`web/scripts/gif2ascii.py` —— **离线工具，CI 不运行，不进任何构建流程**。

算法照搬 `vietnh1009/ASCII-generator` 的 `img2txt.py`（灰度均值 → 字符表），
加三样它没有的东西：

1. **视频/GIF 逐帧循环** —— `cv2.VideoCapture`，按 `--fps` 抽帧（源 30fps → 目标 12fps 即每 2–3 帧取一帧）。
2. **输出 JSON 而非 txt** —— §2 的格式。
3. **自校验**（R1.5）—— 写文件前断言每帧 `rows` 行、每行 `cols` 列，不满足直接非零退出。

参数：`--input` / `--output` / `--cols` / `--fps` / `--aspect`（默认 2.0，与 §6 配套）/
`--charset`（`simple` | `complex`，默认 complex）/ `--invert`（浅底素材需要）。

调参建议写进脚本 docstring：先用 asciistudio.space 肉眼试出满意的密度/对比度，再落到本脚本参数。

## 10. 兼容性与回滚

- **纯增量**：不改上报契约、不改 `shared/`、不改服务端、不改 `useDeviceStream`、
  不改任何既有组件的对外行为。`DeviceCard` 只是多渲染一个子组件。
- **回滚** = 从 `DeviceCard` 摘掉 `<AsciiAnimation>` 一行，删 `assets/ascii/`、`config/scenes.json`、
  `lib/scene.ts`、`components/AsciiAnimation.tsx`、`hooks/useAsciiFrames.ts`、`scripts/gif2ascii.py`，
  重新 `npm run build` 并提交产物。无数据迁移，无用户侧影响。
- **旧客户端**：不受影响。没有 `Locked` 字段的老客户端解码为 `false`，
  匹配退到 idle / app 分支，行为合理。

## 11. 主要风险

| 风险 | 缓解 |
|------|------|
| 客户端映射规则改名后动画失配 | 掉回 fallback 是设计内的降级（D1）；`scenes.json` 与 `config.yaml` 的对应关系写进 `web/README.md` |
| 帧数据体积失控 | §2.2 的预算 + AC6 的硬上限；超了先降 fps/帧数，不降尺寸 |
| 行高/宽高比没配套，动画被拉长 | §6 钉死 `leading-[1.2]` 与 `aspect=2.0` 的对应关系，并在实现时肉眼校准一次 |
| `font-mono` 波及卡片中文文案 | 只加在 `<pre>` 上；`component-guidelines.md:111` 已有前车之鉴 |
| 多卡片 rAF 造成性能问题 | 直写 `textContent`（§4.1）；`document.hidden` 暂停；实测若仍有问题再考虑共享时钟 |
| 产物与源 GIF 不同步 | D3 已接受（无关卡）；源 GIF 的出处记在 `web/scripts/README` 或脚本注释里 |
| 下一个人按规范把 rAF 改回 Framer Motion | §8 的规范同步（R6.1）是本任务的交付物之一，不是可选项 |
