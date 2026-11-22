import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { READER_DEFAULTS } from "../readerSettings";
import { ReaderPagedBody, type ReaderPagedBodyProps } from "./ReaderPagedBody";

/**
 * U10：目录远跳时，新章正文还没到货的那几百毫秒里不许整屏换空态。
 *
 * 改前：paragraphs 一空就整屏渲染 ReaderStateArea，连续流里这时 loading=false（流不算空）
 * → 读者先看到一帧「还没有正文」，正文到了再换回来。
 * 改后：标题也没到 = 正文还在路上 → 沿用本页排版骨架；标题到了才算「这一章确实没有正文」。
 */

function props(patch: Partial<ReaderPagedBodyProps> = {}): ReaderPagedBodyProps {
  return {
    chapterId: 11,
    title: "",
    chapterIndex: -1,
    paragraphs: [],
    hitsByPara: new Map(),
    currentHit: -1,
    searching: false,
    settings: READER_DEFAULTS,
    effect: "slide",
    reducedMotion: true,
    storedParaIndex: 0,
    progressLoading: false,
    jumpPara: null,
    onJumpConsumed: vi.fn(),
    prev: null,
    next: null,
    jumpChapter: vi.fn(),
    onAnchor: vi.fn(),
    onPageChange: vi.fn(),
    handleRef: { current: null },
    paused: true,
    loading: false,
    error: null,
    retrying: false,
    retryFailures: 0,
    onRetry: vi.fn(),
    onNextChapter: vi.fn(),
    ...patch,
  };
}

describe("U10 远跳不闪空态", () => {
  it("正文还在路上（标题也没到）：给排版骨架，不渲染「还没有正文」", () => {
    render(<ReaderPagedBody {...props()} />);
    expect(screen.queryByTestId("reader-empty")).toBeNull();
    expect(screen.queryByTestId("reader-error")).toBeNull();
    expect(screen.getByTestId("reader-paged-skeleton")).toBeTruthy();
  });

  it("标题到了但确实没有段落：这才给空态（这一章确实没正文）", () => {
    render(<ReaderPagedBody {...props({ title: "第十一章 示例" })} />);
    expect(screen.getByTestId("reader-empty")).toBeTruthy();
    expect(screen.queryByTestId("reader-paged-skeleton")).toBeNull();
  });

  it("加载态优先：还在取正文时给加载态，不拿骨架冒充「正在读」", () => {
    render(<ReaderPagedBody {...props({ loading: true })} />);
    expect(screen.getByTestId("reader-loading")).toBeTruthy();
    expect(screen.queryByTestId("reader-paged-skeleton")).toBeNull();
  });

  it("出错时给错误态，骨架不能把失败藏起来", () => {
    render(<ReaderPagedBody {...props({ error: { kind: "network", message: "断网" } as never })} />);
    expect(screen.getByTestId("reader-error")).toBeTruthy();
    expect(screen.queryByTestId("reader-paged-skeleton")).toBeNull();
  });

  it("骨架不进读屏（aria-hidden）：它没有内容，念出来只会打断读者", () => {
    render(<ReaderPagedBody {...props()} />);
    expect(screen.getByTestId("reader-paged-skeleton").getAttribute("aria-hidden")).toBe("true");
  });
});
