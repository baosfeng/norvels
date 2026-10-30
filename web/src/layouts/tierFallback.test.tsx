import { describe, expect, it } from "vitest";
import { tierFallbackText } from "./tierFallback";

describe("错档落点提示（契约 C-04 / C-05 边界）", () => {
  it("窄屏访问 PC 专属页 → 回首页并说明「在窄屏不提供」", () => {
    expect(tierFallbackText("/dashboard")).toBe("「驾驶舱」在窄屏不提供，已回到首页。");
    expect(tierFallbackText("/authors")).toBe("「作者」在窄屏不提供，已回到首页。");
    expect(tierFallbackText("/authors/12")).toBe("「作者」在窄屏不提供，已回到首页。");
    expect(tierFallbackText("/admin/books/1")).toBe("「书籍管理」在窄屏不提供，已回到首页。");
    expect(tierFallbackText("/import")).toBe("「导入管理」在窄屏不提供，已回到首页。");
  });

  // 回归：系列 / 阅读历史已对 H5 放开，不再产生重定向，也就不该再有提示
  it("两档都开的页面不产生错档提示", () => {
    expect(tierFallbackText("/series")).toBeNull();
    expect(tierFallbackText("/series/长街拾遗")).toBeNull();
    expect(tierFallbackText("/history")).toBeNull();
    expect(tierFallbackText("/shelf")).toBeNull();
    expect(tierFallbackText("/book/7")).toBeNull();
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
