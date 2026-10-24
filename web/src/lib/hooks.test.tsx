import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { useResource } from "./useResource";
import { useTheme, applyTheme } from "./theme";
import { useTierMedia } from "./tier";
import { stripTags } from "./text";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("useResource 四态", () => {
  it("先给加载态，拿到数据后转有数据", async () => {
    const { result } = renderHook(() => useResource(async () => 42, ["a"]));
    expect(result.current.loading).toBe(true);
    expect(result.current.data).toBeNull();
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toBe(42);
    expect(result.current.error).toBeNull();
  });

  it("错误归类后进错误态，reload 会重新取数", async () => {
    let attempt = 0;
    const { result } = renderHook(() =>
      useResource(async () => {
        attempt += 1;
        if (attempt === 1) throw new ApiError("http", "boom", 500);
        return "ok";
      }, ["retry"]),
    );
    await waitFor(() => expect(result.current.error).not.toBeNull());
    expect(result.current.error?.status).toBe(500);
    act(() => result.current.reload());
    await waitFor(() => expect(result.current.data).toBe("ok"));
  });

  it("调用方取消不算错误", async () => {
    const gate = deferred<number>();
    const { result, unmount } = renderHook(() =>
      useResource((signal) => {
        signal.addEventListener("abort", () => gate.reject(new ApiError("aborted", "canceled")));
        return gate.promise;
      }, ["abort"]),
    );
    unmount();
    await waitFor(() => expect(result.current.error).toBeNull());
    expect(result.current.data).toBeNull();
  });

  it("deps 变化时重新加载并丢掉旧结果", async () => {
    const { result, rerender } = renderHook(
      ({ id }: { id: number }) => useResource(async () => `v${id}`, [id]),
      { initialProps: { id: 1 } },
    );
    await waitFor(() => expect(result.current.data).toBe("v1"));
    rerender({ id: 2 });
    await waitFor(() => expect(result.current.data).toBe("v2"));
  });

  it("enabled=false 时不发请求也不展示加载态", () => {
    const fetcher = vi.fn(async () => "x");
    const { result } = renderHook(() => useResource(fetcher, ["off"], { enabled: false }));
    expect(result.current.loading).toBe(false);
    expect(fetcher).not.toHaveBeenCalled();
  });
});

describe("useTheme", () => {
  it("默认日间，切换后写进 localStorage 并改根节点属性", async () => {
    const { result } = renderHook(() => useTheme());
    expect(result.current.theme).toBe("day");
    act(() => result.current.setTheme("night"));
    await waitFor(() => expect(result.current.theme).toBe("night"));
    expect(document.documentElement.dataset.theme).toBe("night");
    expect(localStorage.getItem("norvels.theme")).toBe("night");
    act(() => result.current.setTheme("parchment"));
    await waitFor(() => expect(document.documentElement.dataset.theme).toBe("parchment"));
  });

  it("applyTheme 在夜间把 color-scheme 设为 dark", () => {
    const el = document.createElement("div");
    applyTheme("night", el);
    expect(el.dataset.theme).toBe("night");
    expect(el.style.colorScheme).toBe("dark");
    applyTheme("day", el);
    expect(el.style.colorScheme).toBe("light");
    applyTheme("day", null);
  });
});

describe("useTierMedia", () => {
  it("默认宽度下是 PC 档", () => {
    const { result } = renderHook(() => useTierMedia());
    expect(result.current).toBe("pc");
  });
});

describe("stripTags", () => {
  it("去掉标签并压平空白", () => {
    expect(stripTags("<img src=x> 正文\n\n 第二段 ")).toBe("正文 第二段");
    expect(stripTags("")).toBe("");
  });
});
