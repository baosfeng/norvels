import { describe, expect, it } from "vitest";
import type { Capabilities } from "../../api/types";
import { chapterLimitsOf, extLabel, innerExtLabel, tagLimitsOf, uploadLimitsOf } from "./limits";

const caps: Capabilities = {
  splitters: ["di-unit", "unmarked"],
  codecs: ["utf-8", "gb18030"],
  filters: [{ name: "bom", priority: 10 }],
  metas: [{ name: "filename", weight: 80 }],
  extensions: ["txt", "epub", "zip"],
  limits: {
    chapterRules: { maxCount: 20, maxPatternRunes: 200, maxNameRunes: 60 },
    tagRules: { maxCount: 50, maxTagRunes: 20, maxKeywords: 100, maxKeywordRunes: 20 },
    sourceProfiles: { maxCount: 20, maxRulesPerProfile: 30, maxPatternRunes: 200, maxNameRunes: 60 },
    upload: { maxFileBytes: 64 * 1024 * 1024, maxTotalBytes: 256 * 1024 * 1024, maxFiles: 20 },
    import: { maxWorkers: 32, defaultMaxChapterChars: 30000, defaultFallbackChunkChars: 8000 },
    dryRun: { maxFiles: 200, samples: 20, errors: 50 },
  },
};

describe("面板上限：一律从 capabilities 派生", () => {
  it("三组上限与扩展名都取自下发值", () => {
    expect(uploadLimitsOf(caps)).toEqual({
      extensions: ["txt", "epub", "zip"],
      maxFiles: 20,
      maxFileBytes: 64 * 1024 * 1024,
      maxTotalBytes: 256 * 1024 * 1024,
    });
    expect(chapterLimitsOf(caps)).toEqual({ maxCount: 20, maxPatternRunes: 200 });
    expect(tagLimitsOf(caps)).toEqual({ maxCount: 50, maxKeywords: 100, maxTagRunes: 20, maxKeywordRunes: 20 });
  });

  it("下发小值时上限跟着变小（不是写死的大值）", () => {
    const small: Capabilities = {
      ...caps,
      limits: {
        ...caps.limits,
        upload: { maxFileBytes: 1024, maxTotalBytes: 2048, maxFiles: 2 },
        chapterRules: { maxCount: 1, maxPatternRunes: 5, maxNameRunes: 60 },
        tagRules: { maxCount: 3, maxTagRunes: 4, maxKeywords: 5, maxKeywordRunes: 6 },
      },
    };
    expect(uploadLimitsOf(small).maxFiles).toBe(2);
    expect(chapterLimitsOf(small).maxCount).toBe(1);
    expect(tagLimitsOf(small).maxCount).toBe(3);
    expect(tagLimitsOf(small).maxKeywordRunes).toBe(6);
  });

  it("读不到能力清单时保守兜底：上限全部为 null（不限制）、扩展名为空", () => {
    expect(uploadLimitsOf(undefined)).toEqual({
      extensions: [],
      maxFiles: null,
      maxFileBytes: null,
      maxTotalBytes: null,
    });
    expect(chapterLimitsOf(null)).toEqual({ maxCount: null, maxPatternRunes: null });
    expect(tagLimitsOf(undefined)).toEqual({ maxCount: null, maxKeywords: null, maxTagRunes: null, maxKeywordRunes: null });
  });

  it("0 值 / 缺失字段同样按「不限制」处理，不替后端猜默认值", () => {
    const zeros = {
      ...caps,
      limits: {
        ...caps.limits,
        upload: { maxFileBytes: 0, maxTotalBytes: 0, maxFiles: 0 },
      },
    } as Capabilities;
    expect(uploadLimitsOf(zeros)).toMatchObject({ maxFiles: null, maxFileBytes: null, maxTotalBytes: null });
  });

  it("扩展名渲染：格式清单与包内条目都从同一份扩展名派生", () => {
    expect(extLabel(["txt", "epub", "zip"])).toBe("TXT、EPUB、ZIP");
    expect(extLabel([])).toBe("");
    expect(innerExtLabel(["txt", "epub", "zip"])).toBe("txt/epub");
    expect(innerExtLabel([])).toBe("");
  });
});
