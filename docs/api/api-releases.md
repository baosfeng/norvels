# 发版说明（releases）

应用内「更新日志」页（`/changelog`）的数据源。内容是 `internal/releases/CHANGELOG.md`，
用 `go:embed` 打进二进制：用户跑在 NAS 上只看网页，产品里读不到说明等于没写。

## GET /api/releases

无参数，按版本号倒序（不是按日期）。

```json
{
  "current": "0.9.0",
  "items": [
    {
      "version": "0.9.0",
      "date": "2026-11-02",
      "retro": false,
      "sections": { "added": ["…"], "changed": [], "fixed": [], "upgrade": [] }
    }
  ]
}
```

| 字段 | 含义 |
|---|---|
| `current` | 运行版本在说明里的对应版本号；**对不上时为空串**（`dev` 构建、说明缺该版本）。前端据此决定是否高亮「正在使用」，不猜 |
| `items[].version` | 版本号，不带 `v` 前缀 |
| `items[].date` | 发布日 `YYYY-MM-DD`；回溯条目取自 tag 创建日期 |
| `items[].retro` | 该版条目是事后从 git 历史回溯整理的（早期版本发版时没写说明） |
| `items[].sections` | 四组固定键 `added` / `changed` / `fixed` / `upgrade`；空组是 `[]` 不是 `null` |

## 失败口径

- 说明文件解析失败 → `500` `{"error":{"code":"internal","message":"发版说明不可用"}}`。
  **不返回空数组**：说明是随二进制发布的产物，解析不了就是发布事故，空列表会让它看起来像正常情况。
- 方法不对 → `405`（与其它端点同一套路由表判定，带 `Allow` 头）。

## 一致性

`Makefile` 的 `VERSION` 必须在说明文件里有对应 `## [x.y.z]` 一节，由门禁 10
（`scripts/check-build-parity.sh`）强制；写发版说明的流程见 [release.md](../release.md) §5。
