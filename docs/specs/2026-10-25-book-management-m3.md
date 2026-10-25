# 书籍管理 M3：卷 CRUD 与章节跨卷移动（C2 后端）

> 批次 C2 的后端实现说明。契约已冻结（task-2），本文只记实现口径、不变量、异常路径与未覆盖项。
> 端点文档：[docs/api/api-books.md](../api/api-books.md)；错误码表：[docs/api/README.md](../api/README.md)。

## 1. 端点

均挂 `/api` 前缀、需登录态，`{id}` 为书籍 id。

| 方法 | 路径 | 请求体 | 成功 |
|---|---|---|---|
| POST | `/api/books/{id}/volumes` | `{"title":"卷三"}` | 201 `{"volume":{"id":9,"idx":3,"title":"卷三","chapters":[]}}` |
| PATCH | `/api/books/{id}/volumes/{vid}` | `{"title":"卷一"}` | 200 同上（卷节点） |
| DELETE | `/api/books/{id}/volumes/{vid}` | — | 204 |
| PUT | `/api/books/{id}/volumes/order` | `{"ids":[3,1,2]}` | 200 `{}` |
| POST | `/api/books/{id}/chapters/move` | `{"chapterIds":[11,12],"volumeId":3}` | 200 `{"moved":2}` |

`volumeId=0` 表示「未分卷」；卷节点的 `chapters` 恒为空数组——卷结构刚变过，章节归属由前端重拉详情。

## 2. 不变量

1. **卷名唯一**：同书内按 `TRIM(title)` 比较（去首尾空白）；空名 400，重名 409 `volume_name_taken`。
   查重保留在事务内，并发竞态由 0013 的表达式唯一索引 `uq_volumes_book_title(book_id, TRIM(title))` 兜底——撞索引同样返回 409（多进程同时建同名卷只有一条落库）。
2. **卷序连续**：本书卷 `idx` 恒为 `0..n-1`；新建卷落在末尾（`idx` = 当前卷数），删除/重排后重排。
3. **计数同源**：`books.volume_count` 每次卷结构变动后由 `COUNT(*) FROM volumes` 重算，不靠增量加减。
4. **章节序号不动**：移动 / 删除卷只改 `chapters.volume_id`，不改 `chapter.idx`、`title`、正文与 `word_count`；删除卷后该卷章节落 `volume_id=0`。
5. **单事务**：五个写入口的校验与写入在同一事务内；任一步失败整体回滚（含 `updated_at` 刷新）。
6. **`books.updated_at` 刷新**：建卷 / 改名 / 删卷 / 重排 / 移动成功后都刷新。

## 3. 异常路径处置

| 场景 | 结果 | 库状态 |
|---|---|---|
| 卷名为空 / 全空白 / 超 200 字 | 400 `invalid_param`（message 带原因） | 不变 |
| 同书重名（含仅首尾空白不同） | 409 `volume_name_taken`「这本书已经有同名卷了」 | 不变 |
| 卷不属于本书 / 卷不存在 | 404 `not_found` | 不变 |
| 书不存在 | 404 `not_found` | 不变 |
| `order.ids` 缺 id / 多 id（含重复）/ 含非本书卷 / 非正整数 | 400 `invalid_param` | 不变 |
| `chapterIds` 为空 / 重复 / 非正整数 | 400 `invalid_param` | 不变 |
| 目标卷不属于本书 / 章节不属于本书 | 404 `not_found` | 不变 |
| `volumeId < 0` | 400 `invalid_param`（0 合法 = 未分卷） | 不变 |
| 路径 id / 卷 id 非正整数 | 400 `invalid_id` | 不变 |

> 契约里的「400 `invalid_argument`」在 API 表面是 `invalid_param`：仓库既有口径（`codeInvalidParam`）把库层 `ErrInvalidArgument` 统一映射成它，本次沿用未新增别名。

## 4. 实现落点

| 文件 | 内容 |
|---|---|
| `internal/store/volume_write.go` | `CreateVolume` / `RenameVolume` / `DeleteVolume` / `ReorderVolumes` / `MoveChaptersToVolume` + 事务体与校验 |
| `internal/store/errors.go` | 新增哨兵 `ErrVolumeNameTaken`（api 映 409） |
| `internal/store/migrate_steps.go` | 0013 的 Go 步骤：老库重名卷改名去重（按 `idx,id` 保留首个）+ 建唯一索引，改名条数进迁移日志 |
| `internal/store/volume_unique.go` | 撞唯一索引 / 并发写被打断时的错误收敛：都映射成 `ErrVolumeNameTaken` |
| `internal/api/handlers_volumes.go` | 5 个 handler + `failVolume`（400 带原因 / 其余走统一 `fail`） |
| `internal/api/errors.go` | 新增错误码 `volume_name_taken` 与 `fail` 中的映射 |
| `internal/api/server.go` | 5 行路由注册（`PUT .../volumes/order` 为字面量段，优先于 `{vid}`） |

设计取舍：
- 卷序重排用**两段式**（先整体 `idx += 1_000_000` 再写 `0..n-1`）。`volumes` 有 `UNIQUE(book_id, idx)`，逐行改写会在中间态撞约束；两段式对小表只多一条 UPDATE。
- 删除卷不删章节、不改正文块：章节只是换归属，`chapter.idx` 是全书序号（阅读进度/书签按 id 指向），动它会静默改掉读者位置。
- 卷名长度上限沿用章标题的 200 字（前端输入框一致），不在契约里，属于边界校验补充。

## 5. 测试覆盖（`go test ./internal/store/ ./internal/api/ -count=1`）

新建卷 `idx` 递增且 `volume_count` 更新 / 重名 409（含首尾空白）/ 空名与超长 400 / 改名成功与自身同名允许 / 删除卷后章节落未分卷且 `idx` 重排、剩余卷连续 / `order` 缺 id、重复、多 id、非本书卷、非法 id 均 400 / 移动跨卷与移回未分卷（`idx`、字数不变）/ 卷或章节 404 / 路径 id 非法 400 / 上述失败用例逐一断言库未变。共 15 个用例（store 9 + api 6）。

0013 的用例（`internal/store/migrate_0013_test.go`、`volume_unique_test.go`）：含重名卷的 v12 库迁移后卷名按 `idx,id` 保留首个、其余改「原标题 (n)」且跳过本书已占用的名字 / 迁移日志报改名条数 / 删版本记录重跑幂等且报 0 条 / `TRIM` 相等的直接插库被索引拒绝 / 两个连接池（等价两进程）并发建同名卷一成功一 409。

## 6. 未覆盖项

- **前端联调**：C2 前端（`web/src`）不在本任务写区，接口按本文契约交付；`PUT order` 返回空对象，前端需重拉详情。
- **空书 0 卷**：`PUT order` 传 `{"ids":[]}` 对 0 卷书视为合法空排列（200），未单独写用例。
- **长卷名复检**：200 字上限只按 `utf8.RuneCount` 计，未校验视觉宽度；前端若放宽输入上限需同步。
- **性能**：删除/重排卷是 O(n) 逐行 UPDATE，卷数通常个位数，未做基准。
