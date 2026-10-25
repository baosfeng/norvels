import { describe, expect, it } from "vitest";
import { bodyIsDirty, bodyStateText, countBodyParagraphs, countBodyWords } from "./chapterBody";

describe("countBodyWords（非空白字符数）", () => {
  it("空串与纯空白都是 0 字", () => {
    expect(countBodyWords("")).toBe(0);
    expect(countBodyWords(" \n\t\r\n ")).toBe(0);
  });

  it("中文逐字计，标点也各算 1，空白不算", () => {
    expect(countBodyWords("你好，世界")).toBe(5);
    expect(countBodyWords("雨从屋檐上垂下来。\n\n守夜人把灯芯挑亮。")).toBe(18);
  });

  it("换行与缩进不计入字数", () => {
    expect(countBodyWords("  第一段\n\n  第二段  ")).toBe(6);
  });

  it("按码点算：扩展 B 区汉字只算 1 个字，不按 UTF-16 码元算 2", () => {
    expect(countBodyWords("𠀀")).toBe(1);
  });

  it("拉丁串按字符计（与后端 countWords 的差异已知，面板是本地估算）", () => {
    expect(countBodyWords("hello world")).toBe(10);
  });
});

describe("countBodyParagraphs", () => {
  it("单行正文是 1 段", () => {
    expect(countBodyParagraphs("只有一行。")).toBe(1);
  });

  it("空行分段，段内空行合并", () => {
    expect(countBodyParagraphs("第一段\n\n第二段\n\n\n第三段")).toBe(3);
  });

  it("纯空白段不计，全空正文是 0 段", () => {
    expect(countBodyParagraphs("\n\n   \n\n")).toBe(0);
    expect(countBodyParagraphs("第一段\n\n   \n\n第二段")).toBe(2);
  });

  it("段内的单个换行不切段", () => {
    expect(countBodyParagraphs("第一行\n第二行")).toBe(1);
  });
});

describe("bodyIsDirty / bodyStateText", () => {
  it("逐字比较，改一个字算改动", () => {
    expect(bodyIsDirty("正文", "正文")).toBe(false);
    expect(bodyIsDirty("正文。", "正文")).toBe(true);
  });

  it("多一个行尾空格也算改动（保存会原样替换正文）", () => {
    expect(bodyIsDirty("正文 ", "正文")).toBe(true);
  });

  it("状态词只有两种，改动才提示未保存", () => {
    expect(bodyStateText(true)).toBe("有未保存的改动");
    expect(bodyStateText(false)).toBe("未改动");
  });
});
