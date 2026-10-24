# react-agent 交接文档（第一段）

## 基线
- 开工时 main @ `1be56e9`；收工时 main @ `5c50557`（期间他人合入 ea43114 / 555be2c / 5c50557，与本任务写区无重叠）。
- 未切分支、未 commit。写区：`web/`、`Makefile`；`internal/webui/` 只被 `make build` 写入构建产物（`dist/*` 已在 .gitignore 里），源码零改动。

## 任务契约（task-10，第一段）
把 `.open-design/08-prototype.html` 落成真实 React 应用：五视图 + 驾驶舱骨架跑真数据、路由、tier 判断、四态、API 封装、主题切换；
书架 / 详情 / 阅读器连真数据跑通。第二段再做六图、动效、细节、odops parity。

## 第一段完成项
- Vite 工程（React 19 + TS + Tailwind v4 + react-router 7 + @tanstack/react-virtual）。
- `src/api/client.ts` 唯一请求出口：超时、AbortSignal 取消、错误分类（timeout/network/aborted/http/parse）、鉴权 token。
- `src/lib/useResource.ts`：四态取数 + 过期请求丢弃（请求序号机制，不在 effect 里 setState）。
- 视图：`features/{start,shelf,book,reader,search,import,authors,dashboard}`，全部接真实接口。
- 分档：`lib/tier.ts` + `layouts/TierRoute.tsx`，H5 下管理类路由不挂载（实测直达 `/import` 被送回 `/home`，DOM 里导入/作者/驾驶舱链接数 = 0）。
- 主题：`lib/theme.ts`，日间/夜间/羊皮纸写 `html[data-theme]` + localStorage。
- 虚拟滚动：`features/book/ChapterList.tsx` 用 `@tanstack/react-virtual`，详情页卷内与阅读器抽屉共用。
- 样式：`styles/tokens.css`（逐字取 `02-style.md`）+ `styles/prototype.css`（原型 626 行 CSS 移植，去掉 body[data-view]/data-tier 切视图规则）+ `globals.css` 补丁。
- 构建：`make build` → `make web`（构建 + rsync 进 `internal/webui/dist`）→ `make build-server`。

## 已验证（第一段证据）
| 项 | 命令 | 结果 |
|---|---|---|
| 构建 | `pnpm build` | exit 0，`dist/assets/index-BfnrMsv1.js` 344.56 kB（gzip 107.97 kB）+ `index-Cf29hg1R.css` 51.13 kB |
| 类型 | `pnpm typecheck` | exit 0 |
| Lint | `pnpm lint` | exit 0，0 error / 1 warning（tanstack-virtual 的 react-hooks/incompatible-library，官方已知限制） |
| 单测 | `pnpm exec vitest run --coverage` | Tests 48 passed；语句 93.41% / 分支 86.81% / 函数 91.52%，阈值 70% 通过 |
| 二进制 | `make build` + `./bin/norvels serve` | 内嵌资源 `internal/webui/dist/{index.html,assets/*}`；`/shelf` 渲染 19 张书卡、`/book/3` 直达可渲染、浏览器 console 0 error |
| 隐私 | `scripts/privacy-scan.sh --all` | 零命中 |
| 四断点自检 | 浏览器 eval | 1440 六视图 + 390 四视图：横向溢出 0、术语 0、重复信息 0、H5 可点元素 <44px 计数 0 |

## 截图
落在 `.open-design/shots/web-impl/`（该目录被 .gitignore 排除，不进版本库）：
`shelf-1440` / `dashboard-1440` / `import-1440` / `authors-1440` / `shelf-390` / `home-390` / `book-1-390` / `read-390` / `state-reader-error-390`。

## 已知差异 / 第二段待办
1. 驾驶舱六图未画（当前是说明位），接口数据已就绪：`/stats`、`/health/report`、`/stats/import-trend`、`/reading/progress`、`/authors?sort=words`。
2. 阅读器 H5 左右滑动翻页未做（当前按钮 + 键盘；键盘在 H5 也可用）。
3. 导入页失败明细依赖 `ImportJob.errors`，只有任务详情接口返回，当前只在运行中任务上读该字段。
4. 书架作者筛选 / 标签筛选未接（后端 `/api/tags` 当前返回空，先不铺 UI）。
5. `odops parity` 对照未跑，留到第二段。
6. 原型 `.demo-bar`（状态切换演示条）按要求未进产品。