# 阅读域交互评审 · r2（独立评审）

> 判据：`designing-interactions/SKILL.md` + `reviewer.md` + `references/interaction-contract.md`。作者自述不作为证据，下列结论全部出自本轮自跑命令。
> 平台 = web（H5 ≤768 / PC ≥1024，分档常量 `web/src/lib/tier.ts:9` `H5_QUERY`）。
> 修订链：r1 首评 FAIL → r2 复审 FAIL → **r3 复审（2026-10-27）**。本轮按**锚点 + 单元格内容**定位，不引用易漂移的行号。

## 结论

**复审修订 · PASS-WITH-TODO**（历史结论见 §历史）
理由：上轮 5 条 P0 全部真解决，且原型侧逐条实测复现（重试转 loading、平铺真隐藏卷头、确认框真拦截、三处禁用说明常驻可见）；余下只有 P1/P2（`C-31` hover 行列错位 1 处、目录空态二义未拆、`00-features.md` 未随本轮同步），不阻塞放行开发。

## 第二轮复审覆盖度

- 查了 **5 项**（lead 指定范围）：① C-58 一族收敛 ② C-59/C-60 标新增且无编造行号 ③ C-59 遮罩说明 + C-24 同步 ④ C-61/C-62 真渲染 ⑤ 长按三类残留。
- 未回全量：作者本轮只改契约文案与两处状态渲染，**未动抽屉定位**（`grep -c 'overflow: clip'` 仍为 2，位置未变），按上轮划的边界不回全量。
- `ixcheck` 自跑退出码 **0**：契约行 252 / 锚点 62 / 原型锚点 62 / focus 62-62。
- 真点验证 5 组共 14 个状态读数（C-34 确认框 4 读、C-59/C-60 3 读、C-61 4 读、C-62 3 读）。
- **没查**：夜间/羊皮纸主题（预算）、1024 断点（预算）、`00-features.md` 全表行号（文件未改动，上轮 7 处结论沿用，见 P1-3）、`odops verify` 本轮未重跑（DOM 结构未变，上轮 13 ok / 1 warn）。

## P0 缺陷（复看后状态）

| 对象 | 上轮问题 | 状态 | 本轮验证（命令 + 关键输出） |
|---|---|---|---|
| 原型打开抽屉整屏空白 | 点目录/书签后 `sl=394`、抽屉出屏 | **已解决**（r2 已验，本轮未回归） | 上轮两档六入口 `sl` 恒 0；本轮 `grep -c 'overflow: clip'` = 2 且位置未变，无回归 |
| C-15 先读下一章 | 标现有但产品无 | **已解决** | 控件列「正文空态 · 先读下一章【新增】」，无源码行号 |
| C-31 清空搜索 | 标现有但产品无 | **已解决** | 控件列「目录抽屉 · 空态清空搜索【新增】」，无源码行号 |
| C-58 一族写撤销 | C-34/C-58 仍要求撤销，与确认框矛盾 | **已解决** | C-34 改为「弹出二次确认框（详见 C-58），此时书签尚未删除」；C-58 四行全为确认流程；剩余 2 处「撤销」均为**不做的理由**（「不做撤销条（改造全站提示条代价高于收益）」「删除走二次确认、不做撤销」）。实测：取消后 3 条、确认后 2 条、`#toast-undo.hidden=true` |
| C-59 / C-60 编造行号 | 挂 `ReaderPage.tsx:652`/`BookmarksDrawer.tsx:88` 却无新增标注 | **已解决** | 控件列改「删除书签确认框 · 取消【新增】」「…删除书签【新增】」，**全行无任何 `.tsx:数字`**；脚本核 `NEW_BUT_HAS_LINE` → `none`。边界列已写「现状：产品无确认框，删除直接生效（BookmarksDrawer.tsx）；需：新增确认框与取消/删除两个按钮」 |
| C-59 遮罩语义 | 需写清遮罩按钮 ≠ 取消按钮 | **已解决** | C-59 四行边界均含「确认框复用遮罩层（遮罩本身是既有 C-24）；遮罩可关闭确认框，但遮罩按钮 ≠ 取消按钮，取消是确认框内的独立按钮 C-59」；C-24 同步写「关闭确认框时遮罩只负责关窗…（取消由 C-59 承载）」 |
| C-61 重试假渲染 | 点后按钮文案与 disabled 均不变 | **已解决** | 实测 `t+50ms` → `{text:"正在重试…", disabled:true}`，`#toc-loading` 真显示；`t+1.6s` 错误面板消失、列表回来、焦点落到 `C-28` 首行 |
| C-62 平铺假渲染 | 点后卷头一个没少 | **已解决** | 实测 `{isFlat:true, volHeadersVisible:0, errHidden:true}`，焦点落 `C-28`；平铺后搜「第四」仍可用（`匹配 4 章`，可见行 10→4） |
| 长按三类残留 | C-17/C-20/C-23 写「长按无反应/长按提示」 | **已解决** | `grep -c '长按提示|长按无反应'` → **0**；三条 click 行均改「禁用时旁边常驻 12px 说明『…』」 |

## P1 缺陷（未清完，故 PASS-WITH-TODO）

| 对象 | 问题 | 证据 | 建议改法 |
|---|---|---|---|
| C-31 hover 行列错位 | 「移除后」文案仍填在**前置列**，移除后列是 `—` | 脚本扫 `PRE_POLLUTED` → `L137:C-31:hover`（唯一一处） | 前置列写「—」，移除后列回填焦点/滚动/计数 |
| 目录空态二义 | 上轮 P1，本轮未拆：切「空」档与搜索无匹配渲染同一块 `#toc-empty` | 两路文案实测完全相同（`same=true`）；产品 `TocDrawer.tsx:109` 有独立 `EmptyPanel title="还没有目录"` | 拆「目录为空 / 搜索无匹配」两块；C-31 边界已写此分支，原型补上即可 |
| `00-features.md` 未同步 | 文件仍是 23:07 版本，本轮新增的 C-58/C-59/C-60/C-61/C-62 与改标的 C-15/C-31 均未进 §6.5；上轮 7 处错行号仍在 | `ls -la interaction/reader/00-features.md` → Oct 26 23:07；§6.5 仍只有 3 条 | 业务上不阻塞（契约已自证），但盘点与契约出现口径差；随下次改动补齐 |
| C-58 click 结果列措辞 | click 行「结果」列写「等待用户在确认框里选择」，属状态而非系统结果 | 单元格内容 | 改为「打开确认框，书签保持未删」更贴「结果」定义 |

## P2

| 对象 | 问题 | 证据 | 建议改法 |
|---|---|---|---|
| C-25 目录搜索 | 契约写「不做防抖」，与 `interaction-contract.md` 输入触发默认 300–400ms 相左 | C-25 边界列 | 写明为何偏离（数据在内存） |
| C-16 演示 | 提示条常驻，「无上次记录时不出现」分支无法演示 | 重载后 `#restore-hint` 恒可见 | 演示条加「无恢复记录」档 |
| C-13 名称 | 「返回目录」实际跳书籍详情 | C-13 控件列 | 改名「返回书籍详情」 |
| 12px 元信息 | `odops verify` warn | 上轮 verify 输出 | 保留（token 已冻结，不进产品） |

## 不符直觉清单

1. **「返回目录」不回目录**（r1 起沿用，未改）：章末按钮叫「返回目录」，点了跳书籍详情页；真目录在顶栏「目录」抽屉。按字面理解必然猜错。
2. **同一份契约里删书签有两条路径描述**：C-34（书签行删除按钮）与 C-58（确认框）都写了完整「取消/确认」分支与焦点去向，两处措辞不完全一致（C-34 结果列写「确认删除 → 书签立即移除」，C-58 写「确认删除 → 书签立即移除、不可恢复」）。读契约的人要读两遍才确认是同一件事。
3. **目录搜索每敲一个字整表重排**：不防抖（C-25），长目录下键感明显。
4. **同一「关闭确认框」有三种方式**：C-59 取消按钮、点遮罩（C-24）、Esc 键——三条都在不同锚点里，实现方需自行合并。

## 我剔除的误报（本轮）

- 「C-61 焦点仍落在 BODY」：初测 `focus:BODY`，怀疑焦点管理没做；复核发现是测试脚本在抽屉关闭后才读 `activeElement`，而在抽屉打开态重测得到 `{tag:BUTTON, od:C-28}`——不成立。
- 「C-62 卷组 `hidden=[false,false,false]` 说明没隐藏」：`is-flat` 用 CSS `.toc-vol{display:none}` 隐藏卷头而非 `hidden` 属性，实测 `volHeadersVisible=0` 才是真判据——不成立。
- 「C-58 还残留撤销实现要求」：3 处「撤销」里 2 处在契约、1 处在台账，全部是「不做撤销」的理由句——不成立。
- 「新增锚点仍挂源码行号」：脚本核 9 个新增锚点，`NEW_BUT_HAS_LINE=none`——不成立。

## 历史

- **2026-10-27 r3 复审：PASS-WITH-TODO**（本轮）。5 条 P0 全清，余 P1 4 条 / P2 4 条。
- **2026-10-26 r2 复审：FAIL**（P0=6）。未解决项：C-58 一族未删净撤销口径、C-59/C-60 挂编造行号无新增标注、C-61/C-62 假渲染。
- **2026-10-26 r1 首评：FAIL**（P0=6 / P1=6 / P2=4）。理由：4 个锚点标「现有实现」但产品没有；H5 打开目录抽屉把界面推出视口。取证：`grep -rn 先读下一章 web/src` → 0；`grep -rn 清空搜索 web/src` → 仅注释；`grep -rn 撤销 web/src/lib/toast*.ts*` → 0。

## 可复跑命令

```bash
# 1) 机器门禁（退出码 0；契约 252 行 / 锚点 62 / focus 62-62）
node ~/.agents/skills/designing-interactions/scripts/ixcheck.mjs interaction/reader --platform web

# 2) C-58 一族是否真收敛（期望：2 处「撤销」都是"不做"的理由；新增锚点无源码行号）
grep -n '撤销' interaction/reader/01-contract.md
grep -n '^| C-59 \|^| C-60 ' interaction/reader/01-contract.md | grep -cE '\.(tsx|ts):[0-9]+'   # 期望 0

# 3) C-61 / C-62 真渲染（原型）
export AGENT_BROWSER_SESSION=norvels-ixrev-reader3
agent-browser set viewport 1440 900
agent-browser open "file://$PWD/interaction/reader/02-prototype.html"
agent-browser eval "[...document.querySelectorAll('[data-od-id=\"demo-only:tocstate\"]')].find(b=>b.textContent.trim()==='失败').click();1"
agent-browser eval "document.querySelector('[data-od-id=\"C-61\"]').click();1"
agent-browser eval "JSON.stringify({t:document.querySelector('[data-od-id=\"C-61\"]').textContent.trim(),d:document.querySelector('[data-od-id=\"C-61\"]').disabled})"
# 期望 {t:"正在重试…", d:true}
```
