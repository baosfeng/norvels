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
    // 覆盖率阈值 = 防退化基线（只升不降，与 go 侧 gate-cover.sh 同一口径）。
    // 定成实测值而不是 70 这种宽松数字：70 时删掉一个测试文件覆盖率只掉 0.28 个点，
    // 照样绿——那等于没有防线。当前实测 92.11 / 89.47 / 78.99（三次完全一致），
    // 取整到一位小数并留 0.3 余量给「新增少量未覆盖代码」的正常迭代。
    // 提升覆盖后把这里的数字调到新实测；下调必须有理由并写在提交信息里。
    coverage: {
      provider: "v8",
      include: ["src/api/**", "src/lib/**"],
      exclude: ["src/**/*.test.{ts,tsx}", "src/test-setup.ts", "src/api/types.ts"],
      thresholds: { lines: 91.8, functions: 78.6, branches: 89.1, statements: 91.8 },
    },
  },
});