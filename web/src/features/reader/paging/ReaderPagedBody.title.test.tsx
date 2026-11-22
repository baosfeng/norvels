import { render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { READER_DEFAULTS } from "../readerSettings";
import { ReaderPagedBody, type ReaderPagedBodyProps } from "./ReaderPagedBody";

/**
 * U20 分页路径：正文 h2 与顶栏「第 N 章」同口径——标题印的章号对不上就剥掉。
 *
 * 分页的 h2 由排版源 chapterFragment 建（引擎按它分页），所以这里读舞台里的 h2；
 * 与连续流（ReaderArticle.title.test.tsx）是同一条判据在两个渲染路径上各验一次。
 */

function propsFor(title: string, chapterIndex: number): ReaderPagedBodyProps {
  return {
    chapterId: 11,
    title,
    chapterIndex,
    paragraphs: ["第一段正文。", "第二段正文。"],
    hitsByPara: new Map(),
    currentHit: -1,
    searching: false,
    settings: READER_DEFAULTS,
    effect: "none",
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
  };
}

function headings(): string[] {
  return Array.from(document.querySelectorAll(".pg-stage h2")).map((node) => node.textContent ?? "");
}

describe("U20 分页正文标题", () => {
  it("标题印的章号 ≠ 库内序号：h2 只留标题文字", () => {
    render(<ReaderPagedBody {...propsFor("第十九章 武道要旨", 17)} />);
    expect(headings()).toContain("武道要旨");
    expect(headings().join("")).not.toContain("第十九章");
  });

  it("标题印的章号与库内序号一致：原文原样", () => {
    render(<ReaderPagedBody {...propsFor("第十九章 空谷仙影", 18)} />);
    expect(headings()).toContain("第十九章 空谷仙影");
  });
});
