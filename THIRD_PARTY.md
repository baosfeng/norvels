# THIRD_PARTY：第三方依赖与许可证

> 本文件是 distroless 发布物里唯一的许可证载体：镜像内只有静态链接的二进制，没有 license 文件。
> 全部依赖都是**纯 Go、无 CGO**（`CGO_ENABLED=0` 实测通过），静态链接进单二进制；
> MIT / BSD / Apache-2.0 / MPL-2.0 均允许静态链接与再分发，条件是在分发包内附版权与许可证文本，
> 以本表 + 各依赖仓库的 LICENSE 原文为准。新增依赖时同步更新本表。

## 直接依赖（参与编译进二进制）

| 依赖 | 版本 | 许可证 | 用途 |
|---|---|---|---|
| github.com/PuerkitoBio/goquery | v1.13.0 | BSD-3-Clause | 在线书源规则执行器的 CSS 选择器（internal/ruleexec） |
| github.com/antchfx/htmlquery | v1.3.7 | MIT | 规则执行器的 XPath 求值 |
| github.com/klauspost/compress | v1.20.3 | Apache-2.0 | 正文 zstd 压缩 |
| golang.org/x/net | v0.58.0 | BSD-3-Clause | HTML 解析（goquery 与 ebook 共用） |
| golang.org/x/text | v0.43.0 | BSD-3-Clause | 编码转换（GB18030 / Big5 / Shift-JIS） |
| modernc.org/sqlite | v1.61.0 | BSD-3-Clause | 纯 Go SQLite 驱动（CGO-free 的前提） |
| github.com/dop251/goja | v0.0.0-20261030192516-032d024709ca | MIT | 规则脚本的 JS 沙箱执行器（internal/jsruntime） |

## 直接依赖（仅测试使用，不进发布二进制）

| 依赖 | 版本 | 许可证 | 用途 |
|---|---|---|---|
| github.com/cucumber/godog | v0.16.0 | MIT | Gherkin 验收测试 |

## 间接依赖（随直接依赖进入二进制）

| 依赖 | 版本 | 许可证 | 用途 |
|---|---|---|---|
| github.com/andybalholm/cascadia | v1.3.4 | BSD-2-Clause | goquery 的 CSS 选择器引擎 |
| github.com/antchfx/xpath | v1.3.9 | MIT | htmlquery 的 XPath 引擎 |
| golang.org/x/sys | v0.49.0 | BSD-3-Clause | 系统调用封装（x/net / sqlite 依赖） |
| modernc.org/libc | v1.79.0 | BSD-3-Clause | SQLite 的 C 库 Go 移植 |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause | sqlite 依赖的数学工具 |
| modernc.org/memory | v1.12.1 | BSD-3-Clause | sqlite 依赖的内存分配器 |
| github.com/dustin/go-humanize | v1.1.0 | MIT | sqlite 依赖的数值格式化 |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause | sqlite 依赖的 UUID |
| github.com/golang/groupcache | v0.0.0-20210331224755-41bb18bfe9da | Apache-2.0 | sqlite 依赖的 LRU 缓存 |
| github.com/hashicorp/go-immutable-radix | v1.3.1 | MPL-2.0 | 验收测试框架依赖（godog） |
| github.com/hashicorp/go-memdb | v1.3.5 | MPL-2.0 | 验收测试框架依赖（godog） |
| github.com/hashicorp/golang-lru | v0.5.4 | MPL-2.0 | 验收测试框架依赖（godog） |
| github.com/mattn/go-isatty | v0.0.24 | MIT | 终端判断（godog 输出） |
| github.com/ncruces/go-strftime | v1.1.0 | MIT | sqlite 的时间格式化 |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause | sqlite 的 big.Int 实现 |
| github.com/spf13/pflag | v1.0.10 | BSD-3-Clause | godog 的命令行参数 |
| github.com/dlclark/regexp2/v2 | v2.11.0 | MIT | goja 的正则实现 |
| github.com/go-sourcemap/sourcemap | v2.1.3+incompatible | BSD-2-Clause | goja 的 sourcemap 支持 |

## 仅测试工具链（不参与任何发布构建）

| 依赖 | 版本 | 许可证 | 用途 |
|---|---|---|---|
| github.com/cucumber/gherkin/go/v42 | v42.0.0 | MIT | Gherkin 解析 |
| github.com/cucumber/messages/go/v34 | v34.2.0 | MIT | godog 的消息类型 |

## 说明

- **为什么不用标准库**：标准库没有 CSS 选择器与 XPath 求值器（`golang.org/x/net/html` 只提供 DOM），
  也没有纯 Go 的 SQLite 实现；自研选择器/XPath 的成本与出错面远大于引入成熟库。
- **CGO 与 distroless**：以上全部为纯 Go 包，`CGO_ENABLED=0 go build ./...` 通过，镜像基于 distroless/static。
- **升级依赖**：`go get <module>@<version>` 后同步改本表版本号；提交前跑 `go mod tidy` 与 `make gate`。