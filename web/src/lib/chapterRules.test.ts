import { describe, expect, it } from "vitest";
import { countChapters, previewChapters, ruleError } from "./chapterRules";
import { formatWords } from "./format";

describe("自定义切章规则：试切计算", () => {
  it("按行命中计数，返回第一条命中的规则", () => {
    const text = "=== 第1节 ===\n正文\n=== 第2节 ===\n正文";
    expect(countChapters(text, "^=== 第\\d+节 ===$")).toBe(2);
    expect(previewChapters(text, [{ name: "分节", pattern: "^=== 第\\d+节 ===$" }])).toEqual({
      ruleName: "分节",
      chapters: 2,
    });
  });

  it("前面的规则没命中就试下一条", () => {
    const text = "第1章 开端\n正文\n第2章 承接";
    expect(
      previewChapters(text, [
        { name: "分节", pattern: "^=== 第\\d+节 ===$" },
        { name: "章节", pattern: "^第\\d+章" },
      ]),
    ).toEqual({ ruleName: "章节", chapters: 2 });
  });

  it("一条都没命中返 null，表示导入时会回退内置策略", () => {
    expect(previewChapters("没有标记的正文", [{ name: "分节", pattern: "^=== 第\\d+节 ===$" }])).toBeNull();
    expect(previewChapters("", [])).toBeNull();
    // 非法正则不能把预览算成「切不开」，这里同样按 0 命中处理
    expect(countChapters("第1章", "([")).toBe(0);
  });
});

describe("自定义切章规则：保存前校验", () => {
  it("非法正则、空名称、空正则分别给出人话错误", () => {
    expect(ruleError("分节", "([", 0)).toContain("第 1 条规则的正则写得不对");
    expect(ruleError("  ", "^第", 1)).toContain("第 2 条规则要有名字");
    expect(ruleError("分节", "  ", 0)).toContain("还没填");
    expect(ruleError("分节", "^第\\d+章", 0)).toBe("");
  });
});

describe("单章字数格式化", () => {
  it("千分位、四舍五入到整数，空值按 0 字", () => {
    expect(formatWords(1234)).toBe("1,234 字");
    expect(formatWords(1234.6)).toBe("1,235 字");
    expect(formatWords(0)).toBe("0 字");
    expect(formatWords(Number.NaN)).toBe("0 字");
  });
});
