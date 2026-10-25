# 导入 P1 前端交接（w3-web / task-3）

## 基线
- 开工 `9e4f914`（`feat/import-formats`），交付 `b63bb3f`；未切分支、未推送、未动 web/ 之外的文件。

## 任务契约
实现 `docs/specs/2026-10-25-import-p1.md` §2.6：
- `BookMetaPatch` 允许空串与 0；`buildPatch` 以「改过即下发」为准。
- 作者多值编辑（逗号/顿号分隔、去重去空、至少 1 个）。
- 上传预检放行 `.zip` 并提示包内逐条导入；结果表展示包内条目。
- 追更开关随上传/目录导入提交 `update`。

## 已确认契约（leader 2026-10-25 补充）
- PATCH 后端宽容：`authors` 优先，同时传 `author` 会被忽略；只传 `author` 等价 `authors=[author]`。
- 因此前端作者改动**只下发 `authors`**；`author` 仅作只读展示与兼容字段。

## 改动清单
- `api/types.ts`：`BookDetail.authors: string[]`（必需）；`BookMetaPatch.authors?: string[]`，intro/series/seriesIndex 注明 ""/0 = 清空。
- `api/client.ts`：`UploadOptions.update`，上传表单追加 `update`；`startImport` body 增 `update`。
- `features/book/BookPage.tsx`：作者多值输入（`parseAuthors`）、`parseSeriesIndex`（空串→0）、buildPatch 改「与初值不同即下发」、作者清空与非法序号拦下、页头按 `authors` 顿号连接展示。
- `features/import/UploadPanel.tsx`：支持 zip（accept/文案/提示 `upload-zip-hint`）、新增错误码文案（invalid_zip/empty_zip/zip_too_large）、结果改逐条表格、追更勾选 `upload-update`。
- `features/import/ImportPage.tsx`：目录导入同样加追更勾选；单文件判断含 zip。
- 测试：`BookPage.test.tsx` 13 例、`ImportPage.test.tsx` 10 例、`client.test.ts` 断言 update 表单字段、`hooks.test.tsx` 夹具补 authors。

## 验证口径（本机实测）
- `cd web && pnpm run typecheck`（= `tsc --noEmit -p tsconfig.app.json`）→ exit 0。
- `cd web && pnpm test -- --run` → 11 文件 / 131 例全绿（基线 124）。
- `cd web && pnpm run lint` → 0 error（既有 `ChapterList.tsx` 的 react-hooks 警告与本任务无关）。

## 遗留
- 前端假设后端已按 §2.5 返回 `authors`；旧后端缺字段时表单回落到 `authorName`（`authorsOf` 兜底），不会崩。
- 系列序号输入 `type=number min=0`，浏览器原生挡掉负数/非数字，故 `parseSeriesIndex` 的 NaN 分支是防御性代码，无 UI 用例覆盖。
- 未跑 `make gate`（归 leader 在写操作停止后单点跑）。
