import { describe, expect, it } from "vitest";
import { chapterLabel } from "./chapterLabel";

/**
 * 一行只说一次章号：章节标题常常自带阿拉伯章号（导入的 txt 目录就是「第150章 灯影未归人」），
 * 再拼一次序号就成了「第 150 章 · 第150章 灯影未归人」。
 */
describe("chapterLabel", () => {
  it("标题自带阿拉伯章号时剥掉，序号只出现一次", () => {
    expect(chapterLabel(149, "第150章 灯影未归人")).toBe("第 150 章 · 灯影未归人");
    expect(chapterLabel(2, "第 3 章 雨停之前")).toBe("第 3 章 · 雨停之前");
    expect(chapterLabel(6, "第7章")).toBe("第 7 章");
  });

  it("中文数字标题不剥：它本来就不跟序号重复，剥了反而丢信息", () => {
    expect(chapterLabel(1, "第二章")).toBe("第 2 章 · 第二章");
  });

  it("没有序号时原样返回标题，不误剥章号", () => {
    expect(chapterLabel(undefined, "第150章 灯影未归人")).toBe("第150章 灯影未归人");
  });

  it("标题为空 / 只有空白：只给章号，不留多余分隔符", () => {
    expect(chapterLabel(0, "")).toBe("第 1 章");
    expect(chapterLabel(0, "   ")).toBe("第 1 章");
    expect(chapterLabel(3, undefined)).toBe("第 4 章");
  });

  it("序号非法（负数 / NaN）时退回纯标题", () => {
    expect(chapterLabel(-1, "第2章 街角小雨")).toBe("第2章 街角小雨");
    expect(chapterLabel(Number.NaN, "序章")).toBe("序章");
  });
});
