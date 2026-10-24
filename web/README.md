# web/ · norvels 前端

React 19 + TypeScript + Vite + Tailwind v4，按定稿原型 `.open-design/08-prototype.html` 实现。
构建产物落 `web/dist`，由 `make build` 同步进 `internal/webui/dist` 后经 `go:embed` 打进二进制。

## 命令

```bash
pnpm install
pnpm dev          # 开发服务器 :5173，/api 代理到 :8080
pnpm build        # 产物落 dist/
pnpm typecheck    # tsc --noEmit
pnpm lint         # eslint
pnpm test         # vitest
pnpm exec vitest run --coverage   # 覆盖率（阈值 70%）
```

联调前先起后端（另一个终端）：

```bash
./bin/norvels import --dir <txt 目录> --db /tmp/web-dev.db
./bin/norvels serve --addr :8080 --db /tmp/web-dev.db
```

设置了 `NORVELS_AUTH_TOKEN` 时，dev 代理会自动带上同一个 Bearer 头（见 `vite.config.ts`）。

## 目录

```
src/
  api/        client.ts（唯一请求出口：超时 / 取消 / 错误分类）+ hooks.ts（各域取数）+ types.ts
  components/ StatePanel（加载 / 空 / 错误共用结构）、Cover、Icons
  features/   start（H5 首屏）/ shelf / book / reader / search / import / authors / dashboard
  layouts/    AppShell（PC 外壳 + H5 汉堡抽屉）、Sidebar、Topbar、TierRoute
  lib/        tier（PC/H5 分档）、theme（三主题）、format、snippet（受限渲染）、audit（自检）、useResource
  styles/     tokens.css（warm-editorial）+ prototype.css（原型样式逐条移植）+ globals.css
```

## 几个已定的事实

- **路由用 BrowserRouter**：服务端对非 `/api/*` 的未知路径回退 `index.html`（`docs/api.md` §6，实测 `/book/1` 返回 200），不需要 hash。
- **H5/PC 是两套信息架构**：`useTier()` 读 `(max-width: 768px)`，`TierRoute` 在 H5 下**不挂载**导入 / 作者 / 驾驶舱，直达这些地址会被送回 `/home`。不是 CSS 隐藏。
- **四态**：每个数据区都由 `useResource` 给出 loading / error，空态由数据判断，面板统一走 `StatePanel`。
- **不引图表库**：第二段的六张图继续手写 SVG/CSS。
- **术语与重复信息自检**：`lib/audit.ts` 的 `auditDuplicates` / `findForbiddenTerms` 有单测，也可在浏览器控制台对同一份选择器跑。
