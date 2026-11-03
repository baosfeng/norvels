import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RESTORE_HINT_MS, ReaderRestoreHint } from "./ReaderRestoreHint";

/**
 *  进度恢复提示：一个只包住自己文字的胶囊，落下就说明「已回到上次读到的地方」。
 * 3 秒自动消失——恢复本身已经发生，不该先要求读者关掉浮条才能继续读。
 */
describe("ReaderRestoreHint", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("到达即说明位置已恢复，并以 status 让读屏播报", () => {
    render(<ReaderRestoreHint visible onDismiss={vi.fn()} />);

    expect(screen.getByTestId("reader-restore-hint").getAttribute("role")).toBe("status");
    expect(screen.getByText("已回到上次读到的地方")).toBeTruthy();
  });

  it("3 秒后自动消失，不需要点「知道了」", () => {
    vi.useFakeTimers();
    const onDismiss = vi.fn();
    render(<ReaderRestoreHint visible onDismiss={onDismiss} />);

    act(() => {
      vi.advanceTimersByTime(RESTORE_HINT_MS - 1);
    });
    expect(onDismiss).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(onDismiss).toHaveBeenCalledTimes(1);
    // 自动消失不算「读者点了知道了」：不把焦点抢回正文
    expect(onDismiss).toHaveBeenCalledWith("timeout");
  });

  it("父组件不断重渲染（换新的 onDismiss 引用）也不会重置计时", () => {
    vi.useFakeTimers();
    const last = vi.fn();
    const { rerender } = render(<ReaderRestoreHint visible onDismiss={vi.fn()} />);

    // 阅读器每帧都会重渲染，回调每次都是新引用：旧实现会让 3 秒永远走不完
    for (let i = 0; i < 5; i += 1) {
      rerender(<ReaderRestoreHint visible onDismiss={vi.fn()} />);
      act(() => {
        vi.advanceTimersByTime(200);
      });
    }
    rerender(<ReaderRestoreHint visible onDismiss={last} />);
    act(() => {
      vi.advanceTimersByTime(RESTORE_HINT_MS);
    });

    expect(last).toHaveBeenCalledWith("timeout");
  });

  it("点「知道了」立刻收起；收起后不再渲染浮条", () => {
    const onDismiss = vi.fn();
    const { rerender } = render(<ReaderRestoreHint visible onDismiss={onDismiss} />);

    fireEvent.click(screen.getByRole("button", { name: "知道了" }));
    expect(onDismiss).toHaveBeenCalledTimes(1);
    expect(onDismiss).toHaveBeenCalledWith("user");

    rerender(<ReaderRestoreHint visible={false} onDismiss={onDismiss} />);
    expect(screen.queryByTestId("reader-restore-hint")).toBeNull();
  });
});
