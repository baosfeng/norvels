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
      thresholds: { lines: 70, functions: 70, branches: 70, statements: 70 },
    },
  },
});