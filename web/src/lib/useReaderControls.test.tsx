import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HINT_SEEN_KEY, FIRST_HINT_MS, useFirstGestureHint, useReaderControls } from "./useReaderControls";

/** 用一个最小壳子挂 hook：控制层内是 .reader-top，正文是 .reader-main。 */
function Harness({ isH5 = true }: { isH5?: boolean }) {
  const controls = useReaderControls({ isH5, paused: false });
  return (
    <div>
      <div className="reader-top">
        <button type="button" onClick={controls.reveal}>打开控制层</button>
      </div>
      <div className="reader-main" onClick={controls.reveal}>正文</div>
      <span data-testid="visible">{String(controls.visible)}</span>
    </div>
  );
}

/** 带底栏的壳子：按住滑块 / 按钮时控制层不该自己收起。 */
function BarHarness({ isH5 = true }: { isH5?: boolean }) {
  const controls = useReaderControls({ isH5, paused: false });
  return (
    <div>
      <div className="reader-top">
        <button type="button" onClick={controls.reveal}>打开控制层</button>
      </div>
      <div className="reader-main">正文</div>
      <span data-testid="visible">{String(controls.visible)}</span>
      {controls.visible ? (
        <nav className="h5-reader-bar">
          <input aria-label="进度滑块" type="range" min={0} max={3} readOnly />
          <button type="button">目录</button>
        </nav>
      ) : null}
    </div>
  );
}

function visibleText(): string {
  return screen.getByTestId("visible").textContent ?? "";
}

function slider(): HTMLElement {
  return screen.getByLabelText("进度滑块");
}

/** 只看 hook 的一次性提示状态。 */
function HintHarness({ isH5 = true }: { isH5?: boolean }) {
  const shown = useFirstGestureHint(isH5);
  return <span data-testid="hint">{String(shown)}</span>;
}

function hintText(): string {
  return screen.getByTestId("hint").textContent ?? "";
}

afterEach(() => {
  vi.useRealTimers();
  localStorage.clear();
});

describe("H5 控制层自动收起", () => {
  it("H5 默认收起；唤出后 3 秒无操作自动收起", () => {
    vi.useFakeTimers();
    render(<Harness />);
    expect(visibleText()).toBe("false");

    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "打开控制层" }));
    });
    expect(visibleText()).toBe("true");

    act(() => {
      vi.advanceTimersByTime(2999);
    });
    expect(visibleText()).toBe("true");

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(visibleText()).toBe("false");
  });

  it("焦点在控制层内时计时暂停；焦点离开后重新起算 3 秒", () => {
    vi.useFakeTimers();
    render(<Harness />);
    const trigger = screen.getByRole("button", { name: "打开控制层" });

    act(() => {
      fireEvent.click(trigger);
    });
    act(() => {
      fireEvent.focusIn(trigger);
    });

    // 键盘用户在控制层里操作时，控制层不会在他手上消失
    act(() => {
      vi.advanceTimersByTime(10000);
    });
    expect(visibleText()).toBe("true");

    act(() => {
      fireEvent.focusOut(trigger);
    });
    act(() => {
      vi.advanceTimersByTime(2999);
    });
    expect(visibleText()).toBe("true");

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(visibleText()).toBe("false");
  });

  it("控制层内按钮之间 Tab（relatedTarget 仍在层内）不重新起算", () => {
    vi.useFakeTimers();
    render(<Harness />);
    const trigger = screen.getByRole("button", { name: "打开控制层" });

    act(() => {
      fireEvent.click(trigger);
    });
    act(() => {
      fireEvent.focusIn(trigger);
    });
    act(() => {
      fireEvent.focusOut(trigger, { relatedTarget: document.querySelector(".reader-top") });
    });
    act(() => {
      vi.advanceTimersByTime(10000);
    });
    expect(visibleText()).toBe("true");
  });

  it("PC：恒显，不受自动收起计时影响", () => {
    vi.useFakeTimers();
    render(<Harness isH5={false} />);
    expect(visibleText()).toBe("true");
    act(() => {
      vi.advanceTimersByTime(10000);
    });
    expect(visibleText()).toBe("true");
  });
});

/**
 * U13：拖进度滑块要按住好几秒，而按下那一刻之后再没有事件。
 * 旧实现只认「控制层内焦点」，pointerdown 落在底栏上不算交互 —— 到第 3 秒整条底栏消失，
 * 读者的手还按在滑块上，底下的正文已经换了个位置。判据是「按住 3.4 秒仍 visible」。
 */
describe("U13 按住底栏时不自动收起", () => {
  it("按下底栏滑块后按住 3.4 秒，底栏仍在；松手后重新起算 3 秒", () => {
    vi.useFakeTimers();
    render(<BarHarness />);
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "打开控制层" }));
    });
    expect(visibleText()).toBe("true");

    act(() => {
      fireEvent.pointerDown(slider());
    });
    act(() => {
      vi.advanceTimersByTime(3400);
    });
    expect(visibleText()).toBe("true");

    act(() => {
      fireEvent.pointerUp(slider());
    });
    act(() => {
      vi.advanceTimersByTime(2999);
    });
    expect(visibleText()).toBe("true");
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(visibleText()).toBe("false");
  });

  it("没按住底栏时照旧 3 秒收起（续一拍不改变常态）", () => {
    vi.useFakeTimers();
    render(<BarHarness />);
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "打开控制层" }));
    });
    act(() => {
      vi.advanceTimersByTime(3000);
    });
    expect(visibleText()).toBe("false");
  });

  it("正文上的按下不算底栏交互：不续计时", () => {
    vi.useFakeTimers();
    render(<BarHarness />);
    act(() => {
      fireEvent.click(screen.getByRole("button", { name: "打开控制层" }));
    });
    act(() => {
      fireEvent.pointerDown(document.querySelector(".reader-main") as Element);
    });
    act(() => {
      vi.advanceTimersByTime(3000);
    });
    expect(visibleText()).toBe("false");
  });
});

/**
 * U18：第一次进阅读器满屏正文，没人告诉读者「点中间出菜单」。
 * 一次性提示 1.5 秒（动画同长），显示的同时写下 seenHint，之后进任何书都不再出现。
 */
describe("U18 首次进阅读器的一次性手势提示", () => {
  it("没有 seenHint 标记时显示 1.5 秒并写下标记", () => {
    vi.useFakeTimers();
    localStorage.removeItem(HINT_SEEN_KEY);
    render(<HintHarness />);
    expect(hintText()).toBe("true");

    act(() => {
      vi.advanceTimersByTime(FIRST_HINT_MS - 1);
    });
    expect(hintText()).toBe("true");
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(hintText()).toBe("false");
    expect(localStorage.getItem(HINT_SEEN_KEY)).toBe("1");
  });

  it("已有标记时直接不渲染（只在第一次进入时打扰一次）", () => {
    localStorage.setItem(HINT_SEEN_KEY, "1");
    render(<HintHarness />);
    expect(hintText()).toBe("false");
  });

  it("存储读不到（隐私模式）时当作已经看过：宁可不提示，也不每次进书都弹", () => {
    const spy = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    try {
      render(<HintHarness />);
      expect(hintText()).toBe("false");
    } finally {
      spy.mockRestore();
    }
  });

  it("PC 上不出触屏提示：没有手势可教，也不写标记", () => {
    localStorage.removeItem(HINT_SEEN_KEY);
    render(<HintHarness isH5={false} />);
    expect(hintText()).toBe("false");
    expect(localStorage.getItem(HINT_SEEN_KEY)).toBeNull();
  });
});
