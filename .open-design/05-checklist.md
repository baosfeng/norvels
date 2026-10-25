# 05 自测清单（H5 专项 · 2026-10-25）

定稿：`.open-design/08-prototype.html`（H5 态已合并；PC 未改）

## verify

命令：`odops verify .open-design/08-prototype.html --template web-prototype --platform web --browser`

结果：退出码 0，「机械可判的项目全过」（2026-10-25 实跑）

- 自包含 / CSS 内联 / token 56 槽 / 无裸色值 / data-od-id 21 个 / 四断点 / 画布一致 / 无横向溢出 / 对比度：全 ok
- 官方 anti-slop lint（--fail-on p1）：干净
- warn（不阻塞）：--accent 每屏 4 次；PC 侧栏 16 个 38px 触控目标（1440 度量）；若干 <14px 既有小字

## 断言（定稿实测）

- 横向溢出：390 / 768 / 1440 均为 `scrollWidth - innerWidth = 0`
- 书架 390：初始 30 本；滚到底两次 → 90 本（每页 30 自动追加）；排序弹层 5 项；选「书名」即时重排
- 阅读器 390 收起：`h5-controls=false`，顶栏 top=-62、底栏 top=844（视口高 844）、正文 top=0，无浮层
- 单击唤出：顶栏 top=0、底栏出现；顶部含 返回/书名/第 X/Y 章/目录/设置；可见按钮最小边长 44
- 3.2s 后自动收起；设置/目录打开时暂停自动收起
- 底部「下一章」42→43、左右滑动 43→44，唤出态均保持
- 文案红线：`body.innerText` 命中 0
- PC 像素对比（PIL，1440×900，书架与阅读器各一张）：除右下评审用演示状态条外 0 差异；差异 bbox 均为 (920, 808, 1440, 900)

## 截图

- `shots/h5-390-shelf.jpg` · `shots/h5-390-reader-closed.jpg` · `shots/h5-390-reader-open.jpg`
- `shots/final-390.jpg` · `shots/final-768.jpg` · `shots/final-1440.jpg`

## 已知未做

- 目录虚拟列表行高 36px（共享虚拟滚动常量，未动 PC）；H5 抽屉行高未达 44
- 真机 iOS 未验（Chromium 下全屏能力探测与沉浸式降级已实测）
- 书目为前端合成数据（865 本虚构书名），真实接口未接
