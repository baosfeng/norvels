# 02 风格

- 风格 id：`warm-editorial`
- 来源：用户拍板 2026-10-24（lead 转达）；非默认 apple。
- 包位置：`/Applications/Open Design.app/Contents/Resources/open-design/design-systems/warm-editorial`
- 读取方式：`odops style warm-editorial`、`odops style warm-editorial --tokens`

## DESIGN.md 要点

- 气质：温暖、不慌不忙、杂志感；「线上版的《纽约客》访谈栏目」。大量留白、长文可读性优先、克制的界面装饰。
- 底色 `#fbf6ee`（米黄纸），前景 `#201914`（近黑偏暖）；**不出现纯黑纯白**。
- 强调 `#9b5b32`（陶土棕），用于链接、主 CTA、每页最多一个主角元素；次强调 `#4f8a4f` 仅用于成功态，避免抢主色。
- 字体：标题衬线（Georgia / Times New Roman），正文 Inter/system-ui，等宽用于数字与元信息。
- 字号阶梯：12 · 14 · 17 · 20 · 28 · 42 · 64 · 88；正文行高 1.62，标题行高 1。
- 组件：按钮扁平填充、圆角 10/16/24、卡片 1px 暖灰边框、无阴影（除 hover 浮层）；输入框下划线式，聚焦陶土色下划线。
- 布局：12 列、最大宽 1180px、桌面 36px 边距；区块上下 112/80/56px 呼吸。
- 动效：150ms / 240ms，标准曲线 cubic-bezier(0.2, 0, 0, 1)，尊重 prefers-reduced-motion。
- 禁用：渐变、emoji、>24px 圆角、玻璃拟态、新造色值。

## tokens.css 变量表（逐字，填色阶段整段粘进第一个 <style>）

```css
:root {
  --bg: #fbf6ee;
  --surface: #fffdf8;
  --surface-warm: #f1e3cf;
  --fg: #201914;
  --fg-2: #4c4037;
  --muted: #7a6d63;
  --meta: #9b5b32;
  --border: #ded2c3;
  --border-soft: #eee4d7;
  --accent: #9b5b32;
  --accent-on: #ffffff;
  --accent-hover: color-mix(in oklab, var(--accent), black 8%);
  --accent-active: color-mix(in oklab, var(--accent), black 14%);
  --success: #4f8a4f;
  --warn: #c9822f;
  --danger: #b33a3a;
  --font-display: Georgia, "Times New Roman", serif;
  --font-body: Inter, system-ui, sans-serif;
  --font-mono: "SF Mono", ui-monospace, Menlo, monospace;
  --text-xs: 12px;
  --text-sm: 14px;
  --text-base: 17px;
  --text-lg: 20px;
  --text-xl: 28px;
  --text-2xl: 42px;
  --text-3xl: 64px;
  --text-4xl: 88px;
  --leading-body: 1.62;
  --leading-tight: 1;
  --tracking-display: -0.025em;
  --space-1: 4px;
  --space-2: 8px;
  --space-3: 12px;
  --space-4: 16px;
  --space-5: 20px;
  --space-6: 24px;
  --space-8: 32px;
  --space-12: 48px;
  --section-y-desktop: 112px;
  --section-y-tablet: 80px;
  --section-y-phone: 56px;
  --radius-sm: 10px;
  --radius-md: 16px;
  --radius-lg: 24px;
  --radius-pill: 9999px;
  --elev-flat: none;
  --elev-ring: 0 0 0 1px var(--border);
  --elev-raised: 0 20px 52px rgba(32, 25, 20, 0.12);
  --focus-ring: 0 0 0 4px rgba(155, 91, 50, 0.24);
  --motion-fast: 150ms;
  --motion-base: 240ms;
  --ease-standard: cubic-bezier(0.2, 0, 0, 1);
  --container-max: 1180px;
  --container-gutter-desktop: 36px;
  --container-gutter-tablet: 24px;
  --container-gutter-phone: 16px;
}
```

## 阅读器三主题的映射计划（填色阶段落地）

| 主题 | 页面底 | 表面 | 正文 | 次要文字 | 边框 | 说明 |
|---|---|---|---|---|---|---|
| 日间 | #fbf6ee | #fffdf8 | #201914 | #7a6d63 | #ded2c3 | 默认，暖纸感 |
| 夜间 | #171514 | #211e1b | #e8e0d6 | #a89c90 | #35302b | 不纯黑底、不纯白字，暖灰调 |
| 羊皮纸 | #f1e3cf | #f7ecd9 | #3a2d22 | #7a5f45 | #d8c3a4 | 更暖、更低对比，适合长时间夜读 |

> 夜间为派生主题：仅重绑语义 token（背景 / 表面 / 文字 / 边框），强调色保持 `--accent` 或按需提亮；不新增品牌色。

## 骨架阶段口径

阶段 3 只做灰阶结构：用中性灰测试层级与断点，不套用上表颜色、不引入衬线标题、不加圆角与动效；主题切换先以灰阶三档验证交互结构，填色阶段替换为正式 token。
