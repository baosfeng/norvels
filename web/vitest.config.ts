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
      exclude: ["src/**/*.test.{ts,tsx}", "src/test-setup.ts", "src/api/types.ts"],
      // 防退化基线（只升不降）。数值 = vitest 5 新口径实测（88.2 / 85.14 / 80.87 / 90.26）留 0.3 余量。
      // 口径变化：@vitest/coverage-v8 5 把 v8-to-istanbul（sourcemap 逐行映射）换成 ast-v8-to-istanbul，
      // 同一份代码从旧口径 92.11 / 89.47 / 78.99 / 92.11 变为 88.2 / 85.14 / 80.87 / 90.26；
      // istanbul provider 实测同量级（88.2 / 84.41 / 80.87 / 90.26），确认不是单一 provider 的映射问题。
      // 旧值不可直接沿用（沿用会恒红），本行基线待 leader 按新口径重钉。
      thresholds: { lines: 89.9, functions: 80.5, branches: 84.8, statements: 87.9 },
    },
  },
});