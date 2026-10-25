import { describe, expect, it } from "vitest";
import { bodyIsDirty, bodyStateText, countBodyParagraphs, countBodyWords } from "./chapterBody";

/**
 * 与后端对齐的固定期望值：文本交给 Go 侧 internal/store/util.go 的 countWords
 * （保存正文后重算 chapters.word_count 用的就是它）跑出来的结果，逐条抄在这里。
 * 改这里的期望值 = 改后端口径，必须两边一起改。
 */
const GO_ALIGNED: ReadonlyArray<readonly [string, number]> = [
  ["你好，世界 hello 123 world!", 7],
  ["雨从屋檐上垂下来。\n\n守夜人把灯芯挑亮。", 16],
  ["第1章 起风了：他走了 3.5 公里，e-mail 未读（2 封）。", 19],
  ["「引号」…——破折号、省略号\u3000全角空格 ＡＢＣ １２３", 14],
  ["\u{20000}\u{20001} 扩展B区两个字 😀 emoji 与 e\u0301 组合符号", 15],
  ["  \n\t第一段\r\n\n  第二段  ", 6],
];

describe("countBodyWords（与后端 store.countWords 同口径）", () => {
  it.each(GO_ALIGNED)("中英数标点混排：%j → %i 字（Go 侧固定期望值）", (text, words) => {
    expect(countBodyWords(text)).toBe(words);
  });

  it("空串与纯空白都是 0 字", () => {
    expect(countBodyWords("")).toBe(0);
    expect(countBodyWords(" \n\t\r\n ")).toBe(0);
  });

  it("中文逐字计，标点与空白不算", () => {
    expect(countBodyWords("你好，世界")).toBe(4);
    expect(countBodyWords("  第一段\n\n  第二段  ")).toBe(6);
  });

  it("连续拉丁字母 / 数字串算 1 个词，标点会把词断开", () => {
    expect(countBodyWords("hello world")).toBe(2);
    expect(countBodyWords("3.5")).toBe(2);
    expect(countBodyWords("Well-known")).toBe(2);
  });

  it("汉字会把拉丁词断开，紧挨着也算两个字数", () => {
    expect(countBodyWords("第1章")).toBe(3);
  });

  it("符号（含 emoji）不计数", () => {
    expect(countBodyWords("好😀")).toBe(1);
    expect(countBodyWords("★★★")).toBe(0);
  });

  it("扩展 B 区汉字 store 记作「词」而不是逐字：连续一段算 1（parser 导入期口径的既有差异）", () => {
    expect(countBodyWords("\u{20000}")).toBe(1);
    expect(countBodyWords("\u{20000}\u{20001}")).toBe(1);
    expect(countBodyWords("\u{20000}你好")).toBe(3);
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
