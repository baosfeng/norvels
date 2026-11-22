import { afterEach, describe, expect, it, vi } from "vitest";
import { markTurnStart, measureTurnEnd, turnMarkName, turnMeasureName } from "./turnPerf";

/**
 * 假 User Timing：jsdom 不实现 performance.mark / measure（都是 undefined），
 * 用例自己造一份能记账的实现，才看得见「打没打、打的是什么名字、参数对不对」。
 */
function fakeTiming() {
  const marks: string[] = [];
  const measures: { name: string; start?: string }[] = [];
  const cleared: string[] = [];
  return {
    marks,
    measures,
    cleared,
    api: {
      mark: (name: string) => void marks.push(name),
      measure: (name: string, start?: string) => void measures.push({ name, start }),
      clearMarks: (name: string) => void cleared.push(name),
    } as unknown as Performance,
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("翻页埋点（turnPerf）", () => {
  /**
   * 防复发：老浏览器没有 User Timing，埋点不能因此报错——
   * 「记不上数」是允许的，「翻页被埋点搞挂」不是。
   * 不依赖测试环境到底有没有 performance：显式换上一份没有 mark / measure 的对象。
   */
  it("没有 performance.mark / measure 时静默跳过", () => {
    const bare = { now: () => 0 } as unknown as Performance;
    vi.stubGlobal("performance", bare);

    expect(() => markTurnStart("slide")).not.toThrow();
    expect(() => measureTurnEnd("slide")).not.toThrow();
  });

  it("performance 整个缺失时也不抛", () => {
    vi.stubGlobal("performance", undefined);
    expect(() => markTurnStart("curl")).not.toThrow();
    expect(() => measureTurnEnd("curl")).not.toThrow();
  });

  it("正常链路：起点 mark 与终点 measure 成对，名字按效果区分", () => {
    const fake = fakeTiming();
    vi.stubGlobal("performance", fake.api);

    markTurnStart("slide");
    measureTurnEnd("slide");

    expect(fake.marks).toEqual(["norvels.turn.slide.start"]);
    expect(fake.measures).toEqual([{ name: "norvels.turn.slide", start: "norvels.turn.slide.start" }]);
    // 起点打之前先清一次（丢掉上一轮没配对的残留）、终点之后清一次，逐页翻下去
    // 同名 mark 不会在缓冲区里堆积，也不会让下一次 measure 配到上一轮的时间
    expect(fake.cleared).toEqual(["norvels.turn.slide.start", "norvels.turn.slide.start"]);
  });

  it("measure 抛错（没有配对起点）时不向上冒泡，仍会清掉起点", () => {
    const fake = fakeTiming();
    const measure = vi.fn(() => {
      throw new DOMException("start mark not found", "SyntaxError");
    });
    vi.stubGlobal("performance", { ...fake.api, measure } as unknown as Performance);

    expect(() => measureTurnEnd("fade")).not.toThrow();
    expect(measure).toHaveBeenCalledWith("norvels.turn.fade", "norvels.turn.fade.start");
    expect(fake.cleared).toEqual(["norvels.turn.fade.start"]);
  });

  it("mark 抛错（缓冲区满 / 名字被拒）时翻页照常", () => {
    const mark = vi.fn(() => {
      throw new DOMException("boom", "InvalidStateError");
    });
    vi.stubGlobal("performance", {
      mark,
      measure: vi.fn(),
      clearMarks: vi.fn(),
    } as unknown as Performance);

    expect(() => markTurnStart("cover")).not.toThrow();
    expect(mark).toHaveBeenCalledWith("norvels.turn.cover.start");
  });

  it("名字约定：对外 measure 不带 .start 后缀，起点带", () => {
    expect(turnMeasureName("slide")).toBe("norvels.turn.slide");
    expect(turnMarkName("slide")).toBe("norvels.turn.slide.start");
  });
});