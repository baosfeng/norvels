import { describe, expect, it } from "vitest";
import { UNLIMITED_TAG_RULES, keywordsToText, parseKeywords, tagRulesError } from "./tagRules";

/** 上限来自 capabilities；这里用后端默认值（50 / 100 / 20 / 20）当夹具。 */
const limits = { maxCount: 50, maxKeywords: 100, maxTagRunes: 20, maxKeywordRunes: 20 };

describe("词表规则：关键词解析与保存前校验", () => {
  it("关键词按逗号 / 顿号 / 换行拆开，去空白并去重", () => {
    expect(parseKeywords(" 魔法，精灵, 魔法、龙\n精灵 ")).toEqual(["魔法", "精灵", "龙"]);
    expect(parseKeywords("   ")).toEqual([]);
    expect(keywordsToText(["魔法", "精灵"])).toBe("魔法，精灵");
  });

  it("合法规则通过校验", () => {
    expect(tagRulesError([{ tag: "奇幻", keywords: ["魔法"] }], limits)).toBe("");
    expect(tagRulesError([], limits)).toBe("");
  });

  it("标签名缺失 / 超长 / 重名、关键词为空 / 超量 / 单词超长都拦下", () => {
    expect(tagRulesError([{ tag: "  ", keywords: ["魔法"] }], limits)).toContain("缺少标签名");
    expect(tagRulesError([{ tag: "一".repeat(21), keywords: ["魔法"] }], limits)).toContain("超过 20 字");
    expect(
      tagRulesError(
        [
          { tag: "奇幻", keywords: ["魔法"] },
          { tag: "奇幻", keywords: ["精灵"] },
        ],
        limits,
      ),
    ).toContain("出现了两次");
    expect(tagRulesError([{ tag: "奇幻", keywords: [] }], limits)).toContain("还没有关键词");
    expect(
      tagRulesError(
        [{ tag: "奇幻", keywords: Array.from({ length: limits.maxKeywords + 1 }, (_, i) => `词${i}`) }],
        limits,
      ),
    ).toContain("关键词超过");
    expect(tagRulesError([{ tag: "奇幻", keywords: ["一".repeat(21)] }], limits)).toContain("超过 20 字");
  });

  it("条数上限跟着传入值走（不再是写死的 50）", () => {
    const rules = Array.from({ length: 2 }, (_, i) => ({ tag: `标签${i}`, keywords: ["魔法"] }));
    expect(tagRulesError(rules, { ...limits, maxCount: 1 })).toContain("最多 1 条");
    expect(tagRulesError(rules, limits)).toBe("");
  });

  it("上限读不到时（全 null）不做该项检查，交给服务端", () => {
    const many = Array.from({ length: 60 }, (_, i) => ({ tag: `标签${i}`, keywords: ["魔法"] }));
    expect(tagRulesError(many, UNLIMITED_TAG_RULES)).toBe("");
    expect(tagRulesError([{ tag: "一".repeat(30), keywords: ["魔法"] }], UNLIMITED_TAG_RULES)).toBe("");
    expect(tagRulesError([{ tag: "奇幻", keywords: ["一".repeat(30)] }], UNLIMITED_TAG_RULES)).toBe("");
  });
});
