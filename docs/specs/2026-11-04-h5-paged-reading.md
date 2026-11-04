# H5 分页阅读与翻页效果（2026-11-04）

> 目标：手机上有「左右翻页」这种真正的翻页阅读，而不是只能上下滚；五种效果可选，默认滑动。
> PC 一行都不改：宽屏正文继续是连续滚动。

## 1 用户可见行为

| 场景 | 行为 |
|---|---|
| 手机进阅读器 | 默认**左右翻页**；点中间唤出控制栏，左右 30% 翻一页 |
| 章末页再向右 | 进下一章第 0 页（不是「到底了」） |
| 章首页再向左 | 上一章**最后一页**（不是章首） |
| 跨章时新章还没到 | 先显示加载态，排完版再落位；不白屏、不闪回上一章 |
| 设置 → 阅读模式 | 「左右翻页 / 上下滚动」，默认左右翻页 |
| 设置 → 翻页效果 | 滑动（默认）/ 覆盖 / 淡入淡出 / 仿真折页 / 无动画 |
| 自动滚动开关 | 分页下变成「自动翻页」：每页停留 N 秒翻一页，浮条写「自动翻页中」 |
| 底部浮标 | 分页下是「第 p / P 页」，细条按页序走；停手 1.2 秒淡出 |
| PC | 两段设置置灰，常驻「手机专用：电脑上始终是上下滚动」 |

## 2 契约

### 2.1 分页几何（多栏排版真值 + 页层裁剪窗口）

```
.pg-source   宽=页宽 高=页高 column-width:页宽 column-gap:0 column-fill:auto overflow:visible
             scrollWidth / 页宽 = 总页数（唯一真值）
.pg-layer    一页：position:absolute; width:100%; height:页高; overflow:hidden
  .pg-window left:版心左; width:页宽; height:页高; overflow:hidden   ← 裁剪在这里
    .pg-source 的 cloneNode，translateX(-pageIndex × 页宽)
```

- **排版源不能 overflow:hidden**：列是横向溢出排布的，一裁就只剩第一列，translateX 永远翻不出后面的页。
- **页高向下取整到行高整数倍**：`pageH = floor(可用高 / lineH) × lineH`，余量上下均分（`--pg-top`）。
- 量尺寸用 `offsetWidth/offsetHeight`，不用 `getBoundingClientRect`（缩放 / 变换下不可信）。
- 版心左右留白取 `--pg-pad-x`（18px），页宽 = 可视宽 − 2×留白。

### 2.2 手势参数（五种效果共用）

```
起始判定 |dx| > 6 且 |dx| > |dy|      dir = sign(-dx)   左滑 = 下一页
p = |dx| / 可视宽                     提交 p > 0.22 || (|v| > 0.45 px/ms && sign(v)=sign(dx))
回弹 easeOutCubic, base × |p| × 0.75    提交 easeOutCubic, base × (1 - p)
首末页 不建第二层，阻尼 0.18，220ms 回弹 + 到边界提示
时长 none 0 / fade 200 / slide 260 / cover 280 / curl 380ms
```

### 2.3 效果清单

| 效果 | 实现 | 关键点 |
|---|---|---|
| `none` | 松手直接换页 | 拖动时不建第二层，不跟手 |
| `slide` | 两层同步 `translate3d` | 位移基准是**可视宽**不是版心宽（否则两页之间露缝） |
| `cover` | 新页 z3 压入 + 旧页左移 14% + 左缘 44px 投影 | 投影 `opacity = min(1, 3p)` |
| `fade` | **顺序**淡出/淡入 | `outP=min(1,2p)`、`inP=max(0,2p-1)`；交叉淡入会让中段两页文字互相穿透 |
| `curl` | `clip-path` C 形折痕 + 纸背卷边 + Canvas 只画光影 | **不做 3D 硬纸板**：透视重采样会让中文发糊 |

`curl` 折痕：`t = p`（下一页）或 `1-p`（上一页）、`lift = sin(π·t)`、
`crease = padX + W·(1-t)`、`bulge = 6 + 16·lift`、`band = 8 + 34·lift`。
光影按主题明暗取色（夜间不是一块白条）。

### 2.4 设置键

| 键 | 取值 | 默认 | 生效范围 |
|---|---|---|---|
| `norvels.reader.pageMode` | `page` / `scroll` | `page` | 只 H5；PC 一律滚动 |
| `norvels.reader.pageEffect` | `slide` / `cover` / `fade` / `curl` / `none` | `slide` | 只 H5 且分页 |
| `norvels.reader.autoScroll` | 布尔 | `false` | 分页下 = 自动翻页 |
| `norvels.reader.autoScrollSpeed` | 0.5–5 | `3` | 自动翻页：`pageHoldMs` 线性插值，0.5→14000ms、5→3500ms |
| `norvels.reader.autoScrollNext` | 布尔 | `false` | 章末是否继续跨章 |

`prefers-reduced-motion` → 一切效果退化为 `none`（时长 0）。

### 2.5 位置真值与锚点

- 「当前在哪」的真值是 **(chapterId, pageIndex)**，不是 scrollTop。
- 重排锚点是**段落号**不是页码：字号 / 行距 / 字体 / 视口高变化后按段号反查新页。
- 锚点**只在用户真正翻页 / 跳转时更新**；重排触发的重画不许改写锚点（否则字号往返会往下漂）。
- 落点等进度就绪再定，避免「先落章首、进度回来再跳一下」。

## 3 验收数据（390×844 实测）

| 项 | 数值 |
|---|---|
| 视口 / 页宽 / 页高 | 390×748 / 354 / 722（=19×38 行高，padY 13） |
| 本章页数（默认字号 20px / 行距 1.9） | 10 页（scrollWidth 3540） |
| 拖动中途 `slide`（dx=-120, p≈0.31） | cur `translate3d(-120px)`、inc `translate3d(270px)` |
| 拖动中途 `cover`（p≈0.3） | cur `-16.8px`、inc `270px`、投影 `270px` / opacity 0.92 / 宽 44px |
| 拖动中途 `fade`（p≈0.55） | cur opacity 0、inc opacity 0.108（从不同时可见） |
| 拖动中途 `curl`（p≈0.35） | clip-path `M0 0 H263.1 C 282.2 239.4, 255.4 510.7, 267.9 798 H0 Z`、flap left 263.1 / w 36、canvas 390×722 |
| 首末页阻尼（第一页向右拖 144px） | 单层 `translate3d(26.29px)`（=0.18×390×0.5 上限），未翻页 |
| 跨章向前 | 405459 p8/9 → 405460 p0/10 → 405461 p0/12 |
| 跨章回向 | 405461 p0/12 → 405460 p9/10（末页） |
| 书签 | 第 5 页加书签写入 paraIndex=22（该页首段）；跳回 page 4 |
| 进度恢复 | 翻到 page 3 落库 paraIndex=15；退出再进回到 page 3 |
| 跨章书签 | 第 3 章 para14 → /read/804/405461 page=3，1.2s 后仍是 3 |
| 搜索 | 命中页 0→1→2，命中 mark 在页层可见 |
| 滑块跳章 | 405461 p5 → 405462 p0 |
| 键盘 | ←/→ 翻页、Home/End 首末页；焦点留在正文区 |
| 自动翻页 | 5 档 3.5 秒/页（1→2→3）；章末且 autoScrollNext=off 时停下并关开关 |

## 4 工程化决定

| 决定 | 理由 |
|---|---|
| 只给 H5，PC 不动 | 宽屏连续滚动更合适；分页 hook 只在 `mode === "page" && isH5` 挂载，PC 走原路径 |
| 分页引擎与「当前页」分离 | 引擎只管测量与翻页，页码由 React 状态持有；两者混在一起会互相覆盖 |
| 效果 runner 每次翻页开始时创建 | 设置面板换效果只影响下一次翻页，当前页不重建、不闪 |
| 静态页层不加 `will-change` / `backface-visibility` | 常驻会占显存，且回栅格化让文字发虚；只在翻页期间加 `.is-turning` |
| 舞台 `aria-hidden`，另给当页文字层 | 舞台里是整份多栏排版源，读屏读它会把全章念一遍 |
| 零新增 npm 依赖 | 全部用 DOM + CSS 实现，效果层是命令式模块（每帧写样式，不走 React 渲染） |
| 读屏按「页内段落区间」取文 | 区间由 `paragraphRangeOfPage` 纯函数算，跨页长段不会退回章首 |

## 5 未做与已知限制

- **分页模式下不能选词复制**（`.reader-paged` 是 `user-select: none`）。为的是横向拖动翻页不被文字选择打断；想复制请切到「上下滚动」模式。
- `curl` 的折痕是**竖直**的，不是「从右下角斜卷起」：真正的对角卷曲需要网格形变，那会让文字发糊。
- 首末页阻尼不跟手速度（固定 easeOutCubic 回弹），快甩时手感略钝。
- 没做双指捏合调字号（设置面板里调）。
- 分页下不启用捏合 / 长按选词 / 双击缩放。
- 性能数据来自本机桌面 Chrome，未在真机低端安卓上验过。
