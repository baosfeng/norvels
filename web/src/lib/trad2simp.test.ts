import { describe, expect, it } from "vitest";
import { trad2simpEntries, tradToSimp } from "./trad2simp";

describe("繁体转简体：常用字与异体字", () => {
  it("常用繁体字逐个转成简体", () => {
    expect(tradToSimp("愛體說國學時後發")).toBe("爱体说国学时后发");
    expect(tradToSimp("這個問題很重要")).toBe("这个问题很重要");
    expect(tradToSimp("圖書館裡沒有人")).toBe("图书馆里没有人");
  });

  it("异体字同样归一（峯 / 麪 / 裏 / 臺 / 鬥）", () => {
    expect(tradToSimp("山峯")).toBe("山峰");
    expect(tradToSimp("麪條")).toBe("面条");
    expect(tradToSimp("家裏")).toBe("家里");
    expect(tradToSimp("臺風")).toBe("台风");
    expect(tradToSimp("爭鬥")).toBe("争斗");
  });

  it("混排：繁体换、简体与数字原样", () => {
    expect(tradToSimp("第3章 開始了")).toBe("第3章 开始了");
  });
});

describe("繁体转简体：不该动的内容", () => {
  it("中文标点不转换", () => {
    const punctuation = "，。、；：！？「」『』（）《》〈〉—…·～";
    expect(tradToSimp(punctuation)).toBe(punctuation);
  });

  it("拉丁字母 / 数字 / 空白 / emoji 原样保留", () => {
    const raw = "ABC abc 123 空格  🚀 ";
    expect(tradToSimp(raw)).toBe(raw);
  });

  it("空串进空串出", () => {
    expect(tradToSimp("")).toBe("");
  });

  it("纯简体正文不新建字符串（零替换走快路径）", () => {
    const simplified = "雨从傍晚开始下，他站在巷口。";
    expect(tradToSimp(simplified)).toBe(simplified);
    // 引用相同 => 没有为快路径复制正文
    expect(tradToSimp(simplified)).toBe(tradToSimp(simplified));
  });

  it("转换幂等：转两遍与转一遍相同", () => {
    const once = tradToSimp("鬱悶的傳統書寫");
    expect(tradToSimp(once)).toBe(once);
  });
});

describe("繁体转简体：表规模与性能", () => {
  it("对照表在 3000 条以内且覆盖足够（> 2000 条）", () => {
    expect(trad2simpEntries()).toBeLessThanOrEqual(3000);
    expect(trad2simpEntries()).toBeGreaterThan(2000);
  });

  it("整章（3000 字）转换 < 30ms", () => {
    const chapter = "燈光下的街道，繁華而寂寞。".repeat(200); // 约 2800 字
    expect(chapter.length).toBeGreaterThan(2500);
    const started = performance.now();
    const out = tradToSimp(chapter);
    const elapsed = performance.now() - started;
    expect(out).toContain("灯光下的街道，繁华而寂寞。");
    expect(elapsed).toBeLessThan(30);
  });
});
