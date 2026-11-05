import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    environmentOptions: { jsdom: { url: "http://localhost:5173" } },
    setupFiles: ["./src/test-setup.ts"],
    globals: true,
    include: ["src/**/*.test.{ts,tsx}"],
    // 默认 5s 在门禁（go test + 覆盖率插桩 + vitest 并行）叠加时会把 100ms 的用例判成超时，
    // 失败用例每次都不一样。放宽到 20s：真挂死的用例仍会失败，但不再因机器忙而假红。
    testTimeout: 20000,
    coverage: {
      provider: "v8",
      include: ["src/api/**", "src/lib/**"],
      // src/lib/toast.tsx 是纯 re-export 兼容入口（一行 export { ToastHost } from "../layouts/ToastHost"）：
      // vitest 5 的 AST 口径不把 re-export 计为语句，文件会以 0/0 显示成 0%，误导阅读；实现本体在 layouts/ToastHost.tsx。
      exclude: ["src/**/*.test.{ts,tsx}", "src/test-setup.ts", "src/api/types.ts", "src/lib/toast.tsx"],
      // 防退化基线（只升不降，与 go 侧 gate-cover.sh 同一口径）。数值 = vitest 5 新口径实测
      // 88.2 / 85.14 / 80.87 / 90.26 留 0.3 余量；删掉一个测试文件会掉出阈值。
      // 口径变化：@vitest/coverage-v8 5 把 v8-to-istanbul（sourcemap 逐行映射）换成 ast-v8-to-istanbul，
      // 同一份代码从旧口径 92.11 / 89.49 / 78.99 / 92.11 变为 88.2 / 85.14 / 80.87 / 90.26。
      // 下降是口径更精确而不是丢统计：旧口径把 import/顶层声明按行记成已执行，新口径把默认实现里的
      // 箭头函数体也算语句（如 lib/auth.ts 的 ANONYMOUS.enter 等 7 个默认函数从未被调用）。
      // 提升真实覆盖率另立任务，不通过放宽本行数字实现。
      thresholds: { lines: 89.9, functions: 80.5, branches: 84.8, statements: 87.9 },
    },
  },
});
