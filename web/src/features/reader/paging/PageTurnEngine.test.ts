import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { computePageBox } from "../../../lib/paging/pageGeometry";
import { PageTurnEngine } from "./PageTurnEngine";
import { clearTurnStart, markTurnStart, measureTurnEnd } from "../../../lib/paging/turnPerf";

// 埋点是旁路：单测只验「起点终点成对打、回弹不记样本」，不重复 turnPerf 自己的用例
vi.mock("../../../lib/paging/turnPerf", () => ({
  markTurnStart: vi.fn(),
  measureTurnEnd: vi.fn(),
  clearTurnStart: vi.fn(),
}));

const box = computePageBox({ stageW: 390, stageH: 844, padX: 18, lineH: 42 });

/**
 * 舞台 + 引擎的最小装配。页层用裸 div 充当（引擎只写 dataset / 内联样式）。
 * effect 取 fade：它的收尾靠 requestAnimationFrame 逐帧推进——正是「done 可能永远不来」的那一类。
 */
function setup(effectName: "fade" | "none" = "fade") {
  const stage = document.createElement("div");
  document.body.appendChild(stage);
  const onCommit = vi.fn();
  const engine = new PageTurnEngine({
    stage,
    createLayer: () => {
      const layer = document.createElement("div");
      layer.className = "pg-layer";
      stage.appendChild(layer);
      return layer;
    },
    setLayerPage: () => undefined,
    box: () => box,
    effect: () => effectName,
    reducedMotion: () => false,
    onEdge: () => undefined,
    onCommit,
  });
  engine.reset(0, 4);
  return { engine, onCommit, stage };
}

/** 从 translate3d(x,y,z) 里取第一个参数（x）；空值算 NaN。 */
function xOf(transform: string): number {
  const match = /translate3d\((-?[\d.]+)px/.exec(transform);
  return match ? Number.parseFloat(match[1]) : Number.NaN;
}

function currentLayer(stage: HTMLElement): HTMLElement {
  return stage.querySelector<HTMLElement>(".pg-layer") as HTMLElement;
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  document.body.replaceChildren();
});

describe("翻页收尾兜底", () => {
  /**
   * 防复发：fade / curl 的收尾靠 rAF 逐帧推进，后台标签页把 rAF 暂停后 done 永远不来，
   * busy 一直为真，读者随后每一次点击 / 滑动翻页都被「翻页中」挡掉——现象就是「翻不动页」。
   */
  it("rAF 停摆时按理论时长兜底落定，页码照样前进", () => {
    // rAF 永远不回调：模拟后台标签页 / 合成器暂停
    vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => 1);
    const { engine, onCommit } = setup();

    expect(engine.turn(1)).toBe(true);
    expect(engine.busy).toBe(true);
    expect(onCommit).not.toHaveBeenCalled();

    // fade 200ms + 240ms 兜底余量
    vi.advanceTimersByTime(500);
    expect(engine.busy).toBe(false);
    expect(engine.currentPage).toBe(1);
    expect(onCommit).toHaveBeenCalledWith(1);
  });

  it("兜底之后还能继续翻页（busy 没有卡死）", () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => 1);
    const { engine } = setup();
    engine.turn(1);
    vi.advanceTimersByTime(500);
    expect(engine.turn(1)).toBe(true);
    vi.advanceTimersByTime(500);
    expect(engine.currentPage).toBe(2);
  });
});
describe("翻页埋点", () => {
  beforeEach(() => {
    vi.mocked(markTurnStart).mockClear();
    vi.mocked(measureTurnEnd).mockClear();
    vi.mocked(clearTurnStart).mockClear();
  });

  it("点按翻页：起点与新页落定各打一次，效果名一路带下去", () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => 1);
    const { engine, onCommit } = setup();

    engine.turn(1);
    // 起点在「第二层建好、动画开跑」那一刻，此时还没落页
    expect(markTurnStart).toHaveBeenCalledWith("fade");
    expect(measureTurnEnd).not.toHaveBeenCalled();

    vi.advanceTimersByTime(500);
    expect(onCommit).toHaveBeenCalledWith(1);
    expect(measureTurnEnd).toHaveBeenCalledWith("fade");
    expect(vi.mocked(measureTurnEnd)).toHaveBeenCalledTimes(1);
  });

  it("回弹（取消 / 松手不翻）只丢掉起点，不记 measure", () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => 1);
    const { engine } = setup();

    engine.pointerDown(100, 400);
    engine.pointerMove(40, 402); // 横移 60px：越过方向锁但与提交阈值差得远
    engine.pointerUp(40);
    vi.advanceTimersByTime(600);

    expect(markTurnStart).toHaveBeenCalledTimes(1);
    expect(engine.currentPage).toBe(0);
    // 回弹不落页：不记 measure，起点一并清掉（缓冲区里不留孤儿样本）
    expect(measureTurnEnd).not.toHaveBeenCalled();
    expect(clearTurnStart).toHaveBeenCalledWith("fade");
  });

  it("连续翻页：每次落定正好一条 measure，不串页也不少记", () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation(() => 1);
    const { engine } = setup();

    engine.turn(1);
    vi.advanceTimersByTime(500);
    engine.turn(1);
    vi.advanceTimersByTime(500);

    expect(markTurnStart).toHaveBeenCalledTimes(2);
    expect(measureTurnEnd).toHaveBeenCalledTimes(2);
  });
});

describe("无动画（none）拖动跟手", () => {
  /**
   * 防复发：none 早先完全不跟手（横向拖 160 px 页面一动不动），读者会以为手势没被识别。
   * 现在的口径：拖动中给 15% 阻尼跟手，松手仍然瞬时翻页——翻页耗时不变。
   */
  it("拖 120 px 单层位移 ≠ 0，方向跟着手指走", () => {
    const { engine, stage } = setup("none");
    const layer = currentLayer(stage);

    engine.pointerDown(300, 400);
    expect(engine.pointerMove(180, 402)).toBe(true);

    // 左滑（下一页）时页面跟着往左让：120 × 15% = 18 px
    expect(xOf(layer.style.transform)).toBeCloseTo(-18, 1);
  });

  it("跟手有封顶：拖满一屏也只走可视宽的 15%", () => {
    const { engine, stage } = setup("none");
    const layer = currentLayer(stage);

    engine.pointerDown(300, 400);
    engine.pointerMove(0, 402); // 横移 300 px
    expect(Math.abs(xOf(layer.style.transform))).toBeLessThanOrEqual(58.5);
    expect(xOf(layer.style.transform)).not.toBe(0);
  });

  it("松手过阈值：瞬时换页（不等动画）且新页没有残留位移", () => {
    const { engine, stage, onCommit } = setup("none");
    const layer = currentLayer(stage);

    engine.pointerDown(300, 400);
    engine.pointerMove(60, 402); // 横移 240 px → p ≈ 0.62 > 0.22
    engine.pointerUp(60);

    // none 收尾时长 0：不推进任何定时器就该已经落页
    expect(onCommit).toHaveBeenCalledWith(1);
    expect(engine.currentPage).toBe(1);
    expect(layer.isConnected).toBe(false);
    expect(currentLayer(stage).style.transform).toBe("");
  });

  it("松手不够阈值：就地回位，页码与层都不动", () => {
    const { engine, stage } = setup("none");
    const layer = currentLayer(stage);

    engine.pointerDown(300, 400);
    engine.pointerMove(220, 402); // 横移 80 px → p ≈ 0.21 < 0.22
    engine.pointerUp(220);

    expect(engine.currentPage).toBe(0);
    expect(layer.isConnected).toBe(true);
    expect(layer.style.transform).toBe("");
  });

  it("越界方向拖动不跟手（首/末页仍交给回弹与跨章）", () => {
    const { engine, stage } = setup("none");
    const layer = currentLayer(stage);

    engine.pointerDown(100, 400);
    engine.pointerMove(220, 402); // 右滑 = 上一页，第 0 页已经没得翻
    expect(layer.style.transform).toBe("");
    engine.pointerUp(220);
  });
});
