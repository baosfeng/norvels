import { describe, expect, it } from "vitest";
import { tierFallbackText } from "./tierFallback";

describe("错档落点提示（契约 C-04 / C-05 边界）", () => {
  it("窄屏访问 PC 专属页 → 回首页并说明「在窄屏不提供」", () => {
    expect(tierFallbackText("/series")).toBe("「系列」在窄屏不提供，已回到首页。");
    expect(tierFallbackText("/series/长街拾遗")).toBe("「系列」在窄屏不提供，已回到首页。");
    expect(tierFallbackText("/history")).toBe("「阅读历史」在窄屏不提供，已回到首页。");
    expect(tierFallbackText("/dashboard")).toBe("「驾驶舱」在窄屏不提供，已回到首页。");
    expect(tierFallbackText("/authors")).toBe("「作者」在窄屏不提供，已回到首页。");
  });

  it("宽屏访问首页 → 回书架并说明「只在窄屏提供」", () => {
    expect(tierFallbackText("/home")).toBe("「首页」只在窄屏提供，已回到书架。");
  });

  it("未知路径不硬造提示", () => {
    expect(tierFallbackText("/nope")).toBeNull();
    expect(tierFallbackText(undefined)).toBeNull();
    expect(tierFallbackText(null)).toBeNull();
  });
});
