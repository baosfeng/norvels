import { describe, expect, it } from "vitest";
import {
  autoCuts,
  canSplit,
  countWords,
  isCjk,
  normalizeCuts,
  paragraphsOf,
  splitPieces,
  splitTitles,
  toCutAfter,
} from "./splitPlan";

describe("拆分章节：字数口径与后端一致", () => {
  it("CJK 逐字计、标点空白不算、连续拉丁词算一个", () => {
    expect(countWords("第一段落。")).toBe(4);
    expect(countWords("hello world")).toBe(2);
    expect(countWords("中文 hello 混合 123")).toBe(6);
    expect(countWords("   ")).toBe(0);
  });

  it("CJK 区间覆盖汉字与假名，非 CJK 字母不算", () => {
    expect(isCjk("汉".codePointAt(0) ?? 0)).toBe(true);
    expect(isCjk("あ".codePointAt(0) ?? 0)).toBe(true);
    expect(isCjk("a".codePointAt(0) ?? 0)).toBe(false);
  });
});

describe("拆分章节：段落与切点", () => {
  const content = "第一段落。\n第二段落。\n第三段落。\n第四段落。";

  it("按换行切段，段落下标从 0 起", () => {
    const paragraphs = paragraphsOf(content);
    expect(paragraphs).toHaveLength(4);
    expect(paragraphs[0]).toMatchObject({ index: 0, text: "第一段落。", words: 4 });
    expect(paragraphs[3].index).toBe(3);
  });

  it("切点归一化：去重、升序、丢掉首尾", () => {
    expect(normalizeCuts(4, [3, 1, 1, 0, 4, -2])).toEqual([1, 3]);
  });

  it("按切点算每一片的段落区间与字数", () => {
    const pieces = splitPieces(paragraphsOf(content), [1]);
    expect(pieces).toEqual([
      { from: 0, to: 1, words: 4 },
      { from: 1, to: 4, words: 12 },
    ]);
    const total = pieces.reduce((sum, piece) => sum + piece.words, 0);
    expect(total).toBe(16);
  });

  it("没有切点时只有一片（等于没拆）", () => {
    expect(splitPieces(paragraphsOf(content), [])).toEqual([{ from: 0, to: 4, words: 16 }]);
  });

  it("标题：单段不改名，多段加（i/N）", () => {
    expect(splitTitles("长章", 1)).toEqual(["长章"]);
    expect(splitTitles("长章", 3)).toEqual(["长章（1/3）", "长章（2/3）", "长章（3/3）"]);
  });

  it("按目标字数自动布点", () => {
    const paragraphs = paragraphsOf(content);
    expect(autoCuts(paragraphs, 5)).toEqual([2]);
    expect(autoCuts(paragraphs, 100)).toEqual([]);
    expect(autoCuts(paragraphs, Number.NaN)).toEqual([]);
  });

  it("前端切点转成后端的 cutAfter（下标减一）", () => {
    expect(toCutAfter([1, 3])).toEqual([0, 2]);
    expect(toCutAfter([0, 5])).toEqual([4]);
  });

  it("只有一片或存在空片时不可提交", () => {
    const paragraphs = paragraphsOf("第一段落。\n\n第二段落。");
    expect(canSplit(splitPieces(paragraphs, []))).toBe(false);
    // 空行本身是一片：切点在它两侧就会切出空章
    expect(canSplit(splitPieces(paragraphs, [1, 2]))).toBe(false);
    expect(canSplit(splitPieces(paragraphs, [1]))).toBe(true);
  });
});
