import { describe, expect, it } from "vitest";
import { MAX_SEARCH_HITS, searchParagraphs, splitParagraphs } from "./readerSearch";

const PARAS = ["雨从傍晚开始下。", "巷口的灯还亮着。", "他站着，没有打伞。", "雨声更大了一些。", "远处有人收摊，雨停了。"];

describe("章节内搜索：分段与命中", () => {
  it("按 \n 切段并保留空段：段落下标与后端 paraIndex 一致", () => {
    const paragraphs = splitParagraphs("第一段\n\n第三行\n   \n第五行");
    expect(paragraphs).toEqual(["第一段", "", "第三行", "", "第五行"]);
    expect(paragraphs.indexOf("第三行")).toBe(2);
  });

  it("5 段 3 处命中：序号连续，段内偏移可定位", () => {
    const result = searchParagraphs(PARAS, "雨");
    expect(result.total).toBe(3);
    expect(result.hits.map((hit) => [hit.index, hit.paraIndex, hit.start])).toEqual([
      [0, 0, 0],
      [1, 3, 0],
      [2, 4, 7],
    ]);
    expect(result.truncated).toBe(false);
  });

  it("大小写不敏感，且段内不重叠计数", () => {
    const result = searchParagraphs(["Abc abc ABC", "abc"], "ABC");
    expect(result.total).toBe(4);
    expect(result.hits.map((hit) => hit.paraIndex)).toEqual([0, 0, 0, 1]);
  });

  it("空关键词不算搜索，也不产生命中", () => {
    const result = searchParagraphs(PARAS, "   ");
    expect(result.query).toBe("");
    expect(result.hits).toHaveLength(0);
    expect(result.total).toBe(0);
  });

  it("超过上限只保留前 200 处并给出截断标记", () => {
    const result = searchParagraphs(["雨".repeat(250)], "雨");
    expect(result.hits).toHaveLength(MAX_SEARCH_HITS);
    expect(result.total).toBe(250);
    expect(result.truncated).toBe(true);
  });
});
