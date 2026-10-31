import { describe, expect, it } from "vitest";
import { chapterPosition, readPercent } from "./progressText";

describe("进度口径：第 N 章 / 共 M 章", () => {
  it("下标 0 起、展示 +1；总数走 formatCount 与详情页目录头同一格式化", () => {
    expect(chapterPosition(0, 220)).toBe("第 1 章 / 共 220 章");
    expect(chapterPosition(4, 10)).toBe("第 5 章 / 共 10 章");
    expect(chapterPosition(0, 12000)).toBe("第 1 章 / 共 1.2 万 章");
  });

  it("只有总数（没读到哪）时退化成「共 M 章」", () => {
    expect(chapterPosition(null, 10)).toBe("共 10 章");
    expect(chapterPosition(undefined, 10)).toBe("共 10 章");
    expect(chapterPosition(-1, 10)).toBe("共 10 章");
  });

  it("章数不可用时不产出文案（调用方据此不渲染这一行）", () => {
    expect(chapterPosition(3, 0)).toBe("");
    expect(chapterPosition(3, Number.NaN)).toBe("");
  });
});

describe("已读百分比", () => {
  it("读过至少显示 1%，上限 100%", () => {
    expect(readPercent({ percent: 0.424 })).toBe(42);
    expect(readPercent({ percent: 0.0001 })).toBe(1);
    expect(readPercent({ percent: 1.4 })).toBe(100);
  });

  it("没有进度 / 百分比为 0 时不显示数字", () => {
    expect(readPercent(undefined)).toBeNull();
    expect(readPercent(null)).toBeNull();
    expect(readPercent({ percent: 0 })).toBeNull();
    expect(readPercent({ percent: Number.NaN })).toBeNull();
  });
});
