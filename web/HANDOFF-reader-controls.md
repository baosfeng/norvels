# 阅读器控制层交接（w3-web / task-6）

## 基线
- 开工 `b63bb3f`（`feat/import-formats`）；交付提交 `ca46747`（主体）+ `b87e290`（390 实测修出的裁切修复）。
- 本轮只动 web/src/features/reader、web/src/lib、web/src/styles，外加 task-5 遗留①（书架弹层锁 body 滚动）。
- 原型定稿：`.open-design/candidates/h5-c.html`；390 收/开态截图见 `.open-design/shots/h5-c-390-reader-{closed,open}.jpg`。

## 契约（用户拍板原话）
「单击唤出需要，其他的不需要，反正好好的适配一下全屏和触摸屏」→ 只做单击唤出 + 全屏/触摸适配；
不做分区点击、不做双击、不加设置开关。

## 改动清单
- `lib/useReaderControls.ts`（新）：H5 默认收起；单击正文唤出、3 秒无操作收起、再点立即收起；
  `paused`（设置/目录打开）期间暂停计时；判定为「短按 + 位移 ≤12px 且 ≤700ms」，滑动翻章不误触。
  PC 由 hook 直接返回 `visible: true`（常驻）。
- `features/reader/ReaderBottomBar.tsx`（新）：H5 底部翻章栏（上一章/目录/设置/下一章），48px 可点区。
- `features/reader/ReaderPage.tsx`：header 与底栏作为同一控制层（`aria-hidden` + `.is-controls`）；
  全屏按 `document.fullscreenEnabled` 能力探测，无 API（iOS）不渲染按钮，`requestFullscreen` 被拒时
  静默收起控制层当沉浸式；翻章保持显隐状态，进度数字随控制层显隐；滑动/键盘翻章原样保留。
- `styles/globals.css`：H5 阅读器外壳 100dvh 定高 + `position: 相对`（裁掉滑出的控制层）、四边
  safe-area、`overscroll-behavior: none`、控制层 200ms 位移+淡入、reduced-motion 关动画；
  底部弹层打开时 `body.h5-sheet-open{overflow:hidden}`（task-5 遗留①）。
- `features/shelf/ShelfPage.tsx`：弹层开关时切 `body.h5-sheet-open`。
- 测试：`ReaderPage.test.tsx` 10 例；`ShelfPage.test.tsx` 补「弹层锁滚动/选完解锁」断言。

## 验证口径
- `cd web && pnpm run typecheck` → exit 0。
- `cd web && pnpm test -- --run` → 13 文件 / 148 例全绿（上轮 138，新增 10）。
- `cd web && pnpm run lint` → 0 error（仅 `ChapterList.tsx` 既有警告）。
- 390×844 真机尺寸量测（agent-browser + 真实构建 CSS 的布局 harness）：
  收起态 `documentElement.scrollHeight = 844 = innerHeight`、`mainTop = 0`、`contentTop = 24`
  而 header 高 62（覆盖、无占位）、header `opacity 0 / translateY(-62px)`、底栏 `top = 844`（刚好在视口外）；
  唤出态 header `top 0 / opacity 1`、底栏 `top 783 / bottom 844`，文档仍 844。

## 遗留
- 未做控制层外的 body 滚动锁定（阅读器外壳已定高 + overflow hidden，正常不需要）。
- 未跑 `make gate`（归 leader 在写操作停止后单点跑）。
