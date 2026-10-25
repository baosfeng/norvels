import { describe, expect, it } from "vitest";
import { MAX_KEYWORDS, MAX_TAG_RULES, keywordsToText, parseKeywords, tagRulesError } from "./tagRules";

describe("词表规则：关键词解析与保存前校验", () => {
  it("关键词按逗号 / 顿号 / 换行拆开，去空白并去重", () => {
    expect(parseKeywords(" 魔法，精灵, 魔法、龙\n精灵 ")).toEqual(["魔法", "精灵", "龙"]);
    expect(parseKeywords("   ")).toEqual([]);
    expect(keywordsToText(["魔法", "精灵"])).toBe("魔法，精灵");
  });

  it("合法规则通过校验", () => {
    expect(tagRulesError([{ tag: "奇幻", keywords: ["魔法"] }])).toBe("");
    expect(tagRulesError([])).toBe("");
  });

  it("标签名缺失 / 超长 / 重名、关键词为空 / 超量 / 单词超长都拦下", () => {
    expect(tagRulesError([{ tag: "  ", keywords: ["魔法"] }])).toContain("缺少标签名");
    expect(tagRulesError([{ tag: "一".repeat(21), keywords: ["魔法"] }])).toContain("超过 20 字");
    expect(
      tagRulesError([
        { tag: "奇幻", keywords: ["魔法"] },
        { tag: "奇幻", keywords: ["精灵"] },
      ]),
    ).toContain("出现了两次");
    expect(tagRulesError([{ tag: "奇幻", keywords: [] }])).toContain("还没有关键词");
    expect(
      tagRulesError([{ tag: "奇幻", keywords: Array.from({ length: MAX_KEYWORDS + 1 }, (_, i) => `词${i}`) }]),
    ).toContain("关键词超过");
    expect(tagRulesError([{ tag: "奇幻", keywords: ["一".repeat(21)] }])).toContain("超过 20 字");
  });

  it("条数上限与后端一致（50 条）", () => {
    const rules = Array.from({ length: MAX_TAG_RULES + 1 }, (_, i) => ({ tag: `标签${i}`, keywords: ["魔法"] }));
    expect(tagRulesError(rules)).toContain(`最多 ${MAX_TAG_RULES} 条`);
  });
});
