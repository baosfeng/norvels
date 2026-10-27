# 保真对账（parity）操作手册

> 一次性夹具 + 逐页机器对账；判据见 §5，定位与最近结果见 §6。
> 全部产物落 `/tmp/norvels-parity/`，语料与截图不入仓（隐私红线）。

## 1 复跑

```bash
scripts/parity-fixture.sh fixture   # 12 本虚构书 → /tmp/norvels-parity/data.db；后端 :8099
scripts/parity-fixture.sh web       # Vite :5173（被占则 :5180，端口写入 /tmp/norvels-parity/web.port）
scripts/parity-fixture.sh spec|parity|shots   # 原型快照+spec / 逐页对账 / 1440×900+390×844 截图
scripts/parity-fixture.sh stop      # 停后端 / Vite / 浏览器会话
```

`all` = 以上全部。各步幂等：已有库与服务时复用。

## 2 夹具

- 12 本虚构小说（书名 / 作者 / 地名全虚构）；2 个系列：雾岭志异（3 本）、长街拾遗（2 本）；
  《三更杂货铺》分 3 卷（文件名「（第 N 卷）」，导入器自动合并成 1 本 90 章）。
- 每本 60–150 章；分类由目录层级推断（如 `志怪/山野`）；标签、评分、阅读状态、进度、阅读时长、2 个书单由 API 补齐。
- 页面 URL 里的 bookId / chapterId / 系列名由 API 现查，脚本不硬编码。

## 3 页面 → 锚点矩阵（主集 = 默认态可见或常驻）

外壳 PC（11）：C-01 C-02 C-03 C-05 C-06 C-07 C-08 C-09 C-10 C-11 C-12；
外壳 H5（7）：C-01 C-02 C-03 C-11 C-12 C-14 C-16。

| 页面 | 档 | parity 域 | 锚点（主集） | 数 |
|---|---|---|---|---|
| `/home` | H5 | discovery | C-15、C-42..C-44、C-46..C-48 + 外壳 H5 | 14 |
| `/shelf` | PC | discovery | C-13、C-19、C-22..C-26、C-28、C-30..C-32、C-35..C-38 + 外壳 PC | 26 |
| `/search` | PC | discovery | C-49、C-50、C-54、C-55 + 外壳 PC | 15 |
| `/series` | PC | discovery | C-56、C-57、C-60、C-61 + 外壳 PC | 15 |
| `/series/:name` | PC | discovery | C-62、C-63 + 外壳 PC | 13 |
| `/history` | PC | discovery | C-66、C-69、C-70 + 外壳 PC | 14 |
| `/book/:id` | PC | library | C-01..C-11、C-13..C-16、C-32 | 16 |
| `/authors` | PC | library | C-19..C-22 | 4 |
| `/dashboard` | PC | library | C-23..C-30 | 8 |
| `/import` | PC | ops | C-01..C-19 | 19 |
| `/admin` | PC | ops | C-26..C-43 | 18 |
| `/admin/books/:id` | PC | ops | C-45..C-69 | 25 |
| `/read/:id/:cid` | PC | reader | C-01、C-03..C-08、C-11..C-13 | 10 |
| 404（任意未知路径） | PC | discovery | C-18 + 外壳 PC | 12 |

**deferred（不进主集，需单独造态）**：
- discovery（22）：C-04、C-17、C-20、C-21、C-27、C-29、C-33、C-34、C-39、C-40、C-41、C-45、
  C-51、C-52、C-53、C-58、C-59、C-64、C-65、C-67、C-68、C-71。
- library（4）：C-12、C-17、C-18、C-31；ops（17）：C-20..C-25、C-44、C-70..C-79；
  reader（52）：C-02、C-09、C-10、C-14..C-62。

## 4 spec 子集怎么来

原型是「多页合一」单文件，整份 spec 对单页 URL 会整页 anchor-missing。脚本用 jsdom 跑原型脚本（拿到动态章节 / 书卡 / 图表），
按页保留目标 view/page、删 demo bar 与 scripts，`odops spec` 后按 §3 矩阵过滤写 `spec/subsets/<域>-<页>.spec.json`，
其 `proto.anchors` 即该页锚点数。

## 5 判定口径

```bash
odops parity /tmp/norvels-parity/spec/subsets/<域>-<页>.spec.json \
  http://localhost:5173/<页面> --tol 3 --json
```

**过线** = 退出码 0（P0=0）且 `diffs` 里没有 `anchor-missing`、`proto.anchors` 等于 §3 锚点数。

注意：`counts.total` 是**差异条数**（`odops.mjs`：`total: diffs.length`），全一致时为 0，不是锚点数；
旧稿「counts.total == 锚点数」的写法按此纠正。P1（内边距 / 宽高 / 实现多出的锚点）不阻塞过线，逐条按三分法处理。

## 6 定位与最近一次结果（2026-10-27）

13 页全 FAIL：9 页拿到数据，合计 P0 231 / P1 456；4 页工具无输出（会话限制，非实现缺失）。
P0 构成：color 67 / tap-target 52 / radius 50 / font-size 48 / anchor-missing 45 / background 35。
- 样式类占八成：原型与实现的字号、颜色、圆角、底色；tap-target 是 parity 按触屏 44px 判，本项目 PC 口径 32px。
- `anchor-missing` 全是默认夹具没造出的条件控件（分页、批量条、清空按钮）；源码锚点静态对账 244/244，已被证伪。

**结论：P0≠0，不宣称 parity 通过**。`odops parity` 的点击区阈值硬编码、无视口参数，「改 PC 32px」不可行，
因此它只作设计评审工具、**不进 CI 门禁**；交付判据 = 锚点静态对账 + 定向用例 + 全量门禁。
剩余关闭路径：①设计确认后统一字号 / 圆角 / 颜色；②逐页造态（只剩 anchor-missing，收益低）。

## 7 三分法

| 类 | 含义 | 处理 |
|---|---|---|
| A 翻译损耗 | 换栈导致（字体度量、行高取整、布局宽度） | 容差内放过；超差报备 |
| B 设计水准 | 原型本身不达标 | 回作者改原型，不放宽实现 |
| C 原型没画 | 需求存在但原型没覆盖 | 回作者补契约与原型 |

## 8 已知限制

- `odops parity` 不能指定视口（固定 1280×577），1440 / 390 只做截图与锚点存在性，**H5 不做数值 parity**；`/home` 仅 H5，1280 下重定向到 `/shelf`，只做 390 检查。
- 状态 / 弹层锚点不在主集（空 / 加载 / 错误 / 成功、确认框、抽屉打开态）；要单独造态再比。
- C-17（按需渲染）与 C-04（H5 侧栏首页项）未进主集，标「待确认」；原型与契约有偏差处按契约过滤（子集已剔除仅 H5 控件）。
- 外壳锚点只在发现域页面验证；夹具列表条数与原型示例不同，宽高类 P1 会偏多，判据只认 P0 与缺块。
- 浏览器 / daemon 偶发不可用：脚本内已 3–4 次重试；仍失败看 `/tmp/norvels-parity/*.log`。
