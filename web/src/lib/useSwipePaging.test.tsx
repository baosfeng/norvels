import { fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SWIPE_FOLLOW_DAMPING } from "./swipe";
import { useSwipePaging } from "./useSwipePaging";

/**
 * jsdom 没有真实触摸：按事件名造一个带 touches / changedTouches 的事件，够 hook 读坐标。
 * 「起点」与「当前点」分开传——touchmove 的 touches 是当前点，touchend 的 changedTouches 是终点。
 */
function touchEvent(type: string, from: { x: number; y: number }, to?: { x: number; y: number }) {
  const event = new Event(type, { bubbles: true });
  const end = { clientX: to?.x ?? from.x, clientY: to?.y ?? from.y };
  Object.assign(event, {
    touches: type === "touchend" || type === "touchcancel" ? [] : [end],
    changedTouches: [end],
    targetTouches: [end],
  });
  return event;
}

/** 用真实组件挂 ref：ref 回调在 commit 期先于 effect 执行，与线上一致（直接调 ref 拿不到监听）。 */
function Harness({
  enabled,
  reducedMotion,
  onPrev,
  onNext,
}: {
  enabled: boolean;
  reducedMotion: boolean;
  onPrev: () => void;
  onNext: () => void;
}) {
  const ref = useSwipePaging({ onPrev, onNext, enabled, reducedMotion });
  return (
    <div data-testid="reader-swipe-area" ref={ref}>
      <div className="reader-swipe-layer">正文</div>
    </div>
  );
}

function mount(enabled = true, reducedMotion = false) {
  const onPrev = vi.fn();
  const onNext = vi.fn();
  const view = render(
    <Harness enabled={enabled} reducedMotion={reducedMotion} onPrev={onPrev} onNext={onNext} />,
  );
  const surface = view.getByTestId("reader-swipe-area");
  const layerOf = () => surface.querySelector<HTMLElement>(".reader-swipe-layer")!;
  return { onPrev, onNext, surface, layerOf };
}

/** 从起点拖到终点：touchstart → 中间若干 touchmove → touchend。 */
function swipe(surface: HTMLElement, path: { x: number; y: number }[]) {
  const [first, ...rest] = path;
  fireEvent(surface, touchEvent("touchstart", first));
  for (const point of rest) fireEvent(surface, touchEvent("touchmove", first, point));
  const last = path[path.length - 1];
  fireEvent(surface, touchEvent("touchend", first, last));
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useSwipePaging 跟手与翻章", () => {
  it("横向拖动时正文跟着偏（阻尼），松手达阈值翻章", () => {
    const { surface, layerOf, onNext } = mount();
    fireEvent(surface, touchEvent("touchstart", { x: 300, y: 400 }));
    fireEvent(surface, touchEvent("touchmove", { x: 300, y: 400 }, { x: 260, y: 402 }));
    // 位移 40px：跟手偏移 = 40 × 阻尼，向左
    expect(layerOf().style.transform).toBe(`translateX(${-40 * SWIPE_FOLLOW_DAMPING}px)`);

    fireEvent(surface, touchEvent("touchend", { x: 300, y: 400 }, { x: 200, y: 402 }));
    expect(onNext).toHaveBeenCalledTimes(1);
    // 翻章后手势层复位，不再叠第二套动画
    expect(layerOf().style.transform).toBe("translateX(0px)");
  });

  it("松手未达阈值：带过渡弹回原位，不翻章", () => {
    const { surface, layerOf, onNext, onPrev } = mount();
    swipe(surface, [{ x: 300, y: 400 }, { x: 280, y: 402 }, { x: 265, y: 402 }]);
    expect(onNext).not.toHaveBeenCalled();
    expect(onPrev).not.toHaveBeenCalled();
    expect(layerOf().style.transform).toBe("translateX(0px)");
    expect(layerOf().style.transition).toContain("200ms");
  });

  it("方向锁定：先竖后横的斜滑按滚动处理，整段不翻章", () => {
    const { surface, layerOf, onNext } = mount();
    fireEvent(surface, touchEvent("touchstart", { x: 300, y: 400 }));
    // 先向下 → 锁定为纵向滚动，此时不该有跟手位移
    fireEvent(surface, touchEvent("touchmove", { x: 300, y: 400 }, { x: 302, y: 440 }));
    expect(layerOf().style.transform).toBe("");
    // 之后手指横向甩出很远也不改判
    fireEvent(surface, touchEvent("touchmove", { x: 300, y: 400 }, { x: 180, y: 440 }));
    fireEvent(surface, touchEvent("touchend", { x: 300, y: 400 }, { x: 180, y: 440 }));
    expect(layerOf().style.transform).toBe("");
    expect(onNext).not.toHaveBeenCalled();
  });

  it("滚正文时的横向漂移（70/45）不翻章", () => {
    const { surface, onNext } = mount();
    swipe(surface, [{ x: 300, y: 200 }, { x: 260, y: 240 }, { x: 230, y: 245 }]);
    expect(onNext).not.toHaveBeenCalled();
  });

  it("减少动效时不写跟手位移，翻章照旧", () => {
    const { surface, layerOf, onNext } = mount(true, true);
    fireEvent(surface, touchEvent("touchstart", { x: 300, y: 400 }));
    fireEvent(surface, touchEvent("touchmove", { x: 300, y: 400 }, { x: 250, y: 402 }));
    expect(layerOf().style.transform).toBe("");
    fireEvent(surface, touchEvent("touchend", { x: 300, y: 400 }, { x: 200, y: 402 }));
    expect(onNext).toHaveBeenCalledTimes(1);
  });

  it("enabled=false（面板打开）时完全不接管", () => {
    const { surface, layerOf, onNext } = mount(false);
    fireEvent(surface, touchEvent("touchstart", { x: 300, y: 400 }));
    fireEvent(surface, touchEvent("touchmove", { x: 300, y: 400 }, { x: 200, y: 402 }));
    expect(layerOf().style.transform).toBe("");
    fireEvent(surface, touchEvent("touchend", { x: 300, y: 400 }, { x: 200, y: 402 }));
    expect(onNext).not.toHaveBeenCalled();
  });

  it("起点落在按钮上不接管", () => {
    const { surface, onNext } = mount();
    const button = document.createElement("button");
    surface.appendChild(button);
    fireEvent(button, touchEvent("touchstart", { x: 300, y: 400 }));
    fireEvent(button, touchEvent("touchend", { x: 300, y: 400 }, { x: 200, y: 402 }));
    expect(onNext).not.toHaveBeenCalled();
  });
});
