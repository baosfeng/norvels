import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { computePageBox } from "../../../lib/paging/pageGeometry";
import { PageTurnEngine } from "./PageTurnEngine";

const box = computePageBox({ stageW: 390, stageH: 844, padX: 18, lineH: 42 });

/**
 * 舞台 + 引擎的最小装配。页层用裸 div 充当（引擎只写 dataset / 内联样式）。
 * effect 取 fade：它的收尾靠 requestAnimationFrame 逐帧推进——正是「done 可能永远不来」的那一类。
 */
function setup() {
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
    effect: () => "fade",
    reducedMotion: () => false,
    onEdge: () => undefined,
    onCommit,
  });
  engine.reset(0, 4);
  return { engine, onCommit, stage };
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
