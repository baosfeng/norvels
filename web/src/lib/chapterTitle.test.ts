import { describe, expect, it } from "vitest";
import { displayChapterTitle } from "./chapterTitle";

/**
 * U20 判据：正文标题只在「印的章号 ≠ 库内序号」时剥前缀，一致时原文原样；
 * 剥完为空退回原文，任何输入都不抛错。
 *
 * 用虚构书名与章名，数字均与真实语料无关。
 */
describe("U20 正文标题：章号前缀与库内序号对得上才留", () => {
  it("印的章号 == chapterIdx + 1：原文原样", () => {
    expect(displayChapterTitle("第一章 空谷仙影", 0)).toBe("第一章 空谷仙影");
    expect(displayChapterTitle("第十九章 武道要旨", 18)).toBe("第十九章 武道要旨");
    expect(displayChapterTitle("第 12 章 春野樱的表白", 11)).toBe("第 12 章 春野樱的表白");
  });

  it("印的章号 ≠ chapterIdx + 1：剥掉前缀只留标题文字", () => {
    expect(displayChapterTitle("第十九章 武道要旨", 17)).toBe("武道要旨");
    expect(displayChapterTitle("第1章 春野樱的表白", 1)).toBe("春野樱的表白");
    expect(displayChapterTitle("第一百零五章 长夜", 3)).toBe("长夜");
  });

  it("没有前缀：原样返回（没东西可比，也就不动它）", () => {
    expect(displayChapterTitle("巷口的灯", 4)).toBe("巷口的灯");
    expect(displayChapterTitle("序", 0)).toBe("序");
    expect(displayChapterTitle("番外·雪停之后", 7)).toBe("番外·雪停之后");
  });

  it("标题只有章号：剥完为空就退回原文，不渲染空 h2", () => {
    expect(displayChapterTitle("第三章", 4)).toBe("第三章");
    expect(displayChapterTitle("第 9 章 ", 0)).toBe("第 9 章");
  });

  it("空标题：返回空串，不抛错", () => {
    expect(displayChapterTitle("", 3)).toBe("");
    expect(displayChapterTitle("   ", 3)).toBe("");
  });

  it("目录还没就绪（chapterIdx 为负或非数字）：原文原样", () => {
    expect(displayChapterTitle("第十九章 武道要旨", -1)).toBe("第十九章 武道要旨");
    expect(displayChapterTitle("第十九章 武道要旨", Number.NaN)).toBe("第十九章 武道要旨");
  });

  it("全角数字与「回」「节」：与 chapterBatch 同一套前缀口径", () => {
    expect(displayChapterTitle("第１２章 归途", 11)).toBe("第１２章 归途");
    expect(displayChapterTitle("第１２章 归途", 3)).toBe("归途");
    expect(displayChapterTitle("第12回 归途", 11)).toBe("第12回 归途");
    expect(displayChapterTitle("第12回 归途", 1)).toBe("归途");
    expect(displayChapterTitle("第 3 节 归途", 1)).toBe("归途");
  });

  it("嵌套多层前缀：首层对不上就一起剥干净", () => {
    expect(displayChapterTitle("第十二章 第 12 章 巷口的灯", 11)).toBe("第十二章 第 12 章 巷口的灯");
    expect(displayChapterTitle("第十二章 第 12 章 巷口的灯", 4)).toBe("巷口的灯");
  });

  it("认不出的章号写法：不猜，原文原样", () => {
    expect(displayChapterTitle("第十二万章 巷口的灯", 4)).toBe("第十二万章 巷口的灯");
  });
});
