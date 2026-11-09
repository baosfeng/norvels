import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useReaderControls } from "./useReaderControls";

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

function visibleText(): string {
  return screen.getByTestId("visible").textContent ?? "";
}

afterEach(() => {
  vi.useRealTimers();
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
