# norvels REST API 契约 v1（索引）

> 契约正文按域拆分在 `docs/api/` 下，每份 ≤120 行；本文件只做入口与章节号对照。

- [README.md](api/README.md)：通用约定（错误体/状态码/分页）、搜索、错误码一览、前端约定、鉴权
- [api-books.md](api/api-books.md)：书籍（列表/详情/章节/封面/书签）与作者
- [api-import.md](api/api-import.md)：导入任务（dry-run 预检/上传/重试/取消）与设置（章节规则/标签规则）
- [api-source.md](api/api-source.md)：来源规则包（书源规则包）与能力清单 `/api/settings/capabilities`
- [api-source-import.md](api/api-source-import.md)：导入 Legado 书源 JSON（预览 + 应用）
- [api-replace-rules.md](api/api-replace-rules.md)：全局净化规则（replaceRule）读写与试跑
- [api-dashboard.md](api/api-dashboard.md)：元信息（health/stats）、驾驶舱端点、其它端点
- [api-p3.md](api/api-p3.md)：P3 增量（评分/系列/书单/标签 CRUD/阅读时长与历史/批量与新建章节）
- [api-category.md](api/api-category.md)：书籍分类（0015：两级分类、导入推断、`GET /api/categories`）
- [api-account.md](api/api-account.md)：账号（0016：注册/登录/登出/me）与我的书架；个人记录按账号隔离

原章节号对照：§1/§7/§9 → [api-dashboard.md](api/api-dashboard.md)；§2/§3 → [api-books.md](api/api-books.md)；§4/§8/§10 → [README.md](api/README.md)；§5/§6 → [api-import.md](api/api-import.md)；来源规则包 → [api-source.md](api/api-source.md)。
